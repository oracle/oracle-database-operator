# Copyright (c) 2022, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.
#   __  __       _         __ _ _
#  |  \/  | __ _| | _____ / _(_) | ___
#  | |\/| |/ _` | |/ / _ \ |_| | |/ _ \
#  | |  | | (_| |   <  __/  _| | |  __/
#  |_|  |_|\__,_|_|\_\___|_| |_|_|\___|
#  | | | | ___| |_ __   ___ _ __
#  | |_| |/ _ \ | '_ \ / _ \ '__|
#  |  _  |  __/ | |_) |  __/ |
#  |_| |_|\___|_| .__/ \___|_|
#               |_|
#
#  WARNING: Using this makefile helps you to customize yaml  
#           files. Edit parameters.txt with your enviroment 
#           informartion and execute the following steps
#
#         1) make opsetup
#            it configures the operator yaml files with the 
#            watch namelist required by the multitenant controllers
#      
#         2) make secrets
#            It configure the required secrets necessary to operate
#            with pdbs multitenant controllers
#
#         3) make genyaml 
#            It automatically creates all the yaml files based on the 
#            information available in the parameters file 
#            
#            LIST OF GENERAED YAML FILE
#
#               -----------------------------       ----------------------------------
# 		oracle-database-operator.yaml     : oracle database operator
# 		lrestnamespace_binding.yaml       : role binding for lrestnamespace
# 		pdbnamespace_binding.yaml         : role binding for pdbnamespace
# 		create_lrest_secret.yaml          : create secrets for rest server pod
# 		create_lrpdb_secret.yaml          : create secrets for pluggable database
# 		create_lrest_pod.yaml             : create rest server pod
# 		create_pdb1_resource.yaml         : create first pluggable database
# 		create_pdb2_resource.yaml         : create second pluggable database
# 		open_pdb1_resource.yaml           : open first pluggable database
# 		open_pdb2_resource.yaml           : open second pluggable database
# 		close_pdb1_resource.yaml          : close first pluggable database
# 		close_pdb2_resource.yaml          : close second pluggable database
# 		clone_lrpdb_resource.yaml         : clone thrid pluggable database
# 		clone_pdb2_resource.yaml          : clone 4th pluggable database
# 		delete_pdb1_resource.yaml         : delete first pluggable database
# 		delete_pdb2_resource.yaml         : delete sencond pluggable database
# 		delete_pdb3_resource.yaml         : delete thrid pluggable database
# 		unplug_pdb1_resource.yaml         : unplug first pluggable database
# 		plug_pdb1_resource.yaml           : plug first pluggable database
# 		map_pdb1_resource.yaml            : map the first pluggable database
# 		config_map.yam                    : pdb parameters array
# 		altersystem_pdb1_resource.yaml    : chage cpu_count count parameter for the first pdb
# 		config_map_plsql.yaml             : plsql code 
#
DATE  := `date "+%y%m%d%H%M%S"`
######################
# PARAMETER SECTIONS #
######################
   
export PARAMETERS=parameters.txt
#export TNSALIAS=$(shell cat $(PARAMETERS)  |grep -v ^\#|grep TNSALIAS|cut -d :  -f 2)
export TNSALIAS=$(shell cat $(PARAMETERS)  |grep -v ^\#|grep TNSALIAS|awk -F ':' '{$$1="" ;printf("%s",$$2) }')
export DBUSER=$(shell cat $(PARAMETERS)|grep -v ^\#|grep DBUSER|cut -d :  -f 2)
export DBPASS=$(shell cat $(PARAMETERS)|grep -v ^\#|grep DBPASS|cut -d :  -f 2)
export WBUSER=$(shell cat $(PARAMETERS)|grep -v ^\#|grep WBUSER|cut -d :  -f 2)
export WBPASS=$(shell cat $(PARAMETERS)|grep -v ^\#|grep WBPASS|cut -d :  -f 2)
export PDBUSR=$(shell cat $(PARAMETERS)|grep -v ^\#|grep PDBUSR|cut -d :  -f 2)
export PDBPWD=$(shell cat $(PARAMETERS)|grep -v ^\#|grep PDBPWD|cut -d :  -f 2)
export PDBNAMESPACE=$(shell cat $(PARAMETERS)|grep -v ^\#|grep PDBNAMESPACE|cut -d :  -f 2)
export LRSNAMESPACE=$(shell cat $(PARAMETERS)|grep -v ^\#|grep LRSNAMESPACE|cut -d :  -f 2)
export LRESTIMG=$(shell cat $(PARAMETERS)|grep -v ^\#|grep LRESTIMG|cut -d : -f 2,3)
export COMPANY=$(shell cat $(PARAMETERS)|grep -v ^\#|grep COMPANY|cut -d : -f 2)
export APIVERSION=$(shell cat $(PARAMETERS)|grep -v ^\#|grep APIVERSION|cut -d : -f 2)
export SERVICENAM=$(shell cat $(PARAMETERS)|grep -v ^\#|grep SERVICENAMEACCOUNT|cut -d : -f 2)
export OPENSHIFT=$(shell cat $(PARAMETERS)|grep -v ^\#|grep OPENSHIFT|cut -d : -f 2)
export PLSQLMAP=$(shell cat $(PARAMETERS)|grep -v ^\#|grep PLSQLMAP|cut -d : -f 2)
export OPRNAMESPACE=oracle-database-operator-system
export CODE_TREE=$(shell cat $(PARAMETERS)|grep -v ^\#|grep CODE_TREE|cut -d : -f 2)
export ORACLE_OPERATOR_YAML=$(CODE_TREE)/oracle-database-operator.yaml
export AUTODISCOVER=$(shell cat $(PARAMETERS)|grep -v ^\#|grep AUTODISCOVER|cut -d : -f 2)
export CERT_MANAGER=$(shell cat $(PARAMETERS)|grep -v ^\#|grep CERT_MANAGER| awk -F ':' '{$$1="" ;printf("%s:%s",$$2,$$3) }')
export TEST_EXEC_TIMEOUT=3m

REST_SERVER=lrest
SKEY=tls.key
SCRT=tls.crt
CART=ca.crt
PRVKEY=ca.key
PUBKEY=public.pem
COMPANY=oracle
DBUSERFILE=dbuser.txt
DBPASSFILE=dbpass.txt
WBUSERFILE=wbuser.txt
WBPASSFILE=wbpass.txt
PDBUSRFILE=pdbusr.txt
PDBPWDFILE=pdbpwd.txt

#################
## PARAMETERS  ##
#################

export ASSERTDELETION=true

#################
### FILE LIST ###
#################

export LREST_POD=create_lrest_pod.yaml

export LRPDBCRE1=create_pdb1_resource.yaml
export LRPDBCRE2=create_pdb2_resource.yaml

export LRPDBCLOSE1=close_pdb1_resource.yaml
export LRPDBCLOSE2=close_pdb2_resource.yaml
export LRPDBCLOSE3=close_pdb3_resource.yaml

export LRPDBOPEN1=open_pdb1_resource.yaml
export LRPDBOPEN2=open_pdb2_resource.yaml
export LRPDBOPEN3=open_pdb3_resource.yaml

export LRPDBCLONE1=clone_pdb1_resource.yaml
export LRPDBCLONE2=clone_pdb2_resource.yaml

export LRPDBDELETE1=delete_pdb1_resource.yaml
export LRPDBDELETE2=delete_pdb2_resource.yaml
export LRPDBDELETE3=delete_pdb3_resource.yaml

export LRPDBUNPLUG1=unplug_pdb1_resource.yaml
export LRPDBPLUG1=plug_pdb1_resource.yaml

export LRPDBMAP1=map_pdb1_resource.yaml
export LRPDBMAP2=map_pdb2_resource.yaml
export LRPDBMAP3=map_pdb3_resource.yaml

export LRPDBMAP1=map_pdb1_resource.yaml
export LRPDBMAP2=map_pdb2_resource.yaml
export LRPDBMAP3=map_pdb3_resource.yaml

export SECURITYCTX=security_context.yaml
export NODE_RBAC=node_rbac.yaml

export ALTERSYSTEMYAML=altersystem_pdb1_resource.yaml
export CONFIG_MAP=config_map_pdb.yaml
export CONFIG_MAP_SQL=config_map_plsql.yaml




##BINARIES
export KUBECTL=/usr/bin/kubectl
OPENSSL=/usr/bin/openssl
ECHO=/usr/bin/echo
RM=/usr/bin/rm
CP=/usr/bin/cp
TAR=/usr/bin/tar
MKDIR=/usr/bin/mkdir
SED=/usr/bin/sed
MAKE=/usr/bin/make
MAKEFILE=./makefile

check:
	@printf "LRESTIMG...............:%s\n" $(LRESTIMG)
	@printf "TNSALIAS...............:%.60s....\n" $(TNSALIAS)
	@printf "DBUSER.................:%s\n" $(DBUSER)
	@printf "DBPASS.................:%s\n" $(DBPASS)
	@printf "WBUSER.................:%s\n" $(WBUSER)
	@printf "WBPASS.................:%s\n" $(WBPASS)
	@printf "PDBUSR.................:%s\n" $(PDBUSR)
	@printf "PDBPWD.................:%s\n" $(PDBPWD)
	@printf "PDBNAMESPACE...........:%s\n" $(PDBNAMESPACE)
	@printf "LRSNAMESPACE...........:%s\n" $(LRSNAMESPACE)
	@printf "COMPANY................:%s\n" $(COMPANY)
	@printf "SERVICENAMEACCOUNT.....:%s\n" $(SERVICENAM)
	@printf "APIVERSION.............:%s\n" $(APIVERSION)
	@printf "PLSQLMAP...............:%s\n" $(PLSQLMAP)
	@printf "AUTODISCOVER...........:%s\n" $(AUTODISCOVER)
	@printf "ORACLE_OPERATOR_YAML...:%s\n" $(ORACLE_OPERATOR_YAML)
	@printf "CODE_TREE..............:%s\n" $(CODE_TREE)
	@printf "CERT_MANAGER...........:%s\n" $(CERT_MANAGER)
	@printf "OPENSHIFT..............:%s\n" $(OPENSHIFT)

define msg
@printf "\033[31;7m%s\033[0m\r" "......................................]"
@printf "\033[31;7m[\xF0\x9F\x91\x89 %s\033[0m\n" $(1)
endef

tls:
	$(call msg,"TLS GENERATION")
	#$(OPENSSL) genrsa -out $(PRVKEY) 2048	
	$(OPENSSL)  genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > $(PRVKEY)
	$(OPENSSL) req -new -x509 -days 365 -key $(PRVKEY) \
    	       -subj "/C=CN/ST=GD/L=SZ/O=$(COMPANY), Inc./CN=$(COMPANY) Root CA" -out ca.crt
	$(OPENSSL) req -newkey rsa:2048 -nodes -keyout $(SKEY) -subj \
	       	"/C=CN/ST=GD/L=SZ/O=$(COMPANY), Inc./CN=cdb-dev-$(REST_SERVER).$(LRSNAMESPACE)" -out server.csr
	$(ECHO) "subjectAltName=DNS:cdb-dev-$(REST_SERVER).$(LRSNAMESPACE)" > extfile.txt
	$(OPENSSL) x509 -req -extfile extfile.txt -days 365 -in server.csr -CA ca.crt -CAkey $(PRVKEY) -CAcreateserial -out $(SCRT)
	$(OPENSSL) rsa -in $(PRVKEY) -outform PEM  -pubout -out $(PUBKEY)

secrets: tls delsecrets
	$(call msg,"CREATING NEW TLS/PRVKEY/PUBKEY SECRETS")
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(LRSNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(LRSNAMESPACE)
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(PDBNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(PDBNAMESPACE)
	#$(KUBECTL) create secret tls prvkey --key="$(PRVKEY)" --cert=ca.crt  -n $(LRSNAMESPACE)
	$(KUBECTL) create secret generic pubkey --from-file=publicKey=$(PUBKEY) -n $(LRSNAMESPACE)
	$(KUBECTL) create secret generic prvkey --from-file=privateKey=$(PRVKEY)  -n $(LRSNAMESPACE)
	$(KUBECTL) create secret generic prvkey --from-file=privateKey="$(PRVKEY)" -n $(PDBNAMESPACE)
	$(call msg,"CREATING NEW CREDENTIAL SECRETS")
	@$(ECHO) $(DBUSER) > $(DBUSERFILE)
	@$(ECHO) $(DBPASS) > $(DBPASSFILE)
	@$(ECHO) $(WBUSER) > $(WBUSERFILE)
	@$(ECHO) $(WBPASS) > $(WBPASSFILE)
	@$(ECHO) $(PDBUSR) > $(PDBUSRFILE)
	@$(ECHO) $(PDBPWD) > $(PDBPWDFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(DBUSERFILE) |base64 > e_$(DBUSERFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(DBPASSFILE) |base64 > e_$(DBPASSFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(WBUSERFILE) |base64 > e_$(WBUSERFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(WBPASSFILE) |base64 > e_$(WBPASSFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(PDBUSRFILE) |base64 > e_$(PDBUSRFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(PDBPWDFILE) |base64 > e_$(PDBPWDFILE)
	$(KUBECTL) create secret generic dbuser --from-file=e_$(DBUSERFILE) -n  $(LRSNAMESPACE)
	$(KUBECTL) create secret generic dbpass --from-file=e_$(DBPASSFILE) -n  $(LRSNAMESPACE)
	$(KUBECTL) create secret generic wbuser --from-file=e_$(WBUSERFILE) -n  $(LRSNAMESPACE)
	$(KUBECTL) create secret generic wbpass --from-file=e_$(WBPASSFILE) -n  $(LRSNAMESPACE)
	$(KUBECTL) create secret generic wbuser --from-file=e_$(WBUSERFILE) -n  $(PDBNAMESPACE)
	$(KUBECTL) create secret generic wbpass --from-file=e_$(WBPASSFILE) -n  $(PDBNAMESPACE)
	$(KUBECTL) create secret generic pdbusr --from-file=e_$(PDBUSRFILE) -n  $(PDBNAMESPACE)
	$(KUBECTL) create secret generic pdbpwd --from-file=e_$(PDBPWDFILE) -n  $(PDBNAMESPACE)
	$(RM) $(SKEY) $(SCRT) $(CART) $(PRVKEY) $(PUBKEY) server.csr extfile.txt ca.srl \
		$(DBUSERFILE) $(DBPASSFILE) $(WBUSERFILE) $(WBPASSFILE) $(PDBUSRFILE) $(PDBPWDFILE)\
		e_$(DBUSERFILE) e_$(DBPASSFILE) e_$(WBUSERFILE) e_$(WBPASSFILE) e_$(PDBUSRFILE) e_$(PDBPWDFILE)
	$(KUBECTL) get secrets -n  $(LRSNAMESPACE)
	$(KUBECTL) get secrets -n  $(PDBNAMESPACE)

delsecrets:
	$(call msg,"CLEAN OLD SECRETS")
	$(eval SECRETSP:=$(shell kubectl get secrets -n $(PDBNAMESPACE) -o custom-columns=":metadata.name" --no-headers) )	
	$(eval SECRETSL:=$(shell kubectl get secrets -n $(LRSNAMESPACE) -o custom-columns=":metadata.name" --no-headers) )	
	@[ "${SECRETSP}" ] && ( \
	       	printf "Deleteing secrets in namespace -n $(PDBNAMESPACE)\n") &&\
	        ($(KUBECTL) delete secret  $(SECRETSP) -n $(PDBNAMESPACE))\
	        || ( echo "No screts in namespace $(PDBNAMESPACE)")
	@[ "${SECRETSL}" ] && ( \
	       	printf "Deleteing secrets in namespace -n $(LRSNAMESPACE)\n") &&\
	        ($(KUBECTL) delete secret  $(SECRETSL) -n $(LRSNAMESPACE))\
	        || ( echo "No screts in namespace $(PDBNAMESPACE)")

cleanCert:
	$(RM) $(SKEY) $(SCRT) $(CART) $(PRVKEY) $(PUBKEY) server.csr extfile.txt ca.srl \
		$(DBUSERFILE) $(DBPASSFILE) $(WBUSERFILE) $(WBPASSFILE) $(PDBUSRFILE) $(PDBPWDFILE)\
		e_$(DBUSERFILE) e_$(DBPASSFILE) e_$(WBUSERFILE) e_$(WBPASSFILE) e_$(PDBUSRFILE) e_$(PDBPWDFILE)

### YAML FILE SECTION ###
define _opr
cp ${ORACLE_OPERATOR_YAML} .
export OPBASENAME=`basename ${ORACLE_OPERATOR_YAML}`
#export PDBNAMESPACE=$(cat ${PARAMETERS}|grep -v ^\#|grep PDBNAMESPACE|cut -d :  -f 2)

cp ${OPBASENAME} ${OPBASENAME}.ORIGNINAL
printf "\n\t\xF0\x9F\x91\x89 ${OPBASENAME}\n\n"
printf "\n\t\xF0\x9F\x91\x89 ${PDBNAMESPACE}\n\n"
sed -i  's/value: ""/value: ${OPRNAMESPACE},$PDBNAMESPACE,${LRSNAMESPACE}/g'   ${OPBASENAME} 
endef

export opr = $(value _opr)

opsetup: prsyaml crtmgrappl nscrt  bndappl opapply rbacap

#opclean: rbacdl opdelete bnddel crtmgrdel
opclean: rbacdl bnddel crtmgrdel opdelete

opapply:
	@$(call msg,"Applying operator yaml file")
	$(KUBECTL) apply -f `basename ${ORACLE_OPERATOR_YAML}`

opdelete:
	- $(KUBECTL) delete -f `basename ${ORACLE_OPERATOR_YAML}`
	
prsyaml:
#	@ eval "$$opr"	
	$(CP) ${ORACLE_OPERATOR_YAML} .  
	${CP} `basename ${ORACLE_OPERATOR_YAML}` `basename ${ORACLE_OPERATOR_YAML}`.ORG
	$(SED) -i  's/value: ""/value: $(OPRNAMESPACE),$(PDBNAMESPACE),$(LRSNAMESPACE)/g'   `basename ${ORACLE_OPERATOR_YAML}` 

### Certmanager ###
crtmgrappl:
	@$(call msg,"Install cert-manager")
	$(KUBECTL) apply -f $(CERT_MANAGER)
	$(KUBECTL) wait --for jsonpath='{.webhooks[0].clientConfig.caBundle'} \
		validatingwebhookconfiguration cert-manager-webhook --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"Cert manager application completed")

crtmgrdel:
	@$(call msg,"Deleting cert manager $(CERT_MANAGER)")
	- $(KUBECTL) delete -f $(CERT_MANAGER)

### Namespaces creation ###
nscrt:
	@$(call msg,"Namespace scope: creating namespace")
	- $(KUBECTL) create namespace $(LRSNAMESPACE)
	- $(KUBECTL) create namespace $(PDBNAMESPACE)

### Namespace deletion  ###
nsdel:
	@$(call msg,"Deleting namespaces $(LRSNAMESPACE) $(PDBNAMESPACE)")
	$(KUBECTL) delete namespace $(LRSNAMESPACE)
	$(KUBECTL) delete namespace $(PDBNAMESPACE)

### Namespace Scoped Deployment ###
bndappl:
	@$(call msg," Namespace Scoped Deployment: Binding $(PDBNAMESPACE) $(LRSNAMESPACE)")
	- $(KUBECTL) apply -f $(PDBNAMESPACE)_binding.yaml
	- $(KUBECTL) apply -f $(LRSNAMESPACE)_binding.yaml

### Namespace Scope Remove ###
bnddel:
	@$(call msg," Namespace Scoped remove $(PDBNAMESPACE) $(LRSNAMESPACE)")
	- $(KUBECTL) delete -f $(PDBNAMESPACE)_binding.yaml
	- $(KUBECTL) delete -f $(LRSNAMESPACE)_binding.yaml

### Cluster Role Binfing ###
rbacap:
	@$(call msg," ClusterRole and ClusterRoleBinding for NodePort services")
	- $(KUBECTL) apply -f ${NODE_RBAC}

### Remove Cluster Role Binding ###
rbacdl:
	@$(call msg," Clean $(NODE_RBAC)")
	- $(KUBECTL) delete -f ${NODE_RBAC}


define _script00
cat <<EOF > authsection.yaml
  adminpdbUser:
    secret:
      secretName: "pdbusr"
      key: "e_pdbusr.txt"
  adminpdbPass:
    secret:
      secretName: "pdbpwd"
      key: "e_pdbpwd.txt"
  lrpdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "tls.key"
  lrpdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
  lrpdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "ca.crt"
  webServerUser:
    secret:
      secretName: "wbuser"
      key: "e_wbuser.txt"
  webServerPwd:
    secret:
      secretName: "wbpass"
      key: "e_wbpass.txt"
  cdbPrvKey:
    secret:
      secretName: "prvkey"
      key: "privateKey"
EOF


cat <<EOF > ${PDBNAMESPACE}_binding.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: oracle-database-operator-oracle-database-operator-manager-rolebinding1
  namespace: ${PDBNAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: oracle-database-operator-manager-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: oracle-database-operator-system
EOF

cat <<EOF > ${LRSNAMESPACE}_binding.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: oracle-database-operator-oracle-database-operator-manager-rolebinding2
  namespace: ${LRSNAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: oracle-database-operator-manager-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: oracle-database-operator-system
EOF

cat <<EOF > ${NODE_RBAC}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: oracle-database-operator-manager-role-node
rules:
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - list
  - watch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: oracle-database-operator-manager-role-node-cluster-role-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: oracle-database-operator-manager-role-node
subjects:
- kind: ServiceAccount
  name: default
  namespace: oracle-database-operator-system
---
EOF

endef
export script00 = $(value _script00)
secyaml:
	@ eval "$$script00"


#echo lrest pod creation
define _script01
cat <<EOF > ${LREST_POD}
apiVersion: database.oracle.com/${APIVERSION}
kind: LREST
metadata: 
  name: cdb-dev
  namespace: ${LRSNAMESPACE}
spec:
  cdbName: "DB12"
  lrestImage: ${LRESTIMG}
  lrestImagePullPolicy: "Always"
  dbTnsurl : ${TNSALIAS}
  replicas: 1
  deletePdbCascade: true
  autodiscover: ${AUTODISCOVER}
  namespaceAutoDiscover: ${PDBNAMESPACE}
  clusterIp: false
  loadBalancer: false
  trace_level_client : 16 
EOF

if [[ ${OPENSHIFT} == "true" ]];then 
cat <<EOF >> ${LREST_POD}
  serviceAccountName: ${SERVICENAM}
EOF
fi

cat <<EOF >> ${LREST_POD}
  cdbAdminUser:
    secret:
      secretName: "dbuser"
      key: "e_dbuser.txt"
  cdbAdminPwd:
    secret:
      secretName: "dbpass"
      key: "e_dbpass.txt"
  webServerUser:
    secret:
      secretName: "wbuser"
      key: "e_wbuser.txt"
  webServerPwd:
    secret:
      secretName: "wbpass"
      key: "e_wbpass.txt"
  cdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "tls.key"
  cdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
  cdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "ca.crt"
  cdbPubKey:
    secret:
      secretName: "pubkey"
      key: "publicKey"
  cdbPrvKey:
    secret:
      secretName: "prvkey"
      key: "privateKey"
EOF

endef
export script01 = $(value _script01)


define _script02

cat <<EOF >${LRPDBCRE1}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  imperativeLrpdbDeletion: ${ASSERTDELETION}
  fileNameConversions: "NONE"
  unlimitedStorage: false
  pdbconfigmap: "config-map-pdb"
  tdeImport: false
  totalSize: "2G"
  tempSize: "800M"
EOF

cat <<EOF > ${LRPDBCRE2}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbprd"
  imperativeLrpdbDeletion: ${ASSERTDELETION}
  fileNameConversions: "NONE"
  unlimitedStorage: false
  pdbconfigmap: "config-map-pdb"
  tdeImport: false
  totalSize: "2G"
  tempSize: "800M"
EOF

cat <<EOF >${LRPDBOPEN1}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  pdbState: "OPEN"
  modifyOption: "READ WRITE"
EOF

cat <<EOF >${LRPDBOPEN2}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbprd"
  pdbState: "OPEN"
  modifyOption: "READ WRITE"
EOF

cat <<EOF >${LRPDBOPEN3}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb3
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "new_clone"
  pdbState: "OPEN"
  modifyOption: "READ WRITE"
EOF

cat <<EOF >${LRPDBCLOSE1}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  pdbState: "CLOSE"
  modifyOption: "IMMEDIATE"
EOF

cat <<EOF >${LRPDBCLOSE2}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbprd"
  pdbState: "CLOSE"
  modifyOption: "IMMEDIATE"
EOF

cat <<EOF >${LRPDBCLOSE3}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb3
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "new_clone"
  pdbState: "CLOSE"
  modifyOption: "IMMEDIATE"
EOF

cat <<EOF  > ${LRPDBCLONE1}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb3
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "new_clone"
  srcPdbName: "pdbdev"
  fileNameConversions: "NONE"
  totalSize: "UNLIMITED"
  tempSize: "UNLIMITED"
  pdbconfigmap: "config-map-pdb"
  imperativeLrpdbDeletion: true
EOF

cat <<EOF  > ${LRPDBCLONE2}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb4
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "new_clone2"
  srcPdbName: "pdbprd"
  fileNameConversions: "NONE"
  totalSize: "UNLIMITED"
  tempSize: "UNLIMITED"
  pdbconfigmap: "config-map-pdb"
  imperativeLrpdbDeletion: ${ASSERTDELETION}
EOF

cat <<EOF > ${LRPDBDELETE1}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  pdbName: "pdbdev"
  pdbState: "DELETE"
  dropAction: "INCLUDING"
  imperativeLrpdbDeletion: ${ASSERTDELETION}
EOF

cat <<EOF > ${LRPDBDELETE2}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  pdbName: "pdbprd"
  pdbState: "DELETE"
  dropAction: "INCLUDING"
  imperativeLrpdbDeletion: ${ASSERTDELETION}
EOF

cat <<EOF > ${LRPDBDELETE3}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb3
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  pdbName: "new_clone"
  pdbState: "DELETE"
  dropAction: "INCLUDING"
  imperativeLrpdbDeletion: ${ASSERTDELETION}
EOF

cat <<EOF > ${LRPDBUNPLUG1}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  pdbState: "UNPLUG"
  xmlFileName: "/var/tmp/pdb.$$.xml"
EOF

cat <<EOF >${LRPDBPLUG1}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  pdbState: "PLUG"
  xmlFileName: "/var/tmp/pdb.$$.xml"
  fileNameConversions: "NONE"
  sourceFileNameConversions: "NONE"
  copyAction: "MOVE"
  totalSize: "1G"
  tempSize: "100M"
  imperativeLrpdbDeletion: true
  pdbconfigmap: "config-map-pdb"
EOF

cat <<EOF >${LRPDBMAP1}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  imperativeLrpdbDeletion: true
  fileNameConversions: "NONE"
  totalSize: "1G"
  tempSize: "100M"
EOF

cat <<EOF >${LRPDBMAP2}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbprd"
  imperativeLrpdbDeletion: true
  fileNameConversions: "NONE"
  totalSize: "1G"
  tempSize: "100M"
EOF


cat <<EOF >${LRPDBMAP3}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb3
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "new_clone"
  imperativeLrpdbDeletion: true
  fileNameConversions: "NONE"
  totalSize: "1G"
  tempSize: "100M"
EOF

cat <<EOF >${CONFIG_MAP}
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-map-pdb
  namespace: ${PDBNAMESPACE}
data:
  rdbmsparameters.txt: |
     session_cached_cursors;100;spfile
     open_cursors;100;spfile
     db_file_multiblock_read_count;16;spfile
     test_invalid_parameter;16;spfile
EOF


cat <<EOF > ${ALTERSYSTEMYAML}
apiVersion: database.oracle.com/${APIVERSION}
kind: LRPDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${LRSNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  alterSystemParameter : "cpu_count"
  alterSystemValue : "3"
  parameterScope : "memory"
EOF


cat <<EOF > ${CONFIG_MAP_SQL}
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${PLSQLMAP}
  namespace: ${PDBNAMESPACE}
data:
  plblock1.sql: |
        create user k8stestuser identified by tkstestuser
  plblock2.sql: |
        grant dba to k8stestuser
  plblock3.sql: |
        create table k8stestuser.rndnum (c1 number ,c2 number)
  plblock4.sql: |
        alter session set events '942 trace name errorstack level  3'
  plblock5.sql: |
        create or replace  procedure k8stestuser.gennum 
        as 
        begin 
        for cnt in 1..100 
        loop 
           insert into k8stestuser.rndnum values (dbms_random.value(1,100), 
           dbms_random.value(1,100));
           end loop;
           commit;
        end;
  plblock6.sql: |
        begin 
        k8stestuser.gennum;
        end;
EOF


## Auth information
for _file in ${LRPDBCRE1} ${LRPDBCRE2} ${LRPDBOPEN1} ${LRPDBOPEN2} ${LRPDBOPEN3} ${LRPDBCLOSE1} ${LRPDBCLOSE2} ${LRPDBCLOSE3} ${LRPDBCLONE1} ${LRPDBCLONE2} ${LRPDBDELETE1} ${LRPDBDELETE2}  ${LRPDBDELETE3} ${LRPDBUNPLUG1} ${LRPDBPLUG1} ${LRPDBMAP1} ${LRPDBMAP2} ${LRPDBMAP3} ${ALTERSYSTEMYAML} 
do 
ls -ltr ${_file}
     cat authsection.yaml >> ${_file}
done
rm authsection.yaml
endef

define _script03
cat <<EOF >${SECURITYCTX}
#
# Copyright (c) 2024, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.
#
---

# Create a Security Context Contraint
kind: SecurityContextConstraints
apiVersion: security.openshift.io/v1
metadata:
  name: lrest-scc
allowPrivilegedContainer: false
allowedCapabilities:
  - SYS_NICE
runAsUser:
  type: MustRunAs
  uid: 54321
seLinuxContext:
  type: RunAsAny
fsGroup:
  type: MustRunAs
  ranges:
  - min: 54321
    max: 54321
supplementalGroups:
   type: MustRunAs
   ranges:
   - min: 54321
     max: 54321
---

# Create a Security Context Contraint
kind: SecurityContextConstraints
apiVersion: security.openshift.io/v1
metadata:
  name: lrest-root-user-scc
allowPrivilegedContainer: false
allowedCapabilities:
  - SYS_NICE
runAsUser:
  type: MustRunAsRange
  uidRangeMin: 0
  uidRangeMax: 54321
seLinuxContext:
  type: RunAsAny
fsGroup:
  type: MustRunAs
  ranges:
  - min: 0
    max: 54325
supplementalGroups:
  type: MustRunAs
  ranges:
  - min: 0
    max: 54321
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: lrest-sa
  namespace: ${LRSNAMESPACE}
---

kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: use-lrest-scc
  namespace: ${LRSNAMESPACE}
rules:
  - apiGroups:
    - security.openshift.io
    verbs:
    - use
    resources:
    - securitycontextconstraints
    resourceNames:
    - lrest-scc
    - lrest-root-user-scc
---

kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: use-lrest-scc
  namespace: ${LRSNAMESPACE}
subjects:
  - kind: ServiceAccount
    name: lrest-sa
    namespace: ${LRSNAMESPACE}
roleRef:
  kind: Role
  name: use-lrest-scc
  apiGroup: rbac.authorization.k8s.io
EOF

endef

export script02 = $(value _script02)
export script03 = $(value _script03)

genyaml: secyaml
	@ eval "$$script01"
	@ eval "$$script02"
	@ eval "$$script03"

cleanyaml:
	- $(RM) $(LRPDBMAP3) $(LRPDBMAP2) $(LRPDBMAP1) $(LRPDBPLUG1) $(LRPDBUNPLUG1) $(LRPDBDELETE2) $(LRPDBDELETE1) $(LRPDBDELETE3) $(LRPDBCLONE2) $(LRPDBCLONE1) $(LRPDBCLOSE3) $(LRPDBCLOSE2) $(LRPDBCLOSE1) $(LRPDBOPEN3) $(LRPDBOPEN2) $(LRPDBOPEN1) $(LRPDBCRE2) $(LRPDBCRE1) $(LREST_POD) ${ALTERSYSTEMYAML}
	- $(RM) ${CONFIG_MAP} ${PDBNAMESPACE}_binding.yaml ${LRSNAMESPACE}_binding.yaml




#################
### PACKAGING ###
#################

pkg:
	- $(RM) -rf /tmp/pkgtestplan
	$(MKDIR) /tmp/pkgtestplan
	$(CP) -R * /tmp/pkgtestplan
	$(CP) ../../../oracle-database-operator.yaml /tmp/pkgtestplan/
	$(TAR) -C /tmp -cvf ~/pkgtestplan_$(DATE).tar pkgtestplan

################
###   diag   ###
################

login:
	$(KUBECTL) exec   `$(KUBECTL) get pods -n $(LRSNAMESPACE)|grep rest|cut -d ' ' -f 1` -n $(LRSNAMESPACE) -it -- /bin/bash


reloadop:
	echo "RESTARTING OPERATOR"
	$(eval OP1 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1 ))
	$(eval OP2 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1|cut  -d ' ' -f 1 ))
	$(eval OP3 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1 ))
	$(KUBECTL) get pod $(OP1) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP2) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP3) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -


dumpoperator:
	@$(eval TMPSP := $(shell date "+%y%m%d%H%M%S" ))
	@$(eval DIAGFILE := ./opdmp.$(TMPSP))
	@>$(DIAGFILE)
	@echo "OPERATOR DUMP"  >> $(DIAGFILE)
	@echo "~~~~~~~~~~~~~"  >> $(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1 | cut -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)

dumplrest:
	@$(eval TMPSP := $(shell date "+%y%m%d%H%M%S" ))
	@$(eval DIAGFILE := ./lrestdump.$(TMPSP))
	@>$(DIAGFILE)
	$(KUBECTL) logs  `$(KUBECTL) get pods -o custom-columns=:metadata.name -n $(LRSNAMESPACE) --no-headers ` -n $(LRSNAMESPACE) |strings > $(DIAGFILE)

loglrest:
	$(KUBECTL) logs -f `$(KUBECTL) get pods -o custom-columns=:metadata.name -n $(LRSNAMESPACE) --no-headers ` -n $(LRSNAMESPACE)


listimage:
	 $(KUBECTL) get pods --all-namespaces -o jsonpath="{.items[*].spec['initContainers', 'containers'][*].image}" | tr -s '[[:space:]]' '\n' | sort | uniq -c

getvaliduid:
	$(KUBECTL) get namespace  $(LRSNAMESPACE) -ojson | jq '.metadata.annotations | with_entries(select(.key | contains("sa.scc")))'

mgrrestart:
	$(KUBECTL) rollout restart -n $(OPRNAMESPACE) deployment oracle-database-operator-controller-manager


#######################################################
####                 TEST SECTION                  ####
#######################################################
CREATEMSG="create:[op completed]"
OPENMSG="open:[op. completed]"
CLOSEMSG="close:[op. completed]"
CLONEMSG="clone:[op. completed]"
PLUGMSG="plug:[op. completed]"

cleanlrest:
	@$(call msg,"lrest deletion")
	- $(KUBECTL) delete lrest cdb-dev -n $(LRSNAMESPACE)
	- $(KUBECTL) delete replicaset.apps/cdb-dev-lrest-rs -n $(LRSNAMESPACE)
	$(KUBECTL) wait --for=delete lrest cdb-dev -n $(LRSNAMESPACE) --timeout=10m
	$(KUBECTL) wait --for=delete pod/* -n $(LRSNAMESPACE) --timeout=10m

toplrest:
	$(KUBECTL) top pod  `$(KUBECTL) get pods -o custom-columns=:metadata.name -n $(LRSNAMESPACE) --no-headers ` -n $(LRSNAMESPACE)


run00: cleanlrest
	@$(call msg,"lrest pod creation")
	$(KUBECTL) apply -f $(LREST_POD) 
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" lrest cdb-dev -n $(LRSNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"lrest pod completed")
	$(KUBECTL) get lrest -n  $(LRSNAMESPACE)
	$(KUBECTL) get pod -n  $(LRSNAMESPACE)
	$(KUBECTL) logs  `$(KUBECTL) get pods -o custom-columns=:metadata.name -n $(LRSNAMESPACE) --no-headers ` -n $(LRSNAMESPACE)

run01.1:
	@$(call msg,"lrpdb pdb1  creation")
	$(KUBECTL) apply -f $(LRPDBCRE1)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(CREATEMSG) lrpdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "lrpdb pdb1 creation completed")
	$(KUBECTL) get lrpdb pdb1 -n $(PDBNAMESPACE) 

run01.2:
	@$(call msg, "lrpdb pdb2  creation")
	$(KUBECTL) apply -f $(LRPDBCRE2)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(CREATEMSG) lrpdb pdb2 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "lrpdb pdb2  creation completed")
	$(KUBECTL) get lrpdb pdb2 -n $(PDBNAMESPACE) 

open:
	@$(call msg, "lrpdb $(LRPDBNAME)  open")
	$(KUBECTL) patch lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) -p \
		'{"spec":{"pdbState":"OPEN","modifyOption":"READ WRITE","modifyOption2":"NONE"}}' --type=merge
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(OPENMSG) lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "lrpdb $(LRPDBNAME)  completed")
	$(KUBECTL) get lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) 

openrestricted:
	@$(call msg, "lrpdb $(LRPDBNAME)  open")
	$(KUBECTL) patch lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) -p \
		'{"spec":{"pdbState":"OPEN","modifyOption":"READ WRITE","modifyOption2":"RESTRICTED"}}' --type=merge
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(OPENMSG) lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "lrpdb $(LRPDBNAME)  completed")
	$(KUBECTL) get lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) 


close:
	@$(call msg, "lrpdb $(LRPDBNAME)  close")
	$(KUBECTL) patch lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) -p \
		'{"spec":{"pdbState":"CLOSE","modifyOption":"IMMEDIATE"}}' --type=merge
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(CLOSEMSG) lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "lrpdb $(LRPDBNAME)  completed")
	$(KUBECTL) get lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) 

altercpu:
	@$(call msg, "lrpdb $(LRPDBNAME)  alter cpu")
	$(KUBECTL) patch lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) -p \
		'{"spec":{"alterSystemParameter":"cpu_count","alterSystemValue":"3","parameterScope":"memory"}}' --type=merge

openpdb1:
	$(MAKE) -f $(MAKEFILE) open LRPDBNAME=pdb1 

openpdb2:
	$(MAKE) -f $(MAKEFILE) open LRPDBNAME=pdb2 

openpdb3:
	$(MAKE) -f $(MAKEFILE) open LRPDBNAME=pdb3

openpdb1rs:
	$(MAKE) -f $(MAKEFILE) openrestricted LRPDBNAME=pdb1

openpdb2rs:
	$(MAKE) -f $(MAKEFILE) openrestricted LRPDBNAME=pdb2

closepdb1:
	$(MAKE) -f $(MAKEFILE) close LRPDBNAME=pdb1 

closepdb2:
	$(MAKE) -f $(MAKEFILE) close LRPDBNAME=pdb2 

closepdb3:
	$(MAKE) -f $(MAKEFILE) close LRPDBNAME=pdb3 


run02.1:
	@$(call msg, "lrpdb pdb1  open")
	$(KUBECTL) apply -f $(LRPDBOPEN1)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(OPENMSG) lrpdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "lrpdb pdb1 open completed")
	$(KUBECTL) get lrpdb pdb1 -n $(PDBNAMESPACE) 


run02.2:
	@$(call msg,"lrpdb pdb2  open")
	$(KUBECTL) apply -f $(LRPDBOPEN2)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(OPENMSG) lrpdb pdb2 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"lrpdb pdb2 open completed")
	$(KUBECTL) get lrpdb pdb2 -n $(PDBNAMESPACE) 


run03.1:
	@$(call msg,"clone pdb1-->pdb3")
	$(KUBECTL) apply -f $(LRPDBCLONE1)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(CLONEMSG) lrpdb pdb3 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"clone pdb1-->pdb3 completed")
	$(KUBECTL) get lrpdb pdb3 -n $(PDBNAMESPACE) 


run03.2:
	@$(call msg,"clone pdb2-->pdb4")
	$(KUBECTL) apply -f $(LRPDBCLONE2)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(CLONEMSG) lrpdb pdb4 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"clone pdb2-->pdb4 completed")
	$(KUBECTL) get lrpdb pdb3 -n $(PDBNAMESPACE) 


run04.1:
	@$(call msg,"lrpdb pdb1  close")
	$(KUBECTL) apply -f $(LRPDBCLOSE1)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(CLOSEMSG) lrpdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "lrpdb pdb1 close completed")
	$(KUBECTL) get lrpdb pdb1 -n $(PDBNAMESPACE)

run04.2:
	@$(call msg,"lrpdb pdb2  close")
	$(KUBECTL) apply -f $(LRPDBCLOSE2)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(CLOSEMSG) lrpdb pdb2 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"lrpdb pdb2 close completed")
	$(KUBECTL) get lrpdb pdb2 -n $(PDBNAMESPACE)

run05.1:
	@$(call msg,"lrpdb pdb1  unplug")
	$(KUBECTL) apply -f $(LRPDBUNPLUG1)
	$(KUBECTL) wait --for=delete lrpdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"lrpdb pdb1 unplug completed")

run06.1:
	@$(call msg, "lrpdb pdb1  plug")
	$(KUBECTL) apply -f $(LRPDBPLUG1)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(PLUGMSG) lrpdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "lrpdb pdb1 plug completed")
	$(KUBECTL) get lrpdb pdb1 -n $(PDBNAMESPACE)

run07.1:
	@$(call msg,"lrpdb pdb1 delete ")
	- $(KUBECTL) apply -f $(LRPDBCLOSE1)
	$(KUBECTL) wait --for jsonpath='{.status.msg'}=$(CLOSEMSG) lrpdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	$(KUBECTL) apply -f $(LRPDBDELETE1)
	$(KUBECTL) wait --for=delete lrpdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"lrpdb pdb1 delete")
	$(KUBECTL) get lrpdb -n $(PDBNAMESPACE)

run99.1:
	$(KUBECTL) delete lrest cdb-dev -n $(LRSNAMESPACE)
	$(KUBECTL) wait --for=delete lrest cdb-dev -n $(LRSNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	$(KUBECTL) get lrest -n $(LRSNAMESPACE)
	$(KUBECTL) get lrpdb -n $(PDBNAMESPACE)
	

tklifecycle: runall01

tkonepdbcre: runall02

tkapplyinit:
	@$(call msg," init config map creation")
	-- $(KUBECTL) delete -f $(CONFIG_MAP) -n $(PDBNAMESPACE)
	$(KUBECTL) apply -f $(CONFIG_MAP) -n $(PDBNAMESPACE)

tkplsqlexec: 
	@$(call msg,"==>PLSQL TESTING<==")
	$(MAKE) -f $(MAKEFILE) run00 run01.1 run02.1
	@$(call msg,"delete config map ")
	-- $(KUBECTL) delete configmap $(PLSQLMAP) -n $(PDBNAMESPACE)
	@$(call msg,"recreating config sql map")
	$(KUBECTL) apply -f $(CONFIG_MAP_SQL)
	$(KUBECTL) describe configmap $(PLSQLMAP) -n $(PDBNAMESPACE)
	@$(call msg,"applying config sql map")
	$(KUBECTL) patch lrpdb pdb1 -n $(PDBNAMESPACE) -p \
	       '{"spec":{"codeconfigmap":"$(PLSQLMAP)"}}' --type=merge
	sleep 10 
	$(KUBECTL) get events --sort-by='.lastTimestamp' -n $(PDBNAMESPACE)
	$(KUBECTL) get events -n $(PDBNAMESPACE) --sort-by='.lastTimestamp' --field-selector involvedObject.name=pdb1|grep APPLYSQL
	$(KUBECTL) get events -n $(PDBNAMESPACE) --sort-by='.lastTimestamp' --field-selector involvedObject.name=pdb1|grep APPLYSQL


tkplsqlexec01:
	@$(call msg,"delete config map ")
	-- $(KUBECTL) delete configmap $(PLSQLMAP) -n $(PDBNAMESPACE)
	@$(call msg,"recreating config sql map")
	$(KUBECTL) apply -f $(CONFIG_MAP_SQL)
	$(KUBECTL) describe configmap $(PLSQLMAP) -n $(PDBNAMESPACE)
	@$(call msg,"applying config sql map")
	$(KUBECTL) patch lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) -p \
	                '{"spec":{"codeconfigmap":"$(PLSQLMAP)"}}' --type=merge
	sleep 10
	$(KUBECTL) get events --sort-by='.lastTimestamp' -n $(PDBNAMESPACE)
	$(KUBECTL) get events -n $(PDBNAMESPACE) --sort-by='.lastTimestamp' --field-selector involvedObject.name=$(LRPDBNAME)|grep APPLYSQL



tkaudosicov:
	@$(call msg,"==>AUTODISCOVER TESTING<==")
	$(MAKE) -f $(MAKEFILE) genyaml ASSERTDELETION=false
	$(MAKE) -f $(MAKEFILE) run00 run01.1 run02.1
	@$(call msg,"deleting pdb1 (imperativedeletion=false)")
	$(KUBECTL) delete lrpdb pdb1 -n $(PDBNAMESPACE)
	$(KUBECTL) wait --for=delete lrpdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	 @$(call msg,"checking lrpdb pdb1 recreation")
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" \
		lrpdb atd-pdbdev -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	$(MAKE) -f $(MAKEFILE) genyaml ASSERTDELETION=true

# Proceudure to recreate restserver
tklrestew:
	@$(call msg,"Recrete resetserver")
	@$(call msg,"disable autodiscover and delete cascade")
	$(KUBECTL) patch lrest cdb-dev -n $(LRSNAMESPACE) -p '{"spec":{"autodiscover":false,"deletePdbCascade":false}}'  --type=merge
	$(KUBECTL) wait --for jsonpath='{.spec.autodiscover'}=false lrest cdb-dev -n $(LRSNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	$(KUBECTL) wait --for jsonpath='{.spec.deletePdbCascade'}=false lrest cdb-dev -n $(LRSNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	sleep 10
	$(MAKE) -f $(MAKEFILE) run00

# Turn ON/OFF autodiscovery 
#          ON:    make tkautd AUTOD=true
#          OFF:   make tkautd AUTOD=false
tkautd:
	@$(call msg,"autodiscover $(AUTOD)")
	$(KUBECTL) patch lrest cdb-dev -n $(LRSNAMESPACE) -p '{"spec":{"autodiscover":$(AUTOD)}}'  --type=merge
	$(KUBECTL) wait --for jsonpath='{.spec.autodiscover'}=$(AUTOD) lrest cdb-dev -n $(LRSNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)

# Turn ON/OFF lrest pdb delete cascade
#          ON:    make tkdelcs DELETECS=true
#          OFF:   make tkdelcs DELETECS=false
tkdelcs:
	@$(call msg,"delete pdb cascade $(DELETECS)")
	$(KUBECTL) patch lrest cdb-dev -n $(LRSNAMESPACE) -p '{"spec":{"deletePdbCascade":$(DELETECS)}}'  --type=merge
	$(KUBECTL) wait --for jsonpath='{.spec.deletePdbCascade'}=$(DELETECS) lrest cdb-dev -n $(LRSNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)

# Turn ON/OFF lrest pdb imperativeLrpdb deletion (delete CRD & PDB)
#          ON:    make tkdelcrd DELETECRD=true  LRPDBNAME=<crdname>
#          OFF:   make tkdelcrd DELETECRD=false LRPDNNAME=<crdname>
tkdelcrd:
	@$(call msg,"imperativeLrpdbDeletion $(DELETECRD)")
	$(KUBECTL) patch lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) \
                -p '{"spec":{"imperativeLrpdbDeletion":$(DELETECRD)}}' --type=merge
	$(KUBECTL) wait --for jsonpath='{.spec.imperativeLrpdbDeletion'}=$(DELETECRD) lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE)  --timeout=$(TEST_EXEC_TIMEOUT)

checkpdbsize:
	$(KUBECTL) get lrpdb -n $(PDBNAMESPACE)  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.pdbName}{"\t"}{.spec.totalSize}{"\t"}{.status.totalSize}{"\n"}{end}'

checkimpdel:
	$(KUBECTL) get lrpdb -n $(PDBNAMESPACE) \
	       	-o jsonpath='{range .items[*]}{.metadata.name}{" "}{.spec.pdbName}{" "}{.status.openMode}{" "}{.spec.imperativeLrpdbDeletion}{" "}{"\t\t"}{"\n"}{end}'| sed 's/READ WRITE/READ_WRITE/g' |awk ' BEGIN { printf( "%-20s %-10s %-10s %10s\n","CRD","PDB NAME","OPEN MODE","IMPERATIVELRPDBDELETION"); \
		 printf( "%-20s %-10s %-10s %-23s\n","--------------------","----------","----------","-----------------------");\
		 } { printf("%-20s %-10s %-10s %-23s\n",$$1,$$2,$$3,$$4) }'

reset:
	@$(call msg,"Reset state $(LRPDBNAME):$(RESETVALUE)")
	$(KUBECTL) patch lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE) \
		-p '{"spec":{"pdbState":"RESET","reststate":$(RESETVALUE)}}'  \
		--type=merge
	$(KUBECTL) get  lrpdb $(LRPDBNAME) -n $(PDBNAMESPACE)
 



# tkapplyinit - config map creation
# run00       - lrest pod creation 
# run01.1     - pdb1 creation 
# run01.2     - pdb2 creation
# run02.1     - pdb1 open               - declarative  
# run02.2     - pdb2 open               - declarative
# run03.1     - pdb1 clone              - declarative
# run04.1     - pdb1 close              - declarative
# run04.2     - pdb2 close              - declatative  
# run05.1     - pdb1 unplug             - declarative
# run06.1     - pdb1 plug               - declarative
# openpdb1    - pdb1 open               - imperative 
# openpdb2    - pdb2 open               - imperative 
# closepdb1   - pdb1 close              - imperative
# closepdb2   - pdb2 close              - imperative 
# openpdb1rs  - pdb1 open restrict      - imperative  
# openpdb2rs  - pdb2 open restrict      - imperative  
#

runall01: genyaml tkapplyinit run00 run01.1 run01.2 run02.1 run02.2 \
	  run03.1 run03.2 run04.1 run04.2 run05.1 run06.1 run07.1 
runall02: genyaml tkapplyinit run00 run01.1 openpdb1
runall03: genyaml tkapplyinit run00 run01.1 run01.2 run02.1 run02.2 \
       	run04.1 run04.2 openpdb1 openpdb2 closepdb1 closepdb2 openpdb1rs
	$(MAKE) -f $(MAKEFILE) close LRPDBNAME=pdb1
	$(MAKE) -f $(MAKEFILE) open  LRPDBNAME=pdb1
	$(MAKE) -f $(MAKEFILE) close LRPDBNAME=pdb1
	$(MAKE) -f $(MAKEFILE) run05.1 run06.1
runall04: genyaml tkapplyinit run00 run01.1 run01.2 run02.1 run02.2 run04.1 \
       	run04.2 openpdb1 \
          openpdb2 closepdb1 closepdb2 run05.1 run06.1 closepdb1 
runall05: genyaml tkapplyinit run00 run01.1 run02.1 run03.1
## Delete cloned pdb
## setup:create/open/clone
runall06: run01.1 run02.1 run03.1
	$(KUBECTL) apply -f $(LRPDBDELETE3)
	$(KUBECTL) apply -f $(LRPDBDELETE1)








