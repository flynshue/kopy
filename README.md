# kopy
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster

Deploy controller to the K8s cluster specified in ~/.kube/config.

```bash
make deploy IMG=ghcr.io/flynshue/kopy:<IMAGE-TAG>
```

Example:
```bash
$ make deploy IMG=ghcr.io/flynshue/kopy:v0.0.1-f997fc1

/home/flynshue/github.com/flynshue/kopy/bin/controller-gen-v0.14.0 rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
cd config/manager && /home/flynshue/github.com/flynshue/kopy/bin/kustomize-v5.3.0 edit set image controller=ghcr.io/flynshue/kopy:v0.0.1-f997fc1
/home/flynshue/github.com/flynshue/kopy/bin/kustomize-v5.3.0 build config/default | kubectl apply -f -
namespace/kopy created
serviceaccount/kopy-controller-manager created
role.rbac.authorization.k8s.io/kopy-leader-election-role created
clusterrole.rbac.authorization.k8s.io/kopy-manager-role created
clusterrole.rbac.authorization.k8s.io/kopy-metrics-reader created
clusterrole.rbac.authorization.k8s.io/kopy-proxy-role created
rolebinding.rbac.authorization.k8s.io/kopy-leader-election-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/kopy-manager-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/kopy-proxy-rolebinding created
service/kopy-controller-manager-metrics-service created
deployment.apps/kopy-controller-manager created
```

**NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

### To Uninstall
**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=ghcr.io/flynshue/kopy:<IMAGE-TAG>
```

Example:

```bash
$ make build-installer IMG=ghcr.io/flynshue/kopy:v0.0.1-f997fc1

/home/flynshue/github.com/flynshue/kopy/bin/controller-gen-v0.14.0 rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
/home/flynshue/github.com/flynshue/kopy/bin/controller-gen-v0.14.0 object:headerFile="hack/boilerplate.go.txt" paths="./..."
mkdir -p dist
cd config/manager && /home/flynshue/github.com/flynshue/kopy/bin/kustomize-v5.3.0 edit set image controller=ghcr.io/flynshue/kopy:v0.0.1-f997fc1
/home/flynshue/github.com/flynshue/kopy/bin/kustomize-v5.3.0 build config/default > dist/install.yaml
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/kopy/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Running tests
Here's how to run tests using the [envtest](https://book.kubebuilder.io/reference/envtest.html)
```bash
$ ginkgo -v ./internal/controller/
```

> [!IMPORTANT]
> When running tests with envtest, it will skip tests that involve deleting namespaces due limitations with the envtest https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation

Here's how to run tests using kind cluster
```bash
$ ginkgo -v ./internal/controller/ -- --kind
Running Suite: Controller Suite - /home/flynshue/github.com/flynshue/kopy/internal/controller
```

Here's how to filter tests to files using regex
```bash
$ ginkgo -v --focus-file=secret ./internal/controller/
```
This will run tests in files in `./internal/controller/secret_controller_test.go`

To run operator locally on existing cluster
```bash
$ make run
/home/flynshue/github.com/flynshue/kopy/bin/controller-gen-v0.14.0 rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
/home/flynshue/github.com/flynshue/kopy/bin/controller-gen-v0.14.0 object:headerFile="hack/boilerplate.go.txt" paths="./..."
go fmt ./...
go vet ./...
go run ./cmd/main.go
```