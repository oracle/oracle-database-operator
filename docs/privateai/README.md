# Using Oracle PrivateAI Controller with Oracle Database Operator for Kubernetes

Oracle PrivateAI Controller streamlines the deployment and use of the Oracle Private AI Service Container. The Private AI Services Container is a lightweight, containerized web service that provides an interface for performing inference on ONNX format models using REST. This container can run on your laptop, in your data center, or on computer nodes in the public cloud.
For details, refer [Private AI Services Container documentaion](https://docs.oracle.com/en/database/oracle/oracle-database/26/prvai/index.html)

Kubernetes provides foundational infrastructure components—including compute, storage, and networking—exposed programmatically as code. With Kubernetes, resources such as deployments play a central role in managing and updating applications. Deployments support declarative updates, ensuring the desired number of application replicas are running, and automate tasks such as scaling, rolling updates, and rollbacks. 

Within this environment, the PrivateAI controller in the Oracle Database Operator automates the deployment of the Oracle Private AI Service Container to Kubernetes clusters, utilizing the Oracle Private AI Service Container image. The Oracle PrivateAI controller delivers complete automation for deploying Oracle Private AI Service Containers in Kubernetes clusters. 

### Oracle Private AI Controller Capabilities:
#### Automated Container Deployment
Automates the deployment of Oracle PrivateAI Container in Kubernetes clusters using the Oracle Database Operator.Ensures end-to-end automation for lifecycle management (deployment, scaling, updates, rollback). 
#### Inference as a Service
Provides a lightweight, containerized web service interface for performing AI inference on ONNX models via REST APIs. 
#### Compute Offloading
Offloads compute-intensive AI operations (e.g., embedding generation) from the database, freeing up resources for database-native functions like indexing and search. 
#### Integration with Kubernetes
Utilizes Kubernetes resource constructs (deployments, sets) for robust container orchestration.Leverages Kubernetes infrastructure primitives (compute, storage, networking) and makes them available as code (Infrastructure as Code).Enables declarative updates, scaling, rolling updates, and rollbacks for high availability and simplified management.
#### Image-based Deployment
Uses Oracle PrivateAI Container images for reproducible and consistent environment provisioning. 
1. Targeted for AI Vector Search Customers:   
2. Designed to address needs of customers who require efficient handling of AI tasks external to the database, specifically in the context of AI-powered vector search use cases.

## Using Oracle Database Operator PrivateAI Controller

Following sections provide the details for deploying Oracle Private AI Service Container using Oracle Database Operator PrivateAI Controller with different use cases:

* [Prerequisites for running Oracle PrivateAI Controller](#prerequisites-for-running-oracle-privartai-controller)
* [Quick Start](#quick-start)
* [Accessing Private AI Service Container Pods](#accessing-the-privateai-container-pod-in-kubernetes)
* [Debugging and Troubleshooting](#debugging-and-troubleshooting)

**Note:** Before proceeding to the next section, you must complete the instructions given in each section, based on your enviornment, before proceeding to next section.

## Prerequisites for running Oracle PrivartAI Controller

**IMPORTANT:** You must make the changes specified in this section before you proceed to the next section.

### 1. Kubernetes Cluster: To deploy Oracle PrivateAI controller with Oracle Database Operator, you need a Kubernetes Cluster which can be one of the following: 

* A Cloud-based Kubernetes cluster, such as [OCI on Container Engine for Kubernetes (OKE)](https://www.oracle.com/cloud-native/container-engine-kubernetes/) or  
* An On-Premises Kubernetes Cluster, such as [Oracle CNE)](https://docs.oracle.com/en/operating-systems/olcne/) cluster.

To use Oracle PrivateAI Controller, ensure that your system is provisioned with a supported Kubernetes release. Refer to the [Release Status Section](../../README.md#release-status).

#### Mandatory roles and privileges requirements for Oracle PrivateAI Controller 

  Oracle PrivateAI Controller uses Kubernetes objects such as :-

  | Resources | Verbs |
  | --- | --- |
  | Pods | create delete get list patch update watch | 
  | Containers | create delete get list patch update watch |
  | PersistentVolumeClaims | create delete get list patch update watch | 
  | Services | create delete get list patch update watch | 
  | Secrets | create delete get list patch update watch | 
  | Events | create patch |

### 2. Deploy Oracle Database Operator

To deploy Oracle Database Operator in a Kubernetes cluster, go to the section [Install Oracle DB Operator](../../README.md#install-oracle-db-operator) in the README.md, and complete the operator deployment before you proceed further. If you have already deployed the operator, then proceed to the next section.

**IMPORTANT:** Make sure you have completed the steps for [Role Binding for access management](../../README.md#role-binding-for-access-management) as well before installing the Oracle DB Operator. 

### 3. Create a namespace for the Oracle PrivateAI Setup

  Create a Kubernetes namespace named `pai`. All the resources belonging to the Oracle PrivateAI topology setup will be provisioned in this namespace named `pai`. For example:

  ```sh
  #### Create the namespace 
  kubectl create ns pai

  #### Check the created namespace 
  kubectl get ns
  ```

Note: If you are using a different namespace, make sure to update any references from the `pai` namespace to the one you intend to use.

### 4. Oracle Private AI Service Container Image
The pre-built Private AI Service container image is available on [Oracle Container Registry](https://container-registry.oracle.com/) under `Database->private-ai` repository and fully supported by Oracle for production uses.

1. Log into Oracle Container Registry and accept the license agreement for the Private AI Service container image; ignore if you have accepted the license agreement already.

2. If you have not already done so, create an image pull secret for the Oracle Container Registry:
```
$ kubectl create secret docker-registry oracle-container-registry-secret --docker-server=container-registry.oracle.com --docker-username='<oracle-sso-email-address>' --docker-password='<container-registry-auth-token>' --docker-email='<oracle-sso-email-address>' -n pai
```
Note: Generate the auth token from user profile section on top right of the page after logging into container-registry.oracle.com

3. This secret can also be created from the docker config.json or from podman auth.json after a successful login  
```
docker login container-registry.oracle.com
kubectl create secret generic oracle-container-registry-secret  --from-file=.dockerconfigjson=.docker/config.json --type=kubernetes.io/dockerconfigjson -n pai
```
or

```
podman login container-registry.oracle.com
kubectl create secret generic oracle-container-registry-secret  --from-file=.dockerconfigjson=${XDG_RUNTIME_DIR}/containers/auth.json --type=kubernetes.io/ dockerconfigjson  -n pai
```

### 5. Create a configmap for the Oracle PrivateAI Deployment

In this step, a configmap will be created which has the details of the AI Model File. The configmap will be created according to the type of the Private AI Deployment. Below are examples of configmap used in the later examples:

You need to download the models and you need to make sur they are available through HTTPS access such as object store pre-authenticated URL. For the model details, you can refer the section `Available Embedding Models' in [Private AI Services Container docuemntation](https://docs.oracle.com/en/database/oracle/oracle-database/26/prvai/index.html).

- [Configmap using Single AI Model with HTTPS URL](./configmap_single_model_https.md) 
- [Configmap using Multiple AI Models with HTTPS URL](./configmap_multi_model_https.md) 
- [Configmap using Multiple AI Models on File System](./configmap_multi_model_filesystem.md) 

### 6. Reserve LoadBalancer Public IP

- The SSL certificate used during the Private AI Service Container Deployment will need a common name(hostname or IP) to be specified during the certificate creation.
- Later, for a secure communication with the Private AI Service Container Deployed in a Kuberentes Cluster, the client will use the same `cert.pem` file and will send the connection request to same hostname or IP.
- If you are deploying Private AI Service Container on an OKE cluster, you will need to reserve a Public IP in OCI. - OCI allows provisioning a Public LoadBalancer and assigning a reserved public ip to it. Please the [documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).
- To reserve a Public IP in OCI, refer to [OCI LoadBalancer Documentation](https://docs.public.oneportal.content.oci.oraclecloud.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm) for the details.
- Once you have reserved the Public IP, use this Public IP as `Common Name` while generating the openssl certificate in the next step.

**NOTE:** This step is required only if you are going to deploy the Private AI Service Container on OKE Cluster using OCI Public LoadBalancer.

**NOTE:** The option to reserve a Private IP and use that with an OCI Internal LoadBalancer is not available as of now. Please check the [documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

### 7. Create Kubernetes secret for the Oracle PrivateAI Deployment

**IMPORTANT:** Make sure the version of `openssl` in the Oracle Private AI Services container image is compatible with the `openssl` version on the machine where you will run the openssl commands to generate the encrypted password file during the deployment.

Create a file `privateai-ssl-pwd` with the password you want to use. This password will be used in the next step. The script [pai_secret.sh](./provisioning/pai_secret.sh) has the command to generate the required keys and an SSL certificate.

Use the Shell Script `pai_secret.sh` to create the required secrets for the Oracle Private AI Service Container Deployment. Run this file as below and enter the password when prompted.

In case of the Public LoadBalancer, use the reserved Public IP as the `common name`.
In case of the Internal LoadBalancer, do not provide any value for `common name`.

```sh
cd provisioning
echo "<password>" > privateai-ssl-pwd
./pai_secret.sh
```

**NOTE:** In case of the Internal LoadBalancer, we can not use a reserved Private IP. In this case, you can leave `common name` empty.

Use below command to check the Kubernetes Secret Created:

```sh
kubectl get secret -n pai
kubectl describe secret paisecret -n pai
```

After you have the above prerequisites completed, you can proceed to the next section for your environment to deploy the Oracle PrivateAI Controller.


## Quick Start

There are multiple use case possible for deploying the Private AI Service Container in Kubernetes Cluster covered by below examples:

**NOTE:** All the below deployments are using an OCI OKE Cluster.

- **Private AI Service Container using OCI Public LoadBalancer**
  - [Change the Private AI Service Container Image](./change_privateai_container_image.md) 
- [Private AI Service Container using OCI Public LoadBalancer without configmap](./deploy_privateai_publiclb_without_configmap.md) 
- [Private AI Service Container using OCI Public LoadBalancer with multiple replica](./deploy_privateai_publiclb_multi_replica.md) 
  - [Scale-out](./scale_up_privateai.md)
  - [Scale-in](./scale_in_privateai.md)
- [Private AI Service Container using OCI Public LoadBalancer with worker node selection](./deploy_privateai_publiclb_worker_node.md) 
- [Private AI Service Container using OCI Public LoadBalancer with memory and cpu limits for pods](./deploy_privateai_publiclb_mem_cpu_limit.md) 
  - [Change the memory and cpu limits for pods](./change_privateai_publiclb_mem_cpu_limit.md) 
- **Private AI Service Container using an Internal LoadBalancer**
  - [Private AI Service Container using Single AI Model with HTTPS URL and an Internal LoadBalancer](./deploy_privateai_internallb.md) 
  - [Private AI Service Container using Multiple AI Models with HTTPS URL and an Internal LoadBalancer](./deploy_privateai_multi_model_https_internallb.md) 
    - [Add New Model](./deploy_privateai_multi_model_https_internallb_add_model.md) 
    - [Remove an existing model](./deploy_privateai_multi_model_https_internallb_remove_model.md) 
  - [Private AI Service Container using Multiple AI Models on File System and an Internal LoadBalancer](./deploy_privateai_multi_model_filesystem_internallb.md)   

## Accessing the Private AI Service Container Pod in Kubernetes

**IMPORTANT:** This example assumes that you have an existing Oracle Private AI Service Container Deployment in the `pai` namespace in Kuberentes Cluster and you have the Reserved Public IP of the LoadBalancer.

Please refer to [this page](./access_privateai.md) for the details to access the Private AI Service Container Pod in Kubernetes.

## Debugging and Troubleshooting

Please refer to [this page](./debug_privateai.md) for the details to access the Private AI Service Container Pod in Kubernetes.
