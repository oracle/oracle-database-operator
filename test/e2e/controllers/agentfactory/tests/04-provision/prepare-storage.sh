#!/usr/bin/env bash
set -Eeuo pipefail

: "${AGENTFACTORY_STORAGE_CLASS:?AGENTFACTORY_STORAGE_CLASS is required}"
: "${AGENTFACTORY_FSS_AVAILABILITY_DOMAIN:=}"
: "${AGENTFACTORY_SETUP_MANIFEST_DIR:?AGENTFACTORY_SETUP_MANIFEST_DIR is required}"
: "${AGENTFACTORY_SETUP_ORDER_FILE:?AGENTFACTORY_SETUP_ORDER_FILE is required}"
: "${AGENTFACTORY_FSS_MOUNT_TARGET_OCID:=}"
: "${AGENTFACTORY_FSS_MOUNT_TARGET_SUBNET_OCID:=}"
: "${AGENTFACTORY_FSS_EXPORT_PATH:=}"
: "${AGENTFACTORY_FSS_COMPARTMENT_OCID:=}"

mount_target_ocid="$AGENTFACTORY_FSS_MOUNT_TARGET_OCID"
mount_target_subnet_ocid="$AGENTFACTORY_FSS_MOUNT_TARGET_SUBNET_OCID"
if [ -n "$mount_target_ocid" ] && [ -n "$mount_target_subnet_ocid" ]; then
  echo "Agent Factory test04: set only one of AGENTFACTORY_FSS_MOUNT_TARGET_OCID or AGENTFACTORY_FSS_MOUNT_TARGET_SUBNET_OCID" >&2
  exit 2
fi

if [ -n "${AGENTFACTORY_MANAGED_STORAGE_CLASS:-}" ]; then
  provisioner="$(kubectl get storageclass "$AGENTFACTORY_MANAGED_STORAGE_CLASS" -o jsonpath='{.provisioner}' 2>/dev/null || true)"
  [ "$provisioner" = "fss.csi.oraclecloud.com" ] || {
    echo "Agent Factory test04: existing StorageClass $AGENTFACTORY_MANAGED_STORAGE_CLASS is not an OCI FSS class" >&2
    exit 2
  }
  export AGENTFACTORY_STORAGE_CLASS="$AGENTFACTORY_MANAGED_STORAGE_CLASS"
  printf '01 storageclass status=reused name=%s\n' "$AGENTFACTORY_STORAGE_CLASS" >> "$AGENTFACTORY_SETUP_ORDER_FILE"
  kubectl get storageclass "$AGENTFACTORY_STORAGE_CLASS" -o yaml > "$AGENTFACTORY_SETUP_MANIFEST_DIR/01-storageclass.yaml"
  exit 0
fi

if [ -z "$mount_target_ocid" ] && [ -z "$mount_target_subnet_ocid" ]; then
  echo "Agent Factory test04: provide AGENTFACTORY_FSS_MOUNT_TARGET_OCID or AGENTFACTORY_FSS_MOUNT_TARGET_SUBNET_OCID" >&2
  exit 2
fi
: "${AGENTFACTORY_FSS_AVAILABILITY_DOMAIN:?AGENTFACTORY_FSS_AVAILABILITY_DOMAIN is required when creating a StorageClass}"
if [ -n "$AGENTFACTORY_FSS_EXPORT_PATH" ] && [[ "$AGENTFACTORY_FSS_EXPORT_PATH" != /* ]]; then
  echo "Agent Factory test04: AGENTFACTORY_FSS_EXPORT_PATH must start with /" >&2
  exit 2
fi

storage_class_file="$AGENTFACTORY_SETUP_MANIFEST_DIR/01-storageclass.yaml"
{
  printf '%s\n' \
    'apiVersion: storage.k8s.io/v1' \
    'kind: StorageClass' \
    'metadata:' \
    '  name: '"$AGENTFACTORY_STORAGE_CLASS" \
    '  labels:' \
    '    e2e.oracle.com/owner: operator-e2e' \
    '    e2e.oracle.com/controller: agentfactory' \
    '    e2e.oracle.com/test-id: test04' \
    '    e2e.oracle.com/pipeline: "'"${AGENTFACTORY_CASE_PIPELINE:-local}"'"' \
    'provisioner: fss.csi.oraclecloud.com' \
    'reclaimPolicy: Delete' \
    'volumeBindingMode: Immediate' \
    'parameters:' \
    '  availabilityDomain: '"$AGENTFACTORY_FSS_AVAILABILITY_DOMAIN"
  if [ -n "$mount_target_ocid" ]; then
    printf '%s\n' '  mountTargetOcid: '"$mount_target_ocid"
  else
    printf '%s\n' '  mountTargetSubnetOcid: '"$mount_target_subnet_ocid"
  fi
  if [ -n "$AGENTFACTORY_FSS_COMPARTMENT_OCID" ]; then
    printf '%s\n' '  compartmentOcid: '"$AGENTFACTORY_FSS_COMPARTMENT_OCID"
  fi
  if [ -n "$AGENTFACTORY_FSS_EXPORT_PATH" ]; then
    printf '%s\n' '  exportPath: '"$AGENTFACTORY_FSS_EXPORT_PATH"
  fi
} > "$storage_class_file"

kubectl apply -f "$storage_class_file"
printf '01 storageclass status=created name=%s provisioner=fss.csi.oraclecloud.com reclaimPolicy=Delete\n' \
  "$AGENTFACTORY_STORAGE_CLASS" >> "$AGENTFACTORY_SETUP_ORDER_FILE"
