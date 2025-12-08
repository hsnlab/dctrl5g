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
		c      client.WithWatch
		op     *operator.Operator
	)

	BeforeEach(func() {
		ctrl.SetLogger(logger.WithName("dctrl5g-test"))
		ctx, cancel = context.WithCancel(context.Background())
		d, err := testsuite.StartOps(ctx, []dctrl.OpSpec{
			{Name: "amf", File: "amf.yaml"},
			{Name: "ausf", File: "ausf.yaml"},
		}, 0, logger)
		Expect(err).NotTo(HaveOccurred())
		op = d.GetOperator("amf")
		Expect(op).NotTo(BeNil())
		var ok bool
		c, ok = d.GetAPI().Client.(client.WithWatch)
		Expect(ok).To(BeTrue())
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

		cond = findCondition(conds, "NetworkSliceSelected")
		Expect(cond).NotTo(BeNil())
		Expect(cond["type"]).To(Equal("NetworkSliceSelected"))
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
  registrationType: dummy`
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
		Expect(cond["reason"]).To(Equal("InvalidRegistrationType"))
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
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
  ueSecurityCapability:
    encryptionAlgorithms: ["5G-EA0", "5G-EA1", "5G-EA2", "5G-EA3"]
    integrityAlgorithms: ["5G-IA0", "5G-IA1", "5G-IA2", "5G-IA3"]
  ueStatus:
    n1Mode: false`
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
		Expect(cond["status"]).To(Equal("True"))

		cond = findCondition(conds, "Authenticated")
		Expect(cond).NotTo(BeNil())
		Expect(cond["type"]).To(Equal("Authenticated"))
		Expect(cond["status"]).To(Equal("False"))
		Expect(cond["reason"]).To(Equal("MobileIdentityNotProvided"))
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
		Expect(cond["status"]).To(Equal("True"))

		cond = findCondition(conds, "Authenticated")
		Expect(cond).NotTo(BeNil())
		Expect(cond["type"]).To(Equal("Authenticated"))
		Expect(cond["status"]).To(Equal("False"))
		Expect(cond["reason"]).To(Equal("EncyptionNotSupported"))
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
  mobileIdentity:
    type: SUCI
    value: "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"
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

})
