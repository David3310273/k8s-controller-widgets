#!/usr/bin/env bash
set -euo pipefail

CRD_DIR="config/crd/bases"
HELM_CRD_DIR="resources/charts/azureHCINodeWatcher/crds"

if [[ ! -d "$CRD_DIR" ]]; then
  echo "Error: $CRD_DIR not found. Run 'make manifests' first."
  exit 1
fi

echo "Syncing CRDs: $CRD_DIR -> $HELM_CRD_DIR"

mkdir -p "$HELM_CRD_DIR"
cp "$CRD_DIR"/*.yaml "$HELM_CRD_DIR/"
echo "Done. Helm CRDs synced."