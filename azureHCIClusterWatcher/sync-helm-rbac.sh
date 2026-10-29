#!/usr/bin/env bash
set -euo pipefail

ROLE_FILE="config/rbac/role.yaml"
HELM_FILE="resources/charts/azureHCINodeWatcher/templates/clusterrole.yaml"

if [[ ! -f "$ROLE_FILE" ]]; then
  echo "Error: $ROLE_FILE not found. Run 'make manifests' first."
  exit 1
fi

echo "Syncing RBAC: $ROLE_FILE -> $HELM_FILE"

cat > "$HELM_FILE" <<'HEADER'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/name: {{ .Release.Name }}
    app.kubernetes.io/managed-by: {{ .Release.Service }}
  name: {{ .Release.Name }}-manager-role
rules:
HEADER

grep -A 9999 '^rules:' "$ROLE_FILE" | tail -n +2 >> "$HELM_FILE"
echo "Done. Helm RBAC synced."