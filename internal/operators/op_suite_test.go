package operators

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/l7mp/dcontroller/pkg/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	retryInterval = time.Millisecond * 100
)

var (
	// loglevel = -10
	loglevel = -1
	timeout  = time.Second * 5
	interval = time.Millisecond * 50
	logger   logr.Logger
	c        client.WithWatch
)

type statusCond struct{ name, status string }
type labelCond struct{ name, value string }

func TestManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "5G Operators")
}

func findCondition(conds []any, name string) map[string]string {
	for _, v := range conds {
		c := v.(map[string]any)
		if c != nil && c["type"].(string) == name {
			ret := map[string]string{}
			for n, e := range c {
				if e != nil {
					ret[n] = e.(string)
				}
			}
			return ret
		}
	}
	return nil
}

// regTemplate is a template for creating new registrationssessions.
var regTemplate = `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: %s
  namespace: %s
spec:
  registrationType: initial
  trackingArea: "tai-001-01-000001"
  accessType: "3gpp"  # enum: 3gpp | non-3gpp | both
  nasKeySetIdentifier:
    typeOfSecurityContext: native
    keySetIdentifier: noKeyAvailable
  mobileIdentity:
    type: SUCI
    value: %s
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

// initReg creates a new registration and waits until the status conditions are satisfied.
func initReg(ctx context.Context, name, namespace, suci string, conds ...statusCond) object.Object {
	GinkgoHelper()

	obj, err := initRegErr(ctx, name, namespace, suci, conds...)
	Expect(err).NotTo(HaveOccurred())
	return obj
}

// initRegErr creates a new registration and waits until the status conditions are satisfied.
// Returns an error instead of using Ginkgo/Gomega assertions for use in benchmarks.
func initRegErr(ctx context.Context, name, namespace, suci string, conds ...statusCond) (object.Object, error) {
	// load reg 1
	yamlData := fmt.Sprintf(regTemplate, name, namespace, suci)
	reg1 := object.New()
	if err := yaml.Unmarshal([]byte(yamlData), &reg1); err != nil {
		return nil, fmt.Errorf("failed to unmarshal registration YAML: %w", err)
	}

	if err := c.Create(ctx, reg1); err != nil {
		return nil, fmt.Errorf("failed to create registration: %w", err)
	}

	if len(conds) == 0 {
		return nil, nil
	}

	// wait until we get an object with ready status
	retrieved := object.NewViewObject("amf", "Registration")
	object.SetName(retrieved, namespace, name)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-timeoutTimer.C:
			return nil, fmt.Errorf("timeout waiting for registration conditions")
		case <-ticker.C:
			if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err == nil {
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err == nil && ok {
					allCondsMet := true
					for _, c := range conds {
						r := findCondition(cs, c.name)
						if r == nil || r["status"] != c.status {
							allCondsMet = false
							break
						}
					}
					if allCondsMet {
						return retrieved, nil
					}
				}
			}
		}
	}
}

// sessionTemplate is a template for creating new sessions.
var sessionTemplate = `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Session
metadata:
  name: %s
  namespace: %s
spec:
  nssai: eMBB
  guti: %s
  sessionId: %d
  pduSessionType: IPv4
  sscMode: SSC1
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
        precedence: 10
        default: false
        qosFlow: voice-flow
        filters:
          - name: sip-signaling
            direction: Bidirectional
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
        precedence: 255
        default: true
        qosFlow: best-effort-flow
        filters:
          - name: match-all
            direction: Bidirectional
            match:
              type: MatchAll  # Special type for default rule`

// initSession creates a new session and waits until the status conditions are satisfied.
func initSession(ctx context.Context, name, namespace, guti string, id int, conds ...statusCond) object.Object {
	GinkgoHelper()

	obj, err := initSessionErr(ctx, name, namespace, guti, id, conds...)
	Expect(err).NotTo(HaveOccurred())
	return obj
}

// initSessionErr creates a new session and waits until the status conditions are satisfied.
// Returns an error instead of using Ginkgo/Gomega assertions for use in benchmarks.
func initSessionErr(ctx context.Context, name, namespace, guti string, id int, conds ...statusCond) (object.Object, error) {
	// load reg 1
	yamlData := fmt.Sprintf(sessionTemplate, name, namespace, guti, id)
	sess1 := object.New()
	if err := yaml.Unmarshal([]byte(yamlData), &sess1); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session YAML: %w", err)
	}

	if err := c.Create(ctx, sess1); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	if len(conds) == 0 {
		return nil, nil
	}

	// wait until we get an object with ready status
	retrieved := object.NewViewObject("amf", "Session")
	object.SetName(retrieved, namespace, name)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-timeoutTimer.C:
			return nil, fmt.Errorf("timeout waiting for session conditions")
		case <-ticker.C:
			if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err == nil {
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err == nil && ok {
					allCondsMet := true
					for _, c := range conds {
						r := findCondition(cs, c.name)
						if r == nil || r["status"] != c.status {
							allCondsMet = false
							break
						}
					}
					if allCondsMet {
						return retrieved, nil
					}
				}
			}
		}
	}
}

// sessionContextTemplate is a template for creating new sessions contexts.
var sessionContextTemplate = `
apiVersion: smf.view.dcontroller.io/v1alpha1
kind: SessionContext
metadata:
  name: %s
  namespace: %s
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
              type: MatchAll  # Special type for default rule
status:
  conditions:
    validated:
      status: "True"`

// initSessionContext creates a new session and waits until the status conditions are satisfied.
func initSessionContext(ctx context.Context, name, namespace, guti string, id int, conds ...statusCond) object.Object {
	GinkgoHelper()

	obj, err := initSessionContextErr(ctx, name, namespace, guti, id, conds...)
	Expect(err).NotTo(HaveOccurred())
	return obj
}

// initSessionContextErr creates a new session context and waits until the status conditions are satisfied.
// Returns an error instead of using Ginkgo/Gomega assertions for use in benchmarks.
func initSessionContextErr(ctx context.Context, name, namespace, guti string, id int, conds ...statusCond) (object.Object, error) {
	// load reg 1
	yamlData := fmt.Sprintf(sessionContextTemplate, name, namespace, guti, id)
	sess1 := object.New()
	if err := yaml.Unmarshal([]byte(yamlData), &sess1); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session context YAML: %w", err)
	}

	if err := c.Create(ctx, sess1); err != nil {
		return nil, fmt.Errorf("failed to create session context: %w", err)
	}

	if len(conds) == 0 {
		return nil, nil
	}

	// wait until we get an object with ready status
	retrieved := object.NewViewObject("smf", "SessionContext")
	object.SetName(retrieved, namespace, name)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-timeoutTimer.C:
			return nil, fmt.Errorf("timeout waiting for session context conditions")
		case <-ticker.C:
			if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err == nil {
				cs, ok, err := unstructured.NestedMap(retrieved.UnstructuredContent(), "status", "conditions")
				if err == nil && ok {
					allCondsMet := true
					for _, c := range conds {
						status, ok := cs[c.name]
						if !ok {
							allCondsMet = false
							break
						}
						value, ok := status.(map[string]any)
						if !ok || value["status"] != c.status {
							allCondsMet = false
							break
						}
					}
					if allCondsMet {
						return retrieved, nil
					}
				}
			}
		}
	}
}

// contextReleaseTemplate is a template for creating context release requests.
var contextReleaseTemplate = `
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: ContextRelease
metadata:
  name: %s
  namespace: %s
spec:
  guti: %s
  sessionId: %d`

// initContextRelease creates a new context release request and waits until conditions are satisfied.
func initContextRelease(ctx context.Context, name, namespace, guti string, sessionId int, conds ...statusCond) object.Object {
	GinkgoHelper()

	obj, err := initContextReleaseErr(ctx, name, namespace, guti, sessionId, conds...)
	Expect(err).NotTo(HaveOccurred())
	return obj
}

// initContextReleaseErr creates a new context release request and waits until conditions are satisfied.
// Returns an error instead of using Ginkgo/Gomega assertions for use in benchmarks.
func initContextReleaseErr(ctx context.Context, name, namespace, guti string, sessionId int, conds ...statusCond) (object.Object, error) {
	yamlData := fmt.Sprintf(contextReleaseTemplate, name, namespace, guti, sessionId)
	ctxRel := object.New()
	if err := yaml.Unmarshal([]byte(yamlData), &ctxRel); err != nil {
		return nil, fmt.Errorf("failed to unmarshal context release YAML: %w", err)
	}

	if err := c.Create(ctx, ctxRel); err != nil {
		return nil, fmt.Errorf("failed to create context release: %w", err)
	}

	if len(conds) == 0 {
		return nil, nil
	}

	// wait until we get an object with ready status.
	retrieved := object.NewViewObject("amf", "ContextRelease")
	object.SetName(retrieved, namespace, name)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-timeoutTimer.C:
			return nil, fmt.Errorf("timeout waiting for context release conditions")
		case <-ticker.C:
			if err := c.Get(ctx, client.ObjectKeyFromObject(retrieved), retrieved); err == nil {
				cs, ok, err := unstructured.NestedSlice(retrieved.UnstructuredContent(), "status", "conditions")
				if err == nil && ok {
					allCondsMet := true
					for _, c := range conds {
						r := findCondition(cs, c.name)
						if r == nil || r["status"] != c.status {
							allCondsMet = false
							break
						}
					}
					if allCondsMet {
						return retrieved, nil
					}
				}
			}
		}
	}
}
