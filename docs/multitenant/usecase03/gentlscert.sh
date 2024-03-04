#!/bin/bash 
export CDB_NAMESPACE=cdbnamespace 
export PDB_NAMESPACE=pdbnamespace 
export OPR_NAMESPACE=oracle-database-operator-system 
export SKEY=tls.key 
export SCRT=tls.crt 
export CART=ca.crt 
export COMPANY=oracle 
export REST_SERVER=ords

openssl genrsa -out ca.key 2048 
openssl req -new -x509 -days 365 -key ca.key -subj "/C=CN/ST=GD/L=SZ/O=${COMPANY}, Inc./CN=${COMPANY} Root CA" -out ca.crt 
openssl req -newkey rsa:2048 -nodes -keyout ${SKEY} -subj "/C=CN/ST=GD/L=SZ/O=${COMPANY}, Inc./CN=cdb-dev-${REST_SERVER}.${CDB_NAMESPACE}" -out server.csr 
echo "subjectAltName=DNS:cdb-dev-${REST_SERVER}.${CDB_NAMESPACE},DNS:www.example.com" > extfile.txt 
openssl x509 -req -extfile extfile.txt -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out ${SCRT} 

kubectl create secret tls db-tls --key="${SKEY}" --cert="${SCRT}"  -n ${CDB_NAMESPACE} 
kubectl create secret generic db-ca --from-file="${CART}" -n ${CDB_NAMESPACE} 
kubectl create secret tls db-tls --key="${SKEY}" --cert="${SCRT}"  -n ${PDB_NAMESPACE} 
kubectl create secret generic db-ca --from-file="${CART}"  -n ${PDB_NAMESPACE} 
kubectl create secret tls db-tls --key="${SKEY}" --cert="${SCRT}"  -n ${OPR_NAMESPACE} 
kubectl create secret generic db-ca --from-file="${CART}"  -n ${OPR_NAMESPACE}

