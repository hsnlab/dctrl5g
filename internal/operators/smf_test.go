package operators

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/hsnlab/dctrl5g/internal/testsuite"
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
		}, 0, logger)
		Expect(err).NotTo(HaveOccurred())
		op = d.GetOperator("smf")
		Expect(op).NotTo(BeNil())
		c = d.GetCache().GetClient()
		Expect(c).NotTo(BeNil())
	})

	AfterEach(func() {
		cancel()
	})

	Context("When receiving a Session context UE", Ordered, Label("amf"), func() {
		It("should accept a legitimate SessionContext", func() {
			retrieved := initSession(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				labelCond{"state", "Ready"})
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
			retrieved := initSession(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				labelCond{"state", "Ready"})
			Expect(retrieved).NotTo(BeNil())

			retrieved = object.NewViewObject("upf", "Config")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				labels := retrieved.GetLabels()
				return len(labels) != 0 || labels["state"] == "Ready"
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
			retrieved1 := initSession(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				labelCond{"state", "Ready"})
			Expect(retrieved1).NotTo(BeNil())

			retrieved2 := initSession(ctx, "user-2", "user-2", "guti-310-170-3F-152-2A-B7C8D9E1", 5,
				labelCond{"state", "Ready"})
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
			}))
			Expect(flows).To(ContainElement(map[string]any{
				"name":      "user-2",
				"namespace": "user-2",
				"guti":      "guti-310-170-3F-152-2A-B7C8D9E1",
			}))

			// // delete session-1
			// retrieved1.SetLabels(map[string]string{"state": "Ready"})
			// err = c.Delete(ctx, retrieved1)
			// Expect(err).NotTo(HaveOccurred())

			// retrieved = object.NewViewObject("smf", "ActiveSessionTable")
			// object.SetName(retrieved, "", "active-sessions")
			// Eventually(func() bool {
			// 	if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
			// 		return false
			// 	}
			// 	return len(retrieved.UnstructuredContent()["spec"].([]any)) != 0
			// }, timeout, interval).Should(BeTrue())

			// sessions, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "spec")
			// Expect(err).NotTo(HaveOccurred())
			// Expect(ok).To(BeTrue())
			// Expect(sessions).To(HaveLen(2)) // test session
			// Expect(sessions).To(ContainElement(map[string]any{
			// 	"name":      "user-2",
			// 	"namespace": "user-2",
			// 	"guti":      "guti-310-170-3F-152-2A-B7C8D9E1",
			// }))

			// // delete session-2
			// err = c.Delete(ctx, retrieved2)
			// Expect(err).NotTo(HaveOccurred())

			// retrieved = object.NewViewObject("smf", "ActiveSessionTable")
			// object.SetName(retrieved, "", "active-sessions")
			// Eventually(func() bool {
			// 	if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
			// 		return false
			// 	}
			// 	return len(retrieved.UnstructuredContent()["spec"].([]any)) != 0
			// }, timeout, interval).Should(BeTrue())

			// sessions, ok, err = unstructured.NestedSlice(retrieved.UnstructuredContent(), "spec")
			// Expect(err).NotTo(HaveOccurred())
			// Expect(ok).To(BeTrue())
			// Expect(sessions).To(HaveLen(1)) // test session
		})
	})
})

var sessionTemplate = `
apiVersion: smf.view.dcontroller.io/v1alpha1
kind: SessionContext
metadata:
  name: %s
  namespace: %s
  labels:
    state: Validated
spec:
  guti: %s
  nssai: eMBB
  sessionId: %d
  pduSessionType: IPv4  # enum: IPv4 | IPv6 | IPv4v6 | Ethernet | Unstructured
  sscMode: SSC1  # enum: SSC1 (anchor maintained) | SSC2 (released on move) | SSC3 (flexible)
  networkConfiguration:
    requests:
      - type: IPConfiguration
        addressFamily: IPv4  # Internet Protocol Control Protocol
      - type: DNSServer
        addressFamily: IPv4
  qos:
    flows:
      - name: voice-flow
        fiveQI: ConversationalVoice  # Maps to standardized 5QI=1
        bitRates:
          uplinkBwKbps: 256
          downlinkBwKbps: 256
      - name: best-effort-flow
        fiveQI: BestEffort
      - name: dummy-flow
        fiveQI: Dummy
    rules:
      - name: voice-rule
        precedence: 10  # Lower number = higher priority (1-255)
        default: false  # This is NOT the default rule
        qosFlow: voice-flow  # References flow by name
        filters:
          - name: sip-signaling
            direction: Bidirectional  # enum: Uplink | Downlink | Bidirectional
            match:
              type: IPFilter
              parameters:
                protocol: UDP
                destinationPort: 5060
          - name: rtp-voice
            direction: Bidirectional
            match:
              type: IPFilter
              parameters:
                protocol: UDP
                destinationPortRange:
                  start: 16384
                  end: 32767
      - name: default-rule
        precedence: 255  # Lowest priority
        default: true    # Exactly one rule must be default
        qosFlow: best-effort-flow
        filters:
          - name: match-all
            direction: Bidirectional
            match:
              type: MatchAll  # Special type for default rule`

func initSession(ctx context.Context, name, namespace, guti string, id int, conds ...labelCond) object.Object {
	GinkgoHelper()

	// load reg 1
	yamlData := fmt.Sprintf(sessionTemplate, name, namespace, guti, id)
	sess1 := object.New()
	err := yaml.Unmarshal([]byte(yamlData), &sess1)
	Expect(err).NotTo(HaveOccurred())

	err = c.Create(ctx, sess1)
	Expect(err).NotTo(HaveOccurred())

	if len(conds) != 0 {
		// wait until we get an object with readystatus
		retrieved := object.NewViewObject("smf", "SessionContext")
		object.SetName(retrieved, namespace, name)
		Eventually(func() bool {
			if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
				return false
			}
			labels := retrieved.GetLabels()
			for _, c := range conds {
				if len(labels) == 0 || labels[c.name] != c.value {
					return false
				}
			}
			return true
		}, timeout, interval).Should(BeTrue())
		return retrieved
	}
	return nil
}
