package operators

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/l7mp/dcontroller/pkg/object"
)

var _ = Describe("AMF Operator", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		c      client.WithWatch
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		op, err := startOp(ctx, "amf.yaml")
		Expect(err).NotTo(HaveOccurred())
		c = op.GetManager().GetClient().(client.WithWatch)
		Expect(c).NotTo(BeNil())
	})

	AfterEach(func() {
		cancel()
	})

	It("should accept a legitimate registration", func() {
		yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: test-reg
  namespace: default
spec:
  registrationType: initial
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
  mobileIdentity:
    type: SUPI
    value: "imsi-001010123456789"
  ueSecurityCapability:
    encryptionAlgorithms: ["5G-EA0", "5G-EA1", "5G-EA2", "5G-EA3"]
    integrityAlgorithms: ["5G-IA0", "5G-IA1", "5G-IA2", "5G-IA3"]
  ueStatus:
    n1Mode: true
  requestedNSSAI:
    - sliceType: eMBB
      sliceDifferentiator: "000001"
    - sliceType: URLLC
      sliceDifferentiator: "000002"`
		reg := object.New()
		err := yaml.Unmarshal([]byte(yamlData), &reg)
		Expect(err).NotTo(HaveOccurred())

		err = c.Create(ctx, reg)
		Expect(err).NotTo(HaveOccurred())

		// wait until we get an object with nonzero status
		retrieved := object.NewViewObject("amf", "Registration")
		object.SetName(retrieved, "default", "test-reg")
		Eventually(func() bool {
			if c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) != nil {
				return false
			}
			_, ok := retrieved.UnstructuredContent()["status"]
			return ok

		}, timeout, interval).Should(BeTrue())

		// check status
		status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
		Expect(ok).To(BeTrue())

		conds, ok := status["conditions"].([]any)
		Expect(ok).To(BeTrue())
		Expect(conds).To(HaveLen(1))
		cond := conds[0].(map[string]any)
		Expect(cond["type"]).To(Equal("Registered"))
		Expect(cond["status"]).To(Equal("True"))
	})

	It("should reject a registration with an empty mobile identity", func() {
		yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: test-reg
  namespace: default
spec:
  registrationType: initial
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
  ueSecurityCapability:
    encryptionAlgorithms: ["5G-EA0", "5G-EA1", "5G-EA2", "5G-EA3"]
    integrityAlgorithms: ["5G-IA0", "5G-IA1", "5G-IA2", "5G-IA3"]
  ueStatus:
    n1Mode: true`
		reg := object.New()
		err := yaml.Unmarshal([]byte(yamlData), &reg)
		Expect(err).NotTo(HaveOccurred())

		err = c.Create(ctx, reg)
		Expect(err).NotTo(HaveOccurred())

		// wait until we get an object with nonzero status
		retrieved := object.NewViewObject("amf", "Registration")
		object.SetName(retrieved, "default", "test-reg")
		Eventually(func() bool {
			if c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) != nil {
				return false
			}
			_, ok := retrieved.UnstructuredContent()["status"]
			return ok

		}, timeout, interval).Should(BeTrue())

		// check status
		status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
		Expect(ok).To(BeTrue())
		conds, ok := status["conditions"].([]any)
		Expect(ok).To(BeTrue())
		Expect(conds).To(HaveLen(1))

		cond := conds[0].(map[string]any)
		Expect(cond["type"]).To(Equal("Registered"))
		Expect(cond["status"]).To(Equal("False"))
		Expect(cond["reason"]).To(Equal("MobileIdentityNotFound"))
	})

	It("should reject a registration with an unsupported cypher", func() {
		yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: test-reg
  namespace: default
spec:
  registrationType: initial
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
  mobileIdentity:
    type: SUPI
    value: "imsi-001010123456789"
  ueSecurityCapability:
    encryptionAlgorithms: ["dummy"]
    integrityAlgorithms: ["dummy"]
  ueStatus:
    n1Mode: true
  requestedNSSAI:
    - sliceType: eMBB
      sliceDifferentiator: "000001"
    - sliceType: URLLC
      sliceDifferentiator: "000002"`
		reg := object.New()
		err := yaml.Unmarshal([]byte(yamlData), &reg)
		Expect(err).NotTo(HaveOccurred())

		err = c.Create(ctx, reg)
		Expect(err).NotTo(HaveOccurred())

		// wait until we get an object with nonzero status
		retrieved := object.NewViewObject("amf", "Registration")
		object.SetName(retrieved, "default", "test-reg")
		Eventually(func() bool {
			if c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) != nil {
				return false
			}
			_, ok := retrieved.UnstructuredContent()["status"]
			return ok

		}, timeout, interval).Should(BeTrue())

		// check status
		status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
		Expect(ok).To(BeTrue())

		conds, ok := status["conditions"].([]any)
		Expect(ok).To(BeTrue())
		Expect(conds).To(HaveLen(1))
		cond := conds[0].(map[string]any)
		Expect(cond["type"]).To(Equal("Registered"))
		Expect(cond["status"]).To(Equal("False"))
		Expect(cond["reason"]).To(Equal("EncyptionNotSupported"))
	})

	It("should reject a registration with an unsupported standard", func() {
		yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: test-reg
  namespace: default
spec:
  registrationType: initial
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
  mobileIdentity:
    type: SUPI
    value: "imsi-001010123456789"
  ueSecurityCapability:
    encryptionAlgorithms: ["5G-EA0", "5G-EA1", "5G-EA2", "5G-EA3"]
    integrityAlgorithms: ["5G-IA0", "5G-IA1", "5G-IA2", "5G-IA3"]
  ueStatus:
    s1Mode: true
    n1Mode: false
  requestedNSSAI:
    - sliceType: eMBB
      sliceDifferentiator: "000001"
    - sliceType: URLLC
      sliceDifferentiator: "000002"`
		reg := object.New()
		err := yaml.Unmarshal([]byte(yamlData), &reg)
		Expect(err).NotTo(HaveOccurred())

		err = c.Create(ctx, reg)
		Expect(err).NotTo(HaveOccurred())

		// wait until we get an object with nonzero status
		retrieved := object.NewViewObject("amf", "Registration")
		object.SetName(retrieved, "default", "test-reg")
		Eventually(func() bool {
			if c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) != nil {
				return false
			}
			_, ok := retrieved.UnstructuredContent()["status"]
			return ok

		}, timeout, interval).Should(BeTrue())

		// check status
		status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
		Expect(ok).To(BeTrue())

		conds, ok := status["conditions"].([]any)
		Expect(ok).To(BeTrue())
		Expect(conds).To(HaveLen(1))
		cond := conds[0].(map[string]any)
		Expect(cond["type"]).To(Equal("Registered"))
		Expect(cond["status"]).To(Equal("False"))
		Expect(cond["reason"]).To(Equal("StandardNotSupported"))
	})
})
