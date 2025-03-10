#
# Copyright (c) 2006, 2024, Oracle and/or its affiliates.
#
#
#   NAME
#     makefile: 
#     	This makefile helps to set up multipool and sidb cases 
#     	edit the following variables with your system information 
#       and execute make help to list the list of avilable targets
#               

export PDB1=pdb1
export PDB2=pdb2
export TNS1=(DESCRIPTION=(CONNECT_TIMEOUT=90)(RETRY_COUNT=30)(RETRY_DELAY=10)(TRANSPORT_CONNECT_TIMEOUT=70)(LOAD_BALLANCE=ON)(ADDRESS=(PROTOCOL=TCP)(HOST=scan12.testrac.com)(PORT=1521)(IP=V4_ONLY))(LOAD_BALLANCE=ON)(ADDRESS=(PROTOCOL=TCP)(HOST=scan12.testrac.com)(PORT=1521)(IP=V4_ONLY))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=$(PDB1))))
export TNS2=(DESCRIPTION=(CONNECT_TIMEOUT=90)(RETRY_COUNT=30)(RETRY_DELAY=10)(TRANSPORT_CONNECT_TIMEOUT=70)(LOAD_BALLANCE=ON)(ADDRESS=(PROTOCOL=TCP)(HOST=scan12.testrac.com)(PORT=1521)(IP=V4_ONLY))(LOAD_BALLANCE=ON)(ADDRESS=(PROTOCOL=TCP)(HOST=scan34.testrac.com)(PORT=1521)(IP=V4_ONLY))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=$(PDB2))))
export SIDB_PASSWORD=....write password here ....
export PDB1_PWD=.....write password here....
export PDB2_PWD=.....write password....
export ORDS_MULTI_POOL_PWD=....write password here....
export USER_CONTAINER_REGISTRY=username@oracle.com <--Your OCR account
export OPRNAMESPACE=oracle-database-operator-system
export ORDSNAMESPACE=ordsnamespace


#
#   DESCRIPTION
#       Main makefile - see target table 
#
#       | Target      | Description                                            |
#       +-------------+--------------------------------------------------------+
#       | step0a      | create_ordsnamespace.yaml                              |
#       +-------------+--------------------------------------------------------+
#       | step1a      | setup certmaneger                                      | 
#       +-------------+--------------------------------------------------------+
#       | step2a      | setup operator oracle-database-operator.yaml           |
#       +-------------+--------------------------------------------------------+
#       | step3a      | default scoped deployment default-ns-role-binding.yaml |
#       +-------------+--------------------------------------------------------+
#       | step4a      | node - persistent volume - storage class for the db    |
#       +-------------+--------------------------------------------------------+
#       | step5a      | setup secrets                                          |
#       +----------------------------------------------------------------------+
#       | step6a      | setup secrets for OCR                                  |
#       +----------------------------------------------------------------------+
#       | step7a      | setup sidb                                             |
#       +----------------------------------------------------------------------+
#       | step8a      | ⭐Setup REST SERVER ⭐                             |
#       +-------------+--------------------------------------------------------+
#
#       step[1-7]a are required to start mongodb API rest server 
#
#       step[9-11] test mongo API 
#       +-------------+--------------------------------------------------------+
#       | step9       | configure a mongo db user on sidb                      |
#       +-------------+--------------------------------------------------------+
#       | step10      | ⭐Setup REST SERVER FOR MONGO API ⭐               |
#       +-------------+--------------------------------------------------------+
#       | step11      | Test Mongo API                                         |
#       +-------------+--------------------------------------------------------+
#       
#       step[12- ] test multi tns configuration 
#       +-------------+--------------------------------------------------------+
#       | step12      | create tns secret                                      |
#       +-------------+--------------------------------------------------------+
#       | step13      | create passwords secret                                |
#       +-------------+--------------------------------------------------------+
#       | step14      | ⭐SetupMulti Rest Server ⭐                        | 
#       +-------------+--------------------------------------------------------+
#       



export WATCHLIST=$(OPRNAMESPACE),$(ORDSNAMESPACE)
export CREATE_SINGLEINSTANCE=create_singleinstance_db.yaml
export CERTMANAGER=https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml
export SIDB_SECRET=sidb-db-auth
export ORDS_SECRET=ords-db-auth
export MULTI_ORDS_AUTH_SECRET=multi-ords-auth-enc
export PDB1_PRIV_AUTH_SECRET=pdb1-priv-auth-enc
export PDB2_PRIV_AUTH_SECRET=pdb2-priv-auth-enc


export SIDB_IMAGE=container-registry.oracle.com/database/free:23.4.0.0
export ORDS_IMAGE=container-registry.oracle.com/database/ords:24.1.0
export ORDS_IMAGE.1=container-registry.oracle.com/database/ords:24.1.1
export SECRET_CONTAINER_REGISTRY=oracle-container-registry-secret
export ORACLE_CONTAINER_REGISTRY=container-registry.oracle.com
export REST_SERVER_NAME=ords-sidb
export REST_SERVER_NAME_MONGO=ords-sidb-mongo
export MONGOSH=mongosh-2.3.1-linux-x64
export KIND=OrdsSrvs

export TNSNAMES=./tnsnames.ora
export TNSADMIN=`pwd`/tnsadmin
export PRVKEY=ca.key
export PUBKEY=public.pem

## CMD SECTION##
export KUBECTL=/usr/local/go/bin/kubectl
export DIFF=/usr/bin/diff
export MAKE=/usr/bin/make
export CURL=/usr/bin/curl
export TAR=/usr/bin/tar
export OPENSSL=/usr/bin/openssl

## YAML AND OTHER FILES ##
export CREATE_ORDSNAMESPACE=create_$(ORDSNAMESPACE).yaml
export DEFAULT_NAMESPACE_SCOPE=default-ns-role-binding.yaml
export RST_NAMESPACE_SCOPE=ords-ns-role-binding.yaml
export ORACLE_OPERATOR_YAML=../../../oracle-database-operator.yaml
export NODE_RBAC=node-rbac.yaml
export STORAGE_CLASS_RBAC=storage-class-rbac.yaml
export PERSISTENT_VOLUME_RBAC=persistent-volume-rbac.yaml
export SIDB_CREATION=sidb_create.yaml
export SECRET_CONTAINER_REGISTRY_SCRIPT=create_registry_secret.sh
export REST_SERVER_CREATION=rest_server_creation.yaml
export REST_SERVER_CREATION_MONGO=rest_server_creation_mongo.yaml
export MULTISRV_MANIFEST=create_multisrv.yaml
export MONGOORADBUSER=MONGO


MAKEFILE=./makefile
.ONESHELL:

define manpage
@printf "\n"
@printf "\033[7m%s\033[0m \033[7m%s\033[0m \033[7m%s\033[0m\n" "TARGET   " "DESCRIPTION                                        " "YAML FILE                             "
@printf "%s %s %s\n" "---------" " --------------------------------------------------" "--------------------------------------"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step0/a/d    setup new namespace" " " "$(CREATE_ORDSNAMESPACE)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step1/a/d    setup certmaneger " " " "$(CERTMANAGER)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step2/a/d    setup operator" " " "$(shell basename $(ORACLE_OPERATOR_YAML))"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step3/a/d    default scoped deployment" " " "$(DEFAULT_NAMESPACE_SCOPE)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "             ords scoped deployment" " " "$(RST_NAMESPACE_SCOPE)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step4/a/d    node rbac" " " "$(NODE_RBAC)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "             storage class " " " "$(STORAGE_CLASS_RBAC)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "             persistent volume " " " "$(PERSISTENT_VOLUME_RBAC)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step5/a/d    setup db secret" " " "n/a"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step6/a/d    setup registry secret" " " "$(SECRET_CONTAINER_REGISTRY_SCRIPT)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step7/a/d    setup sidb " " " "$(SIDB_CREATION)"
@printf "================================================\n"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step8/a/d    setup RestServer " " " "$(REST_SERVER_CREATION)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step9/-/-    configure        " " " "Mongo ora db user:$(MONGOORADBUSER)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step10/a/d   setup RestServer Mongo " " " "$(REST_SERVER_CREATION_MONGO)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step11/-/-   test mongodb API " " " "----"
@printf "================================================\n"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step12/a/d   create secret for tnsadmin " " " "$(TNSADMIN)"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step13/a/d   create secrets for adminusers" " " "---"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "step14/a/d/e setup Multi Ords services    " " " "---"
@printf "================================================\n"
@printf "%-40s %+20s \033[1m %s\033[0m\n" "diagordsinit" "" "🔬dump initpod logs"



@printf "================================================\n"
@printf " a=apply d=delete ⚡e=generate error ⚡\n"
@printf "\n"
endef


help:man
man:
	$(call manpage)

define namespace
cat<<EOF|sed 's/^#//g' > $(CREATE_ORDSNAMESPACE)
#apiVersion: v1
#kind: Namespace
#metadata:
#  labels:
#    control-plane: controller-manager
#  name: $(2)
EOF
$(KUBECTL) $(1) -f $(CREATE_ORDSNAMESPACE)
$(KUBECTL) get namespace 
endef

step0:
	$(call namespace,$(ACTION),$(ORDSNAMESPACE))
step0a:
	$(MAKE) -f $(MAKEFILE) step0 ACTION=apply 
step0d:
	$(MAKE) -f $(MAKEFILE) step0 ACTION=delete

step1:
	$(KUBECTL) $(ACTION) -f $(CERTMANAGER)
step1a:
	$(MAKE) -f $(MAKEFILE) ACTION=apply step1
	$(KUBECTL) get pod   -n cert-manager
step1d:
	$(MAKE) -f $(MAKEFILE) ACTION=delete step1


define setwatchnamespace
@echo "Setting watch namespace list: $(WATCHLIST)"
sed 's/value: ""/value: "$(WATCHLIST)"/g'  $(ORACLE_OPERATOR_YAML) > `basename $(ORACLE_OPERATOR_YAML)`
$(KUBECTL) $(1) -f  `basename $(ORACLE_OPERATOR_YAML)`
$(DIFF) $(ORACLE_OPERATOR_YAML) `basename $(ORACLE_OPERATOR_YAML)`
$(KUBECTL) get pods -n $(OPRNAMESPACE)
endef

step2:
	$(call setwatchnamespace,$(ACTION))
step2a:
	$(MAKE) -f $(MAKEFILE) ACTION=apply step2
step2d:
	$(MAKE) -f $(MAKEFILE) ACTION=delete step2


define namespacescpe
cat<<EOF|sed 's/^#//g' >$(RST_NAMESPACE_SCOPE)
#apiVersion: rbac.authorization.k8s.io/v1
#kind: RoleBinding
#metadata:
#  name: $(ORDSNAMESPACE)-rolebinding
#  namespace: $(ORDSNAMESPACE)
#roleRef:
#  apiGroup: rbac.authorization.k8s.io
#  kind: ClusterRole
#  name: oracle-database-operator-manager-role
#subjects:
#- kind: ServiceAccount
#  name: default
#  namespace: oracle-database-operator-system
EOF

cat<<EOF|sed 's/^#//g' > $(DEFAULT_NAMESPACE_SCOPE)
#apiVersion: rbac.authorization.k8s.io/v1
#kind: RoleBinding
#metadata:
#  name: oracle-database-operator-oracle-database-operator-manager-rolebinding
#  namespace: default
#roleRef:
#  apiGroup: rbac.authorization.k8s.io
#  kind: ClusterRole
#  name: oracle-database-operator-manager-role
#subjects:
#- kind: ServiceAccount
#  name: default
#  namespace: oracle-database-operator-system
EOF

$(KUBECTL) $(1) -f $(RST_NAMESPACE_SCOPE)
$(KUBECTL) $(1) -f $(DEFAULT_NAMESPACE_SCOPE)
$(KUBECTL) get RoleBinding -n $(ORDSNAMESPACE)

endef

step3:
	$(call namespacescpe,$(ACTION))

step3a:
	$(MAKE) -f $(MAKEFILE) ACTION=apply step3

step3d:
	$(MAKE) -f $(MAKEFILE) ACTION=delete step3


export NODE_RBAC=node-rbac.yaml
export STORAGE_CLASS_RBAC=storage-class-rbac.yaml
export PERSISTENT_VOLUME_RBAC=persistent-volume-rbac.yaml


define persistenvolume

cat<<EOF|sed 's/^#//g' >$(NODE_RBAC)
#---
#apiVersion: rbac.authorization.k8s.io/v1
#kind: ClusterRole
#metadata:
#  name: oracle-database-operator-manager-role-node
#rules:
#- apiGroups:
#  - ""
#  resources:
#  - nodes
#  verbs:
#  - list
#  - watch
#---
#apiVersion: rbac.authorization.k8s.io/v1
#kind: ClusterRoleBinding
#metadata:
#  name: oracle-database-operator-manager-role-node-cluster-role-binding
#roleRef:
#  apiGroup: rbac.authorization.k8s.io
#  kind: ClusterRole
#  name: oracle-database-operator-manager-role-node
#subjects:
#- kind: ServiceAccount
#  name: default
#  namespace: oracle-database-operator-system
EOF

cat<<EOF|sed 's/^#//g' >$(STORAGE_CLASS_RBAC)
#---
#apiVersion: rbac.authorization.k8s.io/v1
#kind: ClusterRole
#metadata:
#  name: oracle-database-operator-manager-role-storage-class
#rules:
#- apiGroups:
#  - storage.k8s.io
#  resources:
#  - storageclasses
#  verbs:
#  - get
#  - list
#  - watch
#---
#apiVersion: rbac.authorization.k8s.io/v1
#kind: ClusterRoleBinding
#metadata:
#  name: oracle-database-operator-manager-role-storage-class-cluster-role-binding
#roleRef:
#  apiGroup: rbac.authorization.k8s.io
#  kind: ClusterRole
#  name: oracle-database-operator-manager-role-storage-class
#subjects:
#- kind: ServiceAccount
#  name: default
#  namespace: oracle-database-operator-system
#---
EOF

cat<<EOF|sed 's/^#//g' >$(PERSISTENT_VOLUME_RBAC)
#
#apiVersion: rbac.authorization.k8s.io/v1
#kind: ClusterRole
#metadata:
#  name: oracle-database-operator-manager-role-persistent-volume
#rules:
#- apiGroups:
#  - ""
#  resources:
#  - persistentvolumes
#  verbs:
#  - get
#  - list
#  - watch
#---
#apiVersion: rbac.authorization.k8s.io/v1
#kind: ClusterRoleBinding
#metadata:
#  name: oracle-database-operator-manager-role-persistent-volume-cluster-role-binding
#roleRef:
#  apiGroup: rbac.authorization.k8s.io
#  kind: ClusterRole
#  name: oracle-database-operator-manager-role-persistent-volume
#subjects:
#- kind: ServiceAccount
#  name: default
#  namespace: oracle-database-operator-system
#---
#
EOF

$(KUBECTL) $(1) -f $(NODE_RBAC)
$(KUBECTL) $(1) -f $(STORAGE_CLASS_RBAC)
$(KUBECTL) $(1) -f $(PERSISTENT_VOLUME_RBAC)

endef

step4:
	$(call persistenvolume,$(ACTION))
step4a:
	$(MAKE) -f $(MAKEFILE) ACTION=apply step4
step4d:
	$(MAKE) -f $(MAKEFILE) ACTION=delete step4


export SYSPWDFILE1=syspwdfile
export ORDPWDFILE=ordspwdfile
export SIDB_PASSWORD_FILE=sidbpasswordfile

export PRVKEY=ca.key
export PUBKEY=public.pem
export OPENSSL=/usr/bin/openssl

step5a:
	echo $(SIDB_PASSWORD) > $(SIDB_PASSWORD_FILE) 
	- $(KUBECTL) delete secret pubkey -n ${ORDSNAMESPACE}
	- $(KUBECTL) delete secret prvkey -n ${ORDSNAMESPACE}
	- $(KUBECTL) delete secret $(SIDB_SECRET)  -n ${ORDSNAMESPACE}
	- $(KUBECTL) delete secret $(ORDS_SECRET) -n ${ORDSNAMESPACE}
	$(OPENSSL) genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > ${PRVKEY}
	$(OPENSSL) rsa -in $(PRVKEY) -outform PEM  -pubout -out $(PUBKEY)
	$(KUBECTL) create secret generic pubkey --from-file=publicKey=$(PUBKEY)   -n $(ORDSNAMESPACE)
	$(KUBECTL) create secret generic prvkey --from-file=privateKey=$(PRVKEY)  -n $(ORDSNAMESPACE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(SIDB_PASSWORD_FILE) |base64 > e_$(SIDB_PASSWORD_FILE)
	$(KUBECTL) create secret generic $(SIDB_SECRET) --from-literal=password=$(SIDB_PASSWORD) -n $(OPRNAMESPACE)
	$(KUBECTL) create secret generic $(ORDS_SECRET) --from-file=password=e_$(SIDB_PASSWORD_FILE) -n  $(ORDSNAMESPACE)
	$(RM) e_$(SIDB_PASSWORD_FILE) $(SIDB_PASSWORD_FILE) 

step5d:
	- $(KUBECTL) delete secret pubkey                    -n ${ORDSNAMESPACE}
	- $(KUBECTL) delete secret prvkey                    -n ${ORDSNAMESPACE}
	- $(KUBECTL) delete secret $(SIDB_SECRET)  -n ${ORDSNAMESPACE}
	- $(KUBECTL) delete secret $(ORDS_SECRET) -n ${ORDSNAMESPACE}


define registry_secret
printf "#!/bin/bash \n" >$(SECRET_CONTAINER_REGISTRY_SCRIPT)
printf "echo enter password for $(USER_CONTAINER_REGISTRY)@$(ORACLE_CONTAINER_REGISTRY) \n" >$(SECRET_CONTAINER_REGISTRY_SCRIPT)
printf "read -s scpwd \n" >>$(SECRET_CONTAINER_REGISTRY_SCRIPT)
printf "$(KUBECTL) create secret docker-registry $(SECRET_CONTAINER_REGISTRY) --docker-server=$(ORACLE_CONTAINER_REGISTRY) --docker-username=$(USER_CONTAINER_REGISTRY) --docker-password=\u0024scpwd --docker-email=$(USER_CONTAINER_REGISTRY) -n $(OPRNAMESPACE) \n" >>$(SECRET_CONTAINER_REGISTRY_SCRIPT)
printf "$(KUBECTL) create secret docker-registry $(SECRET_CONTAINER_REGISTRY) --docker-server=$(ORACLE_CONTAINER_REGISTRY) --docker-username=$(USER_CONTAINER_REGISTRY) --docker-password=\u0024scpwd --docker-email=$(USER_CONTAINER_REGISTRY) -n $(ORDSNAMESPACE) \n" >>$(SECRET_CONTAINER_REGISTRY_SCRIPT)

bash $(SECRET_CONTAINER_REGISTRY_SCRIPT)
endef 

step6a:
	$(call registry_secret)

step6d:
	$(KUBECTL) delete  secret  $(SECRET_CONTAINER_REGISTRY) -n $(OPRNAMESPACE)


define sidb

cat<<EOF|sed 's/^#//g' >$(SIDB_CREATION)
#apiVersion: database.oracle.com/v4
#kind: SingleInstanceDatabase
#metadata:
#  name: oraoper-sidb
#  namespace: $(OPRNAMESPACE)
#spec:
#  replicas: 1
#  image:
#    pullFrom: $(SIDB_IMAGE)
#    pullSecrets: $(SECRET_CONTAINER_REGISTRY)
#    prebuiltDB: true
#  sid: FREE
#  listenerPort: 30001
#  edition: free
#  adminPassword:
#    secretName: $(SIDB_SECRET)
#    secretKey: password
#  pdbName: FREEPDB1
EOF

$(KUBECTL) $(1) -f  $(SIDB_CREATION)
endef

step7:
	$(call sidb,$(ACTION))
step7a:
	$(MAKE) -f $(MAKEFILE) ACTION=apply step7
step7d:
	$(MAKE) -f $(MAKEFILE) ACTION=delete step7


define restservice 
cat<<EOF|sed 's/^#//g' >$(REST_SERVER_CREATION)
#apiVersion: database.oracle.com/v4
#kind: $(KIND) 
#metadata:
#  name: $(REST_SERVER_NAME)
#  namespace: $(ORDSNAMESPACE)
#spec:
#  image: $(ORDS_IMAGE)
#  forceRestart: true
#  encPrivKey:
#    secretName: prvkey
#    passwordKey: privateKey
#  globalSettings:
#    database.api.enabled: true
#  poolSettings:
#    - poolName: default
#      autoUpgradeORDS: true
#      autoUpgradeAPEX: true
#      restEnabledSql.active: true
#      plsql.gateway.mode: direct
#      db.connectionType: customurl
#      db.customURL: jdbc:oracle:thin:@//$(2)
#      db.username: ORDS_PUBLIC_USER
#      db.secret:
#        secretName:  $(ORDS_SECRET)
#      db.adminUser: SYS
#      db.adminUser.secret:
#        secretName:  $(ORDS_SECRET)
#
EOF

[ $(3) -eq 1 ] && {
sed -i  's/SYS/SYT/g' $(REST_SERVER_CREATION)
echo -e "TYPO"
}

$(KUBECTL) $(1) -f  $(REST_SERVER_CREATION)
endef

step8:
	$(eval TNS_ALIAS_CDB := $(shell $(KUBECTL) get SingleInstanceDatabase -n $(OPRNAMESPACE) --template '{{range .items}}{{.status.clusterConnectString}}{{"\n"}}{{end}}'))
	$(eval TNS_ALIAS_PDB := $(shell $(KUBECTL) get SingleInstanceDatabase -n $(OPRNAMESPACE) --template '{{range .items}}{{.status.pdbConnectString}}{{"\n"}}{{end}}'))
	echo $(TNS_ALIAS)
	$(call restservice,$(ACTION),$(TNS_ALIAS_PDB),$(ERR))
step8a:
	$(MAKE) -f $(MAKEFILE) ACTION=apply step8 ERR=0
step8d:
	$(MAKE) -f $(MAKEFILE) ACTION=delete step8 ERR=0
step8e:
	$(MAKE) -f $(MAKEFILE) ACTION=apply step8 ERR=1

reloadop:
	echo "RESTARTING OPERATOR"
	$(eval OP1 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1 ))
	$(eval OP2 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1|cut  -d ' ' -f 1 ))
	$(eval OP3 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1 ))
	$(KUBECTL) get pod $(OP1) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP2) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP3) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -

loginords:
	@$(eval RESTPOD := $(shell $(KUBECTL) get pods --template '{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}' -n $(ORDSNAMESPACE)))
	$(KUBECTL) logs $(RESTPOD) -n $(ORDSNAMESPACE)
	$(KUBECTL) exec  $(RESTPOD) -n $(ORDSNAMESPACE) -it -- /bin/bash 

logindb:
	$(eval PODPDB := $(shell $(KUBECTL) get pods  --template '{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}' -n $(OPRNAMESPACE)|grep -v oracle-database-operator))
	echo $(PODPDB)
	$(KUBECTL) exec $(PODPDB) -n $(OPRNAMESPACE) -it -- bash	


report:
	$(KUBECTL) get pods -n $(OPRNAMESPACE)
	$(KUBECTL) get SingleInstanceDatabase -n $(OPRNAMESPACE)
	$(KUBECTL) get pods -n $(ORDSNAMESPACE)
	$(KUBECTL) get $(KIND) -n $(ORDSNAMESPACE)
	

someattributes:
	kubectl get SingleInstanceDatabase -n oracle-database-operator-system --template '{{range .items}}{{.status.connectString}}{{"\n"}}{{end}}'
	kubectl get SingleInstanceDatabase -n oracle-database-operator-system --template '{{range .items}}{{.status.tcpsConnectString}}{{"\n"}}{{end}}'
	kubectl get SingleInstanceDatabase -n oracle-database-operator-system --template '{{range .items}}{{.status.clusterConnectString}}{{"\n"}}{{end}}'
	kubectl get SingleInstanceDatabase -n oracle-database-operator-system --template '{{range .items}}{{.status.tcpsPdbConnectString}}{{"\n"}}{{end}}'
	kubectl get SingleInstanceDatabase -n oracle-database-operator-system --template '{{range .items}}{{.status.pdbConnectString}}{{"\n"}}{{end}}'





dump:
	@$(eval TMPSP := $(shell date "+%y%m%d%H%M%S" ))
	@$(eval DIAGFILE := ./opdmp.$(TMPSP))
	@>$(DIAGFILE)
	@echo "OPERATOR DUMP"  >> $(DIAGFILE)
	@echo "~~~~~~~~~~~~~"  >> $(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1 | cut -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)



step9: sql
define dbenv
$(1): DB_PWD=`$(KUBECTL) get secrets sidb-db-auth -n  $(OPRNAMESPACE) --template='{{.data.password | base64decode}}'`
$(1): POD_NAME=`$(KUBECTL) get pod -l "app=oraoper-sidb" -o custom-columns=NAME:.metadata.name -n  $(OPRNAMESPACE) --no-headers`
$(1): TNSSTR=`$(KUBECTL) get SingleInstanceDatabase -n $(OPRNAMESPACE) --template '{{range .items}}{{.status.pdbConnectString}}{{"\n"}}{{end}}'`
endef

$(eval $(call dbenv,sqlplus sql))
#$(eval $(call dbenv,sqlplus))

define copyfile
cat <<EOF >create_mong_schema.sql
drop user MONGO cascade;
set echo on 
set head on
create user MONGO identified by "My_Password1!";
grant soda_app, create session, create table, create view, create sequence, create procedure, create job,
unlimited tablespace to MONGO;  
conn MONGO/My_Password1!@${TNSSTR}
exec ords.enable_schema;
exit;
EOF
$(KUBECTL) cp ./create_mong_schema.sql $(POD_NAME):/home/oracle -n $(OPRNAMESPACE)
endef

sql:
	echo $(TNSSTR)
	$(call copyfile)
	@$(KUBECTL) exec -it $(POD_NAME) -n $(OPRNAMESPACE) -- sqlplus SYSTEM/$(DB_PWD)@$(TNSSTR) @/home/oracle/create_mong_schema.sql

sqlplus:
	@$(KUBECTL) exec -it $(POD_NAME) -n $(OPRNAMESPACE) -- sqlplus SYSTEM/$(DB_PWD)@$(TNSSTR) 
	

define restservicemongo
cat <<EOF |sed 's/^#//g' >$(REST_SERVER_CREATION_MONGO)
#apiVersion: database.oracle.com/v4
#kind: $(KIND)
#metadata:
#  name: $(REST_SERVER_NAME_MONGO)
#  namespace: $(ORDSNAMESPACE)
#spec:
#  image: $(ORDS_IMAGE.1)
#  forceRestart: true
#  globalSettings:
#    database.api.enabled: true
#    mongo.enabled: true
#  poolSettings:
#    - poolName: default
#      autoUpgradeORDS: true
#      restEnabledSql.active: true
#      plsql.gateway.mode: direct
#      jdbc.MaxConnectionReuseCount: 5000
#      jdbc.MaxConnectionReuseTime: 900
#      jdbc.SecondsToTrustIdleConnection: 1
#      jdbc.InitialLimit: 100
#      jdbc.MaxLimit: 100
#      db.connectionType: customurl
#      db.customURL: jdbc:oracle:thin:@//${2}
#      db.username: ORDS_PUBLIC_USER
#      db.secret:
#        secretName:  ords-db-auth
#      db.adminUser: SYS
#      db.adminUser.secret:
#        secretName:  ords-db-auth
EOF
$(KUBECTL) $(1) -f $(REST_SERVER_CREATION_MONGO)
endef



step10:
	$(eval TNS_ALIAS_PDB := $(shell $(KUBECTL) get SingleInstanceDatabase -n $(OPRNAMESPACE) --template '{{range .items}}{{.status.pdbConnectString}}{{"\n"}}{{end}}'))
	echo $(TNS_ALIAS_PDB)
	$(call restservicemongo,$(ACTION),$(TNS_ALIAS_PDB))
step10a:
	$(MAKE) -f $(MAKEFILE) ACTION=apply step10
step10d:
	$(MAKE) -f $(MAKEFILE) ACTION=delete step10


step11:
	echo "Open a port-forward to the MongoAPI service"
	@nohup $(KUBECTL) port-forward service/$(REST_SERVER_NAME_MONGO) 27017:27017 -n $(ORDSNAMESPACE) 1>portfwd.log 2>&1 &
	@echo "DOWNLOADING MONGOSH"
	@$(CURL) https://downloads.mongodb.com/compass/$(MONGOSH).tgz --output mongosh-2.3.1-linux-x64.tgz
	@echo "UNTAR FILE"
	@$(TAR) -zxvf  $(MONGOSH).tgz  
	./$(MONGOSH)/bin/mongosh  --tlsAllowInvalidCertificates 'mongodb://MONGO:My_Password1!@localhost:27017/MONGO?authMechanism=PLAIN&authSource=$external&tls=true&retryWrites=false&loadBalanced=true'
	@echo "STOP PORT FRWD"
	@kill `ps -ef | grep kubectl | grep 27017 | grep -v grep | awk '{printf $$2}'`
	$(RM) $(MONGOSH).tgz
	$(RM) -rf ./$(MONGOSH)


define buildtns
echo "Building tnsnames.ora"
cat <<EOF >$(TNSADMIN)/$(TNSNAMES)
$(PDB1)=$(TNS1)

$(PDB2)=$(TNS2)
EOF
$(KUBECTL) create secret generic multi-tns-admin -n  $(ORDSNAMESPACE) --from-file=$(TNSADMIN)/
endef

step12a:
	$(call buildtns)

step12d:
	$(KUBECTL) delete secret multi-tns-admin  -n  $(ORDSNAMESPACE) 

export SYSPWDFILE1=syspwdfile1
export SYSPWDFILE2=syspwdfile2
export ORDPWDFILE=ordspwdfile


step13a:
	echo $(PDB1_PWD) > $(SYSPWDFILE1)
	echo $(PDB2_PWD) > $(SYSPWDFILE2)
	echo $(ORDS_MULTI_POOL_PWD) > $(ORDPWDFILE)
	$(OPENSSL)  genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > ${PRVKEY}
	$(OPENSSL) rsa -in $(PRVKEY) -outform PEM  -pubout -out $(PUBKEY)
	#$(KUBECTL) create secret generic pubkey --from-file=publicKey=$(PUBKEY) -n $(ORDSNAMESPACE)
	$(KUBECTL) create secret generic prvkey --from-file=privateKey=$(PRVKEY)  -n $(ORDSNAMESPACE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(SYSPWDFILE1) |base64 > e_$(SYSPWDFILE1)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(SYSPWDFILE2) |base64 > e_$(SYSPWDFILE2)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(ORDPWDFILE) |base64 > e_$(ORDPWDFILE)
	$(KUBECTL) create secret generic $(PDB1_PRIV_AUTH_SECRET) --from-file=password=e_$(SYSPWDFILE1) -n  $(ORDSNAMESPACE)
	$(KUBECTL) create secret generic $(PDB2_PRIV_AUTH_SECRET) --from-file=password=e_$(SYSPWDFILE2) -n  $(ORDSNAMESPACE)
	$(KUBECTL) create secret generic $(MULTI_ORDS_AUTH_SECRET) --from-file=password=e_$(ORDPWDFILE) -n  $(ORDSNAMESPACE)
	$(RM) $(SYSPWDFILE1) $(SYSPWDFILE2) $(ORDPWDFILE) e_$(SYSPWDFILE1) e_$(SYSPWDFILE2) e_$(ORDPWDFILE)

step13d:
	- $(KUBECTL) delete secret pubkey                     -n $(ORDSNAMESPACE)
	- $(KUBECTL) delete secret prvkey                     -n $(ORDSNAMESPACE)
	- $(KUBECTL) delete secret $(PDB1_PRIV_AUTH_SECRET)   -n $(ORDSNAMESPACE)
	- $(KUBECTL) delete secret $(PDB2_PRIV_AUTH_SECRET)   -n $(ORDSNAMESPACE)
	- $(KUBECTL) delete secret $(MULTI_ORDS_AUTH_SECRET)  -n $(ORDSNAMESPACE)

define multisrv
cat <<EOF |sed 's/^#//g' >$(MULTISRV_MANIFEST)
#apiVersion: database.oracle.com/v4
#kind: $(KIND)
#metadata:
#  name: ords-multi-pool
#  namespace: $(ORDSNAMESPACE)
#spec:
#  image: container-registry.oracle.com/database/ords:24.1.1
#  forceRestart: true
#  encPrivKey:
#    secretName: prvkey
#    passwordKey: privateKey
#  globalSettings:
#    database.api.enabled: true
#  poolSettings:
#    - poolName: pdb1
#      autoUpgradeAPEX: false
#      autoUpgradeORDS: false
#      db.connectionType: tns
#      db.tnsAliasName: pdb1
#      tnsAdminSecret:
#        secretName: multi-tns-admin
#      restEnabledSql.active: true
#      feature.sdw: true
#      plsql.gateway.mode: proxied
#      db.username: ORDS_PUBLIC_USER
#      db.secret:
#        secretName: $(MULTI_ORDS_AUTH_SECRET)
#      db.adminUser: SYS
#      db.adminUser.secret:
#        secretName: $(PDB1_PRIV_AUTH_SECRET)
#    - poolName: pdb2
#      autoUpgradeAPEX: false
#      autoUpgradeORDS: false
#      db.connectionType: tns
#      db.tnsAliasName: PDB2
#      tnsAdminSecret:
#        secretName:  multi-tns-admin
#      restEnabledSql.active: true
#      feature.sdw: true
#      plsql.gateway.mode: proxied
#      db.username: ORDS_PUBLIC_USER
#      db.secret:
#        secretName: $(MULTI_ORDS_AUTH_SECRET)
#      db.adminUser: SYS
#      db.adminUser.secret:
#        secretName: $(PDB1_PRIV_AUTH_SECRET)

#
EOF
[ $(2) -eq 1 ] && {
sed -i  's/SYS/SYT/g' $(MULTISRV_MANIFEST)
echo -e "TYPO"
}

$(KUBECTL) $(1) -f $(MULTISRV_MANIFEST)
endef

step14:
	$(call multisrv,$(ACTION),$(ERR))
step14a:
	$(MAKE) -f $(MAKEFILE) ACTION=apply  ERR=0 step14
step14d:
	$(MAKE) -f $(MAKEFILE) ACTION=delete ERR=0 step14
step14e:
	$(MAKE) -f $(MAKEFILE) ACTION=apply ERR=1 step14


define dumpinit
#!/bin/bash
NAMESPACE=${1}
KUBECTL=/usr/bin/kubectl
for _pod in `${KUBECTL} get pods  --no-headers -o custom-columns=":metadata.name" --no-headers -n $${NAMESPACE}`
do
	for _podinit in   `${KUBECTL} get pod $${_pod} -n $${NAMESPACE} -o="custom-columns=INIT-CONTAINERS:.spec.initContainers[*].name" --no-headers`
	do
        echo "DUMPINIT $${_pod}:$${_podinit}"
	${KUBECTL} logs -f --since=0 $${_pod} -n $${NAMESPACE} -c $${_podinit}
        done
done
endef

diagordsinit:
	$(call dumpinit ,$(ORDSNAMESPACE))

