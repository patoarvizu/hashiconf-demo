# Demo for "End-to-End Automation for Vault on Kubernetes Using the Operator Pattern" presentation at HashiConf Digital October 2020

## Prerequisites

- [`k3d`](https://github.com/rancher/k3d)
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- [`jq`](https://stedolan.github.io/jq/)
- [`helm`](https://helm.sh/docs/intro/install/)
- [`helmfile`](https://github.com/roboll/helmfile)
- [`awscli`](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-install.html) or some other means of generating an IAM role or user with enough permissions to create a KMS key, or to encrypt with an existing one. [This](https://github.com/patoarvizu/terraform-kms-encryption/tree/master/modules/kms_key) Terraform module can help you create a KMS key with an alias.
- A set of credentials (`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`) with permissions to decrypt the key above.

## Launch a local Kubernetes cluster

We'll be using a [`k3s`](https://github.com/rancher/k3s) cluster (managed by `k3d`), but this should mostly work with any other local Kubernetes provisioner (e.g. [`kind`](https://kind.sigs.k8s.io/), [`minikube`](https://github.com/kubernetes/minikube), [`microk8s`](https://github.com/ubuntu/microk8s), etc.).

- Run `k3d cluster create --port 8080:30080@server[0] --port 8081:30081@server[0] --wait`.
  - The `--port` flags are to map local ports to `Service`s running in the cluster, which will come in handy later.
- Once the setup is finished, run something like `kubectl get nodes` to validate that your cluster is running properly.

## Preparing the infra

`helmfile` should help you set up all Vault infra required on your local cluster for this demo.

- First, export the environment variables that will have the IAM credentials used to decrypt.
  - Run `export DEMO_AWS_ACCESS_KEY_ID=<access-key-id>`, replacing `<access-key-id>` with the corresponding value.
  - Run `export DEMO_AWS_SECRET_ACCESS_KEY=<secret-access-key>` replacing `<secret-access-key>` with the corresponding value.
- Run `helmfile sync`
- Done! The command should wait until all resources have been deployed and are ready.
  - If this fails, simply re-running `helmfile sync` helps in most cases.

## Encrypt secret for demo-app

Assuming you already have valid IAM credentials on your environment:

- Run `echo Hello, this is a secret! | aws kms encrypt --key-id alias/<my-alias> --plaintext fileb:///dev/stdin`.
  - Replace `<my-alias>` with the name of your KMS key alias. Alternatively you can use the KMS key id instead of `alias/<my-alias>`. Feel free to encrypt any other text that you'd like.
- Copy the `CiphertextBlob` field of the json response.
- Open `demo-app/demo-app-secret.yaml` and paste the ciphertext above as the value for `encryptedSecret`.
- Run `kubectl apply -f demo-app/demo-app-secret.yaml`.
- Run `kubectl describe kmsvaultsecret demo-app`. Make sure that there's an event at the bottom with a message like `Wrote secret demo-app to secret/demo-app`. This secret was automatically injected by the [`kms-vault-operator`](https://github.com/patoarvizu/vault-dynamic-configuration-operator).
  - Notice that the string you see there is not the plaintext secret but the KMS-encrypted version of it. That's what we want.

## Run demo-app

Now let's tie everything together and make sure that our app can authenticate with Vault and read the secret we just created.

- Before deploying the app, run `kubectl -n vault get vault vault -o json | jq -r '.spec.externalConfig.policies'`. The only pre-existing policy shold be `allow_secrets`, which is used by the `kms-vault-operator`.
- Run `kubectl apply -f demo-app/demo-app.yaml`
- Run `kubectl -n vault get vault vault -o json | jq -r '.spec.externalConfig.policies'` again. Notice that a new policy called `demo-app` has just shown up! This policy was automatically added by the the [`vault-dynamic-configuration-operator`](https://github.com/patoarvizu/vault-dynamic-configuration-operator) and it gives the `demo-app` role read-only access to `secret/demo-app` only.
  - Note that this is the "desired" state for the `Vault` custom resource, and doesn't necessarily reflect the actual configuration of Vault. It can take about a minute or two for the `vault-operator` to sync up the configuration with the backend.
- Run `kubectl get pods` a couple of times until the pods stabilize (or with `watch`).
- Run `kubectl get pods -l app=demo-app -o json | jq -r '.items[0].spec.containers'`. This should show you the cointainers running on that pod.
  - Notice how there's an additional container (`vault-agent`) that's not explicitly defined in the `demo-app` deployment (which you can verify by running `kubectl describe deployment demo-app`).
  - Additionally, the main `demo-app` container has a new environment variable (`VAULT_ADDR`) that was injected automatically as well, to point it to the local `vault-agent` container.
  - These modifications were done by the [`vault-agent-auto-inject-webhook`](https://github.com/patoarvizu/vault-agent-auto-inject-webhook).
- Test it with `curl localhost:8080/hello`.
  - This should return `Hello, this is a secret!` (or whatever text you encrypted).
  - If you get `I can't read that secret :(`, it means the backend policies haven't synced up yet, give it a little bit more time.

## Run another-demo-app

Now it's time to deploy a second app and validate that they can run independently side by side.

First, let's encrypt a different secret for this second app, following similar steps as above, just make the text something else, e.g. `echo Hey, this is another secret! | aws kms encrypt --key-id alias/<my-alias> --plaintext fileb:///dev/stdin`. The rest of the steps should be the same, except you'll modify `demo-app/another-demo-app-secret.yaml` instead. Run `kubectl apply -f demo-app/another-demo-app-secret.yaml`.

Now let's deploy this second application. Run `kubectl apply -f demo-app/another-demo-app.yaml`, give it a few seconds, then run `kubectl get pods` to check that both services are running successfully, and also check that the new policy was added by running `kubectl -n vault get vault vault -o json | jq -r '.spec.externalConfig.policies`. Wait about a minute to allow the `vault-operator` to catch up, then run `curl localhost:8081/hello` and you should get `Hey, this is another secret!`, (or the string you encrypted for this second app above). Remember that if you get `I can't read that secret :(`, you might need to wait a little longer for the policies to sync up.

## Change things up

To demonstrate that the roles and policies that are automatically created provide isolation between services, let's try to change the secret that `another-demo-app` reads from to see if it gets access to `demo-app`'s secrets.

- Run `kubectl edit deployment another-demo-app`.
- Scroll down and edit the `SECRET_PATH` environment variable and set it to `secret/demo-app`. Save and exit.
- Wait a few seconds and run `kubectl get pods` again to check that the new `another-demo-app` pod is `Ready`.
- Run `curl localhost:8081/hello`. The response should read `I can't read that secret :(`, which confirms that the service was trying to access a secret that it doesn't have access to.
- If you run `kubectl edit deployment another-demo-app` and change `SECRET_PATH` back to `secret/another-demo-app` and try `curl localhost:8081/hello` again, it should go back to reading its secret.

## Go further!

Check out [patoarvizu/kms-vault-operator](https://github.com/patoarvizu/kms-vault-operator), [patoarvizu/vault-dynamic-configuration-operator](https://github.com/patoarvizu/vault-dynamic-configuration-operator), and [patoarvizu/vault-agent-auto-inject-webhook](https://github.com/patoarvizu/vault-agent-auto-inject-webhook) to learn more. PRs, issues and comments are always welcome!

## Something not working?

Open a PR, or get in touch with me and I'd be happy to help.