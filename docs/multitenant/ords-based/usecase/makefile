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
#         1) make operator
#            it configures the operator yaml files with the 
#            watch namelist required by the multitenant controllers
#      
#         2) make genyaml 
#            It automatically creates all the yaml files based on the 
#            information available in the parameters file 
#
#         3) make secrets
#            It configure the required secrets necessary to operate
#            with pdbs multitenant controllers
#
#            
#            LIST OF GENERAED YAML FILE
#
#               -----------------------------       ----------------------------------
# 		oracle-database-operator.yaml     : oracle database operator
# 		cdbnamespace_binding.yaml         : role binding for cdbnamespace
# 		pdbnamespace_binding.yaml         : role binding for pdbnamespace
# 		create_cdb_secret.yaml            : create secrets for ords server pod
# 		create_pdb_secret.yaml            : create secrets for pluggable database
# 		create_ords_pod.yaml             : create rest server pod
# 		create_pdb1_resource.yaml       : create first pluggable database
# 		create_pdb2_resource.yaml       : create second pluggable database
# 		open_pdb1_resource.yaml         : open first pluggable database
# 		open_pdb2_resource.yaml         : open second pluggable database
# 		close_pdb1_resource.yaml        : close first pluggable database
# 		close_pdb2_resource.yaml        : close second pluggable database
# 		clone_pdb_resource.yaml         : clone thrid pluggable database
# 		clone_pdb2_resource.yaml        : clone 4th pluggable database
# 		delete_pdb1_resource.yaml       : delete first pluggable database
# 		delete_pdb2_resource.yaml       : delete sencond pluggable database
# 		delete_pdb3_resource.yaml       : delete thrid pluggable database
# 		unplug_pdb1_resource.yaml       : unplug first pluggable database
# 		plug_pdb1_resource.yaml         : plug first pluggable database
# 		map_pdb1_resource.yaml          : map the first pluggable database
# 		config_map.yam                  : pdb parameters array
#
DATE  := `date "+%y%m%d%H%M%S"`
######################
# PARAMETER SECTIONS #
######################
   
export PARAMETERS=parameters.txt
export TNSALIAS=$(shell cat $(PARAMETERS)  |grep -v ^\#|grep TNSALIAS|cut -d :  -f 2)
export ORDPWD=$(shell cat $(PARAMETERS)|grep -v ^\#|grep ORDPWD|cut -d :  -f 2)
export SYSPWD=$(shell cat $(PARAMETERS)|grep -v ^\#|grep SYSPWD|cut -d :  -f 2)
export WBUSER=$(shell cat $(PARAMETERS)|grep -v ^\#|grep WBUSER|cut -d :  -f 2)
export WBPASS=$(shell cat $(PARAMETERS)|grep -v ^\#|grep WBPASS|cut -d :  -f 2)
export PDBUSR=$(shell cat $(PARAMETERS)|grep -v ^\#|grep PDBUSR|cut -d :  -f 2)
export PDBPWD=$(shell cat $(PARAMETERS)|grep -v ^\#|grep PDBPWD|cut -d :  -f 2)
export CDBUSR=$(shell cat $(PARAMETERS)|grep -v ^\#|grep CDBUSR|cut -d :  -f 2)
export CDBPWD=$(shell cat $(PARAMETERS)|grep -v ^\#|grep CDBPWD|cut -d :  -f 2)
export PDBNAMESPACE=$(shell cat $(PARAMETERS)|grep -v ^\#|grep PDBNAMESPACE|cut -d :  -f 2)
export CDBNAMESPACE=$(shell cat $(PARAMETERS)|grep -v ^\#|grep CDBNAMESPACE|cut -d :  -f 2)
export ORDSIMG=$(shell cat $(PARAMETERS)|grep -v ^\#|grep ORDSIMG|cut -d : -f 2,3)
export COMPANY=$(shell cat $(PARAMETERS)|grep -v ^\#|grep COMPANY|cut -d : -f 2)
export APIVERSION=$(shell cat $(PARAMETERS)|grep -v ^\#|grep APIVERSION|cut -d : -f 2)
export OPRNAMESPACE=oracle-database-operator-system
export ORACLE_OPERATOR_YAML=../../../../oracle-database-operator.yaml
export TEST_EXEC_TIMEOUT=3m

REST_SERVER=ords
SKEY=tls.key
SCRT=tls.crt
CART=ca.crt
PRVKEY=ca.key
PUBKEY=public.pem
COMPANY=oracle

#################
### FILE LIST ###
#################

export ORDS_POD=create_ords_pod.yaml

export CDB_SECRETS=create_cdb_secrets.yaml
export PDB_SECRETS=create_pdb_secrets.yaml

export PDBCRE1=create_pdb1_resource.yaml
export PDBCRE2=create_pdb2_resource.yaml

export PDBCLOSE1=close_pdb1_resource.yaml
export PDBCLOSE2=close_pdb2_resource.yaml
export PDBCLOSE3=close_pdb3_resource.yaml

export PDBOPEN1=open_pdb1_resource.yaml
export PDBOPEN2=open_pdb2_resource.yaml
export PDBOPEN3=open_pdb3_resource.yaml

export PDBCLONE1=clone_pdb1_resource.yaml
export PDBCLONE2=clone_pdb2_resource.yaml

export PDBDELETE1=delete_pdb1_resource.yaml
export PDBDELETE2=delete_pdb2_resource.yaml
export PDBDELETE3=delete_pdb3_resource.yaml

export PDBUNPLUG1=unplug_pdb1_resource.yaml
export PDBPLUG1=plug_pdb1_resource.yaml

export PDBMAP1=map_pdb1_resource.yaml
export PDBMAP2=map_pdb2_resource.yaml
export PDBMAP3=map_pdb3_resource.yaml

export PDBMAP1=map_pdb1_resource.yaml
export PDBMAP2=map_pdb2_resource.yaml
export PDBMAP3=map_pdb3_resource.yaml


##BINARIES
export KUBECTL=/usr/bin/kubectl
OPENSSL=/usr/bin/openssl
ECHO=/usr/bin/echo
RM=/usr/bin/rm
CP=/usr/bin/cp
TAR=/usr/bin/tar
MKDIR=/usr/bin/mkdir

define msg
@printf "\033[31;7m%s\033[0m\r" "......................................]"
@printf "\033[31;7m[\xF0\x9F\x91\x89 %s\033[0m\n" $(1)
endef

check:
	$(call msg,"CHECK PARAMETERS")
	@printf "TNSALIAS...............:%.60s....\n" $(TNSALIAS)
	@printf "ORDPWD.................:%s\n" $(ORDPWD)
	@printf "SYSPWD.................:%s\n" $(SYSPWD)
	@printf "WBUSER.................:%s\n" $(WBUSER)
	@printf "WBPASS.................:%s\n" $(WBPASS)
	@printf "PDBUSR.................:%s\n" $(PDBUSR)
	@printf "PDBPWD.................:%s\n" $(PDBPWD)
	@printf "CDBUSR.................:%s\n" $(CDBUSR)
	@printf "CDBPWD.................:%s\n" $(CDBPWD)
	@printf "PDBNAMESPACE...........:%s\n" $(PDBNAMESPACE)
	@printf "CDBNAMESPACE...........:%s\n" $(CDBNAMESPACE)
	@printf "COMPANY................:%s\n" $(COMPANY)
	@printf "APIVERSION.............:%s\n" $(APIVERSION)


tlscrt:
	$(call msg,"TLS GENERATION")
	#$(OPENSSL) genrsa -out $(PRVKEY) 2048	
	$(OPENSSL)  genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > $(PRVKEY)
	$(OPENSSL) req -new -x509 -days 365 -key $(PRVKEY) \
    	       -subj "/C=CN/ST=GD/L=SZ/O=$(COMPANY), Inc./CN=$(COMPANY) Root CA" -out ca.crt
	$(OPENSSL) req -newkey rsa:2048 -nodes -keyout $(SKEY) -subj \
	       	"/C=CN/ST=GD/L=SZ/O=$(COMPANY), Inc./CN=cdb-dev-$(REST_SERVER).$(CDBNAMESPACE)" -out server.csr
	$(ECHO) "subjectAltName=DNS:cdb-dev-$(REST_SERVER).$(CDBNAMESPACE)" > extfile.txt
	$(OPENSSL) x509 -req -extfile extfile.txt -days 365 -in server.csr -CA ca.crt -CAkey $(PRVKEY) -CAcreateserial -out $(SCRT)
	$(OPENSSL) rsa -in $(PRVKEY) -outform PEM  -pubout -out $(PUBKEY)

#- $(KUBECTL) delete secret db-tls  -n $(CDBNAMESPACE) 2>/dev/null
#- $(KUBECTL) delete secret db-ca -n $(CDBNAMESPACE)   2>/dev/null
#- $(KUBECTL) delete secret db-tls  -n $(PDBNAMESPACE) 2>/dev/null
#- $(KUBECTL) delete secret db-ca -n $(PDBNAMESPACE)   2>/dev/null

tlssec:
	$(call msg,"GENERATE TLS SECRET")
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(CDBNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(CDBNAMESPACE)
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(PDBNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)"  -n $(PDBNAMESPACE)

secrets: delsec tlscrt tlssec 
	$(call msg,"GENERATE CDB SECRET")
	$(KUBECTL) apply -f $(CDB_SECRETS)
	$(call msg,"GENERATE PDB SECRET")
	$(KUBECTL) apply -f $(PDB_SECRETS)


delsec:
	$(call msg,"CLEAN OLD SECRETS")
	$(eval SECRETSP:=$(shell kubectl get secrets -n $(PDBNAMESPACE) -o custom-columns=":metadata.name" --no-headers) )	
	$(eval SECRETSL:=$(shell kubectl get secrets -n $(CDBNAMESPACE) -o custom-columns=":metadata.name" --no-headers) )	
	@[ "${SECRETSP}" ] && ( \
	       	printf "Deleteing secrets in namespace -n $(PDBNAMESPACE)\n") &&\
	        ($(KUBECTL) delete secret  $(SECRETSP) -n $(PDBNAMESPACE))\
	        || ( echo "No screts in namespace $(PDBNAMESPACE)")
	@[ "${SECRETSL}" ] && ( \
	       	printf "Deleteing secrets in namespace -n $(CDBNAMESPACE)\n") &&\
	        ($(KUBECTL) delete secret  $(SECRETSL) -n $(CDBNAMESPACE))\
	        || ( echo "No screts in namespace $(PDBNAMESPACE)")

		

### YAML FILE SECTION ###
define _opr
cp ${ORACLE_OPERATOR_YAML} .
export OPBASENAME=`basename ${ORACLE_OPERATOR_YAML}`
cp ${OPBASENAME} ${OPBASENAME}.ORIGNINAL
printf "\n\t\xF0\x9F\x91\x89 ${OPBASENAME}\n\n"
sed -i  's/value: ""/value: "${OPRNAMESPACE},${PDBNAMESPACE},${CDBNAMESPACE}"/g'   ${OPBASENAME} 
endef

export opr = $(value _opr)

operator:
	@ eval "$$opr"	


define _script00
cat <<EOF > authsection01.yaml
  sysAdminPwd:
    secret:
      secretName: "cdb-secret"
      key: "sysadmin_pwd"
  ordsPwd:
    secret:
      secretName: "cdb-secret"
      key: "ords_pwd"
  cdbAdminUser:
    secret:
      secretName: "cdb-secret"
      key: "cdbadmin_user"
  cdbAdminPwd:
    secret:
      secretName: "cdb-secret"
      key: "cdbadmin_pwd"
  webServerUser:
    secret:
      secretName: "cdb-secret"
      key: "webserver_user"
  webServerPwd:
    secret:
      secretName: "cdb-secret"
      key: "webserver_pwd"
  cdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "tls.key"
  cdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
EOF

cat<<EOF > authsection02.yaml
  adminName:
    secret:
      secretName: "pdb-secret"
      key: "sysadmin_user"
  adminPwd:
    secret:
      secretName: "pdb-secret"
      key: "sysadmin_pwd"
  pdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "tls.key"
  pdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
  pdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "ca.crt"
  webServerUser:
    secret:
      secretName: "pdb-secret"
      key: "webserver_user"
  webServerPwd:
    secret:
      secretName: "pdb-secret"
      key: "webserver_pwd"
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

cat <<EOF > ${CDBNAMESPACE}_binding.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: oracle-database-operator-oracle-database-operator-manager-rolebinding2
  namespace: ${CDBNAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: oracle-database-operator-manager-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: oracle-database-operator-system
EOF

endef
export script00 = $(value _script00)
secyaml:
	@ eval "$$script00"

define _script_secrets
ORDS=`echo ${ORDPWD}|base64`
SYSA=`echo ${SYSPWD}|base64`
CDBU=`echo ${CDBUSR}|base64`
CDBP=`echo ${CDBPWD}|base64`
WEBU=`echo ${WBUSER}|base64`
WEBP=`echo ${WBPASS}|base64`
PDBU=`echo ${PDBUSR}|base64`
PDBP=`echo ${PDBPWD}|base64`
cat <<EOF > ${CDB_SECRETS}
apiVersion: v1
kind: Secret
metadata:
  name: cdb-secret
  namespace: ${CDBNAMESPACE}
type: Opaque
data:
  ords_pwd: "${ORDS}"
  sysadmin_pwd: "${SYSA}"
  cdbadmin_user: "${CDBU}"
  cdbadmin_pwd: "${CDBP}" 
  webserver_user: "${WEBU}"
  webserver_pwd: "${WEBP}"
EOF

cat <<EOF > ${PDB_SECRETS}
apiVersion: v1
kind: Secret
metadata:
  name: pdb-secret
  namespace: ${PDBNAMESPACE}
type: Opaque
data:
  sysadmin_user: "${PDBU}"
  sysadmin_pwd: "${PDBP}"
  webserver_user: "${WEBU}"
  webserver_pwd: "${WEBP}"
EOF

endef


export script_secrets=$(value _script_secrets)

dbsecr:
	@ eval "$$script_secrets"

#echo ords pod creation
define _script01
cat <<EOF > ${ORDS_POD}
apiVersion: database.oracle.com/${APIVERSION}
kind: CDB
metadata: 
  name: cdb-dev
  namespace: cdbnamespace
spec:
  cdbName: "DB12"
  ordsImage: ${ORDSIMG}
  ordsImagePullPolicy: "Always"
  dbTnsurl : ${TNSALIAS}
  replicas: 1
  deletePdbCascade: true
EOF

cat  authsection01.yaml >> ${ORDS_POD}

endef
export script01 = $(value _script01)


define _script02

cat <<EOF >${PDBCRE1}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  assertivePdbDeletion: true
  fileNameConversions: "NONE"
  unlimitedStorage: false
  tdeImport: false
  totalSize: "2G"
  tempSize: "800M"
  action: "Create"
EOF

cat <<EOF > ${PDBCRE2}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbprd"
  assertivePdbDeletion: true
  fileNameConversions: "NONE"
  unlimitedStorage: false
  tdeImport: false
  totalSize: "2G"
  tempSize: "800M"
  action: "Create"
EOF

cat <<EOF >${PDBOPEN1}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  action: "Modify"
  pdbState: "OPEN"
  modifyOption: "READ WRITE"
EOF

cat <<EOF >${PDBOPEN2}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbprd"
  action: "Modify"
  pdbState: "OPEN"
  modifyOption: "READ WRITE"
EOF

cat <<EOF >${PDBOPEN3}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb3
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "new_clone"
  action: "Modify"
  pdbState: "OPEN"
  modifyOption: "READ WRITE"
EOF

cat <<EOF >${PDBCLOSE1}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  pdbState: "CLOSE"
  modifyOption: "IMMEDIATE"
  action: "Modify"
EOF

cat <<EOF >${PDBCLOSE2}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbprd"
  pdbState: "CLOSE"
  modifyOption: "IMMEDIATE"
  action: "Modify"
EOF

cat <<EOF >${PDBCLOSE3}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb3
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: ""new_clone"
  pdbState: "CLOSE"
  modifyOption: "IMMEDIATE"
  action: "Modify"
EOF

cat <<EOF  > ${PDBCLONE1}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb3
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "new_clone"
  srcPdbName: "pdbdev"
  fileNameConversions: "NONE"
  totalSize: "UNLIMITED"
  tempSize: "UNLIMITED"
  assertivePdbDeletion: true
  action: "Clone"
EOF

cat <<EOF  > ${PDBCLONE2}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb4
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "new_clone2"
  srcPdbName: "pdbprd"
  fileNameConversions: "NONE"
  totalSize: "UNLIMITED"
  tempSize: "UNLIMITED"
  assertivePdbDeletion: true
  action: "Clone"
EOF


cat <<EOF > ${PDBDELETE1}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  pdbName: "pdbdev"
  action: "Delete"
  dropAction: "INCLUDING"
EOF

cat <<EOF > ${PDBDELETE2}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  pdbName: "pdbprd"
  action: "Delete"
  dropAction: "INCLUDING"
EOF

cat <<EOF > ${PDBUNPLUG1}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  xmlFileName: "/tmp/pdb.xml"
  action: "Unplug"
EOF

cat <<EOF >${PDBPLUG1}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  xmlFileName: "/tmp/pdb.xml"
  action: "plug"
  fileNameConversions: "NONE"
  sourceFileNameConversions: "NONE"
  copyAction: "MOVE"
  totalSize: "1G"
  tempSize: "100M"
  assertivePdbDeletion: true
  action: "Plug"
EOF

cat <<EOF >${PDBMAP1}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb1
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbdev"
  assertivePdbDeletion: true
  fileNameConversions: "NONE"
  totalSize: "1G"
  tempSize: "100M"
  action: "Map"
EOF

cat <<EOF >${PDBMAP2}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb2
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "pdbprd"
  assertivePdbDeletion: true
  fileNameConversions: "NONE"
  totalSize: "1G"
  tempSize: "100M"
  action: "Map"
EOF


cat <<EOF >${PDBMAP3}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb3
  namespace: ${PDBNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${CDBNAMESPACE}"
  cdbName: "DB12"
  pdbName: "new_clone"
  assertivePdbDeletion: true
  fileNameConversions: "NONE"
  totalSize: "1G"
  tempSize: "100M"
  action: "Map"
EOF


## Auth information
for _file in ${PDBCRE1} ${PDBCRE2} ${PDBOPEN1} ${PDBOPEN2} ${PDBOPEN3} ${PDBCLOSE1} ${PDBCLOSE2} ${PDBCLOSE3} ${PDBCLONE1} ${PDBCLONE2} ${PDBDELETE1} ${PDBDELETE2} ${PDBUNPLUG1} ${PDBPLUG1} ${PDBMAP1} ${PDBMAP2} ${PDBMAP3} 
do 
ls -ltr ${_file}
     cat authsection02.yaml >> ${_file}
done
rm authsection02.yaml
rm authsection01.yaml
endef

export script02 = $(value _script02)

genyaml:dbsecr  secyaml
	@ eval "$$script01"
	@ eval "$$script02"

cleanyaml:
	- $(RM) $(PDBMAP3) $(PDBMAP2) $(PDBMAP1) $(PDBPLUG1) $(PDBUNPLUG1) $(PDBDELETE2) $(PDBDELETE1) $(PDBCLONE2) $(PDBCLONE1) $(PDBCLOSE3) $(PDBCLOSE2) $(PDBCLOSE1) $(PDBOPEN3) $(PDBOPEN2) $(PDBOPEN1) $(PDBCRE2) $(PDBCRE1) $(ORDS_POD) $(CDB_SECRETS) $(PDB_SECRETS) 
	- $(RM) ${PDBNAMESPACE}_binding.yaml ${CDBNAMESPACE}_binding.yaml


cleancrt:
	- $(RM) $(SKEY) $(SCRT) $(CART) $(PRVKEY) $(PUBKEY) server.csr extfile.txt ca.srl 


#################
### PACKAGING ###
#################

pkg:
	- $(RM) -rf /tmp/pkgtestplan
	$(MKDIR) /tmp/pkgtestplan
	$(CP) -R * /tmp/pkgtestplan
	$(CP) ../../../../oracle-database-operator.yaml /tmp/pkgtestplan/
	$(TAR) -C /tmp -cvf ~/pkgtestplan_$(DATE).tar pkgtestplan

################
###   diag   ###
################

login:
	$(KUBECTL) exec   `$(KUBECTL) get pods -n $(CDBNAMESPACE)|grep ords|cut -d ' ' -f 1` -n $(CDBNAMESPACE) -it -- /bin/bash


reloadop:
	echo "RESTARTING OPERATOR"
	$(eval OP1 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1 ))
	$(eval OP2 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1|cut  -d ' ' -f 1 ))
	$(eval OP3 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1 ))
	$(KUBECTL) get pod $(OP1) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP2) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP3) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -


dump:
	@$(eval TMPSP := $(shell date "+%y%m%d%H%M%S" ))
	@$(eval DIAGFILE := ./opdmp.$(TMPSP))
	@>$(DIAGFILE)
	@echo "OPERATOR DUMP"  >> $(DIAGFILE)
	@echo "~~~~~~~~~~~~~"  >> $(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1 | cut -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)

#######################################################
####                 TEST SECTION                  ####
#######################################################

run00:
	@$(call msg,"cdb pod creation")
	- $(KUBECTL) delete cdb cdb-dev -n $(CDBNAMESPACE)
	$(KUBECTL) apply -f $(ORDS_POD) 
	time $(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" cdb cdb-dev -n $(CDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"cdb pod completed")
	$(KUBECTL) get cdb -n  $(CDBNAMESPACE)
	$(KUBECTL) get pod -n  $(CDBNAMESPACE)

run01.1:
	@$(call msg,"pdb pdb1  creation")
	$(KUBECTL) apply -f $(PDBCRE1)
	time $(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb1 creation completed")
	$(KUBECTL) get pdb pdb1 -n $(PDBNAMESPACE) 

run01.2:
	@$(call msg, "pdb pdb2  creation")
	$(KUBECTL) apply -f $(PDBCRE2)
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb2 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb2  creation completed")
	$(KUBECTL) get pdb pdb2 -n $(PDBNAMESPACE) 

run02.1:
	@$(call msg, "pdb pdb1  open")
	$(KUBECTL) apply -f $(PDBOPEN1)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="READ WRITE" pdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb1 open completed")
	$(KUBECTL) get pdb pdb1 -n $(PDBNAMESPACE) 

run02.2:
	@$(call msg,"pdb pdb2  open")
	$(KUBECTL) apply -f $(PDBOPEN2)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="READ WRITE" pdb pdb2 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"pdb pdb2 open completed")
	$(KUBECTL) get pdb pdb2 -n $(PDBNAMESPACE) 


run03.1:
	@$(call msg,"clone pdb1-->pdb3")
	$(KUBECTL) apply -f $(PDBCLONE1)
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb3 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"clone pdb1-->pdb3 completed")
	$(KUBECTL) get pdb pdb3 -n $(PDBNAMESPACE) 


run03.2:
	@$(call msg,"clone pdb2-->pdb4")
	$(KUBECTL) apply -f $(PDBCLONE2)
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb4 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"clone pdb2-->pdb4 completed")
	$(KUBECTL) get pdb pdb3 -n $(PDBNAMESPACE) 


run04.1:
	@$(call msg,"pdb pdb1  close")
	$(KUBECTL) apply -f $(PDBCLOSE1)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="MOUNTED" pdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb1 close completed")
	$(KUBECTL) get pdb pdb1 -n $(PDBNAMESPACE)

run04.2:
	@$(call msg,"pdb pdb2  close")
	$(KUBECTL) apply -f $(PDBCLOSE2)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="MOUNTED" pdb pdb2 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"pdb pdb2 close completed")
	$(KUBECTL) get pdb pdb2 -n $(PDBNAMESPACE)

run05.1:
	@$(call msg,"pdb pdb1  unplug")
	$(KUBECTL) apply -f $(PDBUNPLUG1)
	$(KUBECTL) wait --for=delete pdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"pdb pdb1 unplug completed")

run06.1:
	@$(call msg, "pdb pdb1  plug")
	$(KUBECTL) apply -f $(PDBPLUG1)
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb1 plug completed")
	$(KUBECTL) get pdb pdb1 -n $(PDBNAMESPACE)

run07.1:
	@$(call msg,"pdb pdb1 delete ")
	- $(KUBECTL) apply -f $(PDBCLOSE1)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="MOUNTED" pdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	$(KUBECTL) apply -f $(PDBDELETE1)
	$(KUBECTL) wait --for=delete pdb pdb1 -n $(PDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"pdb pdb1 delete")
	$(KUBECTL) get pdb -n $(PDBNAMESPACE)

run99.1:
	$(KUBECTL) delete cdb cdb-dev -n cdbnamespace
	$(KUBECTL) wait --for=delete cdb cdb-dev -n $(CDBNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	$(KUBECTL) get cdb -n cdbnamespaace
	$(KUBECTL) get pdb -n pdbnamespaace


## SEQ | ACTION
## ----+----------------
##  00 | create ords pod
##  01 | create pdb
##  02 | open pdb
##  03 | clone pdb
##  04 | close pdb
##  05 | unpug pdb
##  06 | plug pdb
##  07 | delete pdb (declarative)

	
runall01:  run00 run01.1 run01.2 run03.1 run03.2 run04.1 run05.1 run06.1 run02.1  run07.1



