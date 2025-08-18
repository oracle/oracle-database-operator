# Using Oracle PrivateAI Controller with Oracle Database Operator for Kubernetes

Oracle PrivateAI Controller automates the deployment and usage of the Oracle PrivateAI Container. AI Container project aims to deliver to customers a lightweight containerized web service that provides an interface for performing inference on ONNX format models via REST. The AI container will help offload expensive AI computation (e.g., embedding generation) outside the database. This addresses requests made by AI Vector Search customers  who would prefer to use the database compute primarily for indexing and search.

Kubernetes provides infrastructure building blocks, such as compute, storage, and networks. Kubernetes makes the infrastructure available as code. which are a key resource object for managing and updating applications. Deployments enable declarative updates, ensuring a specified number of application replicas are running, and handle scaling, rolling updates, and rollbacks automatically. 


The PrivateAI controller in Oracle Database Operator deploys Oracle PrivateAI container as a deploymentset in the Kubernetes clusters, using Oracle PrivateAI Container image. The Oracle PrivateAI controller provides end-to-end automation of Oracle PrivateAI Container deployment in Kubernetes clusters.

## Using Oracle Database Operator PrivateAI Controller

Following sections provide the details for deploying Oracle PrivateAI container using Oracle Database Operator PrivateAI Controller with different use cases:

* [Prerequisites for running Oracle PrivateAI Controller](#prerequisites-for-running-oracle-privartai-controller)
* [Quick Start](#quick-start)
* [Accessing PrivateAI Container Pods](#accessing-the-privateai-container-pod-in-kubernetes)
* [Debugging and Troubleshooting](#debugging-and-troubleshooting)

**Note:** Before proceeding to the next section, you must complete the instructions given in each section, based on your enviornment, before proceeding to next section.

## Prerequisites for running Oracle PrivartAI Controller

**IMPORTANT:** You must make the changes specified in this section before you proceed to the next section.

### 1. Kubernetes Cluster: To deploy Oracle PrivateAI controller with Oracle Database Operator, you need a Kubernetes Cluster which can be one of the following: 

* A Cloud-based Kubernetes cluster, such as [OCI on Container Engine for Kubernetes (OKE)](https://www.oracle.com/cloud-native/container-engine-kubernetes/) or  
* An On-Premises Kubernetes Cluster, such as [Oracle Linux Cloud Native Environment (OLCNE)](https://docs.oracle.com/en/operating-systems/olcne/) cluster.

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

### 3. Oracle PrivateAI Container Image
The pre-built preivateAI container image is available on [Oracle Container Registry](https://container-registry.oracle.com/ords/f?p=113:10::::::) and fully supported by Oracle for production uses.

### 4. Create a namespace for the Oracle PrivateAI Setup

  Create a Kubernetes namespace named `pai`. All the resources belonging to the Oracle PrivateAI topology setup will be provisioned in this namespace named `pai`. For example:

  ```sh
  #### Create the namespace 
  kubectl create ns pai

  #### Check the created namespace 
  kubectl get ns
  ```

### 5. Create a configmap for the Oracle PrivateAI Deployment

In this step, a configmap will be created which has the details of the AI Model File. The configmap will be created according to the type of the PrivateAI Deployment. Below are examples of configmap used in the later examples:

- [Configmap using Single AI Model with HTTPS URL](./configmap_single_model_https.md) 
- [Configmap using Multiple AI Models with HTTPS URL](./configmap_multi_model_https.md) 
- [Configmap using Multiple AI Models on File System](./configmap_multi_model_filesystem.md) 

### 6. Reserve LoadBalancer Public IP

- The SSL certificate used during the PrivateAI Container Deployment will need a common name(hostname or IP) to be specified during the certificate creation.
- Later, for a secure communication with the PrivateAI Container Deployed in a Kuberentes Cluster, the client will use the same `cert.pem` file and will send the connection request to same hostname or IP.
- If you are deploying PrivateAI Container on an OKE cluster, you will need to reserve a Public IP in OCI. - OCI allows provisioning a Public LoadBalancer and assigning a reserved public ip to it. Please the [documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).
- To reserve a Public IP in OCI, refer to [OCI LoadBalancer Documentation](https://docs.public.oneportal.content.oci.oraclecloud.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm) for the details.
- Once you have reserved the Public IP, use this Public IP as `Common Name` while generating the openssl certificate in the next step.

**NOTE:** This step is required only if you are going to deploy the PrivateAI Container on OKE Cluster using OCI Public LoadBalancer.

**NOTE:** The option to reserve a Private IP and use that with an OCI Internal LoadBalancer is not available as of now. Please check the [documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

### 7. Create Kubernetes secret for the Oracle PrivateAI Deployment

**IMPORTANT:** Make sure the version of `openssl` in the Oracle PrivateAI image is compatible with the `openssl` version on the machine where you will run the openssl commands to generate the encrypted password file during the deployment.

Create a file `oml-ssl-pwd` with the password you want to use. This password will be used in the next step. The script [pai_secret.sh](./provisioning/pai_secret.sh) has the command to generate the required keys and an SSL certificate.

Use the Shell Script `pai_secret.sh` to create the required secrets for the Oracle PrivateAI Container Deployment. Run this file as below and enter the password when prompted.

In case of the Public LoadBalancer, use the reserved Public IP as the `common name`.

```sh
cd provisioning
echo "<password>" > oml-ssl-pwd
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

There are multiple use case possible for deploying the PrivateAI container in Kubernetes Cluster covered by below examples:

**NOTE:** All the below deployments are using an OCI OKE Cluster.

- [PrivateAI Container using OCI Public LoadBalancer](./deploy_privateai_publiclb.md) 
- PrivateAI Container using an Internal LoadBalancer 
  - [PrivateAI Container using Single AI Model with HTTPS URL and an Internal LoadBalancer](./deploy_privateai_internallb.md) 
  - [PrivateAI Container using Multiple AI Models with HTTPS URL and an Internal LoadBalancer](./deploy_privateai_multi_model_https_internallb.md) 
    - [Add New Model](./deploy_privateai_multi_model_https_internallb_add_model.md) 
    - [Remove an existing model](./deploy_privateai_multi_model_https_internallb_remove_model.md) 
  - [PrivateAI Container using Multiple AI Models on File System and an Internal LoadBalancer](./deploy_privateai_multi_model_filesystem_internallb.md)   

## Accessing the PrivateAI Container Pod in Kubernetes

**IMPORTANT:** This example assumes that you have an existing Oracle PrivateAI Container Deployment in the `pai` namespace in Kuberentes Cluster and you have the Reserved Public IP of the LoadBalancer.

Please refer to [this page](./access_privateai.md) for the details to access the PrivateAI Container Pod in Kubernetes.

## Debugging and Troubleshooting

Please refer to [this page](./debug_privateai.md) for the details to access the PrivateAI Container Pod in Kubernetes.
