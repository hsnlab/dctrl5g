#  Declarative 5G control plane simulator 

A simulator for the 5G UE and control plane interactions using the declarative Δ-controller framework.

## Getting started

1. Start the operator fleet

   ```bash
   go run main.go -zap-log-level 4
   ```

2. Make a client request:

   ```bash
   export KUBECONFIG=deploy/dcontroller-config # Use the dcontroller API server
   kubectl apply -f <registration.yaml>        # Create a registration
   ```

## License

MIT License
