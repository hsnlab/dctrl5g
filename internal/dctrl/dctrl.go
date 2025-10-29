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
	"path/filepath"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	opv1a1 "github.com/l7mp/dcontroller/pkg/api/operator/v1alpha1"
	"github.com/l7mp/dcontroller/pkg/apiserver"
	"github.com/l7mp/dcontroller/pkg/auth"
	"github.com/l7mp/dcontroller/pkg/controller"
	"github.com/l7mp/dcontroller/pkg/operator"
)

const (
	DefaultCtrlDir = "operators/"
)

type OpSpec struct {
	Name, Spec string
}

type Options struct {
	CtrlDir               string
	APIServerAddr         string
	APIServerPort         int
	DisableAuth, HTTPMode bool
	CertFile, KeyFile     string
	Logger                logr.Logger
}

type dctrl struct {
	*operator.Group
	apiServer   *apiserver.APIServer
	log, logger logr.Logger
}

func New(opts Options) (*dctrl, error) {
	logger := opts.Logger
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	log := logger.WithName("dctrl")

	opDir := opts.CtrlDir
	if opDir == "" {
		opDir = DefaultCtrlDir
	}

	config := &rest.Config{
		Host: fmt.Sprintf("http://0.0.0.0:%d", opts.APIServerPort),
	}

	g := operator.NewGroup(config, logger)

	apiServerConfig, err := apiserver.NewDefaultConfig("0.0.0.0", opts.APIServerPort, g.GetClient(), opts.HTTPMode, logger)
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
	opFiles, err := os.ReadDir(opDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open controller directory %q: %w", opDir, err)
	}

	for _, opFile := range opFiles {
		if opFile.IsDir() {
			continue
		}

		// skip non-YAML files
		if ext := filepath.Ext(opFile.Name()); ext != ".yaml" && ext != ".yml" {
			continue
		}

		opFileName := opFile.Name()
		filePath := filepath.Join(opDir, opFileName)

		specFile, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open spec file for operator spec file %q: %w",
				opFileName, err)
		}
		defer specFile.Close() //nolint:errcheck

		data, err := io.ReadAll(specFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read spec file for operator spec file %q: %w",
				opFileName, err)
		}

		op := &opv1a1.Operator{}
		if err := yaml.Unmarshal(data, &op); err != nil {
			return nil, fmt.Errorf("failed to parse operator spec file %q: %w", opFileName, err)
		}

		if _, err := g.AddOperator(op.GetName(), &op.Spec); err != nil {
			return nil, fmt.Errorf("unable to create operator %q: %w", op.GetName(), err)
		}
	}

	return &dctrl{Group: g, apiServer: apiServer, log: log, logger: logger}, nil
}

func (d *dctrl) Start(ctx context.Context) error {
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
