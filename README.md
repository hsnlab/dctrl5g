#  Declarative 5G control plane simulator 

A simulator for the 5G UE and control plane interactions using the declarative Δ-controller framework.

## Getting started

You will need the `dctl` command line tool to administer kubeconfigs, obtain it from
[here](https://github.com/l7mp/dcontroller).

### Development

1. Start the operators using unsafe HTTP mode:
   ```bash
   go run main.go --http -zap-log-level 4
   ```

2. Create an admin config:
   ```bash
   dctl generate-config --http --insecure --user=admin --namespaces="*" > ./admin.config
   ```

3. Make a client request:
   ```bash
   export KUBECONFIG=./admin.config 
   kubectl apply -f <registration.yaml>
   ```

### Production

1. Generate the TLS certificate:
   ```bash
   dctl generate-keys
   ```

2. Start the operators:
   ```bash
   go run main.go -zap-log-level 4
   ```

3. Create initial user config (assume the username is `user-1`), which will only allow the user to register:
   ```bash
   dctl generate-config --user=user-1 --namespaces=default --insecure \
    --rules='[{"verbs":["create","get","list","watch"],"apiGroups":["amf.view.dcontroller.io"],"resources":["registration"]}]' \
    > ./user-1-initial.config
   ```

4. Make a client request:
   ```bash
   export KUBECONFIG=./user-1.initial.config
   kubectl apply -f <registration.yaml>
   ```

## License

MIT License
