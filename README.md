# Demo for "End-to-End Automation for Vault on Kubernetes Using the Operator Pattern" presentation at HashiConf Digital October 2020

## Prerequisites

- [`k3d`](https://github.com/rancher/k3d)
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- [`helm`](https://helm.sh/docs/intro/install/)
- [`awscli`](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-install.html)
- An IAM role or user with enough permissions to create a KMS key, or to encrypt with an existing one. [This](https://github.com/patoarvizu/terraform-kms-encryption/tree/master/modules/kms_key) Terraform module can help you create a KMS key with an alias.
- A set of credentials (`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`) with permissions to decrypt.

## Launch a local Kubernetes cluster

We'll be using a [`k3s`](https://github.com/rancher/k3s) cluster (managed by `k3d`), but this should mostly work with any other local Kubernetes provisioner (e.g. [`kind`](https://kind.sigs.k8s.io/), [`minikube`](https://github.com/kubernetes/minikube), [`microk8s`](https://github.com/ubuntu/microk8s), etc.).

- Run `k3d cluster create --port 8080:30080@server[0] --port 8081:30081@server[0] --wait`.
  - The `--port` flags are to map local ports to `Service`s running in the cluster, which will come in handy later.
- Once the setup is finished, run something like `kubectl get nodes` to validate that your cluster is running properly.

## Preparing the infra

We're not going to go into too much detail about why each of these components or resources are required, just know that without they the demo wouldn't work :)

- Run `kubectl apply -f crds/crds.yaml`.
- Run `kubectl apply -f namespaces/namespaces.yaml`.
- Run `kubectl apply -f cert-manager.yaml`.
- Run `kubectl apply -f vault-operator.yaml`.
- Run `kubectl apply -f vault.yaml`.
- Create a Kubernetes `Secret` with the IAM credentials that have permission to decrypt, e.g. `kubectl -n vault create secret generic aws-secrets --from-literal=AWS_ACCESS_KEY_ID=$(echo $AWS_ACCESS_KEY_ID) --from-literal=AWS_SECRET_ACCESS_KEY=$(echo $AWS_SECRET_ACCESS_KEY)` (obviously make sure `$AWS_ACCESS_KEY_ID` and `$AWS_SECRET_ACCESS_KEY` are set on your environment first).
  - These will be used by the `kms-vault-operator`.
- Run `helm repo add kms-vault-operator https://patoarvizu.github.io/kms-vault-operator`.
- Run `helm repo add vault-dynamic-configuration-operator https://patoarvizu.github.io/vault-dynamic-configuration-operator`.
- Run `helm repo add vault-agent-auto-inject-webhook https://patoarvizu.github.io/vault-agent-auto-inject-webhook`.
- Run `helm install kms-vault-operator kms-vault-operator/kms-vault-operator -n vault --version 0.2.0 --set global.prometheusMonitoring.enable=false`
  - Run `kubectl -n vault get pods` after a minute or two (or prefix with `watch`) to check that all components are running. Note: some pods might briefly error or go into `CrashLoopBackOff` but they should eventually recover.

This should be enough to get a Vault cluster going along with the operators/webhooks.

## Encrypt secret for demo-app

Assuming you already have valid IAM credentials on your environment:

- Run `echo Hello, this is a secret! | aws kms encrypt --key-id alias/<my-alias> --plaintext fileb:///dev/stdin`.
  - Replace `<my-alias>` with the name of your KMS key alias. Alternatively you can use the KMS key id instead of `alias/<my-alias>`. Feel free to encrypt any other text that you'd like.
- Copy the `CiphertextBlob` field of the json response.
- Open `demo-app/demo-app-secret.yaml` and paste the ciphertext above as the value for `encryptedSecret`.
- Run `kubectl apply -f demo-app/demo-app-secret.yaml`.
- Run `kubectl describe kmsvaultsecret demo-app`. Make sure that there's an event at the bottom with a message like `Wrote secret demo-app to secret/demo-app`.
  - Notice that the string you see there is not the plaintext secret but the KMS-encrypted version of it. That's what we want.

## Run demo-app

Now let's tie everything together and make sure that our app can authenticate with Vault and read the secret we just created.

- Run `kubectl apply -f demo-app/demo-app.yaml`
- Run `kubectl get pods` a couple of times until the pods stabilize (or with `watch`).
- Run `kubectl get pods -l app=demo-app -o json | jq -r '.items[0].spec.containers'`. This should show you the cointainers running on that pod.
  - Notice how there's an additional container (`vault-agent`) that's not explicitly defined in the `demo-app` deployment. Additionally, the main `demo-app` container has a new environment variable (`VAULT_ADDR`) that was injected automatically as well, to point it to the local `vault-agent` container.
- Test it with `curl localhost:8080/hello`.
  - This should return `Hello, this is a secret!` (or whatever text you encrypted).


## Run another-demo-app

Now it's time to deploy a second app and validate that they can run independently side by side.

First, let's encrypt a different secret for this second app, following similar steps as above, just make the text something else, e.g. `echo Hey, this is another secret! | aws kms encrypt --key-id alias/<my-alias> --plaintext fileb:///dev/stdin`. The rest of the steps should be the same, except you'll modify `demo-app/another-demo-app-secret.yaml` instead. Run `kubectl apply -f demo-app/another-demo-app-secret.yaml`.

Now let's deploy this second application. Run `kubectl apply -f demo-app/another-demo-app.yaml`, give it a few seconds, then run `kubectl get pods` to check that both services are running successfully. Now, run `curl localhost:8081/hello` and you should get `Hey, this is another secret!`, or whatever string you encrypted for this second app.

## Change things up

To demonstrate that the roles and policies that are automatically created provide isolation between services, let's try to change the secret that `another-demo-app` reads from to see if it gets access to `demo-app`'s secrets.

- Run `kubectl edit deployment another-demo-app`.
- Scroll down and edit the `SECRET_PATH` environment variable and set it to `secret/demo-app`. Save and exit.
- Wait a few seconds and run `kubectl get pods` again to check that the new `another-demo-app` pod is `Ready`.
- Run `curl localhost:8081/hello`. The response should read `I can't read that secret :(`, which confirms that the service was trying to access a secret that it doesn't have access to.
- If you run `kubectl edit deployment another-demo-app` and change `SECRET_PATH` back to `secret/another-demo-app` and try `curl localhost:8081/hello` again, it should go back to reading its secret.

## Go further!

Check out [patoarvizu/kms-vault-operator](https://github.com/patoarvizu/kms-vault-operator), [patoarvizu/vault-dynamic-configuration-operator](https://github.com/patoarvizu/vault-dynamic-configuration-operator), and [patoarvizu/vault-agent-auto-inject-webhook](https://github.com/patoarvizu/vault-agent-auto-inject-webhook) to learn more. PRs, issues and comments are always welcome!