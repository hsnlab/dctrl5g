# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a 5G UE and control plane simulator built on the declarative Δ-controller framework (github.com/l7mp/dcontroller). The system simulates 5G core network functions including AMF (Access and Mobility Management Function), AUSF (Authentication Server Function), and UDM (Unified Data Management).

## Build and Run Commands

### Development Mode (HTTP, no authentication)
```bash
go run main.go --http -zap-log-level 4
```

### Production Mode (HTTPS with self-signed certificate and authentication)
```bash
# Generate TLS certificates first
dctl generate-keys

# Start the operators with insecure flag for self-signed cert
go run main.go --insecure -zap-log-level 4
```

### Production Mode (HTTPS with CA-signed certificate)
```bash
# Start with proper TLS certificate (no --insecure flag needed)
go run main.go --tls-cert-file=/path/to/cert.pem --tls-key-file=/path/to/key.pem -zap-log-level 4
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/operators/
go test ./internal/operators/udm/

# Run tests with verbose output
go test -v ./...
```

### Generate Admin Config
```bash
dctl generate-config --http --insecure --user=admin --namespaces="*" > ./admin.config
```

### Generate User Config
```bash
dctl generate-config --user=user-1 --namespaces=user-1 --insecure \
  --rules='[{"verbs":["create","get","list","watch"],"apiGroups":["amf.view.dcontroller.io"],"resources":["*"]}]' \
  > ./user-1.config
```

### Client Requests
```bash
export KUBECONFIG=./admin.config
kubectl apply -f <registration.yaml>
```

## Architecture

### Core Components

1. **main.go**: Entry point that:
   - Configures logging with zap
   - Loads operator YAML specs from `internal/operators/` directory
   - Initializes the dctrl system with API server configuration
   - Handles TLS/authentication setup

2. **internal/dctrl/dctrl.go**: Core framework integration that:
   - Creates operator groups from YAML specifications
   - Configures the embedded API server (default port 8443)
   - Sets up JWT authentication and RBAC authorization
   - Manages operator lifecycle

3. **Operator Specifications** (internal/operators/*.yaml):
   - **AMF (amf.yaml)**: Access and Mobility Management Function
     - Handles UE registration requests
     - Validates encryption/integrity algorithms and mobile identity
     - Coordinates SUPI-to-SUCI mapping via AUSF
     - Manages GUTI assignment and configuration delivery
     - Uses multi-stage pipeline with state labels (Initialized, SupiAvailable, GutiAvailable, ConfigAvailable, etc.)
   - **AUSF (ausf.yaml)**: Authentication Server Function
     - Provides SUCI-to-SUPI mapping service
     - Maintains subscriber identity tables
     - Decrypts concealed subscriber identities

4. **UDM Operator** (internal/operators/udm/udm.go):
   - Implemented as a native Go controller (not YAML-based)
   - Generates kubeconfig credentials for authenticated UEs
   - Uses JWT token generation with configurable namespaces and RBAC rules
   - Watches Config custom resources and updates their status

### Operator Pattern

The system uses the Δ-controller framework which provides:
- Declarative operator specifications using YAML with JSONPath-like pipelines
- Controllers that transform source objects into target objects through aggregation, projection, filtering, and joining
- Built-in predicates (GenerationChanged, etc.) for event filtering
- State management through Kubernetes-style labels and conditions
- Cross-operator communication via custom resource definitions

### 5G Identity Flow

1. **SUPI** (Subscription Permanent Identifier): Permanent subscriber ID (like IMSI)
2. **SUCI** (Subscription Concealed Identifier): Encrypted SUPI sent over air interface
3. **GUTI** (5G Globally Unique Temporary Identity): Temporary ID assigned by AMF
4. Registration flow: UE → AMF (SUCI) → AUSF (SUPI lookup) → AMF (GUTI assignment) → UDM (config generation)

### API Server

- Default address: localhost:8443
- Supports both HTTP (insecure, for development) and HTTPS modes
- JWT-based authentication with RSA key pairs
- RBAC authorization with per-user namespace and resource rules
- Kubernetes-style API with custom resource definitions

### Testing Infrastructure

- Uses Ginkgo/Gomega for BDD-style tests
- Test suite helper (internal/testsuite/suite.go) provides:
  - Automatic TLS certificate generation
  - Random port allocation for parallel test execution
  - Operator lifecycle management
  - Error channel monitoring

## Development Notes

- The project uses Go modules with local replacement for `github.com/l7mp/dcontroller` pointing to `/export/l7mp/dcontroller/`
- Operator YAML files use a specialized pipeline DSL with operators like `@aggregate`, `@project`, `@join`, `@select`, `@cond`, `@has`, `@eq`, `@or`, `@not`, `@in`, `@concat`, `@now`
- Controllers in operator specs can have multiple sources with different predicates and label selectors
- State transitions in AMF are managed through label-based state machines
- The UDM operator is hybrid: it uses the operator framework but implements a custom reconciler in Go

## Important Conventions

- Operator directory: `internal/operators/` contains YAML operator specifications
- Go source follows standard Go project layout with `internal/` for private packages
- API groups follow pattern: `<function>.view.dcontroller.io/v1alpha1`
- Custom resources use Kubernetes-style metadata, spec, and status sections
- Status conditions follow Kubernetes conventions with type, status, reason, message, lastTransitionTime
