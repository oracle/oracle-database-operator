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
#         4) make runall01
#            Start a series of operation create open close delete and so on 
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
export OPRNAMESPACE=$(shell cat $(PARAMETERS)|grep -v ^\#|grep OPRNAMESPACE|cut -d :  -f 2)
export OPRNAMESPACE=$(shell cat $(PARAMETERS)|grep -v ^\#|grep OPRNAMESPACE|cut -d :  -f 2)
export ORDSIMG=$(shell cat $(PARAMETERS)|grep -v ^\#|grep ORDSIMG|cut -d : -f 2,3)
export COMPANY=$(shell cat $(PARAMETERS)|grep -v ^\#|grep COMPANY|cut -d : -f 2)
export APIVERSION=$(shell cat $(PARAMETERS)|grep -v ^\#|grep APIVERSION|cut -d : -f 2)
export OPRNAMESPACE=oracle-database-operator-system
export ORACLE_OPERATOR_YAML=../../../../oracle-database-operator.yaml
export TEST_EXEC_TIMEOUT=3m
export IMAGE=oracle/ords-dboper:latest
export ORDSIMGDIR=../../../../ords

REST_SERVER=ords
SKEY=tls.key
SCRT=tls.crt
CART=ca.crt
PRVKEY=ca.key
PUBKEY=public.pem
COMPANY=oracle
RUNTIME=/usr/bin/podman

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
SED=/usr/bin/sed

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
	@printf "OPRNAMESPACE...........:%s\n" $(OPRNAMESPACE)
	@printf "COMPANY................:%s\n" $(COMPANY)
	@printf "APIVERSION.............:%s\n" $(APIVERSION)


tlscrt:
	$(call msg,"TLS GENERATION")
	#$(OPENSSL) genrsa -out $(PRVKEY) 2048	
	$(OPENSSL)  genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > $(PRVKEY)
	$(OPENSSL) req -new -x509 -days 365 -key $(PRVKEY) \
    	       -subj "/C=CN/ST=GD/L=SZ/O=$(COMPANY), Inc./CN=$(COMPANY) Root CA" -out ca.crt
	$(OPENSSL) req -newkey rsa:2048 -nodes -keyout $(SKEY) -subj \
	       	"/C=CN/ST=GD/L=SZ/O=$(COMPANY), Inc./CN=cdb-dev-$(REST_SERVER).$(OPRNAMESPACE)" -out server.csr
	$(ECHO) "subjectAltName=DNS:cdb-dev-$(REST_SERVER).$(OPRNAMESPACE)" > extfile.txt
	$(OPENSSL) x509 -req -extfile extfile.txt -days 365 -in server.csr -CA ca.crt -CAkey $(PRVKEY) -CAcreateserial -out $(SCRT)
	$(OPENSSL) rsa -in $(PRVKEY) -outform PEM  -pubout -out $(PUBKEY)

tlssec:
	$(call msg,"GENERATE TLS SECRET")
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(OPRNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(OPRNAMESPACE)


delsec:
	$(call msg,"CLEAN OLD SECRETS")
	$(eval SECRETSP:=$(shell kubectl get secrets -n $(OPRNAMESPACE) -o custom-columns=":metadata.name" --no-headers|grep -v webhook-server-cert) )	
	@[ "${SECRETSP}" ] && ( \
	       	printf "Deleteing secrets in namespace -n $(OPRNAMESPACE)\n") &&\
	        ($(KUBECTL) delete secret  $(SECRETSP) -n $(OPRNAMESPACE))\
	        || ( echo "No screts in namespace $(OPRNAMESPACE)")


###### ENCRYPTED SECRETS ######
export PRVKEY=ca.key
export PUBKEY=public.pem
WBUSERFILE=wbuser.txt
WBPASSFILE=wbpass.txt
CDBUSRFILE=cdbusr.txt
CDBPWDFILE=cdbpwd.txt
SYSPWDFILE=syspwd.txt
ORDPWDFILE=ordpwd.txt
PDBUSRFILE=pdbusr.txt
PDBPWDFILE=pdbpwd.txt



secrets: delsec tlscrt tlssec
	$(OPENSSL) rsa -in $(PRVKEY) -outform PEM  -pubout -out $(PUBKEY)
	$(KUBECTL) create secret generic pubkey --from-file=publicKey=$(PUBKEY) -n $(OPRNAMESPACE)
	$(KUBECTL) create secret generic prvkey --from-file=privateKey=$(PRVKEY)  -n $(OPRNAMESPACE)
	@$(ECHO) $(WBUSER) > $(WBUSERFILE)
	@$(ECHO) $(WBPASS) > $(WBPASSFILE)
	@$(ECHO) $(CDBPWD) > $(CDBPWDFILE)
	@$(ECHO) $(CDBUSR) > $(CDBUSRFILE)
	@$(ECHO) $(SYSPWD) > $(SYSPWDFILE)
	@$(ECHO) $(ORDPWD) > $(ORDPWDFILE)
	@$(ECHO) $(PDBUSR) > $(PDBUSRFILE)
	@$(ECHO) $(PDBPWD) > $(PDBPWDFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(WBUSERFILE) |base64 > e_$(WBUSERFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(WBPASSFILE) |base64 > e_$(WBPASSFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(CDBPWDFILE) |base64 > e_$(CDBPWDFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(CDBUSRFILE) |base64 > e_$(CDBUSRFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(SYSPWDFILE) |base64 > e_$(SYSPWDFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(ORDPWDFILE) |base64 > e_$(ORDPWDFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(PDBUSRFILE) |base64 > e_$(PDBUSRFILE)
	$(OPENSSL) rsautl -encrypt -pubin -inkey $(PUBKEY) -in $(PDBPWDFILE) |base64 > e_$(PDBPWDFILE)
	$(KUBECTL) create secret generic wbuser --from-file=e_$(WBUSERFILE) -n  $(OPRNAMESPACE)
	$(KUBECTL) create secret generic wbpass --from-file=e_$(WBPASSFILE) -n  $(OPRNAMESPACE)
	$(KUBECTL) create secret generic cdbpwd --from-file=e_$(CDBPWDFILE) -n  $(OPRNAMESPACE)
	$(KUBECTL) create secret generic cdbusr --from-file=e_$(CDBUSRFILE) -n  $(OPRNAMESPACE)
	$(KUBECTL) create secret generic syspwd --from-file=e_$(SYSPWDFILE) -n  $(OPRNAMESPACE)
	$(KUBECTL) create secret generic ordpwd --from-file=e_$(ORDPWDFILE) -n  $(OPRNAMESPACE)
	$(KUBECTL) create secret generic pdbusr --from-file=e_$(PDBUSRFILE) -n  $(OPRNAMESPACE)
	$(KUBECTL) create secret generic pdbpwd --from-file=e_$(PDBPWDFILE) -n  $(OPRNAMESPACE)
	$(RM)  $(WBUSERFILE)  $(WBPASSFILE) $(CDBPWDFILE) $(CDBUSRFILE)  $(SYSPWDFILE)  $(ORDPWDFILE)  $(PDBUSRFILE) $(PDBPWDFILE) 
	$(RM)  e_$(WBUSERFILE)  e_$(WBPASSFILE) e_$(CDBPWDFILE) e_$(CDBUSRFILE)  e_$(SYSPWDFILE)  e_$(ORDPWDFILE)  e_$(PDBUSRFILE) e_$(PDBPWDFILE) 


### YAML FILE SECTION ###
operator:
	 $(CP) ${ORACLE_OPERATOR_YAML} .
	${CP} `basename ${ORACLE_OPERATOR_YAML}` `basename ${ORACLE_OPERATOR_YAML}`.ORG
	$(SED) -i  's/value: ""/value: $(OPRNAMESPACE)/g'   `basename ${ORACLE_OPERATOR_YAML}`


define _script00
cat <<EOF > authsection01.yaml
  sysAdminPwd:
    secret:
      secretName: "syspwd"
      key: "e_syspwd.txt"
  ordsPwd:
    secret:
      secretName: "ordpwd"
      key: "e_ordpwd.txt"
  cdbAdminUser:
    secret:
      secretName: "cdbusr"
      key: "e_cdbusr.txt"
  cdbAdminPwd:
    secret:
      secretName: "cdbpwd"
      key: "e_cdbpwd.txt"
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
  cdbOrdsPrvKey:
    secret:
      secretName: "prvkey"
      key: "privateKey"
EOF

cat<<EOF > authsection02.yaml
  adminName:
    secret:
      secretName: "pdbusr"
      key: "e_pdbusr.txt"
  adminPwd:
    secret:
      secretName: "pdbpwd"
      key: "e_pdbpwd.txt"
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
      secretName: "wbuser"
      key: "e_wbuser.txt"
  webServerPwd:
    secret:
      secretName: "wbpass"
      key: "e_wbpass.txt"
  pdbOrdsPrvKey:
    secret:
      secretName: "prvkey"
      key: "privateKey"
EOF


cat <<EOF > ${OPRNAMESPACE}_binding.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: oracle-database-operator-oracle-database-operator-manager-rolebinding1
  namespace: ${OPRNAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: oracle-database-operator-manager-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: oracle-database-operator-system
EOF

cat <<EOF > ${OPRNAMESPACE}_binding.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: oracle-database-operator-oracle-database-operator-manager-rolebinding2
  namespace: ${OPRNAMESPACE}
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

#echo ords pod creation
define _script01
cat <<EOF > ${ORDS_POD}
apiVersion: database.oracle.com/${APIVERSION}
kind: CDB
metadata: 
  name: cdb-dev
  namespace: oracle-database-operator-system
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
  pdbName: "pdbdev"
  action: "Delete"
  dropAction: "INCLUDING"
EOF

cat <<EOF > ${PDBDELETE2}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb2
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
  pdbName: "pdbprd"
  action: "Delete"
  dropAction: "INCLUDING"
EOF

cat <<EOF > ${PDBUNPLUG1}
apiVersion: database.oracle.com/${APIVERSION}
kind: PDB
metadata:
  name: pdb1
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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
  namespace: ${OPRNAMESPACE}
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "${OPRNAMESPACE}"
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

genyaml:  secyaml
	@ eval "$$script01"
	@ eval "$$script02"

cleanyaml:
	- $(RM) $(PDBMAP3) $(PDBMAP2) $(PDBMAP1) $(PDBPLUG1) $(PDBUNPLUG1) $(PDBDELETE2) $(PDBDELETE1) $(PDBCLONE2) $(PDBCLONE1) $(PDBCLOSE3) $(PDBCLOSE2) $(PDBCLOSE1) $(PDBOPEN3) $(PDBOPEN2) $(PDBOPEN1) $(PDBCRE2) $(PDBCRE1) $(ORDS_POD) $(CDB_SECRETS) $(PDB_SECRETS) 
	- $(RM) ${OPRNAMESPACE}_binding.yaml ${OPRNAMESPACE}_binding.yaml


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
	$(KUBECTL) exec   `$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep ords|cut -d ' ' -f 1` -n $(OPRNAMESPACE) -it -- /bin/bash


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
	- $(KUBECTL) delete cdb cdb-dev -n $(OPRNAMESPACE)
	$(KUBECTL) apply -f $(ORDS_POD) 
	time $(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" cdb cdb-dev -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"cdb pod completed")
	$(KUBECTL) get cdb -n  $(OPRNAMESPACE)
	$(KUBECTL) get pod -n  $(OPRNAMESPACE)

run01.1:
	@$(call msg,"pdb pdb1  creation")
	$(KUBECTL) apply -f $(PDBCRE1)
	time $(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb1 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb1 creation completed")
	$(KUBECTL) get pdb pdb1 -n $(OPRNAMESPACE) 

run01.2:
	@$(call msg, "pdb pdb2  creation")
	$(KUBECTL) apply -f $(PDBCRE2)
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb2 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb2  creation completed")
	$(KUBECTL) get pdb pdb2 -n $(OPRNAMESPACE) 

run02.1:
	@$(call msg, "pdb pdb1  open")
	$(KUBECTL) apply -f $(PDBOPEN1)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="READ WRITE" pdb pdb1 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb1 open completed")
	$(KUBECTL) get pdb pdb1 -n $(OPRNAMESPACE) 

run02.2:
	@$(call msg,"pdb pdb2  open")
	$(KUBECTL) apply -f $(PDBOPEN2)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="READ WRITE" pdb pdb2 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"pdb pdb2 open completed")
	$(KUBECTL) get pdb pdb2 -n $(OPRNAMESPACE) 


run03.1:
	@$(call msg,"clone pdb1-->pdb3")
	$(KUBECTL) apply -f $(PDBCLONE1)
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb3 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"clone pdb1-->pdb3 completed")
	$(KUBECTL) get pdb pdb3 -n $(OPRNAMESPACE) 


run03.2:
	@$(call msg,"clone pdb2-->pdb4")
	$(KUBECTL) apply -f $(PDBCLONE2)
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb4 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"clone pdb2-->pdb4 completed")
	$(KUBECTL) get pdb pdb3 -n $(OPRNAMESPACE) 


run04.1:
	@$(call msg,"pdb pdb1  close")
	$(KUBECTL) apply -f $(PDBCLOSE1)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="MOUNTED" pdb pdb1 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb1 close completed")
	$(KUBECTL) get pdb pdb1 -n $(OPRNAMESPACE)

run04.2:
	@$(call msg,"pdb pdb2  close")
	$(KUBECTL) apply -f $(PDBCLOSE2)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="MOUNTED" pdb pdb2 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"pdb pdb2 close completed")
	$(KUBECTL) get pdb pdb2 -n $(OPRNAMESPACE)

run05.1:
	@$(call msg,"pdb pdb1  unplug")
	$(KUBECTL) apply -f $(PDBUNPLUG1)
	$(KUBECTL) wait --for=delete pdb pdb1 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"pdb pdb1 unplug completed")

run06.1:
	@$(call msg, "pdb pdb1  plug")
	$(KUBECTL) apply -f $(PDBPLUG1)
	$(KUBECTL) wait --for jsonpath='{.status.phase'}="Ready" pdb pdb1 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg, "pdb pdb1 plug completed")
	$(KUBECTL) get pdb pdb1 -n $(OPRNAMESPACE)

run07.1:
	@$(call msg,"pdb pdb1 delete ")
	- $(KUBECTL) apply -f $(PDBCLOSE1)
	$(KUBECTL) wait --for jsonpath='{.status.openMode'}="MOUNTED" pdb pdb1 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	$(KUBECTL) apply -f $(PDBDELETE1)
	$(KUBECTL) wait --for=delete pdb pdb1 -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
	@$(call msg,"pdb pdb1 delete")
	$(KUBECTL) get pdb -n $(OPRNAMESPACE)

run99.1:
	$(KUBECTL) delete cdb cdb-dev -n cdbnamespace
	$(KUBECTL) wait --for=delete cdb cdb-dev -n $(OPRNAMESPACE) --timeout=$(TEST_EXEC_TIMEOUT)
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


###### BUILD ORDS IMAGE ######

createimage:
	$(RUNTIME) build -t $(IMAGE) $(ORDSIMGDIR)

createimageproxy:
	$(RUNTIME) build -t $(IMAGE) $(ORDSIMGDIR) --build-arg  https_proxy=$(HTTPS_PROXY)  --build-arg http_proxy=$(HTTP_PROXY)

tagimage:
	@echo "TAG IMAGE"
	$(RUNTIME) tag $(IMAGE) $(ORDSIMG)

push:
	 $(RUNTIME) push $(ORDSIMG)


