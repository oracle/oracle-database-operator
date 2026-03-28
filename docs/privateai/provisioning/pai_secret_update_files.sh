#!/bin/bash

CURR_DIR=`pwd`
KEYSTORE_SECRET=keystore
API_SECRET=api-key
PAISECRET=privateai-ssl-pwd
NAMESPACE=pai
SECRET_NAME=paisecret

API_KEY_FILE="api-key"
CERT_FILE="cert.pem"
KEY_FILE="key.pem"
KEYSTORE_FILE="keystore"
PRIVATEAI_SSL_PWD="privateai-ssl-pwd"
OUTPUT_YAML="secretupdate.yaml"

rm -f ${CURR_DIR}/key.pem
rm -f ${CURR_DIR}/cert.pem
rm -f ${CURR_DIR}/api-key
rm -f ${CURR_DIR}/key.pub
rm -f ${CURR_DIR}/keystore

head -c 32 /dev/urandom | xxd -p | tr -d '\n' | head -c 64 > api-key

openssl genrsa -out ${CURR_DIR}/key.pem
openssl rsa -in ${CURR_DIR}/key.pem -out ${CURR_DIR}/key.pub -pubout
openssl req -new -x509 -key ${CURR_DIR}/key.pem -out ${CURR_DIR}/cert.pem -days 365

# Generate keystore. Enter the password when prompted, and be sure to remember it.
openssl pkcs12 -export -inkey ${CURR_DIR}/key.pem -in ${CURR_DIR}/cert.pem -name mykey -out ${CURR_DIR}/keystore


# Encode each piece of data and use `base64 -w 0` on Linux to disable line wrapping.
API_KEY_B64=$(cat "$API_KEY_FILE" | base64 -w 0)
CERT_B64=$(cat "$CERT_FILE" | base64 -w 0)
KEY_B64=$(cat "$KEY_FILE" | base64 -w 0)
KEYSTORE_B64=$(cat "$KEYSTORE_FILE" | base64 -w 0)
PRIVATEAI_SSL_PWD_B64=$(cat "$PRIVATEAI_SSL_PWD" | base64 -w 0)

# remove the $OUTPUT_YAML in case it exists
rm -f $OUTPUT_YAML

# write the YAML file
cat <<EOF > "$OUTPUT_YAML"
data:
  api-key: $API_KEY_B64
  cert.pem: $CERT_B64
  key.pem: $KEY_B64
  keystore: $KEYSTORE_B64
  privateai-ssl-pwd: $PRIVATEAI_SSL_PWD_B64
EOF

echo "Generated $OUTPUT_YAML with base64-encoded secrets."
