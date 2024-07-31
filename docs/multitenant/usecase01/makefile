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
# the OnPremises operator. Although it has few functionality you can adapt to your needs  
# by adding much more targets.
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
# +-----------------------------+---------------------------------------------+
# |oracle-database-operator.yaml| Opertaor yaml file                          |
# +-----------------------------+---------------------------------------------+
# |cdb_secret.yaml              | Secret file for the rest server pod         |
# +-----------------------------+---------------------------------------------+
# |pdb_secret.yaml              | Secret file for the pdb creation            |
# +-----------------------------+---------------------------------------------+
# |tde_secret.yaml              | Secret file for the tablepsace enc.         |
# +-----------------------------+---------------------------------------------+
# |cdb_create.yaml              | Rest server pod creation                    |
# +-----------------------------+---------------------------------------------+
# |pdb_create.yaml              | Pluggable database creation                 |
# +-----------------------------+---------------------------------------------+
# |pdb_close.yaml               | Close pluggable database                    |
# +-----------------------------+---------------------------------------------+
# |pdb_open.yaml                | Open pluggable database                     |
# +-----------------------------+---------------------------------------------+
# |pdb_map.yaml                 | Map an existing pdb                         |
# +-----------------------------+---------------------------------------------+ 
# |oracle-database-operator.yaml| Database operator                           |
# +-----------------------------+---------------------------------------------+
# |Dockerfiles                  | Dockerfile for CBD                          |
# +-----------------------------+---------------------------------------------+
# |runOrdsSSL.sh                | Init script executed by Dockerfile          |
# +-----------------------------+---------------------------------------------+
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
# +-----------------------------+-------------------------------------+-------+
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
# |step10                       | Create pluggable database                   |
# |  |                          +---------------------------------------------+
# |  +---> checkpdb             | Monitor PDB status                          |
# +-----------------------------+---------------------------------------------+
# |step11                       | Close pluggable database                    |
# +-----------------------------+---------------------------------------------+
# |step12                       | Open pluggable database                     |
# +-----------------------------+---------------------------------------------+
# |step13                       | Map pluggable database                      |
# +-----------------------------+---------------------------------------------+
# | Before testing step13 delete the crd:                                     |
# |    kubectl delete pdb pdb1 -n oracle-database-operator-system             |
# +---------------------------------------------------------------------------+
# |                        DIAGNOSTIC  TARGETS                                |
# +-----------------------------+---------------------------------------------+
# | dump                        | Dump  pods info into a file                 |
# +-----------------------------+---------------------------------------------+
# | reloadop                    | Reload the db operator                      |
# +-----------------------------+---------------------------------------------+
# | login                       | Login into cdb pod                          |
# +-----------------------------+---------------------------------------------+


################ TAB 2 VARIABLES ############
OCIR=[...........YOUR REGISTRY...........]
OCIRPATH=[...PATH IN YOUR REGISTRY.....]/$(REST_SERVER)-dboper:$(ORDSVERSION)
#############################################
REST_SERVER=ords
ORDSVERSION=latest
DOCKER=/usr/bin/docker
KUBECTL=/usr/bin/kubectl
ORDS=/usr/local/bin/ords
CONFIG=/etc/ords/config
IMAGE=oracle/$(REST_SERVER)-dboper:$(ORDSVERSION)
DBOPERATOR=oracle-database-operator.yaml
URLPATH=/_/db-api/stable/database/pdbs/
OPENSSL=/usr/bin/openssl
ORDSPORT=8888
MAKE=/usr/bin/make
DOCKERFILE=../../../ords/Dockerfile
RUNSCRIPT=../../../ords/runOrdsSSL.sh
RM=/usr/bin/rm
CP=/usr/bin/cp
ECHO=/usr/bin/echo
NAMESPACE=oracle-database-operator-system
CERTMANAGER=https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml
CDB_SECRET=cdb_secret.yaml
PDB_SECRET=pdb_secret.yaml
TDE_SECRET=tde_secret.yaml
CDB=cdb_create.yaml
PDB=pdb_create.yaml
PDB_CLOSE=pdb_close.yaml
PDB_OPEN=pdb_open.yaml 
PDB_MAP=pdb_map.yaml
SKEY=tls.key
SCRT=tls.crt
CART=ca.crt
COMPANY=oracle
LOCALHOST=localhost
RESTPREFIX=cdb-dev

step1:   createimage
step2:   tagimage
step3:   push 
step4:   certmanager
step5:   dboperator
step6:   tlscert 
step7:   tlssecret 
step8:   dbsecret 
step9:   cdb
step10:  pdb
step11:  close 
step12:  open
step13:  map

checkstep9: checkcdb


createimage:
	$(CP) $(DOCKERFILE) . 
	$(CP) $(RUNSCRIPT) .
	$(DOCKER) build -t $(IMAGE) . 
	$(RM) ./Dockerfile ./runOrdsSSL.sh

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
	$(KUBECTL) apply -f $(DBOPERATOR)


#C: Country
#ST: State
#L: locality (city)
#O: Organization Name Organization Unit
#CN: Common Name

tlscert:
	 @echo "CREATING TLS CERTIFICATES"
	$(OPENSSL) genrsa -out ca.key 2048
	$(OPENSSL) req -new -x509 -days 365 -key ca.key -subj "/C=US/ST=California/L=SanFrancisco/O=$(COMPANY) /CN=$(RESTPREFIX)-$(REST_SERVER) /CN=$(LOCALHOST)  Root CA " -out ca.crt
	$(OPENSSL) req -newkey rsa:2048 -nodes -keyout $(SKEY) -subj "/C=US/ST=California/L=SanFrancisco/O=$(COMPANY) /CN=$(RESTPREFIX)-$(REST_SERVER) /CN=$(LOCALHOST)" -out server.csr
	$(ECHO) "subjectAltName=DNS:$(RESTPREFIX)-$(REST_SERVER),DNS:www.example.com" > extfile.txt
	$(OPENSSL) x509 -req -extfile extfile.txt -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out $(SCRT)

tlssecret:
	@echo "CREATING TLS SECRETS"
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(NAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(NAMESPACE)

dbsecret:
	@echo "CREATING DB SECRETS"
	$(KUBECTL) apply -f $(CDB_SECRET) -n $(NAMESPACE)
	$(KUBECTL) apply -f $(PDB_SECRET) -n $(NAMESPACE)
	$(KUBECTL) apply -f $(TDE_SECRET) -n $(NAMESPACE)

cdb:
	@echo "CREATING REST SRV POD"
	$(KUBECTL) apply -f $(CDB)

checkcdb:
	$(KUBECTL) logs -f `$(KUBECTL) get pods -n $(NAMESPACE)|grep $(REST_SERVER)|cut -d ' ' -f 1` -n $(NAMESPACE)

pdb:
	$(KUBECTL) apply -f $(PDB)

close:
	$(KUBECTL) apply -f $(PDB_CLOSE)

open:
	$(KUBECTL) apply -f $(PDB_OPEN)

map:
	$(KUBECTL) apply -f $(PDB_MAP)

checkpdb:
	$(KUBECTL) get pdbs -n $(NAMESPACE)

dump:
	@$(eval TMPSP := $(shell date "+%y%m%d%H%M%S" ))
	@$(eval DIAGFILE := ./opdmp.$(TMPSP))
	@>$(DIAGFILE)
	@echo "OPERATOR DUMP"  >> $(DIAGFILE)
	@echo "~~~~~~~~~~~~~"  >> $(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(NAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1` -n $(NAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(NAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1 | cut -d ' ' -f 1` -n $(NAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(NAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1` -n $(NAMESPACE) >>$(DIAGFILE)
	@echo "CDB LOG DUMP" >> $(DIAGFILE)
	@echo "~~~~~~~~" >> $(DIAGFILE)
	$(KUBECTL) logs `$(KUBECTL) get pods -n $(NAMESPACE)|grep $(REST_SERVER)|  cut -d ' ' -f 1` -n $(NAMESPACE)  >>$(DIAGFILE)
	@echo "SECRET DMP" >>$(DIAGFILE)
	@echo "~~~~~~~~" >> $(DIAGFILE)
	$(KUBECTL) get secrets  -o yaml -n $(NAMESPACE) >> $(DIAGFILE)
	@echo "CDB/PDB DMP" >> $(DIAGFILE)
	$(KUBECTL) get pdbs  -o yaml -n $(NAMESPACE) >> $(DIAGFILE)
	$(KUBECTL) get cdb  -o yaml -n $(NAMESPACE) >> $(DIAGFILE)
	@echo "CLUSTER INFO" >>  $(DIAGFILE)
	$(KUBECTL) get nodes -o wide
	$(KUBECTL) get svc --namespace=kube-system

reloadop:
	echo "RESTARTING OPERATOR"
	$(eval OP1 := $(shell $(KUBECTL) get pods -n $(NAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1 ))
	$(eval OP2 := $(shell $(KUBECTL) get pods -n $(NAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1|cut  -d ' ' -f 1 ))
	$(eval OP3 := $(shell $(KUBECTL) get pods -n $(NAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1 ))
	$(KUBECTL) get pod $(OP1) -n $(NAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP2) -n $(NAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP3) -n $(NAMESPACE) -o yaml | kubectl replace --force -f -

login:
	$(KUBECTL) exec -it  `$(KUBECTL) get pods -n $(NAMESPACE)|grep $(REST_SERVER)|cut -d ' ' -f 1` -n $(NAMESPACE) bash

