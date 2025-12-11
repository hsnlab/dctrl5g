package operators

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hsnlab/dctrl5g/internal/testsuite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/l7mp/dcontroller/pkg/object"
	"github.com/l7mp/dcontroller/pkg/operator"

	"github.com/hsnlab/dctrl5g/internal/dctrl"
)

var _ = Describe("SMF Operator", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		op     *operator.Operator
	)

	BeforeEach(func() {
		ctrl.SetLogger(logger.WithName("dctrl5g-test"))
		ctx, cancel = context.WithCancel(context.Background())
		d, err := testsuite.StartOps(ctx, []dctrl.OpSpec{
			{Name: "smf", File: "smf.yaml"},
			{Name: "pcf", File: "pcf.yaml"},
		}, 0, loglevel)
		Expect(err).NotTo(HaveOccurred())
		logger = d.GetLogger()
		op = d.GetOperator("smf")
		Expect(op).NotTo(BeNil())
		c = d.GetCache().GetClient()
		Expect(c).NotTo(BeNil())
	})

	AfterEach(func() {
		cancel()
	})

	Context("When receiving a Session context UE", Ordered, Label("smf"), func() {
		It("should accept a legitimate SessionContext", func() {
			retrieved := initSessionContext(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				statusCond{"policy", "True"}, statusCond{"upf", "True"})
			Expect(retrieved).NotTo(BeNil())

			cs, ok, err := unstructured.NestedMap(retrieved.UnstructuredContent(),
				"status", "conditions", "policy")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(cs["status"]).To(Equal("True"))
			Expect(cs["reason"]).To(Equal("PolicyApplied"))

			cs, ok, err = unstructured.NestedMap(retrieved.UnstructuredContent(),
				"status", "conditions", "upf")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(cs["status"]).To(Equal("True"))
			Expect(cs["reason"]).To(Equal("UPFConfigured"))

			flows, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(),
				"status", "qos", "flows")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(flows).To(HaveLen(2))
			Expect(flows).To(ContainElement(map[string]any{
				"bitRates": map[string]any{
					"downlinkBwKbps": int64(128),
					"uplinkBwKbps":   int64(128),
				},
				"fiveQI": "ConversationalVoice",
				"name":   "voice-flow",
			}))
			Expect(flows).To(ContainElement(map[string]any{
				"fiveQI": "BestEffort",
				"name":   "best-effort-flow",
			}))

			rules, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(),
				"status", "qos", "rules")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(rules).NotTo(BeEmpty())

			dns, ok, err := unstructured.NestedMap(retrieved.UnstructuredContent(),
				"status", "networkConfiguration", "dnsConfiguration")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(dns).To(HaveKey("primaryDNS"))
			Expect(dns).To(HaveKey("secondaryDNS"))

			ip, ok, err := unstructured.NestedMap(retrieved.UnstructuredContent(),
				"status", "networkConfiguration", "ipConfiguration")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(ip).To(HaveKey("ipAddress"))
			Expect(ip["ipAddress"]).To(HavePrefix("10.45.0."))
			Expect(ip).To(HaveKey("defaultGateway"))
		})

		It("should create a UPF config for a legitimate SessionContext", func() {
			retrieved := initSessionContext(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				statusCond{"upf", "True"})
			Expect(retrieved).NotTo(BeNil())

			retrieved = object.NewViewObject("upf", "Config")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				return c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) == nil
			}, timeout, interval).Should(BeTrue())

			networkConfig, ok, err := unstructured.NestedMap(retrieved.UnstructuredContent(),
				"spec", "networkConfiguration")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(networkConfig).NotTo(BeEmpty())

			qos, ok, err := unstructured.NestedMap(retrieved.UnstructuredContent(),
				"spec", "qos")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(qos).NotTo(BeEmpty())
		})

		It("should maintain the active session table", func() {
			retrieved1 := initSessionContext(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				statusCond{"policy", "True"}, statusCond{"upf", "True"})
			Expect(retrieved1).NotTo(BeNil())

			retrieved2 := initSessionContext(ctx, "user-2", "user-2", "guti-310-170-3F-152-2A-B7C8D9E1", 5,
				statusCond{"policy", "True"}, statusCond{"upf", "True"})
			Expect(retrieved2).NotTo(BeNil())

			retrieved := object.NewViewObject("smf", "ActiveSessionTable")
			object.SetName(retrieved, "", "active-sessions")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				return len(retrieved.UnstructuredContent()["spec"].([]any)) != 0
			}, timeout, interval).Should(BeTrue())

			flows, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "spec")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(flows).To(HaveLen(3)) // test session
			Expect(flows).To(ContainElement(map[string]any{
				"name":      "user-1",
				"namespace": "user-1",
				"guti":      "guti-310-170-3F-152-2A-B7C8D9E0",
				"idle":      false,
				"sessionId": int64(5),
			}))
			Expect(flows).To(ContainElement(map[string]any{
				"name":      "user-2",
				"namespace": "user-2",
				"guti":      "guti-310-170-3F-152-2A-B7C8D9E1",
				"idle":      false,
				"sessionId": int64(5),
			}))

			// delete session-1
			err = c.Delete(ctx, retrieved1)
			Expect(err).NotTo(HaveOccurred())

			retrieved = object.NewViewObject("smf", "ActiveSessionTable")
			object.SetName(retrieved, "", "active-sessions")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				return len(retrieved.UnstructuredContent()["spec"].([]any)) == 2
			}, timeout, interval).Should(BeTrue())

			sessions, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "spec")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(sessions).To(HaveLen(2)) // test session
			Expect(sessions).To(ContainElement(map[string]any{
				"name":      "user-2",
				"namespace": "user-2",
				"guti":      "guti-310-170-3F-152-2A-B7C8D9E1",
				"idle":      false,
				"sessionId": int64(5),
			}))

			// delete session-2
			err = c.Delete(ctx, retrieved2)
			Expect(err).NotTo(HaveOccurred())

			retrieved = object.NewViewObject("smf", "ActiveSessionTable")
			object.SetName(retrieved, "", "active-sessions")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				return len(retrieved.UnstructuredContent()["spec"].([]any)) == 1
			}, timeout, interval).Should(BeTrue())

			sessions, ok, err = unstructured.NestedSlice(retrieved.UnstructuredContent(), "spec")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(sessions).To(HaveLen(1)) // test session
		})
	})

	Context("When initiating an active->idle->active status transition", Ordered, Label("smf"), func() {
		It("should let a session to be idled", func() {
			retrieved := initSessionContext(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				statusCond{"validated", "True"}, statusCond{"policy", "True"}, statusCond{"upf", "True"})
			Expect(retrieved).NotTo(BeNil())

			retrieved = object.NewViewObject("upf", "Config")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				return c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) == nil
			}, timeout, interval).Should(BeTrue())

			// idling
			yamlData := `
apiVersion: smf.view.dcontroller.io/v1alpha1
kind: SessionContext
metadata:
  name: user-1
  namespace: user-1
spec:
  idle: true`
			patch := object.New()
			err := yaml.Unmarshal([]byte(yamlData), &patch)
			Expect(err).NotTo(HaveOccurred())

			jsonPatch, err := json.Marshal(object.DeepCopy(patch).UnstructuredContent())
			Expect(err).NotTo(HaveOccurred())

			// re-get the session context (currently it contains the Config)
			retrieved = object.NewViewObject("smf", "SessionContext")
			object.SetName(retrieved, "user-1", "user-1")
			err = c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved)
			Expect(err).NotTo(HaveOccurred())

			err = c.Patch(ctx, retrieved, client.RawPatch(types.MergePatchType, jsonPatch))
			Expect(err).NotTo(HaveOccurred())

			// UPF config should go away
			retrieved = object.NewViewObject("upf", "Config")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// SessionContext: UPF status should become false
			retrieved = object.NewViewObject("smf", "SessionContext")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedMap(retrieved.UnstructuredContent(),
					"status", "conditions", "upf")
				if err != nil || !ok {
					return false
				}
				return ok && cs["status"] == "False"
			}, timeout, interval).Should(BeTrue())

			cs, ok, err := unstructured.NestedMap(retrieved.UnstructuredContent(),
				"status", "conditions", "upf")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(cs["status"]).To(Equal("False"))
			Expect(cs["reason"]).To(Equal("Idle"))

			// redoing the patch should bring back the session from idle state
			yamlData = `
apiVersion: smf.view.dcontroller.io/v1alpha1
kind: SessionContext
metadata:
  name: user-1
  namespace: user-1
spec:
  idle: null`
			patch = object.New()
			err = yaml.Unmarshal([]byte(yamlData), &patch)
			Expect(err).NotTo(HaveOccurred())

			jsonPatch, err = json.Marshal(object.DeepCopy(patch).UnstructuredContent())
			Expect(err).NotTo(HaveOccurred())

			// re-get the session context (just to be on the safe side)
			retrieved = object.NewViewObject("smf", "SessionContext")
			object.SetName(retrieved, "user-1", "user-1")
			err = c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved)
			Expect(err).NotTo(HaveOccurred())

			err = c.Patch(ctx, retrieved, client.RawPatch(types.MergePatchType, jsonPatch))
			Expect(err).NotTo(HaveOccurred())

			// SessionContext: UPF status should become false
			retrieved = object.NewViewObject("smf", "SessionContext")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedMap(retrieved.UnstructuredContent(),
					"status", "conditions", "upf")
				if err != nil || !ok {
					return false
				}
				return ok && cs["status"] == "True"
			}, timeout, interval).Should(BeTrue())

			cs, ok, err = unstructured.NestedMap(retrieved.UnstructuredContent(),
				"status", "conditions", "upf")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(cs["status"]).To(Equal("True"))

			// UPF config should re-appear
			retrieved = object.NewViewObject("upf", "Config")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				return c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) == nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})
