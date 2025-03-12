<span style="font-family:Liberation mono; font-size:0.9em; line-height: 1.1em">

### Quick Oke creation script 

Use this script to create quickly an OKE cluster in your OCI. 

#### Prerequisties:
- ocicli is properly configured on your client 
- make is installed on your client 
- vnc is already configured
- ssh key is configured (public key available under directory ~/.ssh)
- edit make providing all the information about your compartment, vnc,subnet,lb subnet and nd subnet (exported variables in the header section)


#### Execution:

```bash 
make all
```

Monitor the OKE from OCI console 

#### Makefile 
```makefile
.EXPORT_ALL_VARIABLES:

export CMPID=[.... COMPARTMENT ID.............]
export VNCID=[.... VNC ID ....................]
export ENDID=[.... SUBNET END POINT ID .......]
export LBSID=[.....LB SUBNET ID...............]
export NDSID=[.....NODE SUBNET ID.............]


#ssh public key
export KEYFL=~/.ssh/id_rsa.pub

#cluster version 
export KSVER=v1.27.2

#cluster name 
export CLUNM=myoke

#pool name 
export PLNAM=Pool1

#logfile
export LOGFILE=./clustoke.log

#shape 
export SHAPE=VM.Standard.E4.Flex

OCI=/home/oracle/bin/oci
CUT=/usr/bin/cut
KUBECTL=/usr/bin/kubectl
CAT=/usr/bin/cat

all: cluster waitcluster pool waitpool config desccluster

cluster:
	@echo " - CREATING CLUSTER "
	@$(OCI) ce cluster create \
    --compartment-id $(CMPID) \
    --kubernetes-version $(KSVER) \
    --name $(CLUNM) \
    --vcn-id $(VNCID) \
    --endpoint-subnet-id $(ENDID) \
    --service-lb-subnet-ids '["'$(LBSID)'"]' \
    --endpoint-public-ip-enabled true \
    --persistent-volume-freeform-tags '{"$(CLUNM)" : "OKE"}' 1>$(LOGFILE) 2>&1

waitcluster:
	@while [ `$(OCI) ce cluster list  --compartment-id $(CMPID)  \
        --name $(CLUNM) --lifecycle-state ACTIVE --query data[0].id \
        --raw-output |wc -l ` -eq 0 ] ; do sleep 5 ; done
	@echo " - CLUSTER CREATED"


pool:
	@echo " - CREATING POOL"
	@$(eval PBKEY :=$(shell $(CAT) $(KEYFL)|grep -v " PUBLIC KEY"))
	@$(OCI) ce node-pool create \
        --cluster-id `$(OCI) ce cluster list --compartment-id $(CMPID) \
              --name $(CLUNM) --lifecycle-state ACTIVE --query data[0].id --raw-output` \
        --compartment-id $(CMPID) \
        --kubernetes-version $(KSVER) \
        --name $(PLNAM) \
        --node-shape $(SHAPE) \
        --node-shape-config '{"memoryInGBs": 8.0, "ocpus": 1.0}' \
        --node-image-id `$(OCI) compute image list \
              --operating-system 'Oracle Linux' --operating-system-version 7.9 \
              --sort-by TIMECREATED --compartment-id $(CMPID) --shape $(SHAPE) \
              --query data[1].id --raw-output` \
        --node-boot-volume-size-in-gbs 50 \
	--ssh-public-key "$(PBKEY)" \
        --size 3 \
        --placement-configs '[{"availabilityDomain": "'`oci iam availability-domain list \
              --compartment-id $(CMPID) \
              --query data[0].name --raw-output`'", "subnetId": "'$(NDSID)'"}]' 1>>$(LOGFILE) 2>&1

waitpool:
	$(eval CLSID :=$(shell $(OCI) ce cluster list --compartment-id $(CMPID) \
        --name $(CLUNM) --lifecycle-state ACTIVE --query data[0].id --raw-output))
	@while [ `$(OCI) ce node-pool list --compartment-id $(CMPID) \
        --lifecycle-state ACTIVE  --cluster-id $(CLSID) \
        --query data[0].id --raw-output |wc -l ` -eq 0 ] ; do sleep 5 ; done
	@sleep 10 
	$(eval PLLID :=$(shell $(OCI) ce node-pool list --compartment-id $(CMPID) \
          --lifecycle-state ACTIVE  --cluster-id $(CLSID) --query data[0].id --raw-output))
	@echo " - POOL CREATED"

config:
	@$(OCI) ce cluster create-kubeconfig --cluster-id \
              `$(OCI) ce cluster list \
              --compartment-id $(CMPID)  --name $(CLUNM) --lifecycle-state ACTIVE \
              --query data[0].id --raw-output` \
         --file $(HOME)/.kube/config --region  \
           `$(OCI) ce cluster list \
              --compartment-id $(CMPID)  --name $(CLUNM) --lifecycle-state ACTIVE \
              --query data[0].id --raw-output|$(CUT) -f4 -d. ` \
          --token-version 2.0.0  --kube-endpoint PUBLIC_ENDPOINT
	@echo " - KUBECTL PUBLIC ENDPOINT CONFIGURED"
	

desccluster:
	@$(eval TMPSP := $(shell date "+%y/%m/%d:%H:%M" ))
	$(KUBECTL) get nodes -o wide
	$(KUBECTL) get storageclass

checkvol:
	$(OCI) bv volume list \
  --compartment-id $(CMPID) \
  --lifecycle-state AVAILABLE \
  --query 'data[?"freeform-tags".stackgres == '\''OKE'\''].id'
```

</span>
