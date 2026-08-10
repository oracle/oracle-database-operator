#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Creates and manages a PrivateAI auth secret.

The secret always supports:
  - api-key
  - privateai-ssl-pwd

JSON key-management commands also maintain:
  - api-keys.json

Usage:
  create_auth_secret.sh [create] [options]
  create_auth_secret.sh add-key [options]
  create_auth_secret.sh set-active [options]
  create_auth_secret.sh delete-key [options]
  create_auth_secret.sh list-key [options]

Global options:
  --namespace <ns>          Namespace for the secret (default: pai)
  --secret-name <name>      Secret name (default: paisecret)
  --output-dir <path>       Directory for generated helper files (default: ./privateai-auth-secret)
  --list-secret             Show secret keys after apply.
  -h, --help                Show this help text.

Create options:
  --api-key <value>         Add one API key. Repeat this flag for multiple keys.
  --api-keys-file <path>    Use a preformatted file for the api-key secret entry.
  --generate-api-key        Generate one API key when none is supplied.
  --ssl-pwd-file <path>     Read privateai-ssl-pwd from an existing file.
  --ssl-pwd-value <value>   Use a literal value for privateai-ssl-pwd.
  --json-api-keys           Also write api-keys.json for a single key.
  --key-id <id>             JSON key_id for --json-api-keys or add-key.
  --alias <alias>           JSON alias for --json-api-keys or add-key.
  --active <true|false>     JSON active flag (default: true).

Key-management options:
  --key-id <id>             Stable key identity.
  --alias <alias>           Friendly key alias.
  --key <value>             API key value for add-key.
  --generate-api-key        Generate API key value for add-key.
  --active <true|false>     Active flag for add-key or set-active.

Examples:
  ./create_auth_secret.sh \
    --namespace pai \
    --secret-name paisecret \
    --generate-api-key \
    --ssl-pwd-file ./privateai-ssl-pwd \
    --list-secret

  ./create_auth_secret.sh create \
    --namespace pai \
    --secret-name paisecret \
    --generate-api-key \
    --json-api-keys \
    --key-id client1 \
    --alias ServiceA \
    --active true \
    --ssl-pwd-value 'Oracle_26ai'

  ./create_auth_secret.sh add-key \
    --namespace pai \
    --secret-name paisecret \
    --key-id client2 \
    --alias ServiceB \
    --generate-api-key \
    --active false

  ./create_auth_secret.sh set-active --alias ServiceB --active true
  ./create_auth_secret.sh delete-key --key-id client2
  ./create_auth_secret.sh list-key
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

generate_api_key() {
  head -c 32 /dev/urandom | xxd -p | tr -d '\n' | head -c 64
}

validate_bool() {
  local name="$1"
  local value="$2"
  if [[ "$value" != "true" && "$value" != "false" ]]; then
    echo "$name must be true or false" >&2
    exit 1
  fi
}

require_json_tools() {
  require_cmd kubectl
  require_cmd jq
  require_cmd base64
}

ensure_namespace_exists() {
  if ! kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
    echo "Namespace $NAMESPACE does not exist" >&2
    exit 1
  fi
}

secret_exists() {
  kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" >/dev/null 2>&1
}

secret_key_to_file() {
  local key="$1"
  local dest="$2"
  local encoded

  if ! secret_exists; then
    return 1
  fi

  encoded="$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o json | jq -r --arg key "$key" '.data[$key] // empty')"
  if [[ -z "$encoded" ]]; then
    return 1
  fi

  printf '%s' "$encoded" | base64 -d > "$dest"
}

load_api_keys_json() {
  local dest="$1"
  if secret_key_to_file "api-keys.json" "$dest"; then
    jq -e 'type == "array"' "$dest" >/dev/null
    return
  fi
  printf '[]\n' > "$dest"
}

write_json_outputs() {
  local json_file="$1"
  mkdir -p "$OUTPUT_DIR"
  cp "$json_file" "$API_KEYS_JSON_FILE"
  jq -r '.[] | select(.active == true) | .key' "$API_KEYS_JSON_FILE" > "$API_KEY_MATERIAL_FILE"
}

apply_secret_from_files() {
  local args=(
    create secret generic "$SECRET_NAME"
    --namespace "$NAMESPACE"
    --from-file=api-key="$API_KEY_MATERIAL_FILE"
  )

  if [[ -f "$SSL_PWD_MATERIAL_FILE" ]]; then
    args+=(--from-file=privateai-ssl-pwd="$SSL_PWD_MATERIAL_FILE")
  fi
  if [[ -f "$API_KEYS_JSON_FILE" ]]; then
    args+=(--from-file=api-keys.json="$API_KEYS_JSON_FILE")
  fi

  kubectl "${args[@]}" --dry-run=client -o yaml | kubectl apply -f -
}

print_secret_keys_if_requested() {
  if [[ "$LIST_SECRET" == "true" ]]; then
    echo
    kubectl get secret "$SECRET_NAME" -n "$NAMESPACE"
    echo
    kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" -o go-template='{{range $k, $v := .data}}{{printf "%s\n" $k}}{{end}}'
  fi
}

create_secret() {
  require_cmd kubectl
  require_cmd xxd
  ensure_namespace_exists

  if [[ -z "$API_KEYS_FILE" && "${#API_KEYS[@]}" -eq 0 && "$GENERATE_API_KEY" != "true" ]]; then
    echo "Provide --api-key, --api-keys-file, or --generate-api-key" >&2
    exit 1
  fi

  if [[ -n "$API_KEYS_FILE" && "${#API_KEYS[@]}" -gt 0 ]]; then
    echo "Use either --api-key flags or --api-keys-file, not both" >&2
    exit 1
  fi

  if [[ -n "$API_KEYS_FILE" && "$GENERATE_API_KEY" == "true" ]]; then
    echo "Use either --api-keys-file or --generate-api-key, not both" >&2
    exit 1
  fi

  if [[ -n "$SSL_PWD_FILE" && -n "$SSL_PWD_VALUE" ]]; then
    echo "Use either --ssl-pwd-file or --ssl-pwd-value, not both" >&2
    exit 1
  fi

  if [[ -z "$SSL_PWD_FILE" && -z "$SSL_PWD_VALUE" ]]; then
    echo "Provide --ssl-pwd-file or --ssl-pwd-value" >&2
    exit 1
  fi

  if [[ "$JSON_API_KEYS" == "true" ]]; then
    require_cmd jq
    validate_bool "--active" "$ACTIVE"
    if [[ -z "$KEY_ID" || -z "$ALIAS" ]]; then
      echo "Provide --key-id and --alias with --json-api-keys" >&2
      exit 1
    fi
    if [[ -n "$API_KEYS_FILE" || "${#API_KEYS[@]}" -gt 1 ]]; then
      echo "--json-api-keys supports a single generated or literal --api-key value" >&2
      exit 1
    fi
  fi

  mkdir -p "$OUTPUT_DIR"

  if [[ -n "$API_KEYS_FILE" ]]; then
    if [[ ! -f "$API_KEYS_FILE" ]]; then
      echo "API keys file not found: $API_KEYS_FILE" >&2
      exit 1
    fi
    cp "$API_KEYS_FILE" "$API_KEY_MATERIAL_FILE"
  else
    if [[ "$GENERATE_API_KEY" == "true" && "${#API_KEYS[@]}" -eq 0 ]]; then
      API_KEYS+=("$(generate_api_key)")
    fi
    printf '%s\n' "${API_KEYS[@]}" > "$API_KEY_MATERIAL_FILE"
  fi

  if [[ -n "$SSL_PWD_FILE" ]]; then
    if [[ ! -f "$SSL_PWD_FILE" ]]; then
      echo "privateai-ssl-pwd file not found: $SSL_PWD_FILE" >&2
      exit 1
    fi
    cp "$SSL_PWD_FILE" "$SSL_PWD_MATERIAL_FILE"
  else
    printf '%s' "$SSL_PWD_VALUE" > "$SSL_PWD_MATERIAL_FILE"
  fi

  if [[ "$JSON_API_KEYS" == "true" ]]; then
    local key_value
    key_value="$(head -n 1 "$API_KEY_MATERIAL_FILE")"
    jq -n \
      --arg key_id "$KEY_ID" \
      --arg alias "$ALIAS" \
      --arg key "$key_value" \
      --argjson active "$ACTIVE" \
      '[{key_id: $key_id, alias: $alias, key: $key, active: $active}]' > "$API_KEYS_JSON_FILE"
  fi

  apply_secret_from_files

  echo "Applied auth secret $SECRET_NAME in namespace $NAMESPACE"
  echo "api-key material file: $API_KEY_MATERIAL_FILE"
  echo "privateai-ssl-pwd material file: $SSL_PWD_MATERIAL_FILE"
  if [[ "$JSON_API_KEYS" == "true" ]]; then
    echo "api-keys.json material file: $API_KEYS_JSON_FILE"
  fi
  print_secret_keys_if_requested
}

load_existing_ssl_password() {
  if [[ -n "$SSL_PWD_FILE" ]]; then
    if [[ ! -f "$SSL_PWD_FILE" ]]; then
      echo "privateai-ssl-pwd file not found: $SSL_PWD_FILE" >&2
      exit 1
    fi
    cp "$SSL_PWD_FILE" "$SSL_PWD_MATERIAL_FILE"
    return
  fi
  if [[ -n "$SSL_PWD_VALUE" ]]; then
    printf '%s' "$SSL_PWD_VALUE" > "$SSL_PWD_MATERIAL_FILE"
    return
  fi
  if ! secret_key_to_file "privateai-ssl-pwd" "$SSL_PWD_MATERIAL_FILE"; then
    echo "Existing secret must contain privateai-ssl-pwd, or provide --ssl-pwd-file/--ssl-pwd-value" >&2
    exit 1
  fi
}

require_key_selector() {
  if [[ -z "$KEY_ID" && -z "$ALIAS" ]]; then
    echo "Provide --key-id or --alias" >&2
    exit 1
  fi
}

json_match_filter='($key_id != "" and .key_id == $key_id) or ($alias != "" and .alias == $alias)'

add_key() {
  require_json_tools
  require_cmd xxd
  ensure_namespace_exists
  validate_bool "--active" "$ACTIVE"

  if [[ -z "$KEY_ID" || -z "$ALIAS" ]]; then
    echo "Provide --key-id and --alias" >&2
    exit 1
  fi
  if [[ -n "$KEY_VALUE" && "$GENERATE_API_KEY" == "true" ]]; then
    echo "Use either --key or --generate-api-key, not both" >&2
    exit 1
  fi
  if [[ -z "$KEY_VALUE" && "$GENERATE_API_KEY" != "true" ]]; then
    echo "Provide --key or --generate-api-key" >&2
    exit 1
  fi
  if [[ "$GENERATE_API_KEY" == "true" ]]; then
    KEY_VALUE="$(generate_api_key)"
  fi

  mkdir -p "$OUTPUT_DIR"
  local current_json next_json
  current_json="$OUTPUT_DIR/current-api-keys.json"
  next_json="$OUTPUT_DIR/next-api-keys.json"
  load_api_keys_json "$current_json"

  jq \
    --arg key_id "$KEY_ID" \
    --arg alias "$ALIAS" \
    --arg key "$KEY_VALUE" \
    --argjson active "$ACTIVE" '
      if any(.[]; .key_id == $key_id) then
        error("key_id already exists: " + $key_id)
      elif any(.[]; .alias == $alias) then
        error("alias already exists: " + $alias)
      else
        . + [{key_id: $key_id, alias: $alias, key: $key, active: $active}]
      end
    ' "$current_json" > "$next_json"

  write_json_outputs "$next_json"
  load_existing_ssl_password
  apply_secret_from_files

  echo "Added key_id $KEY_ID alias $ALIAS to secret $SECRET_NAME in namespace $NAMESPACE"
  echo "Updated api-keys.json and regenerated api-key from active keys"
  print_secret_keys_if_requested
}

set_active() {
  require_json_tools
  ensure_namespace_exists
  validate_bool "--active" "$ACTIVE"
  require_key_selector

  mkdir -p "$OUTPUT_DIR"
  local current_json next_json matches
  current_json="$OUTPUT_DIR/current-api-keys.json"
  next_json="$OUTPUT_DIR/next-api-keys.json"
  load_api_keys_json "$current_json"

  matches="$(jq --arg key_id "$KEY_ID" --arg alias "$ALIAS" "[.[] | select($json_match_filter)] | length" "$current_json")"
  if [[ "$matches" -eq 0 ]]; then
    echo "No key found for the provided key selector" >&2
    exit 1
  fi
  if [[ "$matches" -gt 1 ]]; then
    echo "Key selector matched more than one entry; use a unique --key-id or --alias" >&2
    exit 1
  fi

  jq --arg key_id "$KEY_ID" --arg alias "$ALIAS" --argjson active "$ACTIVE" \
    "map(if $json_match_filter then .active = \$active else . end)" "$current_json" > "$next_json"

  write_json_outputs "$next_json"
  load_existing_ssl_password
  apply_secret_from_files

  echo "Updated active=$ACTIVE for selected key in secret $SECRET_NAME"
  echo "Updated api-keys.json and regenerated api-key from active keys"
  print_secret_keys_if_requested
}

delete_key() {
  require_json_tools
  ensure_namespace_exists
  require_key_selector

  mkdir -p "$OUTPUT_DIR"
  local current_json next_json matches
  current_json="$OUTPUT_DIR/current-api-keys.json"
  next_json="$OUTPUT_DIR/next-api-keys.json"
  load_api_keys_json "$current_json"

  matches="$(jq --arg key_id "$KEY_ID" --arg alias "$ALIAS" "[.[] | select($json_match_filter)] | length" "$current_json")"
  if [[ "$matches" -eq 0 ]]; then
    echo "No key found for the provided key selector" >&2
    exit 1
  fi
  if [[ "$matches" -gt 1 ]]; then
    echo "Key selector matched more than one entry; use a unique --key-id or --alias" >&2
    exit 1
  fi

  jq --arg key_id "$KEY_ID" --arg alias "$ALIAS" \
    "map(select(($json_match_filter) | not))" "$current_json" > "$next_json"

  write_json_outputs "$next_json"
  load_existing_ssl_password
  apply_secret_from_files

  echo "Deleted selected key from secret $SECRET_NAME"
  echo "Updated api-keys.json and regenerated api-key from active keys"
  print_secret_keys_if_requested
}

list_keys() {
  require_json_tools
  local current_json
  current_json="$(mktemp)"
  load_api_keys_json "$current_json"
  jq -r '
    if length == 0 then
      "No JSON API keys found"
    else
      (["KEY_ID", "ALIAS", "ACTIVE"] | @tsv),
      (.[] | [.key_id, .alias, (.active | tostring)] | @tsv)
    end
  ' "$current_json"
  rm -f "$current_json"
}

COMMAND="create"
if [[ $# -gt 0 ]]; then
  case "$1" in
    create|add-key|set-active|delete-key|list-key|list-keys)
      COMMAND="$1"
      shift
      ;;
  esac
fi

NAMESPACE="pai"
SECRET_NAME="paisecret"
OUTPUT_DIR="$(pwd)/privateai-auth-secret"
LIST_SECRET="false"
GENERATE_API_KEY="false"
API_KEYS_FILE=""
SSL_PWD_FILE=""
SSL_PWD_VALUE=""
JSON_API_KEYS="false"
KEY_ID=""
ALIAS=""
KEY_VALUE=""
ACTIVE="true"
declare -a API_KEYS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace)
      NAMESPACE="$2"
      shift 2
      ;;
    --secret-name)
      SECRET_NAME="$2"
      shift 2
      ;;
    --api-key)
      API_KEYS+=("$2")
      shift 2
      ;;
    --api-keys-file)
      API_KEYS_FILE="$2"
      shift 2
      ;;
    --generate-api-key)
      GENERATE_API_KEY="true"
      shift
      ;;
    --ssl-pwd-file)
      SSL_PWD_FILE="$2"
      shift 2
      ;;
    --ssl-pwd-value)
      SSL_PWD_VALUE="$2"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --list-secret)
      LIST_SECRET="true"
      shift
      ;;
    --json-api-keys)
      JSON_API_KEYS="true"
      shift
      ;;
    --key-id)
      KEY_ID="$2"
      shift 2
      ;;
    --alias)
      ALIAS="$2"
      shift 2
      ;;
    --key)
      KEY_VALUE="$2"
      shift 2
      ;;
    --active)
      ACTIVE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

API_KEY_MATERIAL_FILE="$OUTPUT_DIR/api-key"
API_KEYS_JSON_FILE="$OUTPUT_DIR/api-keys.json"
SSL_PWD_MATERIAL_FILE="$OUTPUT_DIR/privateai-ssl-pwd"

case "$COMMAND" in
  create)
    create_secret
    ;;
  add-key)
    add_key
    ;;
  set-active)
    set_active
    ;;
  delete-key)
    delete_key
    ;;
  list-key|list-keys)
    list_keys
    ;;
esac
