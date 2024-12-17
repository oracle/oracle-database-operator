#!/bin/bash
read pwd
kubectl create secret docker-registry oracle-container-registry-secret --docker-server=container-registry.oracle.com --docker-username=matteo.malvezzi@oraclecom --docker-password=$pwd --docker-email=matteo.malvezzi@oracle.com -n oracle-database-operator-system
  

