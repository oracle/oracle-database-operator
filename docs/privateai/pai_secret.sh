#!/bin/bash
 
CURR_DIR=`pwd`
KEYSTORE_SECRET=keystore
API_SECRET=api-key
OMLSECRET=omlsslpwd
NAMESPACE=pai
SECRET_NAME=paisecret
 
rm -f ${CURR_DIR}/key.pem
rm -f ${CURR_DIR}/cert.pem
rm -f ${CURR_DIR}/api-key
rm -f ${CURR_DIR}/key.pub
 
head -c 32 /dev/urandom | xxd -p | tr -d '\n' | head -c 64 > api-key
openssl genrsa -out ${CURR_DIR}/key.pem
openssl rsa -in ${CURR_DIR}/key.pem -out ${CURR_DIR}/key.pub -pubout
openssl req -new -x509 -key ${CURR_DIR}/key.pem -out ${CURR_DIR}/cert.pem -days 365
 
# Generate keystore. Enter the password when prompted, and be sure to remember it.
openssl pkcs12 -export -inkey ${CURR_DIR}/key.pem -in ${CURR_DIR}/cert.pem -name mykey -out ${CURR_DIR}/keystore
 
kubectl delete secret $SECRET_NAME -n $NAMESPACE
kubectl create secret generic $SECRET_NAME --from-file=keystore --from-file=api-key --from-file=oml-ssl-pwd -n $NAMESPACE
#kubectl create secret generic $API_SECRET --from-file=api-key.txt -n $NAMESPACE
#kubectl create secret generic $OMLSECRET --from-file=oml-ssl-pwd -n $NAMESPACE