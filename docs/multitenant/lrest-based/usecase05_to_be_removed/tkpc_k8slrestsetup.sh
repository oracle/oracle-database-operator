#!/bin/sh
#
# $Header: has/test/k8s/src/tkpc_k8slrestsetup.sh sohibane_lrest_negative_tests/3 2024/07/31 15:02:16 sohibane Exp $
#
# tkpc_k8slrestsetup.sh
#
# Copyright (c) 2024, Oracle and/or its affiliates. 
#
#    NAME
#      tkpc_k8slrestsetup.sh -  script to be used to setup lrest in k8s env
#
#    DESCRIPTION
#	script to be used to setup lrest in k8s env
#
#    NOTES
#      <other useful comments, qualifications, etc.>
#
#    MODIFIED   (MM/DD/YY)
#    prrastog    06/04/24 - Creation
#
alias k=kubectl
export KUBECONFIG=$HOME/.kube/config

lrest_path=./
lrest_path2=./
cert_path="./cert_path"
operator_yaml="oracle-database-operator.yaml"

while getopts ':o:' opt; do
        case "$opt" in
                o) operator_yaml=${OPTARG};;
        esac
done
echo using operator file: $operator_yaml

kubectl get all -n rac

execute(){
        echo -e "\nExecuting: $1 \n"
        $1
}

prerequisites(){
echo "####### setting up prerequisites #######"
# create namespace cdbnamespace, pdbnamespace
create_ns1="kubectl create ns cdbnamespace"
create_ns2="kubectl create ns pdbnamespace"
execute "$create_ns1"
execute "$create_ns2"

}

step1(){

echo "######## STEP 1: create cert manager ###########"
cert_manager="kubectl apply -f $lrest_path/cert-manager.yaml"
#echo -e "\nExecuting: $cert_manager \n"
echo "BEGIN_IGNORE"
execute "$cert_manager"
#$cert_manager 
echo "END_IGNORE"
sleep 300
}

step2(){
echo "########## STEP 2: create operator pod ############"
operator_pod="kubectl apply -f $lrest_path2/$operator_yaml"
#echo -e "\nExecuting: $operator_pod \n"
echo "BEGIN_IGNORE"
execute "$operator_pod"
#$operator_pod
echo "END_IGNORE"
sleep 30

echo "######### STEP 2.1: check operator pods #########"
check_cmd="kubectl get pods -n oracle-database-operator-system"
#echo -e "\nExecuting: $check_cmd \n"
execute "$check_cmd"

echo "############ STEP 2.2: create rolebindings ##############"
cdb_bind="kubectl apply -f $lrest_path2/cdbnamespace_binding.yaml"
pdb_bind="kubectl apply -f $lrest_path2/pdbnamespace_binding.yaml"
execute "$cdb_bind"
execute "$pdb_bind"
check_cdb_bind="kubectl get rolebinding -n cdbnamespace"
check_pdb_bind="kubectl get rolebinding -n pdbnamespace"
execute "$check_cdb_bind"
execute "$check_pdb_bind"
}

step3(){
# NOTE: only required when running on env the first time on new env, or if certificates are deleted
echo "########## STEP 3: generate certificates ############"
echo BEGIN_IGNORE
mkdir $cert_path/certificates
SCRT=lrest_server.crt
CANAME=ca
SKEY=lrest_server.key
REST_SERVER=lrest.cdbnamespace
SCSR=lrest_server.csr
COMPANY=oracle

openssl genrsa -out $cert_path/certificates/$CANAME.key 2048
sleep 2
openssl req -new -x509 -days 365 -key $cert_path/certificates/$CANAME.key -subj "/C=CN/ST=GD/L=SZ/O=$COMPANY,Inc./CN=$COMPANY Root CA" -out $cert_path/certificates/$CANAME.crt
sleep 2
openssl req -newkey rsa:2048 -nodes -keyout $cert_path/certificates/$SKEY -subj "/C=CN/ST=GD/L=SZ/O=$COMPANY,Inc./CN=cdb-dev-$REST_SERVER" -out $cert_path/certificates/$SCSR
sleep 2
echo "subjectAltName=DNS:cdb-dev-$REST_SERVER,DNS:www.example.com" > $cert_path/certificates/extfile.txt
sleep 2
openssl x509 -req -extfile $cert_path/certificates/extfile.txt -days 365 -in $cert_path/certificates/$SCSR -CA $cert_path/certificates/$CANAME.crt -CAkey $cert_path/certificates/$CANAME.key -CAcreateserial -out $cert_path/certificates/$SCRT
sleep 2
openssl rsa -in $cert_path/certificates/$CANAME.key -outform PEM -pubout -out $cert_path/certificates/public.pem
echo END_IGNORE
}

step4(){

echo "########## STEP 4: store certificates - create secrets ############"

#echo "Executing: kubectl create secret tls db-tls --key="$cert_path/certificates/lrest_server.key" --cert="$cert_path/certificates/lrest_server.crt" -n oracle-database-operator-system"
#kubectl create secret tls db-tls --key="$cert_path/certificates/lrest_server.key" --cert="$cert_path/certificates/lrest_server.crt" -n oracle-database-operator-system
#echo "Executing: kubectl create secret generic db-ca --from-file="$cert_path/certificates/ca.crt" -n oracle-database-operator-system"
#kubectl create secret generic db-ca --from-file="$cert_path/certificates/ca.crt" -n oracle-database-operator-system

secret_cdbns1="kubectl create secret tls db-tls --key=$cert_path/certificates/lrest_server.key --cert=$cert_path/certificates/lrest_server.crt -n cdbnamespace"
secret_cdbns2="kubectl create secret generic db-ca --from-file=$cert_path/certificates/ca.crt -n cdbnamespace"
secret_pdbns1="kubectl create secret tls db-tls --key=$cert_path/certificates/lrest_server.key --cert=$cert_path/certificates/lrest_server.crt -n pdbnamespace"
secret_pdbns2="kubectl create secret generic db-ca --from-file=$cert_path/certificates/ca.crt -n pdbnamespace"
secret_cdbns3="kubectl create secret tls prvkey --key=$cert_path/certificates/ca.key --cert=$cert_path/certificates/ca.crt  -n cdbnamespace"
secret_cdbns4="kubectl create secret generic pubkey --from-file=publicKey=$cert_path/certificates/public.pem -n cdbnamespace"

execute "$secret_cdbns1"
execute "$secret_cdbns2"
execute "$secret_pdbns1"
execute "$secret_pdbns2"
execute "$secret_cdbns3"
execute "$secret_cdbns4"
sleep 10;
echo "Executing: kubectl get secrets -n oracle-database-operator-system"
kubectl get secrets -n oracle-database-operator-system
check_cdbns="kubectl get secrets -n cdbnamespace"
check_pdbns="kubectl get secrets -n pdbnamespace"
execute "$check_cdbns"
execute "$check_pdbns"
# create dbsecrets
#lrest_secret="kubectl apply -f $lrest_path2/create_lrest_secret.yaml -n cdbnamespace"
#lrpdb_secret="kubectl apply -f $lrest_path2/create_lrpdb_secret.yaml -n pdbnamespace"
#execute "$lrest_secret"
#execute "$lrpdb_secret"

}

step4a(){ 

echo "############ create secret cred ##############"

secret_del1="kubectl delete secret prvkey -n cdbnamespace"
secret_del2="kubectl delete secret pubkey -n cdbnamespace"

execute "$secret_del1"
execute "$secret_del2"

echo "restdba"      > $cert_path/certificates/dbuser.txt 
echo "CLWKO655321"  > $cert_path/certificates/dbpass.txt 
echo "welcome"      > $cert_path/certificates/wbuser.txt 
echo "welcome1"     > $cert_path/certificates/wbpass.txt 
echo "welcome"      > $cert_path/certificates/pdbusr.txt 
echo "welcome1"     > $cert_path/certificates/pdbpwd.txt 

secretcred_cdbns1="kubectl create secret generic prvkey --from-file=privateKey=$cert_path/certificates/ca.key -n cdbnamespace"
secretcred_cdbns2="kubectl create secret generic pubkey --from-file=publicKey=$cert_path/certificates/public.pem -n cdbnamespace"

execute "$secretcred_cdbns1"
execute "$secretcred_cdbns2"

/usr/bin/openssl rsautl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/dbuser.txt |base64 > $cert_path/certificates/e_dbuser.txt
/usr/bin/openssl rsautl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/dbpass.txt |base64 > $cert_path/certificates/e_dbpass.txt
/usr/bin/openssl rsautl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/wbuser.txt |base64 > $cert_path/certificates/e_wbuser.txt
/usr/bin/openssl rsautl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/wbpass.txt |base64 > $cert_path/certificates/e_wbpass.txt
/usr/bin/openssl rsautl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/pdbusr.txt |base64 > $cert_path/certificates/e_pdbusr.txt
/usr/bin/openssl rsautl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/pdbpwd.txt |base64 > $cert_path/certificates/e_pdbpwd.txt

secretcred_cdbns3="kubectl create secret generic dbuser --from-file=$cert_path/certificates/e_dbuser.txt -n  cdbnamespace"
secretcred_cdbns4="kubectl create secret generic dbpass --from-file=$cert_path/certificates/e_dbpass.txt -n  cdbnamespace"
secretcred_cdbns5="kubectl create secret generic wbuser --from-file=$cert_path/certificates/e_wbuser.txt -n  cdbnamespace"
secretcred_cdbns6="kubectl create secret generic wbpass --from-file=$cert_path/certificates/e_wbpass.txt -n  cdbnamespace"
secretcred_pdbns1="kubectl create secret generic wbuser --from-file=$cert_path/certificates/e_wbuser.txt -n  pdbnamespace"
secretcred_pdbns2="kubectl create secret generic wbpass --from-file=$cert_path/certificates/e_wbpass.txt -n  pdbnamespace"
secretcred_pdbns3="kubectl create secret generic pdbusr --from-file=$cert_path/certificates/e_pdbusr.txt -n  pdbnamespace"
secretcred_pdbns4="kubectl create secret generic pdbpwd --from-file=$cert_path/certificates/e_pdbpwd.txt -n  pdbnamespace"
secretcred_pdbns5="kubectl create secret generic prvkey --from-file=privateKey=$cert_path/certificates/ca.key -n pdbnamespace"

execute "$secretcred_cdbns3"
execute "$secretcred_cdbns4"
execute "$secretcred_cdbns5"
execute "$secretcred_cdbns6"
execute "$secretcred_pdbns1"
execute "$secretcred_pdbns2"
execute "$secretcred_pdbns3"
execute "$secretcred_pdbns4"
execute "$secretcred_pdbns5"
sleep 10;

check_sc_cdbns="kubectl get secrets -n cdbnamespace"
check_sc_pdbns="kubectl get secrets -n pdbnamespace"
execute "$check_sc_cdbns"
execute "$check_sc_pdbns"

}

step5(){

        echo "############ create lrest pod ##############"
        create_lrest_pod="kubectl apply -f $lrest_path2/create_lrest_pod.yaml"
        execute "$create_lrest_pod"
	sleep 120
	echo "############## check lrest pod creation #############"
	check_lrest_pod=`kubectl get pods -n cdbnamespace |grep lrest`
	echo $check_lrest_pod
	#execute "$check_lrest_pod"
	echo "############### check lrest pod creation log ##########"
	echo "BEGIN_IGNORE"
	check_log_lrest_pod="kubectl logs `kubectl get pods -n cdbnamespace |grep lrest|cut -d ' ' -f 1` -n cdbnamespace"
	execute "$check_log_lrest_pod"
	echo "END_IGNORE"

}

step5a(){
echo "############## STEP 5a: create db service and lrest user restdba #############"
kubectl exec -it racnode1-0 -n rac -- su - oracle -c 'srvctl add service -d ORCLCDB -s lrest -r ORCLCDB1,ORCLCDB2; srvctl start service -d ORCLCDB -s lrest; srvctl status service -d orclcdb -s lrest'

kubectl exec -it racnode1-0 -n rac -- su - oracle -c "export ORACLE_SID=ORCLCDB1; sqlplus -S / as sysdba <<EOF
show pdbs;
alter session set \"_oracle_script\"=true;
create user restdba identified by CLWKO655321;
grant create session to restdba container=all;
grant sysdba to restdba container=all;
exit;
EOF
"
}

step6(){

echo "########## STEP 6: create cdb pos ##############"
cmd="kubectl apply -f $lrest_path/cdb_create_test.yaml"
echo "Executing: $cmd"
$cmd
sleep 60
cmd="kubectl get pods -n oracle-database-operator-system"
echo -e "\nExecuting: $cmd"
$cmd
#echo "######### Step 6.1: check lrest log ############"
#cmd="kubectl logs -f `kubectl get pods -n oracle-database-operator-system|grep lrest | cut -d ' ' -f 1` -n oracle-database-operator-system"
#echo -e "\nExecuting $cmd"
#$cmd
#sleep 30
echo "########### Step 6.2: connect to cdb pod ############"
cdb_dev_pod=`kubectl get pods -n oracle-database-operator-system|grep lrest|cut -d ' ' -f 1`
cmd="kubectl exec -it $cdb_dev_pod -n oracle-database-operator-system -- bash -c 'echo \$HOSTNAME' "
echo -e "\nExecuting: $cmd"
kubectl exec -it $cdb_dev_pod -n oracle-database-operator-system -- bash -c 'echo $HOSTNAME'

}


prerequisites
#step1
#step2
# NOTE: only required when running on env the first time on new env, or if certificates are deleted
step3
#echo "hit enter"
#read
step4
step4a
step5a
step5






#step6
#secretcred_o1="/usr/bin/openssl pkeyutl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/dbuser.txt |base64 > $cert_path/certificates/e_dbuser.txt"
#rm $cert_path/certificates/e_dbuser.txt
#secretcred_o1="openssl pkeyutl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/dbuser.txt"
#secretcred_o1="/usr/bin/openssl rsautl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/dbuser.txt |base64 > $cert_path/certificates/e_dbuser.txt"
#echo "executing $secretcred_o1"
#execute $secretcred_o1
#/usr/bin/openssl pkeyutl -encrypt -pubin -inkey $cert_path/certificates/public.pem -in $cert_path/certificates/dbuser.txt |base64 > $cert_path/certificates/e_dbuser.txt
#/usr/bin/openssl pkeyutl -encrypt -pubin -inkey /home/opc/rack8s/operator/lrest/certificates/public.pem -in /home/opc/rack8s/operator/lrest/certificates/dbuser.txt > /home/opc/rack8s/operator/lrest/certificates/e_dbuser.txt
