package operators

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/l7mp/dcontroller/pkg/cache"
	"github.com/l7mp/dcontroller/pkg/object"
	"github.com/l7mp/dcontroller/pkg/operator"

	"github.com/hsnlab/dctrl5g/internal/dctrl"
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
		d, err := testsuite.StartOps(ctx, []dctrl.OpSpec{
			{Name: "ausf", File: "ausf.yaml"},
		}, 0, logger)
		Expect(err).NotTo(HaveOccurred())
		op = d.GetOperator("ausf")
		Expect(op).NotTo(BeNil())
		var ok bool
		c, ok = op.GetManager().GetClient().(client.WithWatch)
		Expect(ok).To(BeTrue())
		Expect(c).NotTo(BeNil())
	})

	AfterEach(func() {
		cancel()
	})

	It("should create the SUCI-to-SUPI table", func() {
		list := cache.NewViewObjectList("ausf", "SuciToSupiTable")
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
kind: MobileIdentity
metadata:
  name: test-reg
  namespace: default
spec:
  suci: "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"`
		req := object.New()
		err := yaml.Unmarshal([]byte(yamlData), req)
		Expect(err).NotTo(HaveOccurred())
		err = c.Create(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		retrieved := object.NewViewObject("ausf", "MobileIdentity")
		object.SetName(retrieved, "default", "test-reg")
		Eventually(func() bool {
			if c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) != nil {
				fmt.Println("AAAAAAA", object.Dump(retrieved))
				return false
			}
			cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
			if err != nil || !ok {
				return false
			}
			r := findCondition(cs, "Ready")
			return r != nil && r["status"] != "Pending"
		}, timeout, interval).Should(BeTrue())

		// check status
		Expect(retrieved.GetLabels()["state"]).To(Equal("Ready"))

		status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
		Expect(ok).To(BeTrue())

		conds, ok := status["conditions"].([]any)
		Expect(ok).To(BeTrue())
		Expect(conds).NotTo(BeEmpty())

		cond := findCondition(conds, "Ready")
		Expect(cond).NotTo(BeNil())
		Expect(cond["type"]).To(Equal("Ready"))
		Expect(cond["status"]).To(Equal("True"))

		suci, ok := status["suci"]
		Expect(ok).To(BeTrue())
		Expect(suci.(string)).To(Equal("suci-0-999-01-02-4f2a7b9c8d13e7a5c0"))
		supi, ok := status["supi"]
		Expect(ok).To(BeTrue())
		Expect(supi.(string)).To(Equal("imsi-999010000000123"))
	})

	It("should reject an invalid SUPI request", func() {
		yamlData := `
apiVersion: ausf.view.dcontroller.io/v1alpha1
kind: MobileIdentity
metadata:
  name: test-reg
  namespace: default
spec:
  suci: "dummy"`
		req := object.New()
		err := yaml.Unmarshal([]byte(yamlData), req)
		Expect(err).NotTo(HaveOccurred())
		err = c.Create(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		retrieved := object.NewViewObject("ausf", "MobileIdentity")
		object.SetName(retrieved, "default", "test-reg")
		Eventually(func() bool {
			if c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) != nil {
				return false
			}
			cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
			if err != nil || !ok {
				return false
			}
			r := findCondition(cs, "Ready")
			return r != nil && r["status"] != "Pending"
		}, timeout, interval).Should(BeTrue())

		// check status
		Expect(retrieved.GetLabels()["state"]).To(Equal("Ready"))
		status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
		Expect(ok).To(BeTrue())

		conds, ok := status["conditions"].([]any)
		Expect(ok).To(BeTrue())
		Expect(conds).NotTo(BeEmpty())

		cond := findCondition(conds, "Ready")
		Expect(cond).NotTo(BeNil())
		Expect(cond["type"]).To(Equal("Ready"))
		Expect(cond["status"]).To(Equal("False"))
		Expect(cond["reason"]).To(Equal("MobileIdentityNotFound"))
	})
})
