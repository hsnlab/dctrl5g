package dctrl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/go-logr/logr"

	"github.com/l7mp/dcontroller/pkg/apiserver"
	"github.com/l7mp/dcontroller/pkg/auth"
	"github.com/l7mp/dcontroller/pkg/cache"
	"github.com/l7mp/dcontroller/pkg/controller"
	"github.com/l7mp/dcontroller/pkg/operator"

	"github.com/hsnlab/dctrl5g/internal/operators/udm"
)

// OpSpec holds the defs for the declarative opeators. Native operators have to be loaded manually.
type OpSpec struct {
	Name, File string
}

type Options struct {
	OpSpecs                         []OpSpec
	APIServerAddr                   string
	APIServerPort                   int
	DisableAuth, HTTPMode, Insecure bool
	CertFile, KeyFile               string
	Logger                          logr.Logger
}

type Dctrl struct {
	api         *cache.API
	ops         map[string]*operator.Operator
	apiServer   *apiserver.APIServer
	errorChan   chan error
	log, logger logr.Logger
}

func New(opts Options) (*Dctrl, error) {
	logger := opts.Logger
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	log := logger.WithName("dctrl")

	addr := opts.APIServerAddr
	if addr == "" {
		addr = "localhost"
	}
	port := opts.APIServerPort
	if port == 0 {
		port = 18443
	}

	// Step 1: Create a shared view cache.
	api, err := cache.NewAPI(nil, cache.APIOptions{
		CacheOptions: cache.CacheOptions{Logger: logger},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create the shared view cache: %w", err)
	}

	// Step 2: Create the API server
	apiServerConfig, err := apiserver.NewDefaultConfig(addr, port, api.Client, opts.HTTPMode, opts.Insecure, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create the config for the embedded API server: %w", err)
	}

	// Step 2: Configure authentication and authorization unless explicitly disabled or running in HTTP-only mode.
	if opts.HTTPMode || opts.DisableAuth {
		log.Info("WARNING: Running API server without authentication - unrestricted access enabled")
	} else {
		// Load TLS key/cert.
		if err := checkCert(log, opts.CertFile, opts.KeyFile); err != nil {
			return nil, fmt.Errorf("failed to load TLS key/cert: %w", err)
		}
		// Load public key.
		publicKey, err := auth.LoadPublicKey(opts.CertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load public key: %w (hint: generate keys with "+
				"'dctl generate-keys' or use --disable-authentication)", err)
		}

		apiServerConfig.Authenticator = auth.NewJWTAuthenticator(publicKey)
		apiServerConfig.Authorizer = auth.NewCompositeAuthorizer()
		apiServerConfig.CertFile = opts.CertFile
		apiServerConfig.KeyFile = opts.KeyFile

		log.V(2).Info("generated authentication token for internal controllers")
	}

	apiServer, err := apiserver.NewAPIServer(apiServerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create the embedded API server: %w", err)
	}

	// 3. Create the operators
	errorChan := make(chan error, 64)
	ops := map[string]*operator.Operator{}
	for _, opSpec := range opts.OpSpecs {
		op, err := operator.NewFromFile(opSpec.Name, nil, opSpec.File, operator.Options{
			Cache:        api.Cache,
			APIServer:    apiServer,
			ErrorChannel: errorChan,
			Logger:       logger,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to create operator %q: %w", opSpec.Name, err)
		}
		ops[opSpec.Name] = op
	}

	// 4. Load the UDM operator. The constructor returns an actual operator (calls
	// AddNativeController internally).
	udmOp, err := udm.New(apiServer, udm.Options{
		API:      api,
		HTTPMode: opts.HTTPMode,
		Insecure: opts.Insecure,
		KeyFile:  opts.KeyFile,
		Logger:   logger,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create operator UDM: %w", err)
	}
	ops["udm"] = udmOp.Operator

	return &Dctrl{api: api, ops: ops, apiServer: apiServer, errorChan: errorChan, log: log, logger: logger}, nil
}

func (d *Dctrl) GetAPI() *cache.API { return d.api }

func (d *Dctrl) Start(ctx context.Context) error {
	defer close(d.errorChan)

	go func() {
		d.log.V(1).Info("starting API server")
		if err := d.apiServer.Start(ctx); err != nil {
			d.log.Error(err, "embedded API server error")
		}
	}()

	go func() {
		for {
			select {
			case err := <-d.errorChan:
				var operr controller.Error
				if errors.As(err, &operr) {
					d.log.Error(err, "controller error", "operator", operr.Operator,
						"controller", operr.Controller)
				} else {
					d.log.Error(err, "error")
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	for n, o := range d.ops {
		d.log.V(1).Info("starting the operator", "name", n)
		go func() {
			if err := o.Start(ctx); err != nil {
				d.log.Error(err, "operator error", "name", n)
			}
		}()
	}

	d.log.V(1).Info("starting the shared storage")
	return d.api.Cache.Start(ctx)

}

func (d *Dctrl) GetErrorChannel() chan error                { return d.errorChan }
func (d *Dctrl) GetOperator(name string) *operator.Operator { return d.ops[name] }

func checkCert(log logr.Logger, certFile, keyFile string) error {
	// 1. Load the raw bytes from the certificate and key files.
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("failed to read certificate file %q: %w", certFile, err)
	}

	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("failed to read private key file %q: %w", keyFile, err)
	}

	// 2. The core validation step: Attempt to create a tls.Certificate object.
	// This function will fail if the PEM blocks are malformed or if the private key
	// does not match the public key in the certificate.
	_, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("failed to validate certificate and key pair: %w", err)
	}

	// 3. If validation was successful, proceed to log the certificate's details.
	// We can be confident now that the certPEM contains a valid certificate.
	block, _ := pem.Decode(certPEM)
	cert, _ := x509.ParseCertificate(block.Bytes)

	ipStrings := make([]string, len(cert.IPAddresses))
	for i, ip := range cert.IPAddresses {
		ipStrings[i] = ip.String()
	}

	log.Info("validated TLS certificate and key pair", "cert_path", certFile, "key_path", keyFile,
		"subject", cert.Subject.CommonName, "dns_names", cert.DNSNames, "ip_addresses", ipStrings,
		"valid-to", cert.NotAfter)

	return nil
}
