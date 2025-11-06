package operators

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/l7mp/dcontroller/pkg/composite"
	"github.com/l7mp/dcontroller/pkg/object"
	"github.com/l7mp/dcontroller/pkg/operator"

	"github.com/hsnlab/dctrl5g/internal/testsuite"
)

var _ = Describe("AUSF Operator", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		c      client.WithWatch
		op     *operator.Operator
	)

	BeforeEach(func() {
		ctrl.SetLogger(logger.WithName("dctrl5g-test"))
		ctx, cancel = context.WithCancel(context.Background())
		d, err := testsuite.StartOps(ctx, logger, "ausf.yaml")
		Expect(err).NotTo(HaveOccurred())
		op = d.GetOperator("ausf")
		Expect(op).NotTo(BeNil())
		c = op.GetManager().GetClient().(client.WithWatch)
		Expect(c).NotTo(BeNil())
	})

	AfterEach(func() {
		cancel()
	})

	It("should create the SUCI-to-SUPI table", func() {
		list := composite.NewViewObjectList("ausf", "SuciToSupiTable")
		Eventually(func() bool {
			err := c.List(ctx, list)
			return err == nil && len(list.Items) == 1
		}, timeout, interval).Should(BeTrue())
		Expect(list.Items).To(HaveLen(1))
		obj := list.Items[0]
		Expect(obj.GetName()).To(Equal("suci-to-supi"))
		spec, ok, err := unstructured.NestedSlice(obj.UnstructuredContent(), "spec")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		Expect(spec).NotTo(BeEmpty())
	})

	It("should handle a valid SUPI request", func() {
		yamlData := `
apiVersion: ausf.view.dcontroller.io/v1alpha1
kind: SupiToSuciMapping
metadata:
  name: test-req
spec:
  suci: "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"`
		req := object.New()
		err := yaml.Unmarshal([]byte(yamlData), req)
		Expect(err).NotTo(HaveOccurred())
		err = c.Create(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		obj := object.NewViewObject("ausf", "SupiToSuciMapping")
		Eventually(func() bool {
			err := c.Get(ctx, types.NamespacedName{Name: "test-req"}, obj)
			if err != nil {
				return false
			}
			_, ok := obj.UnstructuredContent()["status"]
			return ok
		}, timeout, interval).Should(BeTrue())

		status, ok, err := unstructured.NestedMap(obj.UnstructuredContent(), "status")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())

		suci, ok := status["suci"]
		Expect(ok).To(BeTrue())
		Expect(suci.(string)).To(Equal("suci-0-999-01-02-4f2a7b9c8d13e7a5c0"))
		supi, ok := status["supi"]
		Expect(ok).To(BeTrue())
		Expect(supi.(string)).To(Equal("imsi-999010000000123"))

		conds, ok := status["conditions"].([]any)
		Expect(ok).To(BeTrue())
		Expect(conds).To(HaveLen(1))
		cond := conds[0].(map[string]any)
		Expect(cond["type"]).To(Equal("Ready"))
		Expect(cond["status"]).To(Equal("True"))
	})

	It("should fail an valid SUPI request", func() {
		yamlData := `
apiVersion: ausf.view.dcontroller.io/v1alpha1
kind: SupiToSuciMapping
metadata:
  name: test-req-fail
spec:
  suci: "suci-dummy"`
		req := object.New()
		err := yaml.Unmarshal([]byte(yamlData), req)
		Expect(err).NotTo(HaveOccurred())
		err = c.Create(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		obj := object.NewViewObject("ausf", "SupiToSuciMapping")
		Eventually(func() bool {
			err := c.Get(ctx, types.NamespacedName{Name: "test-req-fail"}, obj)
			if err != nil {
				return false
			}
			_, ok := obj.UnstructuredContent()["status"]
			return ok
		}, timeout, interval).Should(BeTrue())

		status, ok, err := unstructured.NestedMap(obj.UnstructuredContent(), "status")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())

		suci, ok := status["suci"]
		Expect(ok).To(BeTrue())
		Expect(suci.(string)).To(Equal("suci-dummy"))
		supi, ok := status["supi"]
		Expect(ok).To(BeTrue())
		Expect(supi).To(BeNil())

		conds, ok := status["conditions"].([]any)
		Expect(ok).To(BeTrue())
		Expect(conds).To(HaveLen(1))
		cond := conds[0].(map[string]any)
		Expect(cond["type"]).To(Equal("Ready"))
		Expect(cond["status"]).To(Equal("False"))
	})
})
