package testsuite

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/l7mp/dcontroller/pkg/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	"github.com/hsnlab/dctrl5g/internal/dctrl"
)

const (
	keyFile  = "apiserver.key"
	certFile = "apiserver.crt"
)

func StartOps(ctx context.Context, opSpecs []dctrl.OpSpec, port int, logger logr.Logger) (*dctrl.Dctrl, error) {
	cert, key, err := auth.GenerateSelfSignedCertWithSANs([]string{"localhost"})
	if err != nil {
		return nil, fmt.Errorf("failed to generate keys: %w", err)
	}
	if err := auth.WriteCertAndKey(keyFile, certFile, key, cert); err != nil {
		return nil, fmt.Errorf("failed to write key/cert into file %q/%q: %w", keyFile, certFile, err)
	}

	if port == 0 {
		port = randomPort()
	}

	d, err := dctrl.New(dctrl.Options{
		OpSpecs:       opSpecs,
		APIServerPort: port,
		KeyFile:       keyFile,
		HTTPMode:      true,
		DisableAuth:   true,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}

	go func() {
		GinkgoHelper()
		defer GinkgoRecover()
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-d.GetErrorChannel():
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}()

	go func() {
		GinkgoHelper()
		defer GinkgoRecover()
		err := d.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	return d, nil
}

func randomPort() int {
	const minPort = 49152
	const maxPort = 65535
	n, err := rand.Int(rand.Reader, big.NewInt(maxPort-minPort+1))
	if err != nil {
		return 0
	}
	return int(n.Int64()) + minPort
}
