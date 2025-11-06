package udm

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	"github.com/l7mp/dcontroller/pkg/apiserver"
	"github.com/l7mp/dcontroller/pkg/auth"
	"github.com/l7mp/dcontroller/pkg/object"
	"github.com/l7mp/dcontroller/pkg/operator"
)

const (
	keyFile       = "apiserver.key"
	certFile      = "apiserver.crt"
	timeout       = time.Second * 5
	interval      = time.Millisecond * 50
	retryInterval = time.Millisecond * 100
)

var (
	loglevel = -10
	logger   = zap.New(zap.UseFlagOptions(&zap.Options{
		Development:     true,
		DestWriter:      GinkgoWriter,
		StacktraceLevel: zapcore.Level(3),
		TimeEncoder:     zapcore.RFC3339NanoTimeEncoder,
		Level:           zapcore.Level(loglevel),
	}))
)

func TestManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "5G UDM")
}

var _ = Describe("UDM Operator", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		c      client.WithWatch
	)

	BeforeEach(func() {
		ctrl.SetLogger(logger.WithName("dctrl5g-test"))
		ctx, cancel = context.WithCancel(context.Background())

		// must load op manually: testsuite.StartOps would create a dctrl object that would import us
		cert, key, err := auth.GenerateSelfSignedCertWithSANs([]string{"localhost"})
		Expect(err).NotTo(HaveOccurred())
		err = auth.WriteCertAndKey(keyFile, certFile, key, cert)
		Expect(err).NotTo(HaveOccurred())

		port := randomPort()
		config := &rest.Config{
			Host: fmt.Sprintf("http://localhost:%d", port),
		}

		g := operator.NewGroup(config, logger)
		apiServerConfig, err := apiserver.NewDefaultConfig("localhost", port, g.GetClient(), true, logger)
		Expect(err).NotTo(HaveOccurred())

		apiServer, err := apiserver.NewAPIServer(apiServerConfig)
		Expect(err).NotTo(HaveOccurred())

		udm, err := New(keyFile, apiServer, logger)
		Expect(err).NotTo(HaveOccurred())

		g.AddOperator(udm.Operator)

		go func() {
			defer GinkgoRecover()
			for {
				select {
				case <-ctx.Done():
					return
				case err := <-g.GetErrorChannel():
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}()

		go func() {
			defer GinkgoRecover()
			err := g.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()

		c = udm.GetManager().GetClient().(client.WithWatch)
		Expect(c).NotTo(BeNil())
	})

	AfterEach(func() {
		cancel()
	})

	It("should handle a valid config request", func() {
		yamlData := `
apiVersion: udm.view.dcontroller.io/v1alpha1
kind: Config
metadata:
  name: test-guti`
		req := object.New()
		err := yaml.Unmarshal([]byte(yamlData), req)
		Expect(err).NotTo(HaveOccurred())
		err = c.Create(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		obj := object.NewViewObject("udm", "Config")
		Eventually(func() bool {
			err := c.Get(ctx, types.NamespacedName{Name: "test-guti"}, obj)
			if err != nil {
				return false
			}
			_, ok := obj.UnstructuredContent()["status"]
			return ok
		}, timeout, interval).Should(BeTrue())

		status, ok, err := unstructured.NestedMap(obj.UnstructuredContent(), "status")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())

		conds, ok := status["conditions"].([]any)
		Expect(ok).To(BeTrue())
		Expect(conds).To(HaveLen(1))
		cond := conds[0].(map[string]any)
		Expect(cond["type"]).To(Equal("Ready"))
		Expect(cond["status"]).To(Equal("True"))

		config, ok := status["config"]
		Expect(ok).To(BeTrue())
		Expect(config).NotTo(BeEmpty())

		clusters, ok, err := unstructured.NestedMap(config.(map[string]any), "clusters")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		Expect(clusters).To(HaveLen(1))
		Expect(clusters).To(HaveKey("dctrl5g"))

		cluster, ok := clusters["dctrl5g"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(cluster["server"]).To(HavePrefix("https://127.0.0.1"))
		Expect(cluster["insecure-skip-tls-verify"]).To(BeTrue())

		users, ok, err := unstructured.NestedMap(config.(map[string]any), "users")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		Expect(users).To(HaveLen(1))
		Expect(users).To(HaveKey("test-guti"))

		user, ok := users["test-guti"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(user["token"]).NotTo(BeEmpty())
	})
})

func randomPort() int {
	const minPort = 49152
	const maxPort = 65535
	n, err := rand.Int(rand.Reader, big.NewInt(maxPort-minPort+1))
	if err != nil {
		return 0
	}
	return int(n.Int64()) + minPort
}
