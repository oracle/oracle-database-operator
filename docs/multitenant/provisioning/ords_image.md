<span style="font-family:Liberation mono; font-size:0.8em; line-height: 1.2em">

# Build ORDS Docker Image

This file contains the steps to create an ORDS based image to be used solely by the PDB life cycle multitentant controllers. 

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

#### Clone the software using git:

> Under directory ./oracle-database-operator/ords you will find the [Dockerfile](../../../ords/Dockerfile) and [runOrdsSSL.sh](../../../ords/runOrdsSSL.sh) required to build the image.

```sh
 git clone git@orahub.oci.oraclecorp.com:rac-docker-dev/oracle-database-operator.git
 cd oracle-database-operator/ords/
```

#### Login to the registry: container-registry.oracle.com

**NOTE:** To login to this registry, you will need to the URL https://container-registry.oracle.com , Sign in, then click on "Java" and then accept the agreement.

```bash
docker login container-registry.oracle.com
```

#### Login to the your container registry

Login to a repo where you want to push your docker image (if needed) to pull during deployment in your environment.

```bash
docker login <repo where you want to push the created docker ORDS image>
```

#### Build the image

Build the docker image by using below command:

```bash
docker build -t oracle/ords-dboper:latest .
```
> If your are working behind a proxy mind to specify https_proxy and http_proxy during image creation  

Check the docker image details using:

```bash
docker images
```

> OUTPUT EXAMPLE
```bash
REPOSITORY                                                           TAG                 IMAGE ID            CREATED             SIZE
oracle/ords-dboper                                                   latest              fdb17aa242f8        4 hours ago         1.46GB

```

#### Tag and push the image 

Tag and push the image to your image repository.

NOTE: We have the repo as `phx.ocir.io/<repo_name>/oracle/ords:latest`. Please change as per your environment.

```bash
docker tag oracle/ords-dboper:ords-latest phx.ocir.io/<repo_name>/oracle/ords:latest
docker push phx.ocir.io/<repo_name>/oracle/ords:latest
```

#### In case of private image

If you the image not be public then yuo need to create a secret containing the password of your image repository.
Create a Kubernetes Secret for your docker repository to pull the image during deployment using the below command:

```bash
kubectl create secret generic container-registry-secret --from-file=.dockerconfigjson=./.docker/config.json --type=kubernetes.io/dockerconfigjson -n oracle-database-operator-system
```

Use the parameter `ordsImagePullSecret` to specify the container secrets in pod creation yaml file

#### [Image createion example](../usecase01/logfiles/BuildImage.log)


</span>
