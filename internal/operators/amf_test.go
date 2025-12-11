package operators

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hsnlab/dctrl5g/internal/testsuite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/l7mp/dcontroller/pkg/object"
	"github.com/l7mp/dcontroller/pkg/operator"

	"github.com/hsnlab/dctrl5g/internal/dctrl"
)

var _ = Describe("AMF Operator", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		op     *operator.Operator
	)

	BeforeEach(func() {
		ctrl.SetLogger(logger.WithName("dctrl5g-test"))
		ctx, cancel = context.WithCancel(context.Background())
		d, err := testsuite.StartOps(ctx, []dctrl.OpSpec{
			{Name: "amf", File: "amf.yaml"},
			{Name: "ausf", File: "ausf.yaml"},
			{Name: "smf", File: "smf.yaml"},
			{Name: "pcf", File: "pcf.yaml"},
			{Name: "upf", File: "upf.yaml"},
			// UDM is manual
		}, 0, logger)
		Expect(err).NotTo(HaveOccurred())
		op = d.GetOperator("amf")
		Expect(op).NotTo(BeNil())
		c = d.GetCache().GetClient()
		Expect(c).NotTo(BeNil())
	})

	AfterEach(func() {
		cancel()
	})

	Context("When registering an UE", Ordered, Label("amf"), func() {
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
    type: SUCI
    value: "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"
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
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "True"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("True"))

			cond = findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("True"))

			cond = findCondition(conds, "Authenticated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Authenticated"))
			Expect(cond["status"]).To(Equal("True"))

			cond = findCondition(conds, "SubscriptionInfoRetrieved")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("SubscriptionInfoRetrieved"))
			Expect(cond["status"]).To(Equal("True"))
		})

		It("should reject a registration with invalid reg-type", func() {
			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: test-reg
  namespace: default
spec:
  registrationType: dummy
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
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Validated")
				return r != nil && r["status"] == "False"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))

			cond = findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("InvalidType"))
		})

		It("should reject a registration with invalid standard", func() {
			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: test-reg
  namespace: default
spec:
  registrationType: initial
  trackingArea: "tai-001-01-000001"
  accessType: "3gpp"  # enum: 3gpp | non-3gpp | both
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
  ueSecurityCapability:
    encryptionAlgorithms: ["5G-EA0", "5G-EA1", "5G-EA2", "5G-EA3"]
    integrityAlgorithms: ["5G-IA0", "5G-IA1", "5G-IA2", "5G-IA3"]
  ueStatus:
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
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Validated")
				return r != nil && r["status"] == "False"

			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))

			cond = findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("StandardNotSupported"))
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
  trackingArea: "tai-001-01-000001"
  accessType: "3gpp"  # enum: 3gpp | non-3gpp | both
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
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
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "False"

			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))

			cond = findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("SuciNotFound"))

			cond = findCondition(conds, "Authenticated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Authenticated"))
			Expect(cond["status"]).To(Equal("Unknown"))
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
  trackingArea: "tai-001-01-000001"
  accessType: "3gpp"  # enum: 3gpp | non-3gpp | both
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
  mobileIdentity:
    type: SUCI
    value: "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"
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
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "False"

			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())

			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))

			cond = findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("EncyptionNotSupported"))

			cond = findCondition(conds, "Authenticated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Authenticated"))
			Expect(cond["status"]).To(Equal("Unknown"))
		})

		It("should reject an unknown user", func() {
			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: test-reg
  namespace: default
spec:
  registrationType: initial
  trackingArea: "tai-001-01-000001"
  accessType: "3gpp"  # enum: 3gpp | non-3gpp | both
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
  mobileIdentity:
    type: SUCI
    value: "dummy"
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
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Authenticated")
				return r != nil && r["status"] == "False"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())

			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))

			cond = findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("True"))

			cond = findCondition(conds, "Authenticated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Authenticated"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("SupiNotFound"))
		})

		It("should delete a registration and linked resources", func() {
			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: test-reg
  namespace: default
spec:
  registrationType: initial
  trackingArea: "tai-001-01-000001"
  accessType: "3gpp"  # enum: 3gpp | non-3gpp | both
  mobileIdentity:
    type: SUCI
    value: "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"
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
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "True"
			}, timeout, interval).Should(BeTrue())

			// internal object exists
			retrieved = object.NewViewObject("amf", "RegState")
			object.SetName(retrieved, "default", "test-reg")
			Eventually(func() bool {
				return c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) == nil
			}, timeout, interval).Should(BeTrue())

			// mobile-identity mapping exists
			retrieved = object.NewViewObject("ausf", "MobileIdentity")
			object.SetName(retrieved, "default", "test-reg")
			Eventually(func() bool {
				return c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) == nil
			}, timeout, interval).Should(BeTrue())

			// Config exists
			retrieved = object.NewViewObject("udm", "Config")
			object.SetName(retrieved, "default", "guti-310-170-3F-152-2A-B7C8D9E0")
			Eventually(func() bool {
				return c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) == nil
			}, timeout, interval).Should(BeTrue())

			// get the new resource, that's what we indend to delete (the controller will issue
			// the Delete for the cached resource anyway)
			err = c.Get(ctx, client.ObjectKeyFromObject(reg), reg)
			Expect(err).NotTo(HaveOccurred())

			// delete registration
			err = c.Delete(ctx, reg)
			Expect(err).NotTo(HaveOccurred())

			// internal object removed
			retrieved = object.NewViewObject("amf", "RegState")
			object.SetName(retrieved, "default", "test-reg")
			Eventually(func() bool {
				err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved)
				return err != nil && apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// mobile-identity mapping removed
			retrieved = object.NewViewObject("ausf", "MobileIdentity")
			object.SetName(retrieved, "default", "test-reg")
			Eventually(func() bool {
				err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved)
				return err != nil && apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Config removed
			retrieved = object.NewViewObject("udm", "Config")
			object.SetName(retrieved, "default", "guti-310-170-3F-152-2A-B7C8D9E0")
			Eventually(func() bool {
				err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved)
				return err != nil && apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should register 2 registrations", func() {
			// load reg 1
			retrieved1 := initReg(ctx, "user-1", "user-1", "suci-0-999-01-02-4f2a7b9c8d13e7a5c0",
				statusCond{"Ready", "True"})
			Expect(retrieved1).NotTo(BeNil())
			// load reg 2
			retrieved2 := initReg(ctx, "user-2", "user-2", "suci-0-999-01-02-4f2a7b9c8d13e7a5c1",
				statusCond{"Ready", "True"})
			Expect(retrieved2).NotTo(BeNil())

			// check registration table
			regTable := object.NewViewObject("amf", "ActiveRegistrationTable")
			object.SetName(regTable, "", "active-registrations")
			err := c.Get(ctx, client.ObjectKeyFromObject(regTable), regTable)
			Expect(err).NotTo(HaveOccurred())

			specs, ok, err := unstructured.NestedSlice(regTable.UnstructuredContent(), "spec")
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(specs).To(HaveLen(3)) // test-reg
			Expect(specs).To(ContainElement(map[string]any{
				"name":      "user-1",
				"namespace": "user-1",
				"suci":      "suci-0-999-01-02-4f2a7b9c8d13e7a5c0",
				"guti":      "guti-310-170-3F-152-2A-B7C8D9E0",
			}))
			Expect(specs).To(ContainElement(map[string]any{
				"name":      "user-2",
				"namespace": "user-2",
				"suci":      "suci-0-999-01-02-4f2a7b9c8d13e7a5c1",
				"guti":      "guti-310-170-3F-152-2A-B7C8D9E1",
			}))

			// delete reg-1
			err = c.Delete(ctx, retrieved1)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				regTable = object.NewViewObject("amf", "ActiveRegistrationTable")
				object.SetName(regTable, "", "active-registrations")
				if c.Get(ctx, client.ObjectKeyFromObject(regTable), regTable) != nil {
					return false
				}
				specs, ok, err = unstructured.NestedSlice(regTable.UnstructuredContent(), "spec")
				return err == nil && ok && len(specs) == 2
			}, timeout, interval).Should(BeTrue())

			Expect(specs).To(HaveLen(2)) // test-reg!
			Expect(specs).To(ContainElement(map[string]any{
				"name":      "user-2",
				"namespace": "user-2",
				"suci":      "suci-0-999-01-02-4f2a7b9c8d13e7a5c1",
				"guti":      "guti-310-170-3F-152-2A-B7C8D9E1",
			}))

			// delete reg-2
			err = c.Delete(ctx, retrieved2)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				regTable = object.NewViewObject("amf", "ActiveRegistrationTable")
				object.SetName(regTable, "", "active-registrations")
				if c.Get(ctx, client.ObjectKeyFromObject(regTable), regTable) != nil {
					return false
				}
				specs, ok, err = unstructured.NestedSlice(regTable.UnstructuredContent(), "spec")
				return err == nil && ok && len(specs) == 1
			}, timeout, interval).Should(BeTrue())

			Expect(specs).To(HaveLen(1)) // test-reg!
		})
	})

	Context("When creating a session for an UE", Ordered, Label("amf"), func() {
		It("should accept a legitimate session request", func() {
			// load reg 1
			retrieved := initReg(ctx, "user-1", "user-1", "suci-0-999-01-02-4f2a7b9c8d13e7a5c0",
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			retrieved = initSession(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			// wait until we get an object with nonzero status
			retrieved = object.NewViewObject("amf", "Session")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "True"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("True"))

			cond = findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("True"))

			cond = findCondition(conds, "PolicyApplied")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("PolicyApplied"))
			Expect(cond["status"]).To(Equal("True"))

			cond = findCondition(conds, "UPFConfigured")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("UPFConfigured"))
			Expect(cond["status"]).To(Equal("True"))
		})

		It("should reject a session with no network config request", func() {
			// load reg 1
			retrieved := initReg(ctx, "user-1", "user-1", "suci-0-999-01-02-4f2a7b9c8d13e7a5c0",
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			// create session
			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Session
metadata:
  name: user-1
  namespace: user-1
spec:
  nssai: eMBB
  guti: "guti-310-170-3F-152-2A-B7C8D9E0"
  qos:
    flows: [1,2]
    rules: [1,2]`
			session := object.New()
			err := yaml.Unmarshal([]byte(yamlData), &session)
			Expect(err).NotTo(HaveOccurred())

			err = c.Create(ctx, session)
			Expect(err).NotTo(HaveOccurred())

			// wait until we get an object with nonzero status
			retrieved = object.NewViewObject("amf", "Session")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "False"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("InvalidSession"))

			cond = findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("SessionFailed"))

			cond = findCondition(conds, "PolicyApplied")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("PolicyApplied"))
			Expect(cond["status"]).To(Equal("Unknown"))

			cond = findCondition(conds, "UPFConfigured")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("UPFConfigured"))
			Expect(cond["status"]).To(Equal("Unknown"))
		})

		It("should reject a session with no flowspec", func() {
			// load reg 1
			retrieved := initReg(ctx, "user-1", "user-1", "suci-0-999-01-02-4f2a7b9c8d13e7a5c0",
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			// create session
			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Session
metadata:
  name: user-1
  namespace: user-1
spec:
  nssai: eMBB
  guti: "guti-310-170-3F-152-2A-B7C8D9E0"
  networkConfiguration:
    requests:
      - addressFamily: IPv4
        type: IPConfiguration
      - addressFamily: IPv4
        type: DNSServer`
			session := object.New()
			err := yaml.Unmarshal([]byte(yamlData), &session)
			Expect(err).NotTo(HaveOccurred())

			err = c.Create(ctx, session)
			Expect(err).NotTo(HaveOccurred())

			// wait until we get an object with nonzero status
			retrieved = object.NewViewObject("amf", "Session")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "False"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("InvalidSession"))

			cond = findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("SessionFailed"))

			cond = findCondition(conds, "PolicyApplied")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("PolicyApplied"))
			Expect(cond["status"]).To(Equal("Unknown"))

			cond = findCondition(conds, "UPFConfigured")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("UPFConfigured"))
			Expect(cond["status"]).To(Equal("Unknown"))
		})

		It("should reject a session with invalid NSSAI", func() {
			// load reg 1
			retrieved := initReg(ctx, "user-1", "user-1", "suci-0-999-01-02-4f2a7b9c8d13e7a5c0",
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			// create session
			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Session
metadata:
  name: user-1
  namespace: user-1
spec:
  nssai: dummy
  guti: "guti-310-170-3F-152-2A-B7C8D9E0"
  networkConfiguration:
    requests:
      - addressFamily: IPv4
        type: IPConfiguration
      - addressFamily: IPv4
        type: DNSServer
  qos:
    flows: [1,2]
    rules: [1,2]`
			session := object.New()
			err := yaml.Unmarshal([]byte(yamlData), &session)
			Expect(err).NotTo(HaveOccurred())

			err = c.Create(ctx, session)
			Expect(err).NotTo(HaveOccurred())

			// wait until we get an object with nonzero status
			retrieved = object.NewViewObject("amf", "Session")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "False"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("NSSAINotPermitted"))

			cond = findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("SessionFailed"))

			cond = findCondition(conds, "PolicyApplied")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("PolicyApplied"))
			Expect(cond["status"]).To(Equal("Unknown"))

			cond = findCondition(conds, "UPFConfigured")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("UPFConfigured"))
			Expect(cond["status"]).To(Equal("Unknown"))
		})

		It("should reject a session with no GUTI", func() {
			// load reg 1
			retrieved := initReg(ctx, "user-1", "user-1", "suci-0-999-01-02-4f2a7b9c8d13e7a5c0",
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			// create session
			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Session
metadata:
  name: user-1
  namespace: user-1
spec:
  nssai: eMBB
  networkConfiguration: something
  qos:
    flows: [1,2]
    rules: [1,2]`
			session := object.New()
			err := yaml.Unmarshal([]byte(yamlData), &session)
			Expect(err).NotTo(HaveOccurred())

			err = c.Create(ctx, session)
			Expect(err).NotTo(HaveOccurred())

			// wait until we get an object with nonzero status
			retrieved = object.NewViewObject("amf", "Session")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "False"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Validated")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Validated"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("GutiNotSpeficied"))

			cond = findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("SessionFailed"))

			cond = findCondition(conds, "PolicyApplied")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("PolicyApplied"))
			Expect(cond["status"]).To(Equal("Unknown"))

			cond = findCondition(conds, "UPFConfigured")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("UPFConfigured"))
			Expect(cond["status"]).To(Equal("Unknown"))
		})
	})

	Context("When initiating an active->idle state transition", Ordered, Label("amf"), func() {
		It("should deactive an active session", func() {
			retrieved := initReg(ctx, "user-1", "user-1", "suci-0-999-01-02-4f2a7b9c8d13e7a5c0",
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			retrieved = initSession(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			// we should have a valid UPF Configuration
			retrieved = object.NewViewObject("upf", "Config")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				return c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) == nil
			}, timeout, interval).Should(BeTrue())

			// we should get 2 configs in the active UPF Config table
			Eventually(func() bool {
				regTable := object.NewViewObject("upf", "ActiveConfigTable")
				object.SetName(regTable, "", "active-configs")
				if c.Get(ctx, client.ObjectKeyFromObject(regTable), regTable) != nil {
					return false
				}
				specs, ok, err := unstructured.NestedSlice(regTable.UnstructuredContent(), "spec")
				// test-session created by the smf generates a test config
				return err == nil && ok && len(specs) == 2
			}, timeout, interval).Should(BeTrue())

			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: ContextRelease
metadata:
  name: user-1
  namespace: user-1
spec:
  guti: "guti-310-170-3F-152-2A-B7C8D9E0"
  sessionId: 5`
			session := object.New()
			err := yaml.Unmarshal([]byte(yamlData), &session)
			Expect(err).NotTo(HaveOccurred())

			err = c.Create(ctx, session)
			Expect(err).NotTo(HaveOccurred())

			// wait until we get a nonempty status in the context-release
			retrieved = object.NewViewObject("amf", "ContextRelease")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "True"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("True"))

			// wait until we get session with Idle status
			retrieved = object.NewViewObject("amf", "Session")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "UPFConfigured")
				return r != nil && r["status"] == "False" && r["reason"] == "Idle"
			}, timeout, interval).Should(BeTrue())

			// we shouldn't see a valid UPF Configuration
			retrieved = object.NewViewObject("upf", "Config")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				return c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved) != nil
			}, timeout, interval).Should(BeTrue())

			// we should get 1 configs in the active UPF Config table
			Eventually(func() bool {
				regTable := object.NewViewObject("upf", "ActiveConfigTable")
				object.SetName(regTable, "", "active-configs")
				if c.Get(ctx, client.ObjectKeyFromObject(regTable), regTable) != nil {
					return false
				}
				specs, ok, err := unstructured.NestedSlice(regTable.UnstructuredContent(), "spec")
				// test-session created by the smf generates a test config
				return err == nil && ok && len(specs) == 1
			}, timeout, interval).Should(BeTrue())
		})

		It("should reject a deactivation request for an unknown registration", func() {
			retrieved := initReg(ctx, "user-1", "user-1", "suci-0-999-01-02-4f2a7b9c8d13e7a5c0",
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			retrieved = initSession(ctx, "user-1", "user-1", "guti-310-170-3F-152-2A-B7C8D9E0", 5,
				statusCond{"Ready", "True"})
			Expect(retrieved).NotTo(BeNil())

			yamlData := `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: ContextRelease
metadata:
  name: user-1
  namespace: user-1
spec:
  guti: dummy
  sessionId: 5`
			session := object.New()
			err := yaml.Unmarshal([]byte(yamlData), &session)
			Expect(err).NotTo(HaveOccurred())

			err = c.Create(ctx, session)
			Expect(err).NotTo(HaveOccurred())

			// wait until we get a nonempty status in the context-release
			retrieved = object.NewViewObject("amf", "ContextRelease")
			object.SetName(retrieved, "user-1", "user-1")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err != nil {
					return false
				}
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err != nil || !ok {
					return false
				}
				r := findCondition(cs, "Ready")
				return r != nil && r["status"] == "False"
			}, timeout, interval).Should(BeTrue())

			// check status
			status, ok := retrieved.UnstructuredContent()["status"].(map[string]any)
			Expect(ok).To(BeTrue())
			conds, ok := status["conditions"].([]any)
			Expect(ok).To(BeTrue())
			Expect(conds).NotTo(BeEmpty())

			cond := findCondition(conds, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond["type"]).To(Equal("Ready"))
			Expect(cond["status"]).To(Equal("False"))
			Expect(cond["reason"]).To(Equal("GutiNotFound"))
		})
	})
})
