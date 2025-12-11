#  Declarative 5G control plane simulator

The **dctrl5g** project implements a declarative 5G Core Network simulator built on top of the Δ-controller framework, modeling key control plane functions (AMF, SMF, AUSF, UDM and UPF) as Kubernetes-style operators. It enables the simulation of UE registration, authentication, identity resolution, PDU session establishment and idle-active transition through standard Custom Resource workflows and declarative pipelines.

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Getting stated](#getting-stated)
4. [Registration](#registration)
5. [Session establishment](#session-establishment)
6. [Session idle transition](#session-idle-transition)
7. [Benchmarking](#benchmarking)
8. [Testing](#testing)
9. [Caveats](#caveats)
10. [License](#license)

## Overview

**dctrl5g** provides a simulated 5G Core Network control plane environment built upon the [Δ-controller](https://github.com/l7mp/dcontroller) framework. Unlike traditional imperative simulators, this project models Network Functions as declarative operators that transform state through JSONPath-like pipelines.

The system allows users to act as User Equipment (UE) by interacting with Kubernetes Custom Resources via a standard extension API server. It fully simulates the lifecycle of 5G connectivity, including:
- **Network Functions:** Simulates interactions between AMF, AUSF, UDM, SMF, PCF, and UPF.
- **Registration Flow:** Handles SUCI-to-SUPI resolution (privacy), AKA authentication, and GUTI allocation.
- **Session Management:** Models PDU Session establishment with QoS policy enforcement, IP allocation, and UPF configuration.
- **State Transitions:** Supports active-to-idle transitions and context releases via declarative state changes.

The logic for most operators (AMF, SMF, AUSF) is defined in high-level YAML pipelines, while the UDM is implemented as a native Go controller to handle complex credential generation (kubeconfigs) and RBAC token issuance.

## Architecture

The below diagram shows the general architecture of **dctrl5g**:

``` mermaid
graph TD
    %% Styling
    classDef user fill:#f9f,stroke:#333,stroke-width:2px;
    classDef cr fill:#e1f5fe,stroke:#0277bd,stroke-width:2px;
    classDef logic fill:#fff3e0,stroke:#ef6c00,stroke-width:2px,stroke-dasharray: 5 5;
    classDef native fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px;
    classDef framework fill:#eceff1,stroke:#455a64,stroke-width:2px;

    User([User / UE Simulator]):::user

    subgraph DCTRL_Framework ["Δ-Controller Framework (dctrl)"]
        style DCTRL_Framework fill:#fafafa,stroke:#999

        APIServer["API Server (Auth / RBAC)"]:::framework
        PipelineEngine["YAML Pipeline Engine"]:::framework
    end

    %% User Interactions
    User -->|kubectl apply| Registration["Registration (CR)"]:::cr
    User -->|kubectl apply| Session["Session (CR)"]:::cr
    User -->|kubectl apply| CtxRel["ContextRelease (CR)"]:::cr

    %% AMF Operator
    subgraph AMF_Operator ["AMF (Access & Mobility)"]
        style AMF_Operator fill:#e3f2fd,stroke:#2196f3

        Registration -->|Watches| AMF_Reg_Pipe{{Pipeline: Register Input/Output}}:::logic
        AMF_Reg_Pipe -->|Creates/Updates| RegState["RegState (Internal)"]:::cr
        ActiveRegistration[("ActiveRegistration Table")]:::cr
        AMF_Reg_Pipe -->|Maintains| ActiveRegistration

        RegState -.->|Read| AMF_Id_Pipe{{Pipeline: MobileIdentity Req/Handler}}:::logic
        RegState -.->|Read| AMF_Config_Pipe{{Pipeline: Config Req/Handler}}:::logic

        Session -->|Watches| AMF_Sess_Pipe{{Pipeline: Session Input/Output}}:::logic
        AMF_Sess_Pipe -->|Validates & Creates| SessionContext["SMF:SessionContext (CR)"]:::cr

        CtxRel -->|Watches| AMF_Rel_Pipe{{Pipeline: Release Input/Output}}:::logic
        AMF_Rel_Pipe -->|Patches Idle| SessionContext
    end

    %% AUSF Operator
    subgraph AUSF_Operator ["AUSF (Authentication)"]
        style AUSF_Operator fill:#f3e5f5,stroke:#9c27b0

        AMF_Id_Pipe -->|Creates| MobileIdentity["MobileIdentity (CR)"]:::cr
        SuciTable[("SUCI-SUPI Table")]:::cr

        MobileIdentity -->|Watches| AUSF_Pipe{{Pipeline: SUPI Lookup}}:::logic
        SuciTable -.->|Join| AUSF_Pipe
        AUSF_Pipe -->|Updates Status| MobileIdentity
    end

    %% UDM Operator
    subgraph UDM_Operator ["UDM (Unified Data)"]
        style UDM_Operator fill:#e8f5e9,stroke:#4caf50

        AMF_Config_Pipe -->|Creates| UdmConfig["Config (CR)"]:::cr

        UdmConfig -->|Watches| UDM_Native[("Native Go Controller<br/>(Generates Kubeconfig)")]:::native
        UDM_Native -->|Updates Status| UdmConfig
    end

    %% SMF Operator
    subgraph SMF_Operator ["SMF (Session Management)"]
        style SMF_Operator fill:#fff3e0,stroke:#ff9800

        SessionContext -->|Watches| SMF_Pipe{{Pipeline: SessionContext Handler}}:::logic
        ActiveSess[("ActiveSession Table")]:::cr

        SMF_Pipe -->|Updates IP/DNS/QoS| SessionContext
        SMF_Pipe -->|Maintains| ActiveSess
    end

    %% PCF Operator
    subgraph PCF_Operator ["PCF (Policy)"]
        style PCF_Operator fill:#fbe9e7,stroke:#ff5722
        PolicyTable[("Policy Table")]:::cr
    end

    %% UPF Operator
    subgraph UPF_Operator ["UPF (User Plane)"]
        style UPF_Operator fill:#e0f7fa,stroke:#00bcd4

        SMF_Pipe -->|Creates| UPFConfig["UPF:Config (CR)"]:::cr
        UPFConfig -->|Watches| UPF_Pipe{{Pipeline: Active Config}}:::logic
        UPF_Pipe -->|Maintains| ActiveConf[("ActiveConfig Table")]:::cr
    end

    %% Cross-Component Relationships
    MobileIdentity -.->|Status Watch| AMF_Id_Pipe
    UdmConfig -.->|Status Watch| AMF_Config_Pipe
    PolicyTable -.->|Join| SMF_Pipe
    SessionContext -.->|Status Watch| AMF_Sess_Pipe
    AMF_Sess_Pipe -->|Updates Status| Session
    AMF_Reg_Pipe -->|Updates Status| Registration
```

The project is built on the Δ-controller framework, shifting the simulator from imperative code to a declarative, data-driven architecture. Instead of managing complex reconciliation loops manually, the system defines state transitions using YAML-based processing pipelines.

**The API Surface (CRDs):** The simulation interface is purely unstructured Kubernetes-style Custom Resources. Users act as User Equipment (UE) by applying Registration or Session manifests via an extension Kubernetes API server. The API Server handles authentication (JWT) and RBAC, simulating the security boundaries of a real network.

**Operator Pipelines:** Most control plane logic (AMF, AUSF, SMF, and UPF) is defined in declarative YAML pipelines located in `internal/operators/`. These pipelines perform relational-algebra inspired operations (projections, joins, selections) on input streams to produce output state. The only exception is the UDM operator, which is implemented as a native Go controller (`internal/operators/udm/`). This is required for complex logic that YAML pipelines cannot handle, such as cryptographic key generation and the issuance of signed kubeconfigs for authenticated UEs.
- **Access & Mobility (AMF)**: The AMF operator acts as the central orchestrator. It validates UE inputs, maintains internal state machines (RegState), and coordinates with the AUSF for security and the SMF for connectivity. It simulates the N1 NAS interface.
- **Identity & Security (AUSF / UDM):** The AUSF resolves SUCI (encrypted IDs) to SUPI (permanent IDs) using a lookup table. The UDM generates the actual subscription information, represented as a scoped Kubernetes Config that grants the UE permissions to proceed with session establishment.
- **Session Management (SMF / PCF):** The **SMF** manages the lifecycle of PDU sessions. It merges user requests with **PCF** policies (QoS rules, bandwidth limits).
- **User plane (UPF):** The UPF represents the data plane. The SMF projects a finalized configuration (UPF:Config) into the UPF namespace, simulating the N4 interface provisioning.

## Getting stated

You will need the `dctl` command line tool to administer kubeconfigs, obtain it from [here](https://github.com/l7mp/dcontroller).

### Development

For testing, the API server can be launched in insecure pure-HTTP mode.

1. Start the operators using unsafe HTTP mode:
   ```bash
   $ go run main.go --http -zap-log-level 4
   ```

2. Create an admin config:
   ```bash
   $ dctl generate-config --http --insecure --user=admin --namespaces="*" > ./admin.config
   ```

3. Make a client request:
   ```bash
   $ export KUBECONFIG=./admin.config
   ```

### Production

For production, the API server must provide full authentication, authorization and encryption for UE interactions.

1. Generate the TLS certificate:
   ```bash
   $ dctl generate-keys
   ```

2. Start the operators:
   ```bash
   $ go run main.go -insecure -zap-log-level 1
   ```

3. Create **initial UE config**, which will only allow the a UE with name `user-1` to register:
   ```bash
   $ dctl generate-config --user=user-1 --namespaces=user-1 --insecure \
    --rules='[{"verbs":["create","get","list","watch","delete"],"apiGroups":["amf.view.dcontroller.io"],"resources":["registration"]}]' \
    > ./user-1-initial.config
   ```

4. To interact with the API server with **full admin access**, load the config generated as follows:

   ```bash
   $ dctl generate-config --user=<admin> --insecure \
    --rules='[{"verbs":["*"],"apiGroups":["*"],"resources":["*"]}]' \
    > ./admin.config
   ```

## Registration

### The Registration resource

The Registration resource is the main driver for creating UE registrations. The UE specifies the registration parameters in the spec of the Registration resource and the AMF will add a status to indicate the registration status plus some useful info. Note that the annotations are optional, but the spec parameters are mandatory (checked and rejected if missing or invalid).

The below dump shows a full Registration resource with a valid status set by the AMF:

``` yaml
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Registration
metadata:
  name: user-1
  namespace: user-1
  labels:
    equipment.type: "smartphone"           # Equipment type: enum: smartphone | iot | vehicle | etc
  annotations:
    interface: "N1-NAS-MM"                 # Protocol interface
    ran.node: "gnb-site-4-sector-2"        # RAN information
spec:
  registrationType: initial                # Options: initial | mobility | periodic | emergency
  accessType: "3gpp"                       # enum: 3gpp | non-3gpp | both
  trackingArea: "tai-001-01-000001"        # Registration area (TAI where registration initiated)
  mobileIdentity:
    type: SUCI                             # Options: SUCI | SUPI | GUTI | IMEI | IMEISV | TMSI
    value: "suci-0-999-01-02-4f2a7b9c8d13e7a5c0"
  ueSecurityCapability:
    encryptionAlgorithms:                  # Ordered by preference (first = most preferred)
      - 5G-EA0                             # Null encryption (no protection)
      - 5G-EA1                             # 128-bit SNOW 3G
      - 5G-EA2                             # 128-bit AES
      - 5G-EA3                             # 128-bit ZUC
    integrityAlgorithms:                   # Ordered by preference
      - 5G-IA0                             # Null integrity (no protection)
      - 5G-IA1                             # 128-bit SNOW 3G
      - 5G-IA2                             # 128-bit AES-CMAC
      - 5G-IA3                             # 128-bit ZUC
  ueStatus:
    s1Mode: false                          # EPC/LTE interworking capability
    n1Mode: true                           # 5GC/NR native mode capability
  ueNetworkCapability:                     # LTE/EPS capabilities for interworking
    epsEncryptionAlgorithms:
      - EEA0                               # Options: EEA0, EEA1, EEA2, EEA3
    epsIntegrityAlgorithms:
      - EIA0                               # Options: EIA0, EIA1, EIA2, EIA3
  requestedNSSAI:                          # Requested network slices
    - sliceType: eMBB                      # Options: eMBB | URLLC | MIoT | V2X | custom
      sliceDifferentiator: "000001"        # Optional, for multiple slices of same type
    - sliceType: URLLC                     # Only eMBB (Enhanced Mobile Broadband) is supported
      sliceDifferentiator: "000002"
status:                                    # Set by the AMF
  guti: guti-310-170-3F-152-2A-B7C8D9E0    # GUTI (Globally Unique Identifier), generated by the AMF
  allowedNSSAI:                            # Selected network slice
  - sliceDifferentiator: "000001"
    sliceType: eMBB
  conditions:
  - message: Registration successful       # Indicates overall registration success
    reason: RegistrationSuccessful
    status: "True"
    type: Ready
  - message: Validated                     # Indicates whether the spec is valid and allowed
    reason: Validated
    status: "True"
    type: Validated
  - message: UE successfully authenticated # Indicates whether encrypted security context could be established
    reason: AuthenticationSuccess
    status: "True"
    type: Authenticated
  - message: UE config successfully loaded # Indicates whether a UE config is available
    reason: ConfigReady
    status: "True"
    type: SubscriptionInfoRetrieved
  config: <full-UE-kubeconfig>
```

### Control loops

Registration resources are first processed by the AMF (Access and Mobility Management Function). Later steps involve the AUSF (Authentication Server Function) and the UDM (Unified Data Management) function.

Consider the below sequence diagram:

``` mermaid
sequenceDiagram
    UE->>+AMF: Create Registration
    Note right of AMF: Validate Registration spec
    AMF->>+AUSF: Create MobileIdentity
    Note right of AUSF: Map SUCI to SUPI and add to status
    AUSF->>-AMF: Return MobileIdentity status
    Note right of AMF: Genetate GUTI from SUPI
    Note right of AMF: Reject if SUPI or GUTI is missing
    AMF->>+UDM: Create Config
    Note right of UDM: Add UE config to Config status
    UDM->>-AMF: Return Config status
    Note right of AMF: Reject if no Config is returned
    Note right of AMF: Set Registration status
    AMF->>-UE: Return Registration status
    Note right of AMF: Maintain the active-registration table
```

The AMF control loops are as follows:
1. **Control loop** `register-input`. **Purpose:** validate AMF:Registration and write to internal state. **Watches:** AMF:Registration. **Predicates:** `GenerationChanged`. **Writes:** AMF:RegState (internal registration state).
   1. Create an empty AMF:RegState resource.
   2. Initialize status fields.
   3. Check registration type. If not `initial`, set `Validated` status to `False` with reason `InvalidType`.
   4. Check 5GC/NR native mode. If not `n1Mode`, set `Validated` status to `False` with reason `StandardNotSupported`.
   5. Check mobile identity. If type is not `SUCI` or the value is empty, set `Validated` status to `False` with reason `SuciNotFound`.
   6. Check UE security capability. If the encryption algorithms list does not contain `5G-EA2` or the integrity algorithms list does not contain `5G-IA2`, set `Validated` status to `False` with reason `EncyptionNotSupported`.
   7. Otherwise set `Validated` status to `True` with reason `Validated`.
   8. Write AMF:RegState.
2. **Control loop** `register-identity-req`. **Purpose:** generate mobile identity requests for the AUSF. **Watches:** AMF:RegState. **Predicates:** runs only if the `Validated` status is `True`. **Writes**: AUSF:MobileIdentity.
   1. Create an empty AUSF:MobileIdentity resource.
   2. Set the SUCI in the spec.
   3. Send to the AUSF.
3. **Control loop** `register-identity-handler`. **Purpose:** handle mobile identity responses from the AUSF. **Watches:** AMF:RegState and AUSF:MobileIdentity. **Predicates:** runs only if AMF:RegState `Validated` status is `True` and the MobileIdeintity is labeled `state:Ready`. **Writes**: AMF:RegState.
   1. Join on metadata.
   2. Check if AUSF:MobileIdentity `Reeady` status is true. If not, set the `Authenticated` status to `False` with reason `SupiNotFound`.
   3. Genetate a GUTI based on the SUPI returned by the AUSF and add to the status.
   4. Set the AMF:RegState `Authenticated` status to `True` with reason `AuthenticationSuccess`.
   5. Write AMF:RegState.
4. **Control loop** `register-config-req`. **Purpose:** generate a config request to the UDM in order to obtain a secure context for the UE. **Watches:** AMF:RegState. **Predicates:** runs only if AMF:RegState `Authenticated` status is `True`. **Writes**: UDM:Config.
   1. Create an empty UDM:Config resource
   2. Set metadata.
   3. Send to the UDM.
5. **Control loop** `register-config-handler`. **Purpose:** handle configs from the UDM. **Watches:** AMF:RegState and UDM:Config. **Predicates:** runs only if AMF:RegState `Authenticated` status is `True`. **Writes**: AMF:RegState.
   1. Join on metadata.
   2. Check if UDM:Config `Ready` status is true. If not, set the `SubscriptionInfoFound` status to `False` with reason `ConfigNotFound`.
   3. Otherwise add the config returned by the UDM to the status and the `SubscriptionInfoFound` status to `True` with reason `ConfigReady`.
   4. Write to AMF:RegState.
6. **Control loop** `register-output`. **Purpose:** write state maintained in the internal AMF:RegState back into the user-visible AMF:Registration resources. **Watches:** AMF:RegState. **Predicates:** runs only if AMF:RegState `SubscriptionInfoFound` status is `True`. **Writes**: AMF:Registration.
   1. If each of the `Validated`, `Authenticated`, and `SubscriptionInfoFound` status is `True`, set the `Ready` status to `True` with reason `RegistrationSuccessful`. Otherwise set the `Ready` status to `False` with reason `RegistrationFailed`.
   2. Copy the `Validated` status from the internal state to the AMF:Registration resource status conditions.
   3. Copy the `Authenticated` status from the internal state to the AMF:Registration resource status conditions.
   4. Copy the `SubscriptionInfoFound` status from the internal state to the AMF:Registration resource status conditions.
   5. Copy the rest of the status fields from the AMF:RegState into the AMF:Registration status.
   6. Write to AMF:Registration.
7. **Control loop** `active-registration`. **Purpose:** maintain the `active-registration` table at the AMF. **Watches:** AMF:RegState. **Predicates:** runs only if AMF:RegState `Ready` status is `True`. **Writes**: AMF:ActiveRegistrationTable.
   1. Create an empty AMF:ActiveRegistrationTable resource.
   2. Gather the name, namespace, GUTI and SUCI from all AMF:RegState resources into a list.
   3. Write registration list into the AMF:ActiveRegistrationTable.

The AUSF control loops are as follows:
1. **Control loop** `supi-req-handler`. **Purpose:** look up the SUPI based on the SUCI. **Watches:** AUSF:MobileIdentity. **Predicates:** `GenerationChanged`. **Writes**: AUSF:MobileIdentity.
   1. Look up the SUPI based on the SUCI in the request. If successful, set the `Ready` status to `True` with reason `Ready`, otherwise set `Ready` to `False` with reason `MobileIdentityNotFound`.
   2. Set the label `state:Ready`
   3. Write status back to AUSF:MobileIdentity.

### Usage

Init the operators using the production mode and assume again username is `user-1`.

1. Load the initial config of the UE:

   ```bash
   $ export KUBECONFIG=./user-1-initial.config
   ```

2. Optionally query the initial config. Observe only basic access rights are enabled for the UE, and only to the `registration` resource in the user's own namespace (`user-1`). This effectively isolates UEs from each other, preventing malicious UEs from modifying the registration state of other UEs.

   ```bash
   $ dctl get-config
   👤 User Information:
      Username:   user-1
      Namespaces: [user-1]
      Rules: 1 RBAC policy rules
        [1] verbs=[create get list watch] apiGroups=[amf.view.dcontroller.io] resources=[registration]

   ⏱️  Token Metadata:
      Issuer:     dcontroller
      Issued At:  ...
      Expires At: ...
      Not Before: ...
   ✅ Token is VALID
   ```

2. Register the UE at the AMF:

   ```bash
   $ kubectl apply -f workflows/registration/registration-user-1.yaml
   ```

3. Check registration status: you should get a valid `Ready` status (plus lots of other useful statuses):

   ```bash
   $ kubectl -n user-1 get registration user-1 -o jsonpath='{.status.conditions}'|jq .
   [
     {
       "message": "Registration successful",
       "reason": "RegistrationSuccessful",
       "status": "True",
       "type": "Ready"
     },
     {
       "message": "Validated",
       "reason": "Validated",
       "status": "True",
       "type": "Validated"
     },
     {
       "message": "UE successfully authenticated",
       "reason": "AuthenticationSuccess",
       "status": "True",
       "type": "Authenticated"
     },
     {
       "message": "UE config successfully loaded",
       "reason": "ConfigReady",
       "status": "True",
       "type": "SubscriptionInfoRetrieved"
     }
   ]
   ```

4. Load the config returned by the AMF. The new sets up fine-grained access policies beyond the basic registration workflow. In particular, the UE from now can create, watch, get and list Registration, Session and ContextRelease resources in their own namespace (`user-1`).

   ```bash
   $ kubectl -n user-1 get registration user-1 -o jsonpath='{.status.config}' > ./user-1-full.config
   $ export KUBECONFIG=./user-1-full.config
   ```

   Check the new credentials:

   ```bash
   $ dctl get-config
   👤 User Information:
      Username:   user-1
      Namespaces: [user-1]
      Rules: 1 RBAC policy rules
        [1] verbs=[create get list watch delete] apiGroups=[amf.view.dcontroller.io] resources=[registration session contextrelease]
   
   ⏱️  Token Metadata:
      Issuer:     dcontroller
      Issued At:  ...
      Expires At: ...
      Not Before: ...
   ✅ Token is VALID
   ```

5. In another terminal load the admin config and check the table maintaining the active registrations. Observe that the registration for `user-1` is added to the list

   ```bash
   $ export KUBECONFIG=./admin.config
   $ kubectl get activeregistrationtable --all-namespaces -o yaml
   apiVersion: v1
   items:
   - apiVersion: amf.view.dcontroller.io/v1alpha1
     kind: ActiveRegistrationTable
     metadata:
       name: active-registrations
       uid: f092b6bb-feed-e377-41e1-652ff1d962ed
     spec:
     - guti: test-guti-000000000000000
       name: test-registration
       namespace: test-registration
       suci: test-suci-000000000000000
     - guti: guti-310-170-3F-152-2A-B7C8D9E0
       name: user-1
       namespace: user-1
       suci: suci-0-999-01-02-4f2a7b9c8d13e7a5c0
   kind: List
   metadata:
     resourceVersion: ""
   ```

6. Optionally, clean up the registration:

   ```bash
   $ kubectl delete -f workflows/registration/registration-user-1.yaml
   ```

## Session establishment

### The Session resource

The Session resource is the main driver for creating UE sessions. The UE specifies the session parameters in the spec of the Registration resource and the AMF and the SMF will collaborate to set up the session and add a session status indicating the results. Note that Session resources cannot be created or listed with the initial UE configuration. Therefore, a valid registration foe the UE must exist for a session to be created. Note that multiple sessions (with different name and session id) can be created over a single Registration.

The below dump shows a full Session resource with a valid status set by the AMF:

``` yaml
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: Session
metadata:
  name: user-1-1
  namespace: user-1
  annotations:
    interface: "N1-NAS-SM"              # Protocol interface this represents
spec:
  guti: guti-310-170-3F-152-2A-B7C8D9E0 # GUTI generated in the registration workflow
  idle: null                            # Indicates whether the session is idle (see later)
  nssai: eMBB                           # NSSAI targeted: SST 1: Enhanced Mobile Broadband (eMBB)
  sessionId: 1                          # Must be unique per UE, used to correlate all session messages
  pduSessionType: IPv4                  # PDU Session Type, enum: IPv4 | IPv6 | IPv4v6 | Ethernet | Unstructured
  # Service/Session Continuity for roaming, enum: SSC1 (anchor maintained) | SSC2 (released on move) | SSC3 (flexible)
  sscMode: SSC1
  networkConfiguration:                 # Network Configuration Requests (Protocol Configuration Options)
    requests:
    - addressFamily: IPv4               # Request #1: IP configuration via IPCP
      type: IPConfiguration
    - addressFamily: IPv4               # Request #2: DNS server addresses
      type: DNSServer
  qos:                                  # Quality Of Service SPECIFICATION
    flows:                              # QoS Flows: Define service quality characteristics
    - name: voice-flow                  # Flow 1: Voice flow for VoLTE/VoNR calls
      # Alternative semantic 5QI (5G Quality of Service Identifier) values:
      # - ConversationalVoice (5QI=1): Voice calls
      # - ConversationalVideo (5QI=2): Video calls, 150ms PDB
      # - RealTimeGaming (5QI=3): Gaming, 50ms PDB
      # - NonConversationalVideo (5QI=4): Streaming video, 300ms PDB
      # - IMSSignaling (5QI=5): SIP signaling, 100ms PDB
      # - Video (5QI=6,7,8,9): Various video streaming
      # - BestEffort (5QI=9): Default, no guarantees
      fiveQI: ConversationalVoice
      bitRates:                         # bitrates, subjected to PCF policies
        downlinkBwKbps: 128
        uplinkBwKbps: 128
    - name: best-effort-flow            # Flow 2: Best effort for general data
      fiveQI: BestEffort
    rules:                              # QoS Rules: Packet classification for flow binding
    - name: voice-flow                  # Rule 1: Voice traffic (SIP + RTP)
      precedence: 10                    # Lower number = higher priority (1-255)
      qosFlow: voice-flow               # Reference to flow by name
      default: false                    # match-all: false
      filters:                          # Packet filters for this rule
      - name: sip-signaling
        direction: Bidirectional        # enum: Uplink | Downlink | Bidirectional
        match:
          type: IPFilter
          parameters:
            destinationPort: 5060
            protocol: UDP
      - name: rtp-voice
        direction: Bidirectional
        match:
          parameters:
          type: IPFilter
            destinationPortRange:
              end: 32767
              start: 16384
            protocol: UDP
    - name: default-rule                # Rule 2: Default rule (REQUIRED - exactly one per session)
      precedence: 255                   # Lowest priority
      qosFlow: best-effort-flow         # Reference to flow by name
      default: true                     # Exactly one rule must be default
      filters:
      - direction: Bidirectional
        match:
          type: MatchAll
        name: match-all
status:
  conditions:
  - message: Session successfully established # Session created
    reason: SessionSuccessful
    status: "True"
    type: Ready
  - message: Session request validated        # Session spec validated
    reason: Validated
    status: "True"
    type: Validated
  - message: PCF policies merged              # Policies obtained from the PCF
    reason: PolicyApplied
    status: "True"
    type: PolicyApplied
  - message: UPF configured                   # UPF config generated
    reason: UPFConfigured
    status: "True"
    type: UPFConfigured
  guti: guti-310-170-3F-152-2A-B7C8D9E0
  suci: suci-0-999-01-02-4f2a7b9c8d13e7a5c0
  networkConfiguration:                       # Generated network conciguration
    dnsConfiguration:
      primaryDNS: 8.8.8.8
      secondaryDNS: 8.8.4.4
    ipConfiguration:
      defaultGateway: 10.45.0.1
      ipAddress: 10.45.0.100
      mtu: 1500
      subnetMask: 255.255.0.0
  qos:                                        # QoS policies processed through the PCF
    flows:
    - bitRates:
        downlinkBwKbps: 128
        uplinkBwKbps: 128
      fiveQI: ConversationalVoice
      name: voice-flow
    - fiveQI: BestEffort
      name: best-effort-flow
    rules:
    - default: false
      filters:
      - direction: Bidirectional
        match:
          parameters:
            destinationPort: 5060
            protocol: UDP
          type: IPFilter
        name: sip-signaling
      - direction: Bidirectional
        match:
          parameters:
            destinationPortRange:
              end: 32767
              start: 16384
            protocol: UDP
          type: IPFilter
        name: rtp-voice
      name: voice-rule
      precedence: 10
      qosFlow: voice-flow
    - default: true
      filters:
      - direction: Bidirectional
        match:
          type: MatchAll
        name: match-all
      name: default-rule
      precedence: 255
      qosFlow: best-effort-flow
```

### Control loops

Session resources are first processed by the AMF (Access and Mobility Management Function). Later steps involve the SMF (Session Management Function), the PCF (Policy Control Function), and the UPF (User Plane Function) function.

Consider the below sequence diagram:

``` mermaid
sequenceDiagram
    Note left of UE: Registration created
    UE->>+AMF: Create Session
    Note right of AMF: Validate Session spec
    AMF->>+SMF: Create SessionContext
    Note right of SMF: Process session through the policies obtained from the PCF
    Note right of SMF: Genetate IP and DNS configuration
    SMF->>UPF: Configure UPF with traffic spec
    SMF->>-AMF: Return SessionContext status
    Note right of SMF: Maintain the active-session table
    Note right of AMF: Copy status to Session
    AMF->>-UE: Return Session status
```

The AMF control loops are as follows:
1. **Control loop** `session-input`. **Purpose:** validate AMF:Session resources and write internal state. **Watches:** AMF:Session. **Predicates:** `GenerationChanged`. **Writes:** SMF:SessionContext (SMF internal session state).
   1. Create an empty SMF:SessionContext resource
   2. Initialize status fields.
   3. Check if network configuration request, QoS flows and QoS rules are present in the spec. If not, set `Validated` status to `False` with reason `InvalidSession`.
   4. Check if the selected network slice is `eMBB`. If not, set `Validated` status to `False` with reason `NSSAINotPermitted`.
   5. Check if the GUTI is present in the spec. If not, set `Validated` status to `False` with reason `GutiNotSpeficied`.
   6. Check if the active registration table contains the GUTI. If not, set `Validated` status to `False` with reason `Unregistered`.
   7. Look up the SUPI for the GUTI. If this fails, set `Validated` status to `False` with reason `SupiNotFound`.
   8. Otherwise set the `Validated` status to `True` with reason `Validated`.
   9. Set the GUTI, SUPI and SUCI in the status
   10. Write to the SMF:SessionContext resource
2. **Control loop** `session-output`. **Purpose:** write state maintained in the internal SMF:SessionContext back into the user-visible AMF:Session resource. **Watches:** AMF:SessionContext. **Predicates:** none. **Writes**: AMF:Session.
   1. If each of the `Validated`, `PolicyApplied`, and `UPFConfigured` status is `True`, set the `Ready` status to `True` with reason `SessionSuccessful`. Otherwise set the `Ready` status to `False` with reason `SessionFailed`.
   2. Copy the `Validated` status from the internal state to the AMF:Session resource status conditions.
   3. Copy the `PolicyApplied` status from the internal state to the AMF:Session resource status conditions.
   4. Copy the `UPFConfigured` status from the internal state to the AMF:Session resource status conditions.
   5. Copy the rest of the status fields from the SMF:SessionContext into the AMF:Session status.
   4. Write to AMF:Session.

The SMF control loops are as follows:
1. **Control loop** `session-context-handler`. **Purpose:** query the PCF and apply the returned policies to the session spec. **Watches:** SMF:SessionContext. **Predicates:** none. **Writes:** SMF:SessionContext.
   1. Obtain session policies from the PCF
   2. Process QoS flows through the session policies; currently filters for `ConversationalVoice` and `BestEffort` 5QI (5G Quality of Service Identifier).
   3. Process QoS bitrates through the session policies; cap uplink/downlink bitrates at the values provided by the PCF.
   4. Check if `pduSessionType` is `IPv4`. If not, set `PolicyApplied` status to `False` with reason `AddressFamilyNotSupported`, otherwise set `PolicyApplied` status to `True` with reason `PolicyApplied`
   5. Check if an IP network configuration is requested. If yes, choose a random IP and set netmask, default gateway and MTU.
   6. Check if an DNS configuration is requested. If yes, set primary and secondary DNS server address.
   7. Check if IDLE state is request. If no, set status `UPFConfigured` to `True` with reason `UPFConfigured`, otherwise set `UPFConfigured` to `False` with reason `Idle`
   8. Write SMF:SessionContext
2. **Control loop** `upf-notifier`. **Purpose:** set session traffic spec in the UPF:Config. **Watches:** SMF:SessionContext. **Predicates:** runs only if SMF:SessionContext `Ready` status is `True`. **Writes:** UPF:Config.
   1. Create an empty UPF:Config resource
   2. Copy traffic spec from the SMF:SessionContext to the UPF:Concig
   3. Send UPF:Concig
3. **Control loop** `active-session`. **Purpose:** maintain the `active-session` table at the SMF. **Watches:** SMF:SessionContext. **Predicates:** runs only if SMF:SessionContext `Validated` and `PolicyApplied` status is `True`. **Writes**: SMF:ActiveSessionTable.
   1. Create an empty SMF:ActiveSessionTable resource.
   2. Gather the name, namespace, GUTI and session id from all SMF:SessionContext resources into a list.
   3. Add the idle status in each list member
   4. Write session list into the SMF:ActiveSessionTable.

The UPF control loops are as follows:
1. **Control loop** `active-config`. **Purpose:** maintain the `active-config` table at the UPF. **Watches:** UPF:Config. **Predicates:** none. **Writes**: UPF:ActiveConfigTable.
   1. Create an empty UPF:ActiveConfigTable resource.
   2. Gather the name, namespace, and traffic spec per each UPF:Config resources into a list.
   4. Write config list into the UPF:ActiveConfigTable resource.

### Usage

Make sure a registration exists for the current user name and the full user config is loaded as above. We assume again that the username is `user-1`.

1. Create a session `user-1-1`:

   ```bash
   $ kubectl apply -f workflows/session/session-1-1.yaml
   ```

2. Check session status: you should get a valid `Ready` status (plus lots of other useful statuses):

   ```bash
   $ kubectl get session -n user-1 user-1-1 -o jsonpath='{.status.conditions}'|yq -P
   - message: Session successfully established
     reason: SessionSuccessful
     status: "True"
     type: Ready
   - message: Session request validated
     reason: Validated
     status: "True"
     type: Validated
   - message: PCF policies merged
     reason: PolicyApplied
     status: "True"
     type: PolicyApplied
   - message: UPF configured
     reason: UPFConfigured
     status: "True"
     type: UPFConfigured
   ```

3. The SMF should have created an UPF config for the session. Note that the user cannot access the UPF config, therefore we have to switch to admin access to see the details.

   ```bash
   $ export KUBECONFIG=./admin.config
   $ kubectl get config.upf -n user-1 user-1-1 -o yaml
   apiVersion: upf.view.dcontroller.io/v1alpha1
   kind: Config
   metadata:
     name: user-1-1
     namespace: user-1
   spec:
     networkConfiguration:
       dnsConfiguration:
         primaryDNS: 8.8.8.8
         secondaryDNS: 8.8.4.4
       ipConfiguration:
         defaultGateway: 10.45.0.1
         ipAddress: 10.45.0.100
         mtu: 1500
         subnetMask: 255.255.0.0
     qos:
       flows: ...
       rules: ...
   ```

4. Again, switching to admin access allows you to browse the active session list:

   ```bash
   $ export KUBECONFIG=./admin.config
   $ kubectl get activesessiontable --all-namespaces -o yaml
   apiVersion: v1
   items:
   - apiVersion: smf.view.dcontroller.io/v1alpha1
     kind: ActiveSessionTable
     metadata:
       name: active-sessions
       uid: e66bcdbc-90e8-ac13-fd3c-5dccb77a88a0
     spec:
     - guti: test-guti-000000000000000
       idle: false
       name: test-session
       namespace: test-session
       sessionId: 0
     - guti: guti-310-170-3F-152-2A-B7C8D9E0
       idle: false
       name: user-1-1
       namespace: user-1
       sessionId: 1
   kind: List
   metadata:
     resourceVersion: ""
   ```

## Session idle transition

### The ContextRelease resource

The ContextRelease resource is the main driver for managing active-idle transitions for UE sessions. The gNode-B (on behalf of the UE) requests an idle transition by specifying the GUTI and the session-id in a ContextRelease resource sent to the AMF. The AMF returns the results in the status conditions and notifies the SMF by patching the SessionContext resource with `spec.idle: true`. This causes the SMF to fail the `UPFConfigured` status, which in turn revokes the UPC configuration. Note that the full session state is still maintained in the SessionContext. An idle-active transition can be requested by deleting the ContextRelease resource, which will restore the original traffic spec in the UPF.

The below dump shows a full ContextRelease resource with a valid status:

``` yaml
apiVersion: amf.view.dcontroller.io/v1alpha1
kind: ContextRelease
metadata:
  name: user-1-1
  namespace: user-1
spec:
  guti: guti-310-170-3F-152-2A-B7C8D9E0       # GUTI (Globally Unique Identifier), generated by the AMF
  sessionId: 1                                # Id of the session to be idled
status:
  conditions:
  - message: Context release request accepted # Indicates whether the request was accepted.
    reason: Ready
    status: "True"
    type: Ready
```

### Control loops

ContextRelease resources are first processed by the AMF (Access and Mobility Management Function). Later steps involve the SMF (Session Management Function) and the UPF (User Plane Function) function.

Consider the below sequence diagram:

``` mermaid
sequenceDiagram
    Note left of UE: Registration and session created
    UE-->gNode-B: UE session inactive
    gNode-B->>+AMF: Create ContextRelease
    Note right of AMF: Validate ContextRelease
    AMF->>+SMF: Patch SessionContext with spec.idle:true
    SMF->>UPF: Remove traffic spec from UPF
    SMF->>-AMF: Return SessionContext status
    Note right of AMF: Set ContextRelease Ready status
    AMF->>UE: Update Session status
    AMF->>-gNode-B: Return ContextRelease status
    UE-->gNode-B: UE session active
    gNode-B->>+AMF: Delete ContextRelease
    AMF->>+SMF: Patch SessionContext with spec.idle:false
    SMF->>UPF: Restore traffic spec at UPF
    SMF->>-AMF: Return SessionContext status
    AMF->>UE: Update Session status
```

The AMF control loops are as follows:
1. **Control loop** `session-context-release-input`. **Purpose:** validate AMF:ContextRelease resource. **Watches:** AMF:ContextRelease. **Predicates:** `GenerationChanged`. **Writes:** AMF:ContextRelease.
   1. Check if GUTI is present in the spec. If not, set `Ready` status to `False` with reason `GutiNotSpecified`
   2. Check if the active-registration table contains the GUTI. If not, set `Ready` status to `False` with reason `GutiNotFound`.
   3. Check if the active-session table contains the session id and the GUTI. If not, set `Ready` status to `False` with reason `SessionNotFound`.
   4. Otherwise, set `Ready` status to `True` with reason `Ready`.
   5. Update the AMF:ContextRelease status.
2. **Control loop** `session-context-release-output`. **Purpose:** notify the UPF. **Watches:** AMF:ContextRelease. **Predicates:** runs only if `Ready` status is `True`. **Writes:** SMF:SessionContext (SMF internal session state).
   1. Create an empty SMF:SessionContext patch.
   2. Set metadata
   3. Set `spec.idle` to `true`.
   4. Patch the SMF:SessionContext resource.

### Usage

Make sure a registration and a session exists for the `user-1` and the full user config is loaded.

1. Optionally, set up a watch for the UPF configs of `user-1`. This will dump a line every time we de-activate or activate a session for `user-1` (note that this requires admin access):

   ```bash
   $ kubectl get config.upf -n user-1 -w
   ```

2. Request an idle transition for session `user-1-1`.

   ```bash
   $ kubectl apply -f workflows/session/contextrelease-1-1.yaml
   ```

3. Check the status: you should get a valid `Ready` status:

   ```bash
   $ kubectl get contextrelease -n user-1 user-1-1 -o jsonpath='{.status.conditions}'|yq -P
   - message: Context release request accepted
     reason: Ready
     status: "True"
     type: Ready
   ```

   Meanwhile, the watch should dump a new line `user-1-1`, indicating that the UPF config for the `user-1-1` session has changed. Listing the actual UPF config will show that the config has gone.

   ```bash
   $ kubectl get config.upf -n user-1 user-1-1
   Error from server (NotFound): the server could not find the requested resource
   ```

4. You can re-activate the session by deleting the context release resource.

   ```bash
   $ kubectl delete -f workflows/session/contextrelease-1-1.yaml
   ```

   Again, the watch should dump a new line. Checking the UPF configs (with admin access) will show the config for the session to re-appear, with exactly the same settings as before:

   ```bash
   kubectl get config.upf -n user-1 user-1-1 -o yaml
   apiVersion: upf.view.dcontroller.io/v1alpha1
   kind: Config
   metadata:
     name: user-1-1
     namespace: user-1
   spec:
     networkConfiguration:
       dnsConfiguration:
         primaryDNS: 8.8.8.8
         secondaryDNS: 8.8.4.4
       ipConfiguration:
         defaultGateway: 10.45.0.1
         ipAddress: 10.45.0.100
         mtu: 1500
         subnetMask: 255.255.0.0
     qos:
       flows: ...
       rules: ...
   ```

## Benchmarking

The project contains a comprehensive operator benchmark suite in `internal/operators` for testing the performance and resource use of the 5G operators.

For all benchmarked worflows there are multiple tests:
- **Sequential benchmarks** perform the tested workflow sequentially and measure the time and memory allocations per iteration and CPU usage.
- **Sequential benchmarks with memory statistics** provide detailed memory statistics including the total memory allocated, memory used per registration, heap allocation and GC statistics, an object allocation/deallocation counts. Note that memory profiling comes with nonzero overhead.
- **Sequential benchmarks with memory growth statistics** track memory growth over multiple iterations to detect memory leaks. Meanwhile the tests measure baseline heap memory, memory growth per registration, and memory after cleanup (leak detection). Note that memory profiling comes with nonzero overhead.
- **Parallel benchmarks** for the registration and the session establishment workflow run the tested workflows in parallel and measure the time and the number of memory allocations per iteration, and the CPU usage.

To run all benchmarks:

```bash
$ go test -bench=. -benchmem -run=^$ -timeout=30m
```

CPU profiling:
```bash
$ go test -bench=BenchmarkRegistration$ -benchmem -run=^$ -cpuprofile=cpu.prof
$ go tool pprof cpu.prof
```

Example output:
```
BenchmarkRegistration-4          2   75640243 ns/op  19857804 B/op  202407 allocs/op
```

This means:
- `BenchmarkRegistration-4`: Benchmark name with 4 CPU cores
- `2`: Number of iterations run
- `75640243 ns/op`: ~75.6 milliseconds per operation
- `19857804 B/op`: ~19.8 MB allocated per operation
- `202407 allocs/op`: ~202k memory allocations per operation

### Registration

Tests the registration process by creating multiple UE registrations and waiting for each to complete with `Ready` status.

#### Usage

- Run sequential registration test with default iterations (auto-determined by Go):
  ```bash
  go test -bench=BenchmarkRegistration$ -benchmem -run=^$
  ```
- Run sequential registration test with specific iteration count:
  ```bash
  go test -bench=BenchmarkRegistration$ -benchtime=10x -benchmem -run=^$
  ```
- Run sequential registration test with minimum time duration:
  ```bash
  go test -bench=BenchmarkRegistration$ -benchtime=30s -benchmem -run=^$
  ```
- Run sequential registration test with memory statistics:
  ```bash
  $ go test -bench=BenchmarkRegistrationWithMemStats$ -benchtime=10x -run=^$
  ```
- Run sequential registration test with memory growth statistics:
  ```bash
  $ go test -bench=BenchmarkRegistrationMemoryGrowth$ -benchtime=20x -run=^$
  ```
- Run parallel registration test with default parallelism:
  ```bash
  $ go test -bench=BenchmarkRegistrationParallel$ -benchmem -run=^$
  ```
- Run parallel registration test with specific CPU count:
  ```bash
  $ go test -bench=BenchmarkRegistrationParallel$ -cpu=1,2,4,8 -benchmem -run=^$
  ```

### Results

The following benchmarks were run on an AMD EPYC 7502P 32-Core (64 cores with hyper-threading
enabled) CPU.

#### Parallel (BenchmarkRegistrationParallel)

It takes about 50-100 ms to set up a registration. At about 200 registration an internal queue fills up and the system starts to drop registrations. Meanwhile, CPU load is minimal. Pausing the load generator for a short time to let the queues drain and restoring it after the system becomes responsive again. This sets a firm upper bound on the number of parallel registration operations at 100-200.

| **#Sessions** | **Running time** (ms/op) | **Memory load** (MB/op) | **Allocations** (k-allocs/op) |
|---------------|--------------------------|-------------------------|-------------------------------|
| 1             | 51                       | 29                      | 290                           |
| 2             | 26                       | 19                      | 206                           |
| 5             | 30                       | 25                      | 286                           |
| 10            | 35                       | 27                      | 314                           |
| 20            | 52                       | 40                      | 465                           |
| 50            | 84                       | 55                      | 686                           |
| 100           | 94                       | 58                      | 761                           |
| 200           | 192                      | 113                     | 1568                          |

### Memory profiling (BenchmarkRegistrationMemoryGrowth)

After extensive memory profiling, the below table summarizes the estimated memory usage per
registration.

| Metric                            | Value      | Notes                          |
|-----------------------------------|------------|--------------------------------|
| **Per-operation allocation**      | ~20-35 MB  | From `-benchmem` flag          |
| **Heap growth per registration**  | ~1.3 MB    | From runtime.MemStats tracking |
| **Allocations per operation**     | ~150k-400k | Number of malloc calls         |
| **Live objects per registration** | ~6.5k      | Objects not yet freed (leak)   |

For production deployment planning:

- Base operator overhead: ~20-30 MB
- Per active registration: ~1-2 MB (persistent heap)
- Per registration operation: ~15-20 MB (peak including GC)
- Temporary allocations during operations: ~13-18 MB (gets GC'd)

Example: For 1000 concurrent registrations:
- Persistent memory: 20 MB + (1000 × 1.5 MB) = ~1.5 GB
- Peak during burst: Add 20-50 MB per concurrent operation

### Session establishment

Another set of benchmarks test the session establishment. Note that for all sessions the tests first create a registration, extract the GUTI, and then use that to establish the session. The tests holds on to the objects created until the end of the benchmarks. Finally registrations and sessions are cleaned up.

#### Usage

- Run the sequential session establishment benchmark with default iterations:
  ```bash
  $ go test -bench=BenchmarkSession$ -benchmem -run=^$
  ```
- Run the sequential session establishment benchmark with specific iteration count:
  ```bash
  $ go test -bench=BenchmarkSession$ -benchtime=5x -benchmem -run=^$
  ```
- Run sequential session establishment benchmark with memory statistics:
  ```bash
  $  go test -bench=BenchmarkSessionWithMemStats$ -benchtime=5x -run=^$
  ```
- Run sequential session establishment benchmark with memory growth statistics:
  ```bash
  $ go test -bench=BenchmarkSessionMemoryGrowth -benchtime=5x -run=^$
  ```
  ```bash
  $ go test -bench=BenchmarkSessionParallel$ -benchmem -run=^$
  ```
- Run parallel registration test with specific CPU count:
  ```bash
  $ go test -bench=BenchmarkSessionParallel$ -cpu=1,2,4,8 -benchmem -run=^$
  ```

### Results

The following benchmarks were run on an AMD EPYC 7502P 32-Core (64 cores with hyper-threading enabled) CPU.

#### Parallel (BenchmarkSessionParallel)

It takes about 100-300 ms to set up a session. Due to that each iteration involves a registration + session establishment step, the system fills up faster. The test did not even run for 200 sessions.

| **#Sessions** | **Running time** (ms/op) | **Memory load** (MB/op) | **Allocations** (k-allocs/op) |
|---------------|--------------------------|-------------------------|-------------------------------|
| 1             | 103                      | 40                      | 422                           |
| 2             | 102                      | 31                      | 344                           |
| 5             | 103                      | 30                      | 333                           |
| 10            | 113                      | 34                      | 381                           |
| 20            | 148                      | 49                      | 521                           |
| 50            | 332                      | 13                      | 1118                          |
| 100           | 810                      | 49                      | 2819                          |

### Memory profiling (BenchmarkSessionMemoryGrowth)

After extensive memory profiling, the below table summarizes the estimated memory usage per session.

| Metric                        | Value     | Notes                                   |
|-------------------------------|-----------|-----------------------------------------|
| **Per-operation allocation**  | ~30-40 MB | From `-benchmem` flag                   |
| **Heap growth per session**   | ~1-1.5 MB | From runtime.MemStats tracking          |
| **Allocations per operation** | ~400k-1M  | Number of malloc calls                  |
| **Live objects per session**  | ~174k     | Objects not yet freed per session(leak) |

Most memory allocations are transient (usually caches and internal queues), and eventually sessions cost about 1-2 MB of memory to persist in storage. It seems that it is the number of parallel registration/session operations that is the most constraining factor.

### Active-idle-active transition

The benchmarks test the time and memory needed for active-idle transition. Note that for all tests a single pair of registration and session is created and then in each iteration a state transition is first requested by creating an AMF:ContextRelease, the test then checks if the UPF:Config is gone, the AMF:ContextRelease is then deleted to initiate the reverse transition and finally the test waits until the UPF:Config reappears. Finally the registration and the session is cleaned up.

#### Usage

- Run the state transition benchmark with default iterations:
  ```bash
  $ go test -bench=BenchmarkTransition$ -benchtime=5x -benchmem -run=^$
  ```
- Run the state transition benchmark with memory statistics:
  ```bash
  $  go test -bench=BenchmarkTransitionWithMemStats$ -benchtime=5x -run=^$
  ```
- Run with memory growth statistics
  ```bash
  $ go test -bench=BenchmarkTransitionMemoryGrowth -benchtime=5x -run=^$
  ```

### Results

The following benchmarks were run on an AMD EPYC 7502P 32-Core (64 cores with hyper-threading enabled) CPU.

#### Sequential (BenchmarkTransition)

It takes about 100 ms to perform an active-idle-active transition.

| **#Iterations** | **Running time** (ms/op) | **Memory load** (MB/op) | **Allocations** (k-allocs/op) |
|-----------------|--------------------------|-------------------------|-------------------------------|
| 1               | 101                      | 31                      | 366                           |
| 2               | 102                      | 32                      | 369                           |
| 5               | 102                      | 32                      | 372                           |
| 10              | 102                      | 32                      | 370                           |
| 20              | 102                      | 32                      | 370                           |
| 50              | 101                      | 32                      | 372                           |
| 100             | 101                      | 32                      | 370                           |
| 200             | 102                      | 32                      | 370                           |

### Memory profiling (BenchmarkSessionMemoryGrowth)

After extensive memory profiling, the below table summarizes the estimated memory usage per
session.

| Metric                        | Value     | Notes                          |
|-------------------------------|-----------|--------------------------------|
| **Per-operation allocation**  | ~30 MB    | From `-benchmem` flag          |
| **Heap growth per session**   | ~5k       | From runtime.MemStats tracking |
| **Allocations per operation** | ~360-370k | Number of malloc calls         |
| **Live objects per session**  | ~6.5k     | Objects not yet freed (leak)   |

Since we are performing the transition over and over again on top of the same registration and session, we do not see major memory scalability issues. We are still leaking memory though.

## Testing

To test the full suite, run the operators through the usual Golang test harness: `go test ./... -v -count 1`. Currently the following unit tests are checked:

### AMF Operator
1. Registration:
   - Accept a legitimate registration
   - Reject a registration with invalid reg-type
   - Reject a registration with invalid 5G standard
   - Reject a registration with an empty mobile identity
   - Reject a registration with an unsupported cypher
   - Reject an unknown user
   - Delete a registration and linked resources
   - Register 2 parallel registrations
2. Session:
   - Creating a session for an UE
   - Accept a legitimate session request
   - Reject a session with no network config request
   - Reject a session with no flowspec
   - Reject a session with invalid NSSAI
   - Reject a session with no GUTI
   - Initiating an active->idle state transition: deactive an active session
   - Reject a deactivation request for an unknown registration

### AUSF Operator
1. SUPI-SUCI mapping
   - Initialize a SUCI-to-SUPI table
   - Accept a valid SUPI request
   - Reject an invalid SUPI request

### SMF Operator
1. SessionContext:
   - Accept a legitimate SessionContext
   - Create a UPF config for a legitimate SessionContext
   - Maintain the active session table
2. Active->idle->active status transition
   - Idle an active session

### UDM Operator
1. Config
   - Handle a valid config request

Based on the analysis of the codebase, particularly the operator implementations in `internal/operators/` and the testing suite, here is a **CAVEATS** section suitable for the README.

This section highlights the distinction between this *declarative simulator* and a *production 3GPP core*.

***

## CAVEATS

The purpose if this project is as a Proof of Concept for demonstrating a viability of the declarative control model on a real control plane. While **dctrl5g** accurately models the functional state transitions of a 5G Core Network, users should be aware of the following architectural abstractions and simplifications.

- Control Plane Only (No Data Plane): The UPF (User Plane Function) operator simulates the N4 signaling interface (session configuration, QoS rule installation) but does not perform actual packet forwarding, GTP-U tunneling, or kernel-level routing. No actual user traffic flows through the system.
- Protocol Abstraction (JSON vs. Binary): This project simulates 3GPP signaling logic but replaces the underlying transport protocols. In particular, on the N1/N2 Interfaces the simulator uses Kubernetes API calls with JSON payloads instead of binary NAS (Non-Access Stratum) over SCTP/NGAP (in fact, the NAS protocol is entirely replaced by HTTPS). For the N11/Nsmf interface, service-based interfaces are modeled as CRD watches rather than HTTP/2 REST calls.
- Security Model Mapping: 5G Security is functionally mapped to Kubernetes primitives. Instead of deriving a key and establishing a NAS security context, the UDM generates a Kubernetes ServiceAccount Token (JWT) embedded into a full Kubernetes client config. Possessing this token via the generated kubeconfig represents "being authenticated," allowing the UE to proceed to Session Establishment.
- Subscriber Database: Subscriber data (SUPI-to-GUTI mappings, valid SUCIs) is currently defined in static tables within the operator YAMLs (`amf.yaml`, `ausf.yaml`). The system does not currently interface with an external UDR (Unified Data Repository) or HSS.
- Real-time Constraints: As the logic relies on the Kubernetes API Server's consistency model and the Δ-controller polling/watch mechanism, signaling latency is determined by the API Server's performance and etcd consistency. It does not guarantee the microsecond-level determinism required for real-world radio signaling.
- Partial implementation: The operators deliberately ignore some functions for simplicity, like TAI and location management, EPC/LTE interworking capability.

As usual, use this software at your own risk.

## License

MIT License
