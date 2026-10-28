# configmapWatcher

A Kubernetes custom controller that watches a ConfigMap key and scales a target Deployment
to match the specified replica count.

The controller itself is deployed as a standalone workload. It operates on **your existing**
ConfigMap and Deployment resources — you provide their names at deploy time via CLI flags.

## How It Works

```
kubectl edit configmap <your-configmap>
       │
       ▼
configmapWatcher detects the change
       │
       ├── Reads ConfigMap → parses JSON → extracts replica count
       ├── Queries target Deployment → current replicas
       ├── Equal → skip
       └── Not equal → update Deployment replicas
```

## Prerequisites

- Go 1.23+
- Docker (or any OCI-compatible container tool)
- A Kubernetes cluster (k3d / kind / minikube / remote)
- Helm 3.x

Before deploying, create the resources the controller will manage:

```yaml
# Example ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-app-config
  namespace: controller-playground
data:
  service-configs: |
    {
      "serviceConfigs": {
        "workerCount": 4,
        "service": { "port": 8080 }
      }
    }
---
# Example Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: controller-playground
  labels:
    app: nginx
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx:1.27
          ports:
            - containerPort: 80
```

## Quick Deploy

```bash
# Run from the configmapWatcher directory

# One-shot build + deploy (k3d cluster)
make k3d-deploy CLUSTER=k3d-default

# Override namespace (default: controller-playground)
make k3d-deploy NAMESPACE=my-namespace

# Helm-only deploy (for non-k3d clusters)
make docker-build IMG=my-registry/configmap-watcher:latest
make docker-push IMG=my-registry/configmap-watcher:latest
make helm-deploy IMG=my-registry/configmap-watcher:latest
```

### What Gets Deployed

`make helm-deploy` deploys **only the controller** in `NAMESPACE` (default `controller-playground`):

| Resource | Purpose |
|----------|---------|
| Namespace | Isolation for the controller |
| ServiceAccount | Controller Pod identity |
| ClusterRole + ClusterRoleBinding | RBAC (ConfigMap + Deployment access) |
| Deployment | Runs the configmapWatcher |

The target ConfigMap and Deployment are **not** deployed by the chart — they are your
pre-existing resources.

### Controller CLI Flags

These configure which resources the controller operates on:

| Flag | Default | Description |
|------|---------|-------------|
| `--target-namespace` | `controller-playground` | Namespace of the target Deployment |
| `--target-deployment` | `nginx-deployment` | Name of the Deployment to scale |
| `--configmap-name` | `nginx-app-config` | Name of the ConfigMap to watch. Empty = watch all |

The controller reads the `service-configs` key from the ConfigMap `data` and decodes its
value as JSON into a `ConfigMapPayload` struct (`{"serviceConfigs": {...}}`), taking
`serviceConfigs.workerCount` as the desired replica count. Other keys in `data` are ignored.

Override them when deploying:

```bash
make k3d-deploy -- \
  --set controller.targetNamespace=production \
  --set controller.targetDeployment=my-app \
  --set controller.configmapName=my-config
```

Or when running locally:

```bash
go run cmd/main.go \
  --target-namespace=production \
  --target-deployment=my-app \
  --configmap-name=my-config
```

See [values.yaml](resources/charts/configmapWatcher/values.yaml) for all configurable parameters.

## Verification

```bash
# 1. Check deployed resources
kubectl get all -n controller-playground

# 2. Watch controller logs
kubectl logs -n controller-playground \
  -l control-plane=controller-manager -f

# 3. Trigger scaling by editing your ConfigMap
kubectl edit configmap nginx-app-config -n controller-playground
# Change workerCount to 6, save and exit

# 4. Watch target Deployment replicas change
kubectl get deployment nginx-deployment -n controller-playground -w
```

## Undeploy

```bash
# Remove the controller (your ConfigMap and Deployment are left untouched)
make helm-undeploy

# Specify namespace
make helm-undeploy NAMESPACE=my-namespace
```

## Development

```bash
# Run locally (no image build)
make run

# Compile only
make build

# Run unit tests
make test

# Lint
make lint
```

## Makefile Target Reference

```
make help              # List all targets

# Development
make manifests         # Generate RBAC (config/rbac/role.yaml)
make generate          # Generate deepcopy code
make build             # Build binary
make run               # Run locally
make test              # Run tests
make lint              # Lint

# Images
make docker-build      # Build Docker image
make docker-push       # Push Docker image

# Deployment
make sync-helm-rbac    # Sync RBAC from config/rbac/role.yaml to Helm chart
make helm-deploy       # Helm deploy (auto-syncs RBAC)
make helm-undeploy     # Helm uninstall
make k3d-deploy        # Build → import to k3d → Helm deploy (one-shot)
```

## RBAC Sync

Every `helm-deploy` or `k3d-deploy` automatically runs `sync-helm-rbac` beforehand,
which copies the rules from `config/rbac/role.yaml` (generated by `make manifests`
from code annotations) into the Helm chart's `clusterrole.yaml`. This ensures
RBAC permissions never drift out of sync.

After editing `// +kubebuilder:rbac` annotations on your controller, simply
deploy as usual — the sync happens automatically.