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

3. Create **initial user config**, which will only allow the a user with name `user-1` to register:
   ```bash
   dctl generate-config --user=user-1 --namespaces=user-1 --insecure \
    --rules='[{"verbs":["create","get","list","watch","delete"],"apiGroups":["amf.view.dcontroller.io"],"resources":["registration"]}]' \
    > ./user-1-initial.config
   ```

4. To interact with the API server with **full admin access**, load the config generated as follows:

   ```bash
   dctl generate-config --user=<admin> --insecure \
    --rules='[{"verbs":["*"],"apiGroups":["*"],"resources":["*"]}]' \
    > ./admin.config
   ```

## Workflows

### Registration

Init the operators using the production mode and assume again username is `<user-1>`. 

1. Load the initial config of the user:

   ```bash
   export KUBECONFIG=./user-1-initial.config
   ```

2. Optionally query the initial config. Observe only basic access rights are enabled for the user to the `registration` resource, and only in their own namespace. This effectively isolates users from each other, preventing malicious users from modifying the registration state of other users.

   ```bash
   dctl get-config 
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

2. Register the user at the AMF:

   ```bash
   kubectl apply -f workflows/registration/registration-user-1.yaml
   ```

3. Check registration status: you should get a valid `Ready` status (plus lots of other useful statuses):

   ```bash
   kubectl -n user-1 get registration user-1 -o jsonpath='{.status.conditions}'|jq .
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
   
4. Load the config returned by the AMF: this should now allow fine-grained access policies beyond the basic registration workflow:

   ```bash
   kubectl -n user-1 get registration user-1 -o jsonpath='{.status.config}' > ./user-1-full.config
   export KUBECONFIG=./user-1-full.config
   ```

5. Check the new credentials:

   ```bash
   dctl get-config 
   ...
   ```

6. Optionally, clean up the registration:

   ```bash
   kubectl delete -f workflows/registration/registration-user-1.yaml
   ```
   
## License

MIT License
