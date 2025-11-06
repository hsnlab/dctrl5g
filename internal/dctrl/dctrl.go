package dctrl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	opv1a1 "github.com/l7mp/dcontroller/pkg/api/operator/v1alpha1"
	"github.com/l7mp/dcontroller/pkg/apiserver"
	"github.com/l7mp/dcontroller/pkg/auth"
	"github.com/l7mp/dcontroller/pkg/controller"
	"github.com/l7mp/dcontroller/pkg/operator"

	"github.com/hsnlab/dctrl5g/internal/operators/udm"
)

const (
	DefaultCtrlDir = "operators/"
)

type OpSpec struct {
	Name, Spec string
}

type Options struct {
	OpFiles               []string
	APIServerAddr         string
	APIServerPort         int
	DisableAuth, HTTPMode bool
	CertFile, KeyFile     string
	Logger                logr.Logger
}

type Dctrl struct {
	*operator.Group
	apiServer   *apiserver.APIServer
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

	config := &rest.Config{
		Host: fmt.Sprintf("http://%s:%d", addr, port),
	}

	g := operator.NewGroup(config, logger)

	apiServerConfig, err := apiserver.NewDefaultConfig(addr, port, g.GetClient(), opts.HTTPMode, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create the config for the embedded API server: %w", err)
	}

	// Configure authentication and authorization unless explicitly disabled or running in HTTP-only mode
	if opts.HTTPMode || opts.DisableAuth {
		log.Info("WARNING: Running API server without authentication - unrestricted access enabled")
	} else {
		// Load TLS key/cert
		if err := checkCert(log, opts.CertFile, opts.KeyFile); err != nil {
			return nil, fmt.Errorf("failed to load TLS key/cert: %w", err)
		}
		// Load public key
		publicKey, err := auth.LoadPublicKey(opts.CertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load public key: %w (hint: generate keys with "+
				"'dctl generate-keys' or use --disable-authentication)", err)
		}

		apiServerConfig.Authenticator = auth.NewJWTAuthenticator(publicKey)
		apiServerConfig.Authorizer = auth.NewCompositeAuthorizer()
		apiServerConfig.CertFile = opts.CertFile
		apiServerConfig.KeyFile = opts.KeyFile
	}

	apiServer, err := apiserver.NewAPIServer(apiServerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create the embedded API server: %w", err)
	}
	g.SetAPIServer(apiServer)

	// 3. Create the operators
	for _, opFile := range opts.OpFiles {
		specFile, err := os.Open(opFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open spec file for operator spec file %q: %w",
				opFile, err)
		}
		defer specFile.Close() //nolint:errcheck

		data, err := io.ReadAll(specFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read spec file for operator spec file %q: %w",
				opFile, err)
		}

		op := &opv1a1.Operator{}
		if err := yaml.Unmarshal(data, &op); err != nil {
			return nil, fmt.Errorf("failed to parse operator spec file %q: %w", opFile, err)
		}

		if _, err := g.AddOperatorFromSpec(op.GetName(), &op.Spec); err != nil {
			return nil, fmt.Errorf("unable to create operator %q: %w", op.GetName(), err)
		}
	}

	// 4. Load the UDM operator
	udm, err := udm.New(opts.KeyFile, apiServer, logger)
	if err != nil {
		return nil, fmt.Errorf("unable to create operator UDM: %w", err)
	}
	g.AddOperator(udm.Operator)
	apiServer.RegisterGVKs(udm.GetGVKs())

	return &Dctrl{Group: g, apiServer: apiServer, log: log, logger: logger}, nil
}

func (d *Dctrl) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		d.log.V(1).Info("starting API server")
		if err := d.apiServer.Start(ctx); err != nil {
			d.log.Error(err, "embedded API server error")
			cancel()
		}
	}()

	go func() {
		d.log.V(1).Info("starting the operator group")
		if err := d.Group.Start(ctx); err != nil {
			d.log.Error(err, "operator group error")
			cancel()
		}
	}()

	for {
		select {
		case err := <-d.GetErrorChannel():
			var operr controller.Error
			if errors.As(err, &operr) {
				d.log.Error(err, "controller error", "operator", operr.Operator,
					"controller", operr.Controller)
			} else {
				d.log.Error(err, "unknown error")
			}

		case <-ctx.Done():
			return nil
		}
	}
}

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
