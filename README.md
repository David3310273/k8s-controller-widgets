# k8s-controller-widgets

A plugin-based collection of Kubernetes custom controllers. Each plugin is a self-contained
[kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) project living in its own subdirectory.

## Directory Structure

```
k8s-controller-widgets/
├── .devcontainer/              # Shared dev container config (kubebuilder scaffold)
├── .github/workflows/          # Shared CI pipelines
├── hack/boilerplate.go.txt     # Shared license header
├── .golangci.yml               # Shared linter rules
├── .custom-gcl.yml             # Shared golangci-lint plugin config
│
├── configmapWatcher/           # Plugin: watches ConfigMap changes to scale nginx
│   ├── cmd/main.go
│   ├── internal/controller/
│   ├── config/                 # kubebuilder RBAC & kustomize manifests
│   ├── resources/charts/       # Helm chart (bundles controller + sample resources)
│   ├── test/
│   ├── Dockerfile
│   ├── Makefile
│   ├── go.mod / go.sum
│   └── PROJECT
│
└── <your-new-plugin>/          # Add new plugins here
    └── ...
```

## Adding a New Plugin

### Prerequisites

- **Go** 1.23+
- **[kubebuilder](https://github.com/kubernetes-sigs/kubebuilder/releases)** CLI (v4.16+)
- A running **Kubernetes cluster** (e.g., [k3d](https://k3d.io/), [kind](https://kind.sigs.k8s.io/), [minikube](https://minikube.sigs.k8s.io/), or any remote cluster)
- **[Helm](https://helm.sh/)** 3.x
- **[Docker](https://www.docker.com/)** (or any OCI-compatible container tool)

### Step-by-Step

#### 1. Scaffold the kubebuilder project

```bash
# Create plugin directory and init kubebuilder inside it
mkdir <plugin-name> && cd <plugin-name>
kubebuilder init \
  --domain my.demo \
  --repo my.demo/<plugin-name> \
  --project-name <plugin-name>
```

This generates `cmd/`, `config/`, `test/`, `Makefile`, `Dockerfile`, `go.mod`, etc.

#### 2. Scaffold your controller

```bash
# Example: create a controller that watches ConfigMaps
kubebuilder create api \
  --controller true \
  --core true \
  --group core \
  --kind ConfigMap \
  --version v1
```

#### 3. Write your reconciliation logic

Edit `internal/controller/*_controller.go`. The skeleton `Reconcile()` method is where
you compare desired state vs. actual state and take action.

#### 4. Fix shared file references (if needed)

If your plugin's `Makefile` uses `controller-gen` with the boilerplate header
(a standard `kubebuilder init` generates this), update the path since
`hack/boilerplate.go.txt` now lives at the repo root:

```makefile
# Change:
"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt",year=$(YEAR) paths="./..."
# To:
"$(CONTROLLER_GEN)" object:headerFile="../hack/boilerplate.go.txt",year=$(YEAR) paths="./..."
```

Skip this step if your plugin does not use the shared boilerplate.

#### 5. Set up your Helm chart

Create `resources/charts/<plugin-name>/` with at minimum:

```
templates/
├── namespace.yaml
├── serviceaccount.yaml
├── clusterrole.yaml        # RBAC: what your controller needs access to
├── clusterrolebinding.yaml
├── deployment.yaml          # Runs your controller
├── <your-sample-resource>.yaml   # Sample resources for testing
└── <your-configmap>.yaml         # ConfigMap your controller watches (if applicable)
Chart.yaml
values.yaml
```

#### 6. Configure the Makefile

Add k3d + Helm deployment targets to your plugin's `Makefile`. Reference the
existing `configmapWatcher/Makefile` as a template — the key targets are:

```makefile
NAMESPACE ?= controller-playground

helm-deploy:
	helm upgrade --install controller resources/charts/<plugin-name> \
		--namespace ${NAMESPACE} --create-namespace \
		--set image.repository=${IMG} --set image.tag=latest \
		--set image.pullPolicy=Never --wait

k3d-import: docker-build
	k3d image import ${IMG} -c $(CLUSTER)

k3d-deploy: k3d-import helm-deploy
```

#### 7. Register RBAC annotations

Add `// +kubebuilder:rbac` annotations above your reconciler struct so
`make manifests` generates the correct `ClusterRole`. Example:

```go
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
```

#### 8. Deploy and test

```bash
cd <plugin-name>

# Build, import to k3d, and deploy
make k3d-deploy CLUSTER=k3d-default NAMESPACE=controller-playground

# Watch logs
kubectl logs -n ${NAMESPACE} -l control-plane=controller-manager -f

# Test: update and verify
# (varies per plugin)

# Clean up
make helm-undeploy
```

### Design Principles

| Principle | Why |
|-----------|-----|
| Each plugin is an independent Go module， don't reference other plugins' package | Avoids dependency hell between plugins |
| Makefile stays per-plugin | Each plugin has its own build targets and deployment config |
| Deployment via Helm | One `helm install` / `helm uninstall` deploys or tears down everything |
| Shared CI and tooling at root | `kubebuilder init` outputs are identical; no need to duplicate |

## Existing Plugins

| Plugin | Watches | Controls |
|--------|---------|----------|
| [configmapWatcher](./configmapWatcher/) | ConfigMap `service-configs` key | nginx Deployment replicas |