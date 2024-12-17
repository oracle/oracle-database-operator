#   __  __       _         __ _ _
#  |  \/  | __ _| | _____ / _(_) | ___
#  | |\/| |/ _` | |/ / _ \ |_| | |/ _ \
#  | |  | | (_| |   <  __/  _| | |  __/
#  |_|  |_|\__,_|_|\_\___|_| |_|_|\___|
#
#    ___
#   / _ \ _ __  _ __  _ __ ___ _ __ ___
#  | | | | '_ \| '_ \| '__/ _ \ '_ ` _ \
#  | |_| | | | | |_) | | |  __/ | | | | |
#   \___/|_| |_| .__/|_|  \___|_| |_| |_|
#              |_|
#    ____            _             _ _
#   / ___|___  _ __ | |_ _ __ ___ | | | ___ _ __
#  | |   / _ \| '_ \| __| '__/ _ \| | |/ _ \ '__|
#  | |__| (_) | | | | |_| | | (_) | | |  __/ |
#   \____\___/|_| |_|\__|_|  \___/|_|_|\___|_|
#
#  
# This makefile helps to speed up the  kubectl commands executions to deploy and test
# the multitenant operator. 
#
# Quick start:
# ~~~~~~~~~~~
#
# - Copy files of tab.1 in the makefile directory.
# - Edit the secret files and other yaml files with the correct credential as 
#   specified in the documentation.
# - Edit makefile updating variables of tab.2
# - Execute commands of tab.3 "make step1" "make step2" "make step3"....
#
# Tab.1 - List of required files
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
# 
# oracle-database-operator.yaml     : oracle database operator
# cdbnamespace_binding.yaml         : role binding for cdbnamespace
# pdbnamespace_binding.yaml         : role binding for pdbnamespace
# create_lrest_secret.yaml          : create secrets for rest server pod 
# create_lrpdb_secret.yaml          : create secrets for pluggable database
# create_lrest_pod.yaml             : create rest server pod
# create_lrpdb1_resource.yaml       : create first pluggable database 
# create_lrpdb2_resource.yaml       : create second pluggable database 
# open_lrpdb1_resource.yaml         : open first pluggable database 
# open_lrpdb2_resource.yaml         : open second pluggable database 
# close_lrpdb1_resource.yaml        : close first pluggable database 
# close_lrpdb2_resource.yaml        : close second pluggable database  
# clone_lrpdb_resource.yaml         : clone thrid pluggable database 
# clone_lrpdb2_resource.yaml        : clone 4th pluggable database 
# delete_lrpdb1_resource.yaml       : delete first pluggable database 
# delete_lrpdb2_resource.yaml       : delete sencond pluggable database 
# delete_lrpdb3_resource.yaml       : delete thrid pluggable database 
# unplug_lrpdb1_resource.yaml       : unplug first pluggable database 
# plug_lrpdb1_resource.yaml         : plug first pluggable database 
# map_lrpdb1_resource.yaml          : map the first pluggable database 
# config_map.yam                    : pdb parameters array 
# altersystem_lrpdb1_resource.yaml  : chage cpu_count count parameter for the first pdb

#
# Tab.2 - List of variables
# ~~~~~~~~~~~~~~~~~~~~~~~~~
#
# +-----------------------------+---------------------------------------------+
# |OCIR                         | Your image registry                         |
# +-----------------------------+---------------------------------------------+
# |OCIRPATH                     | Path of the image in your registry          |
# +-----------------------------+---------------------------------------------+
#
# Tab.3 - Execution steps
# ~~~~~~~~~~~~~~~~~~~~~~~
#
# +-----------------------------+---------------------------------------------+
# |                      MAKEFILE TARGETS  LIST                               |
# |                         ----- ooo -----                                   |
# |          - TARGET -                         - DESCRIPTION -               |
# +-----------------------------+---------------------------------------------+
# |step0                        | list pods and lrpdb resources               |
# +-----------------------------+-------------------------------------+-------|
# |step1                        | Build rest server images            |       |
# +-----------------------------+-------------------------------------+ REST  |
# |step2                        | Tag the immages                     | SRV   |
# +-----------------------------+-------------------------------------+ IMG   |
# |step3                        | Push the image into the repository  |       | 
# +-----------------------------+-------------------------------------+-------+
# |step4                        | Load webhook certmanager            | DB    |
# +-----------------------------+-------------------------------------+ OPER  |
# |step5                        | Create the db operator              |       |
# +-----------------------------+-------------------------------------+-------+
# |step6                        | Create tls certificates             |   T   |
# +-----------------------------+-------------------------------------+   L   |
# |step7                        | Create tls secret                   |   S   |
# +-----------------------------+---------------------------------------------+
# |step8                        | Create database secrets                     |
# +-----------------------------+---------------------------------------------+
# |step9                        | Create restserver pod                       |
# |  |                          +---------------------------------------------+
# |  +---> checkstep9           | Monitor the executions                      |
# +-----------------------------+---------------------------------------------+
# |step10.[1..9]                | Create pluggable database                   |
# |  |                          +---------------------------------------------+
# |  +---> checklrest           | Monitor LRPDB status                        |
# +-----------------------------+---------------------------------------------+
# | step11.[1..9]               | Open pluggable database                     |
# +-----------------------------+---------------------------------------------+
# | step12.[1..9]               | Close pluggable database                    |
# +-----------------------------+---------------------------------------------+
# | step13.[1..9]               | Clone pluggable database                    |
# +-----------------------------+---------------------------------------------+
# | step14.[1..9]               | Delete pluggable database                   |
# +-----------------------------+---------------------------------------------+
# | step15.[1..9]               | Unplug pluggable database                   |
# +-----------------------------+---------------------------------------------+
# | step16.[1..9]               | Plug pluggable database                     |
# +-----------------------------+---------------------------------------------+
# | step17.[1..9]               | Map pluggable database                      |
# +-----------------------------+---------------------------------------------+
# |                        DIAGNOSTIC  TARGETS                                |
# +-----------------------------+---------------------------------------------+
# | dump                        | Dump  pods info into a file                 |
# +-----------------------------+---------------------------------------------+
# | reloadop                    | Reload the db operator                      |
# +-----------------------------+---------------------------------------------+
# | login                       | Login into cdb pod                          |
# +-----------------------------+---------------------------------------------+

DATE  := `date "+%y%m%d%H%M%S"`

################ TAB 2 VARIABLES ############
OCIR=lin.ocir.io
OCIRPATH=intsanjaysingh/mmalvezz/testppr/$(REST_SERVER)-dboper:$(LRESTVERSION)
#############################################

OPRNAMESPACE=oracle-database-operator-system
LRSNAMESPACE=cdbnamespace
PDBNAMESPACE=pdbnamespace
CONFIG_MAP=config-map-pdb

REST_SERVER=lrest
LRESTVERSION=latest
DOCKER=/usr/bin/docker
#KUBECTL=/usr/bin/kubectl
KUBECTL=/usr/local/go/bin/kubectl
CONFIG=/etc/ords/config
IMAGE=oracle/$(REST_SERVER)-dboper:$(LRESTVERSION)
DBOPERATOR=oracle-database-operator.yaml
DBOPERATORNOVERBS=oracle-database-operator-noverbs.yaml
URLPATH=/_/db-api/stable/database/pdbs/
OPENSSL=/usr/bin/openssl
ORDSPORT=8888
MAKE=/usr/bin/make
DOCKERFILE=Dockerfile
RM=/usr/bin/rm
MKDIR=/bin/mkdir
TAR=/bin/tar
CP=/bin/cp
ECHO=/usr/bin/echo
SED=/bin/sed
SLEEP=sleep 32 
OPRNAMESPACE=oracle-database-operator-system
CERTMANAGER=https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml
LREST_SECRET=create_lrest_secret.yaml
LRPDB_SECRET=create_lrpdb_secret.yaml   
TDE_SECRET=tde_secret.yaml
PDBROLEBINDING=pdbnamespace_binding.yaml
CDBROLEBINDING=cdbnamespace_binding.yaml 
LREST_POD=create_lrest_pod.yaml
LRPDB1=create_lrpdb1_resource.yaml
LRPDB2=create_lrpdb2_resource.yaml
LRPDBOPEN1=open_lrpdb1_resource.yaml
LRPDBOPEN2=open_lrpdb2_resource.yaml
LRPDBOPEN3=open_lrpdb3_resource.yaml
LRPDBCLOSE1=close_lrpdb1_resource.yaml
LRPDBCLOSE2=close_lrpdb2_resource.yaml
LRPDBCLOSE3=close_lrpdb3_resource.yaml
LRPDBCLONE1=clone_lrpdb1_resource.yaml
LRPDBDELETE1=delete_lrpdb1_resource.yaml
LRPDBDELETE2=delete_lrpdb2_resource.yaml
LRPDBDELETE3=delete_lrpdb3_resource.yaml
LRPDBUNPLUG1=unplug_lrpdb1_resource.yaml
LRPDBPLUG1=plug_lrpdb1_resource.yaml
LRPDBMAP1=map_lrpdb1_resource.yaml
LRPDBMAP2=map_lrpdb2_resource.yaml
LRPDBMAP3=map_lrpdb3_resource.yaml
ALTERSYSTEMYAML=altersystem_lrpdb1_resource.yaml
SWAPYAML=/tmp/swap.yaml
SKEY=tls.key
SCRT=tls.crt
CART=ca.crt
RESNAME=-o=jsonpath='{range .items[*]}  {.metadata.name}   {.status.openMode}  '
SQLCODE=-o=jsonpath='{range .items[*]}  {.metadata.name}   {.status.sqlCode} '
COMPANY=oracle
MAKEFILE=./makefile
RNDFILE=./rnd.txt

list:
	grep ^step $(MAKEFILE)

step0:     listres              #[OK]
step1:     createimage          #[OK]
step2:     tagimage             #[OK]
step3:     push                 #[OK]
step4:     certmanager          #[OK]
step5:     dboperator           #[OK]
step5.1:   rolebinding          #[OK]
step5.2:   deldbop              #[OK]
step6:     tlscert              #[OK]
step7:     tlssecret            #[OK]
step8:     lrestsecretcred      #[OK]
step9:     lrest_pod            #[OK]
step9.1:   checklrest           #[OK]
step10.1:  lrpdbcreate1         #[OK]
step10.2:  lrpdbcreate2		#[OK]
step11.1:  lrpdbopen1		#[OK]
step11.2:  lrpdbopen2		#[OK]
step12.1:  lrpdbclose1		#[OK]
step12.2:  lrpdbclose2          #[OK]
step13.1:  lrpdbclone1          #[OK]
step13.2:  lrpdbopen3           #[0K]
step13.3:  lrpdbclose3          #[0K]
step14.1:  lrpdbunplug1         #[OK]
step15.1:  lrpdbplug1           #[OK]
step16.1:  lrptkmap1            #[OK]
step16.2:  lrptkmap2            #[OK]
step16.3:  lrptkmap3            #[OK]
step17.1:  lrpdbdelete1		#[OK]
step17.2:  lrpdbdelete2         #[OK]
step17.3:  lrpdbdelete3         #[OK]
step17.4:  lrpdbdelete4         #[OK]
step18.1:  lrpdbgalter          #[OK]
step19:    dellrestcascade
step100.1: lrptkunplug1		#[OK]
step101.1: lrptndelete1         #[OK]   
step102.1: lrptnclone1          #[OK]
step103.1: optnowatch           #[OK]
step104.1: optnoverbs           #[OK]
step105.1: lrpdbnegalter        #[OK]


listres:
	$(KUBECTL) get pods -n $(OPRNAMESPACE) 
	$(KUBECTL) get lrpdb -n $(OPRNAMESPACE)

createimage:
	@echo "BUILDING CDB IMAGES"
	@if [[ ! -f ./Dockerfile ]]; \
         then\
         echo "DOCKERFILE DOES NOT EXISTS";\
         exit 1; \
        fi; 
	@if [[ ! -f ./runLREST.sh ]]; \
         then\
         echo "DOCKERFILE DOES NOT EXISTS";\
         exit 1; \
        fi; 
	$(DOCKER) build -t $(IMAGE) . 

tagimage: 
	@echo "TAG IMAGE"
	$(DOCKER) tag $(IMAGE) $(OCIR)$(OCIRPATH)

push:
	@echo "PUSH IMAGE INTO THE REGISTRY"
	$(DOCKER) push $(OCIR)$(OCIRPATH)

certmanager:
	@echo "WEBHOOK CERT MANAGER"
	$(KUBECTL) apply -f $(CERTMANAGER) 

dboperator:
	@echo "ORACLE DATABASE OPERATOR"
	cat $(DBOPERATOR)| sed 's/value: ""/value: "$(OPRNAMESPACE),$(PDBNAMESPACE),$(LRSNAMESPACE)"/g' >temp_opr.yaml
	$(KUBECTL) apply -f temp_opr.yaml
	$(RM) temp_opr.yaml

rolebinding:
	$(KUBECTL) apply -f $(PDBROLEBINDING)
	$(KUBECTL) apply -f $(CDBROLEBINDING)

PRVKEY=ca.key
PUBKEY=public.pem
tlscert:
	@echo "CREATING TLS CERTIFICATES"
	#OL8
	$(OPENSSL) genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > $(PRVKEY) 
	#OL9
	#$(OPENSSL) genrsa -out $(PRVKEY) 2048
	$(OPENSSL) req -new -x509 -days 365 -key $(PRVKEY) -subj "/C=CN/ST=GD/L=SZ/O=$(COMPANY), Inc./CN=$(COMPANY) Root CA" -out ca.crt
	$(OPENSSL) req -newkey rsa:2048 -nodes -keyout $(SKEY) -subj "/C=CN/ST=GD/L=SZ/O=$(COMPANY), Inc./CN=cdb-dev-$(REST_SERVER).$(LRSNAMESPACE)" -out server.csr
	$(ECHO) "subjectAltName=DNS:cdb-dev-$(REST_SERVER).$(LRSNAMESPACE),DNS:www.example.com" > extfile.txt
	$(OPENSSL) x509 -req -extfile extfile.txt -days 365 -in server.csr -CA ca.crt -CAkey $(PRVKEY) -CAcreateserial -out $(SCRT)
	$(OPENSSL) rsa -in $(PRVKEY) -outform PEM  -pubout -out $(PUBKEY)

tlssecret:
	@echo "CREATING TLS SECRETS"
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(OPRNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(OPRNAMESPACE)
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(LRSNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(LRSNAMESPACE)
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(PDBNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(PDBNAMESPACE)
	$(KUBECTL) create secret tls prvkey --key="$(PRVKEY)" --cert=ca.crt  -n $(LRSNAMESPACE)
	$(KUBECTL) create secret generic pubkey --from-file=publicKey=$(PUBKEY) -n $(LRSNAMESPACE)


opsecret:
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(OPRNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(OPRNAMESPACE)

dbsecret:
	@echo "CREATING DB SECRETS"
	$(KUBECTL) apply -f $(LREST_SECRET) -n $(LRSNAMESPACE)
	$(KUBECTL) apply -f $(LRPDB_SECRET) -n $(PDBNAMESPACE)
	#$(KUBECTL) apply -f $(TDE_SECRET) -n $(OPRNAMESPACE)

DBUSERFILE=dbuser.txt
DBPASSFILE=dbpass.txt
WBUSERFILE=wbuser.txt
WBPASSFILE=wbpass.txt
PDBUSRFILE=pdbusr.txt
PDBPWDFILE=pdbpwd.txt

lrestsecretcred:
	$(KUBECTL) delete secret prvkey -n $(LRSNAMESPACE)
	$(KUBECTL) delete secret pubkey -n $(LRSNAMESPACE)
	$(ECHO) "restdba"      > $(DBUSERFILE)
	$(ECHO) "CLWKO655321"  > $(DBPASSFILE)
	$(ECHO) "welcome"      > $(WBUSERFILE)
	$(ECHO) "welcome1"     > $(WBPASSFILE)
	$(ECHO) "welcome"      > $(PDBUSRFILE)
	$(ECHO) "welcome1"     > $(PDBPWDFILE)
	$(KUBECTL) create secret generic prvkey --from-file=privateKey="$(PRVKEY)" -n $(LRSNAMESPACE)
	$(KUBECTL) create secret generic pubkey --from-file=publicKey="$(PUBKEY)" -n $(LRSNAMESPACE)
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
	$(KUBECTL) create secret generic prvkey --from-file=privateKey="$(PRVKEY)" -n $(PDBNAMESPACE)


s2:
	$(KUBECTL) create secret generic wbuser --from-file=e_$(WBUSERFILE) -n  $(PDBNAMESPACE)
	$(KUBECTL) create secret generic wbpass --from-file=e_$(WBPASSFILE) -n  $(PDBNAMESPACE)


# openssl rsautl -encrypt -pubin -inkey pub.key -in plaintext.txt | base64 > encrypted.txt
# cat encrypted.txt | base64 -d | openssl rsautl -decrypt -inkey priv.key
# kubectl create secret generic testsec --from-file=encrypted.txt --from-file=ca.key -n cdbnamespace



lrest_pod:
	@echo "CREATING LREST POD"
	$(KUBECTL) apply -f $(LREST_POD)
	$(KUBECTL) get pods -n $(LRSNAMESPACE) --watch

checklrest:
	$(KUBECTL) logs -f `$(KUBECTL) get pods -n $(LRSNAMESPACE)|grep $(REST_SERVER)|cut -d ' ' -f 1` -n $(LRSNAMESPACE)

lrpdbcreate1:
	$(KUBECTL) apply -f $(LRPDB1) 

lrpdbcreate1log:
	$(KUBECTL) apply -f $(LRPDB1) &
	$(KUBECTL) logs -f -l control-plane=controller-manager -n oracle-database-operator-system

lrpdbcreate2:
	$(KUBECTL) apply -f $(LRPDB2)

lrpdbopen1log:
	$(KUBECTL) apply -f $(LRPDBOPEN1) &
	$(KUBECTL) logs -f -l control-plane=controller-manager -n oracle-database-operator-system


lrpdbopen1:
	$(KUBECTL) apply -f $(LRPDBOPEN1)

lrpdbopen2:
	$(KUBECTL) apply -f $(LRPDBOPEN2)

lrpdbopen3:
	$(KUBECTL) apply -f $(LRPDBOPEN3)

lrpdbclose1:
	$(KUBECTL) apply -f  $(LRPDBCLOSE1) 

lrpdbclose2:
	$(KUBECTL) apply -f  $(LRPDBCLOSE2) 

lrpdbclose3:
	$(KUBECTL) apply -f  $(LRPDBCLOSE3) 

lrpdbclone1: lrpdbopen1
	@echo "Waiting 10 sec before cloning"
	@sleep 10
	$(KUBECTL) apply -f  $(LRPDBCLONE1) 

lrpdbclone1log:
	$(KUBECTL) apply -f  $(LRPDBCLONE1)  &
	$(KUBECTL) logs -f -l control-plane=controller-manager -n oracle-database-operator-system

lrpdbdelete1:
	$(KUBECTL) apply -f  $(LRPDBDELETE1)  

lrpdbdelete2:
	$(KUBECTL) apply -f  $(LRPDBDELETE2) 

lrpdbdelete3:
	$(KUBECTL) apply -f  $(LRPDBDELETE3) 

lrpdbdelete4:
	echo "Imperative deletion"
	$(KUBECTL) delete lrpdb lrpdb4 -n $(PDBNAMESPACE)

lrpdbunplug1:
	$(KUBECTL) apply -f  $(LRPDBUNPLUG1)

lrpdbplug1:
	$(KUBECTL) apply -f  $(LRPDBPLUG1)

lrpdbplug1log:
	$(KUBECTL) apply -f  $(LRPDBPLUG1) &
	$(KUBECTL) logs -f -l control-plane=controller-manager -n oracle-database-operator-system


lrptkmap1: 
	$(KUBECTL) delete lrpdb lrpdb1 -n $(PDBNAMESPACE)
	$(SLEEP) 
	$(KUBECTL) apply -f  $(LRPDBMAP1)
	$(MAKE) -f $(MAKEFILE) checklrpdbs

lrptkmap2: 
	$(KUBECTL) delete lrpdb lrpdb2 -n $(PDBNAMESPACE)
	$(SLEEP) 
	$(KUBECTL) apply -f  $(LRPDBMAP2)
	$(MAKE) -f $(MAKEFILE) checklrpdbs

lrptkmap3: 
	$(KUBECTL) delete lrpdb lrpdb3 -n $(PDBNAMESPACE)
	$(SLEEP) 
	$(KUBECTL) apply -f  $(LRPDBMAP3)
	$(MAKE) -f $(MAKEFILE) checklrpdbs

lrptkunplug1: 
	@echo "test unpluga and plug database"
	@shuf -i 1-10000000 -n 1 >rnd_file.txt
	@cat $(LRPDBUNPLUG1) |sed 's/pdb.xml/'$(shell cat ./rnd_file.txt)'.xml/g' > rnd_$(LRPDBUNPLUG1)
	@cat $(LRPDBPLUG1)   |sed 's/pdb.xml/'$(shell cat ./rnd_file.txt)'.xml/g' > rnd_$(LRPDBPLUG1)
	$(KUBECTL) apply -f rnd_$(LRPDBUNPLUG1)
	@echo -e "\033[5mWaiting for the unplug operation to  complete\033[0m"
	@while [ `$(KUBECTL) get lrpdb lrpdb1 -n $(PDBNAMESPACE) 2>/dev/null |grep ^lrpdb1 |wc -l` -eq 1 ];\
          do  \
           sleep 1 ;\
          done
	$(KUBECTL) apply -f rnd_$(LRPDBPLUG1)
	@$(RM) ./rnd_file.txt rnd_$(LRPDBPLUG1) rnd_$(LRPDBUNPLUG1)


lrptndelete1:
	@echo "lrptndelete1: Starting negative test: delete an open pdb"
	$(KUBECTL) apply -f $(LRPDB1)
	@echo -e "\033[5mCreating database\033[0m"
	@while [ `$(KUBECTL) get lrpdbs -n $(PDBNAMESPACE)  $(RESNAME) 2>/dev/null |grep lrpdb1 |wc -l` -eq 0 ];\
		do \
		$(SLEEP);\
		done
	$(KUBECTL) apply -f $(LRPDBOPEN1)
	@echo -e "\033[5mOpening database\033[0m"
	@while [ `$(KUBECTL) get lrpdbs -n $(PDBNAMESPACE)  $(RESNAME) 2>/dev/null |grep lrpdb1|grep "READ WRITE" |wc -l` -eq 0 ];\
		do \
		$(SLEEP);\
		done
	$(SLEEP)
	$(KUBECTL) apply -f  $(LRPDBDELETE1)
	$(SLEEP)
	$(KUBECTL) get lrpdb -n $(PDBNAMESPACE)

lrpdbgalter:
	$(KUBECTL) apply -f $(ALTERSYSTEMYAML)
	$(SLEEP)
	$(KUBECTL) describe lrpdb lrpdb1 -n $(PDBNAMESPACE) 	

lrpdbnegalter:
	$(SED) 's/cpu_count=[0-9]/cpu_count=100000/g' $(ALTERSYSTEMYAML) > $(SWAPYAML)
	$(KUBECTL) apply -f  $(SWAPYAML)
	$(KUBECTL) get lrpdbs lrpdb1 -n $(PDBNAMESPACE) $(SQLCODE)
	$(KUBECTL) describe lrpdb lrpdb1 -n $(PDBNAMESPACE)

lrptnclone1:
	@echo "lrptnclone1: Starting negative test: cloning a pdb in mount state" 
	$(KUBECTL) apply -f $(LRPDB1)
	@echo -e "\033[5mCreating database\033[0m"
	@while [ `$(KUBECTL) get lrpdbs -n $(PDBNAMESPACE)  $(RESNAME) 2>/dev/null |grep ^lrpdb1 |wc -l` -eq 1 ];\
                do \
                $(SLEEP);\
                done
	$(KUBECTL) apply -f $(LRPDBCLONE1) 
	$(SLEEP)
	@while [ `$(KUBECTL) get lrpdbs  lrpdb3 -n $(PDBNAMESPACE)  $(SQLCODE) 2>/dev/null |grep lrpdb3 |grep 65036 |wc -l` -eq 0 ];\
	do \
		$(SLEEP);\
		done
	$(KUBECTL) get lrpdb lrpdb3 -n $(PDBNAMESPACE)

optnowatch:
	cat $(DBOPERATOR) |sed 's/$(OPRNAMESPACE),$(PDBNAMESPACE),$(LRSNAMESPACE)/default/g' >temp_opr.yaml
	$(KUBECTL) apply -f temp_opr.yaml 
	$(SLEEP)
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(OPRNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(OPRNAMESPACE)
	$(KUBECTL) apply -f $(LREST_POD)
	$(RM) temp_opr.yaml

optnoverbs:
	$(KUBECTL) apply -f $(DBOPERATORNOVERBS)
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(OPRNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(OPRNAMESPACE)
	$(KUBECTL) apply -f $(LREST_POD)

secretsop:
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(OPRNAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(OPRNAMESPACE)

deldbop:
	$(KUBECTL) delete -f $(DBOPERATOR) --ignore-not-found=true

checklrpdbs:
	$(KUBECTL) get lrpdbs -n $(PDBNAMESPACE)



dump:
	@$(eval TMPSP := $(shell date "+%y%m%d%H%M%S" ))
	@$(eval DIAGFILE := ./opdmp.$(TMPSP))
	@>$(DIAGFILE)
	@echo "OPERATOR DUMP"  >> $(DIAGFILE)
	@echo "~~~~~~~~~~~~~"  >> $(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1 | cut -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) >>$(DIAGFILE)

dump2:
	@echo "CDB LOG DUMP" >> $(DIAGFILE)
	@echo "~~~~~~~~" >> $(DIAGFILE)
	$(KUBECTL) logs `$(KUBECTL) get pods -n $(LRSNAMESPACE)|grep $(REST_SERVER)|  cut -d ' ' -f 1` -n $(LRSNAMESPACE)  >>$(DIAGFILE)
	@echo "SECRET DMP" >>$(DIAGFILE)
	@echo "~~~~~~~~" >> $(DIAGFILE)
	$(KUBECTL) get secrets  -o yaml -n $(OPRNAMESPACE) >> $(DIAGFILE)
	@echo "CDB/LRPDB DMP" >> $(DIAGFILE)
	$(KUBECTL) get lrpdbs  -o yaml -n $(PDBNAMESPACE) >> $(DIAGFILE)
	$(KUBECTL) get lrest  -o yaml -n $(LRSNAMESPACE) >> $(DIAGFILE)
	@echo "CLUSTER INFO" >>  $(DIAGFILE)
	$(KUBECTL) get nodes -o wide
	$(KUBECTL) get svc --namespace=kube-system

cleardump:
	$(RM) opdmp.*

reloadop:
	echo "RESTARTING OPERATOR"
	$(eval OP1 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1 ))
	$(eval OP2 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1|cut  -d ' ' -f 1 ))
	$(eval OP3 := $(shell $(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1 ))
	$(KUBECTL) get pod $(OP1) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP2) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP3) -n $(OPRNAMESPACE) -o yaml | kubectl replace --force -f -

getrolebinding:
	$(KUBECTL) get rolebinding -n $(PDBNAMESPACE)
	$(KUBECTL) get rolebinding -n $(LRSNAMESPACE)

login:
	$(KUBECTL) exec   `$(KUBECTL) get pods -n $(LRSNAMESPACE)|grep $(REST_SERVER)|cut -d ' ' -f 1` -n $(LRSNAMESPACE) -it -- /bin/bash

log1:
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) 

log2:
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1 | cut -d ' ' -f 1` -n $(OPRNAMESPACE) 

log3:
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPRNAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1` -n $(OPRNAMESPACE) 

clean:
	 $(KUBECTL) delete -f $(LRPDB1)
	 $(KUBECTL) delete -f $(LREST_POD)

pkg:
	$(MKDIR) /tmp/pkgtestplan
	$(CP) -R * /tmp/pkgtestplan
	rm /tmp/pkgtestplan/oracle-database-operator.yaml
	$(CP) ../../../../oracle-database-operator.yaml /tmp/pkgtestplan/
	$(TAR) -C /tmp -cvf ~/pkgtestplan_$(DATE).tar pkgtestplan
	rm -rf /tmp/pkgtestplan



delsecrets:
	@echo "delete  $(LRSNAMESPACE) secrets"
	$(KUBECTL)  get secrets -n $(LRSNAMESPACE)|awk ' { print $$1 }'|grep -v NAME  |while read sec ; do \
		$(KUBECTL) delete secrets  $$sec -n $(LRSNAMESPACE) ; \
		done ; 
	@echo "delete $(PDBNAMESPACE) secrets"
	$(KUBECTL)  get secrets -n $(PDBNAMESPACE)|awk ' { print $$1 }'|grep -v NAME  |while read sec ; do \
		$(KUBECTL) delete secrets  $$sec -n $(PDBNAMESPACE) ; \
		done ; 
	$(KUBECTL) delete secrets db-ca db-tls -n  $(OPRNAMESPACE)


dellrestcascade:
	$(KUBECTL) delete lrest cdb-dev -n $(LRSNAMESPACE) 
	$(KUBECTL) get events -n $(LRSNAMESPACE)
	$(KUBECTL) get lrest -n $(LRSNAMESPACE) 
	$(KUBECTL) get lrpdb -n $(PDBNAMESPACE)

#============================== Config Map Section =======================#


define genconfigmap

echo "session_cached_cursors;100;spfile"                 >parameters.txt
echo "open_cursors;100;spfile"                           >>parameters.txt
echo "db_file_multiblock_read_count;16;spfile"           >>parameters.txt
echo "test_invalid_parameter;16;spfile"                  >>parameters.txt

$(KUBECTL) create  configmap $(CONFIG_MAP) -n $(PDBNAMESPACE) --from-file=./parameters.txt 
$(KUBECTL) describe configmap $(CONFIG_MAP) -n  $(PDBNAMESPACE)

endef

crecfgmap:
	$(call genconfigmap)	

delcfgmap:
	- $(KUBECTL) delete configmap $(CONFIG_MAP) -n $(PDBNAMESPACE)

map: delcfgmap crecfgmap


