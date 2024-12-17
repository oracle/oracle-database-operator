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
# the mutlitenant operator. Although it has few functionality you can adapt to your needs  
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
# |cdb_create.yaml              | Rest server pod creation                    |
# +-----------------------------+---------------------------------------------+
# |pdb_create.yaml              | Pluggable database creation                 |
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
# |                        DIAGNOSTIC  TARGETS                                |
# +-----------------------------+---------------------------------------------+
# | dump                        | Dump  pods info into a file                 |
# +-----------------------------+---------------------------------------------+
# | reloadop                    | Reload the db operator                      |
# +-----------------------------+---------------------------------------------+
# | login                       | Login into cdb pod                          |
# +-----------------------------+---------------------------------------------+


################ TAB 2 VARIABLES ############
REST_SERVER=ords
ORDSVERSION=latest

OCIR=[container registry]
OCIRPATH=$(REST_SERVER)-dboper:$(ORDSVERSION)

#examples:
#OCIR=lin.ocir.io
#OCIRPATH=/sampletenancy/samplepath/sampledir/$(REST_SERVER)-dboper:$(ORDSVERSION)
#############################################
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
DOCKERFILE=../../../../ords/Dockerfile
RUNSCRIPT=../../../../ords/runOrdsSSL.sh
RM=/usr/bin/rm
CP=/bin/cp
ECHO=/usr/bin/echo
CERTMANAGER=https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml
CDB_SECRET_YAML=cdb_secret.yaml
PDB_SECRET_YAML=pdb_secret.yaml
TDE_SECRET_YAML=tde_secret.yaml
CDB_NAMESPACE_YAML=ns_namespace_cdb.yaml
PDB_NAMESPACE_YAML=ns_namespace_pdb.yaml
OPR_NAMESPACE=oracle-database-operator-system
PDB_NAMESPACE=$(shell grep namespace $(PDB_NAMESPACE_YAML)  |cut -d: -f 2| tr -d ' ')
CDB_NAMESPACE=$(shell grep namespace $(CDB_NAMESPACE_YAML)  |cut -d: -f 2| tr -d ' ')
CDB=cdb_create.yaml
PDB=pdb_create.yaml
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

checkstep9: checkcdb


createimage:
	@echo "BUILDING CDB IMAGES"
	$(CP) $(DOCKERFILE) .
	$(CP) $(RUNSCRIPT) .
	$(DOCKER) build -t $(IMAGE) . 

createimageproy:
	@echo "BUILDING CDB IMAGES"
	$(CP) $(DOCKERFILE) .
	$(CP) $(RUNSCRIPT) .
	$(DOCKER) build --build-arg https_proxy=$(HTTPS_PROXY) --build-arg http_proxy=$(HTTP_PROXY) -t $(IMAGE) . 

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

namespace:
	$(KUBECTL) get namespaces
	$(KUBECTL) apply -f  $(CDB_NAMESPACE_YAML)
	$(KUBECTL) apply -f  $(PDB_NAMESPACE_YAML)
	$(KUBECTL) get namespaces


tlscert:
	@echo "CREATING TLS CERTIFICATES"
	$(OPENSSL) genrsa -out ca.key 2048
	$(OPENSSL) req -new -x509 -days 365 -key ca.key -subj "/C=US/ST=California/L=SanFrancisco/O=$(COMPANY) /CN=$(RESTPREFIX)-$(REST_SERVER).$(CDB_NAMESPACE) /CN=$(LOCALHOST)  Root CA " -out ca.crt
	$(OPENSSL) req -newkey rsa:2048 -nodes -keyout $(SKEY) -subj "/C=US/ST=California/L=SanFrancisco/O=$(COMPANY) /CN=$(RESTPREFIX)-$(REST_SERVER).$(CDB_NAMESPACE) /CN=$(LOCALHOST)" -out server.csr
	$(ECHO) "subjectAltName=DNS:$(RESTPREFIX)-$(REST_SERVER).$(CDB_NAMESPACE),DNS:www.example.com" > extfile.txt
	$(OPENSSL) x509 -req -extfile extfile.txt -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out $(SCRT)


tlssecret:
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(CDB_NAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)" -n $(CDB_NAMESPACE)
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(PDB_NAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)"  -n $(PDB_NAMESPACE)
	$(KUBECTL) create secret tls db-tls --key="$(SKEY)" --cert="$(SCRT)"  -n $(OPR_NAMESPACE)
	$(KUBECTL) create secret generic db-ca --from-file="$(CART)"  -n $(OPR_NAMESPACE)


dbsecret:
	@echo "CREATING DB SECRETS"
	$(KUBECTL) apply -f $(CDB_SECRET_YAML) 
	$(KUBECTL) apply -f $(PDB_SECRET_YAML) 
	$(KUBECTL) apply -f $(TDE_SECRET_YAML) 


cdb:
	@echo "CREATING REST SRV POD"
	$(KUBECTL) apply -f $(CDB)

checkcdb:
	$(KUBECTL) logs -f `$(KUBECTL) get pods -n $(CDB_NAMESPACE)|grep $(REST_SERVER)|cut -d ' ' -f 1` -n $(CDB_NAMESPACE)

pdb:
	$(KUBECTL) apply -f $(PDB)

checkpdb:
	$(KUBECTL) get pdbs -n $(OPR_NAMESPACE)

dump:
	@$(eval TMPSP := $(shell date "+%y%m%d%H%M%S" ))
	@$(eval DIAGFILE := ./opdmp.$(TMPSP))
	@>$(DIAGFILE)
	@echo "OPERATOR DUMP"  >> $(DIAGFILE)
	@echo "~~~~~~~~~~~~~"  >> $(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1` -n $(OPR_NAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1 | cut -d ' ' -f 1` -n $(OPR_NAMESPACE) >>$(DIAGFILE)
	$(KUBECTL) logs   pod/`$(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1` -n $(OPR_NAMESPACE) >>$(DIAGFILE)
	@echo "CDB LOG DUMP" >> $(DIAGFILE)
	@echo "~~~~~~~~" >> $(DIAGFILE)
	$(KUBECTL) logs `$(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep $(REST_SERVER)|  cut -d ' ' -f 1` -n $(OPR_NAMESPACE)  >>$(DIAGFILE)
	@echo "SECRET DMP" >>$(DIAGFILE)
	@echo "~~~~~~~~" >> $(DIAGFILE)
	$(KUBECTL) get secrets  -o yaml -n $(OPR_NAMESPACE) >> $(DIAGFILE)
	@echo "CDB/PDB DMP" >> $(DIAGFILE)
	$(KUBECTL) get pdbs  -o yaml -n $(OPR_NAMESPACE) >> $(DIAGFILE)
	$(KUBECTL) get cdb  -o yaml -n $(OPR_NAMESPACE) >> $(DIAGFILE)
	@echo "CLUSTER INFO" >>  $(DIAGFILE)
	$(KUBECTL) get nodes -o wide
	$(KUBECTL) get svc --namespace=kube-system

reloadop:
	echo "RESTARTING OPERATOR"
	$(eval OP1 := $(shell $(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep oracle-database-operator-controller|head -1|cut  -d ' ' -f 1 ))
	$(eval OP2 := $(shell $(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1|cut  -d ' ' -f 1 ))
	$(eval OP3 := $(shell $(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep oracle-database-operator-controller|tail -1|cut  -d ' ' -f 1 ))
	$(KUBECTL) get pod $(OP1) -n $(OPR_NAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP2) -n $(OPR_NAMESPACE) -o yaml | kubectl replace --force -f -
	$(KUBECTL) get pod $(OP3) -n $(OPR_NAMESPACE) -o yaml | kubectl replace --force -f -

login:
	$(KUBECTL) exec -it  `$(KUBECTL) get pods -n $(CDB_NAMESPACE) |grep $(REST_SERVER)|cut -d ' ' -f 1` -n $(CDB_NAMESPACE) bash

cdblog:
	$(KUBECTL) logs -f `$(KUBECTL) get pods -n $(CDB_NAMESPACE)|grep $(REST_SERVER)|cut -d ' ' -f 1` -n $(CDB_NAMESPACE)



xlog1:
	$(KUBECTL) logs -f  pod/`$(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep oracle-database-operator-controller|head -1|cut -d ' ' -f 1` -n $(OPR_NAMESPACE)

xlog2:
	$(KUBECTL) logs -f  pod/`$(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep oracle-database-operator-controller|head -2|tail -1|cut -d ' ' -f 1` -n $(OPR_NAMESPACE)

xlog3:
	$(KUBECTL) logs -f  pod/`$(KUBECTL) get pods -n $(OPR_NAMESPACE)|grep oracle-database-operator-controller|tail -1|cut -d ' ' -f 1` -n $(OPR_NAMESPACE)

checkdep:
	 $(KUBECTL) api-resources --verbs=list --namespaced -o name | xargs -n 1 $(KUBECTL) get -n $(OPR_NAMESPACE)
	 $(KUBECTL) api-resources --verbs=list --namespaced -o name | xargs -n 1 $(KUBECTL) get -n $(CBD_NAMESPACE)
	 $(KUBECTL) api-resources --verbs=list --namespaced -o name | xargs -n 1 $(KUBECTL) get -n $(PDB_NAMESPACE)



