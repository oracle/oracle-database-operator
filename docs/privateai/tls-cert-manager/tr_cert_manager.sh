#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Creates or updates a cert-manager Certificate and waits for the TLS Secret.

Required environment variables:
  NAMESPACE           Namespace for the Certificate
  ISSUER_NAME         Issuer or ClusterIssuer name
  COMMON_NAME         Certificate common name

Optional environment variables:
  TARGET              pai or nginx (default: pai)
  CERT_NAME           Certificate resource name
  SECRET_NAME         Output TLS Secret name
  ISSUER_KIND         ClusterIssuer or Issuer (default: ClusterIssuer)
  GENERATE_PKCS12     true/false; include keystore.p12 in the secret (default: false)
  PASSWORD_SECRET_NAME  Secret name holding the PKCS#12 password
  PASSWORD_SECRET_KEY   Secret key holding the PKCS#12 password (default: privateai-ssl-pwd)
  DURATION            Certificate duration (default: 2160h)
  RENEW_BEFORE        Renewal window (default: 360h)
  PRIVATE_KEY_ALGO    RSA or ECDSA (default: RSA)
  PRIVATE_KEY_SIZE    RSA key size or ECDSA curve size hint (default: 3072)
  DNS_NAMES           Comma-separated DNS SANs
  IP_ADDRESSES        Comma-separated IP SANs
  WAIT_TIMEOUT        Wait timeout for certificate readiness (default: 300s)
  APPLY_ONLY          true/false; if true, do not wait (default: false)

Examples:
  NAMESPACE=pai \
  TARGET=pai \
  ISSUER_NAME=tcps-selfsigned-bootstrap \
  ISSUER_KIND=ClusterIssuer \
  COMMON_NAME=api.example.com \
  DNS_NAMES="api.example.com,pai-sample.pai.svc,pai-sample.pai.svc.cluster.local" \
  IP_ADDRESSES="141.148.67.224" \
  ./docs/privateai/tls-cert-manager/tr_cert_manager.sh

  NAMESPACE=pai \
  TARGET=nginx \
  ISSUER_NAME=tcps-selfsigned-bootstrap \
  COMMON_NAME=api.example.com \
  DNS_NAMES="api.example.com,pai-nginx.pai.svc,pai-nginx.pai.svc.cluster.local" \
  ./docs/privateai/tls-cert-manager/tr_cert_manager.sh

  NAMESPACE=pai \
  TARGET=pai \
  ISSUER_NAME=tcps-selfsigned-bootstrap \
  COMMON_NAME=api.example.com \
  GENERATE_PKCS12=true \
  PASSWORD_SECRET_NAME=paisecret \
  PASSWORD_SECRET_KEY=privateai-ssl-pwd \
  ./docs/privateai/tls-cert-manager/tr_cert_manager.sh
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

trim() {
  echo "$1" | xargs
}

is_ipv4() {
  [[ "$1" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]
}

emit_list() {
  local indent="$1"
  shift
  for item in "$@"; do
    [[ -n "$item" ]] && printf "%s- %s\n" "$indent" "$item"
  done
}

TARGET="${TARGET:-pai}"
case "$TARGET" in
  pai)
    DEFAULT_CERT_NAME="privateai-cert"
    DEFAULT_SECRET_NAME="privateai-tls"
    TARGET_REF="PrivateAi.spec.security.tls.secretName"
    ;;
  nginx)
    DEFAULT_CERT_NAME="pai-nginx-cert"
    DEFAULT_SECRET_NAME="pai-nginx-tls"
    TARGET_REF="TrafficManager.spec.security.tls.secretName"
    ;;
  *)
    echo "TARGET must be one of: pai, nginx" >&2
    exit 1
    ;;
esac

: "${NAMESPACE:?set NAMESPACE}"
: "${ISSUER_NAME:?set ISSUER_NAME}"
: "${COMMON_NAME:?set COMMON_NAME}"

CERT_NAME="${CERT_NAME:-$DEFAULT_CERT_NAME}"
SECRET_NAME="${SECRET_NAME:-$DEFAULT_SECRET_NAME}"
ISSUER_KIND="${ISSUER_KIND:-ClusterIssuer}"
DURATION="${DURATION:-2160h}"
RENEW_BEFORE="${RENEW_BEFORE:-360h}"
PRIVATE_KEY_ALGO="${PRIVATE_KEY_ALGO:-RSA}"
PRIVATE_KEY_SIZE="${PRIVATE_KEY_SIZE:-3072}"
GENERATE_PKCS12="${GENERATE_PKCS12:-false}"
PASSWORD_SECRET_NAME="${PASSWORD_SECRET_NAME:-}"
PASSWORD_SECRET_KEY="${PASSWORD_SECRET_KEY:-privateai-ssl-pwd}"
DNS_NAMES="${DNS_NAMES:-}"
IP_ADDRESSES="${IP_ADDRESSES:-}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-300s}"
APPLY_ONLY="${APPLY_ONLY:-false}"

require_cmd kubectl
require_cmd openssl

if ! kubectl get crd certificates.cert-manager.io >/dev/null 2>&1; then
  echo "cert-manager CRD certificates.cert-manager.io not found" >&2
  exit 1
fi

if ! kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
  echo "Namespace $NAMESPACE does not exist" >&2
  exit 1
fi

if [[ "$ISSUER_KIND" == "ClusterIssuer" ]]; then
  kubectl get clusterissuer "$ISSUER_NAME" >/dev/null
else
  kubectl get issuer -n "$NAMESPACE" "$ISSUER_NAME" >/dev/null
fi

if [[ "$GENERATE_PKCS12" == "true" ]]; then
  if [[ -z "$PASSWORD_SECRET_NAME" ]]; then
    echo "set PASSWORD_SECRET_NAME when GENERATE_PKCS12=true" >&2
    exit 1
  fi
  if ! kubectl get secret "$PASSWORD_SECRET_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
    echo "password secret $PASSWORD_SECRET_NAME not found in namespace $NAMESPACE" >&2
    exit 1
  fi
  password_key_value="$(kubectl get secret "$PASSWORD_SECRET_NAME" -n "$NAMESPACE" -o "jsonpath={.data.${PASSWORD_SECRET_KEY}}" 2>/dev/null || true)"
  if [[ -z "$password_key_value" ]]; then
    echo "password secret key $PASSWORD_SECRET_KEY not found in secret $PASSWORD_SECRET_NAME" >&2
    exit 1
  fi
fi

dns=()
ips=()

if is_ipv4 "$COMMON_NAME"; then
  ips+=("$COMMON_NAME")
else
  dns+=("$COMMON_NAME")
fi

IFS=',' read -r -a dns_extra <<< "$DNS_NAMES"
for d in "${dns_extra[@]}"; do
  d="$(trim "$d")"
  [[ -n "$d" ]] && dns+=("$d")
done

IFS=',' read -r -a ip_extra <<< "$IP_ADDRESSES"
for ip in "${ip_extra[@]}"; do
  ip="$(trim "$ip")"
  [[ -n "$ip" ]] && ips+=("$ip")
done

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

cat > "$tmp" <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${CERT_NAME}
  namespace: ${NAMESPACE}
spec:
  secretName: ${SECRET_NAME}
  issuerRef:
    name: ${ISSUER_NAME}
    kind: ${ISSUER_KIND}
  commonName: ${COMMON_NAME}
  duration: ${DURATION}
  renewBefore: ${RENEW_BEFORE}
  privateKey:
    algorithm: ${PRIVATE_KEY_ALGO}
    size: ${PRIVATE_KEY_SIZE}
    rotationPolicy: Always
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF

if [[ "${#dns[@]}" -gt 0 ]]; then
  {
    echo "  dnsNames:"
    emit_list "  " "${dns[@]}"
  } >> "$tmp"
fi

if [[ "${#ips[@]}" -gt 0 ]]; then
  {
    echo "  ipAddresses:"
    emit_list "  " "${ips[@]}"
  } >> "$tmp"
fi

if [[ "$GENERATE_PKCS12" == "true" ]]; then
  cat >> "$tmp" <<EOF
  keystores:
    pkcs12:
      create: true
      passwordSecretRef:
        name: ${PASSWORD_SECRET_NAME}
        key: ${PASSWORD_SECRET_KEY}
EOF
fi

echo "Applying Certificate ${CERT_NAME} in namespace ${NAMESPACE} for TARGET=${TARGET}"
kubectl apply -f "$tmp"

echo
echo "Certificate summary:"
kubectl get certificate "$CERT_NAME" -n "$NAMESPACE" -o wide || true

if [[ "$APPLY_ONLY" == "true" ]]; then
  echo
  echo "Apply-only mode enabled. Skipping wait."
  echo "Wire secret ${SECRET_NAME} into ${TARGET_REF}"
  exit 0
fi

echo
echo "Waiting for Certificate to become Ready..."
if ! kubectl wait --for=condition=Ready "certificate/${CERT_NAME}" -n "$NAMESPACE" --timeout="$WAIT_TIMEOUT"; then
  echo
  echo "Certificate did not become Ready in time. Debug output:"
  kubectl describe certificate "$CERT_NAME" -n "$NAMESPACE" || true
  echo
  echo "Related CertificateRequests:"
  kubectl get certificaterequest -n "$NAMESPACE" || true
  exit 1
fi

echo
echo "Certificate is Ready"
kubectl get certificate "$CERT_NAME" -n "$NAMESPACE" -o wide

echo
echo "TLS Secret:"
kubectl get secret "$SECRET_NAME" -n "$NAMESPACE"

echo
echo "Leaf certificate details:"
kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o jsonpath='{.data.tls\.crt}' \
  | base64 -d \
  | openssl x509 -noout -subject -issuer -dates -ext subjectAltName

echo
echo "Expires according to the certificate Not After value shown above."
echo "cert-manager will try to renew it ${RENEW_BEFORE} before expiry and update secret ${SECRET_NAME} in place."
if [[ "$GENERATE_PKCS12" == "true" ]]; then
  echo "PKCS#12 keystore generation is enabled and secret ${SECRET_NAME} should also contain keystore.p12."
  echo "The PKCS#12 password comes from secret ${PASSWORD_SECRET_NAME} key ${PASSWORD_SECRET_KEY}."
fi
echo "Wire secret ${SECRET_NAME} into ${TARGET_REF}"
