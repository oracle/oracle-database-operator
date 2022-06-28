# Build ORDS Docker Image

In the below steps, we are building an ORDS Docker Image for ORDS Software.

The image built can be later pushed to a local repository to be used later for a deployment.

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

1. Clone the software using git:
```sh
[root@test-server oracle-database-operator]# git clone git@orahub.oci.oraclecorp.com:rac-docker-dev/oracle-database-operator.git
[root@test-server oracle-database-operator]# cd oracle-database-operator/ords/
```

2. Download the ORDS Software for required ORDS version. For Example: For ORDS Version 21.4.3, use this [link](https://www.oracle.com/tools/ords/ords-downloads-2143.html)

3. Copy the downloaded software .zip file to the current location.

4. Login to the registry: container-registry.oracle.com

**NOTE:** To login to this registry, you will need to the URL https://container-registry.oracle.com , Sign in, then click on "Java" and then accept the agreement.

```sh
docker login container-registry.oracle.com
``` 

5. Login to a repo where you want to push your docker image (if needed) to pull during deployment in your environment.

```sh
docker login <repo where you want to push the created docker ORDS image>
```

6. Build the docker image by using below command:

```sh
docker build -t oracle/ords-dboper:ords-21.4.3 .
```

7. Check the docker image details using:

```sh
docker images
```

8. Tag and push the image to your docker repository.

NOTE: We have the repo as `phx.ocir.io/<repo_name>/oracle/ords:21.4.3`. Please change as per your environment.

```sh
docker tag oracle/ords-dboper:ords-21.4.3 phx.ocir.io/<repo_name>/oracle/ords:21.4.3
docker push phx.ocir.io/<repo_name>/oracle/ords:21.4.3
```

9. Verify the image pushed to your docker repository.

You can refer to below sample output for above steps as well.

10. Create a Kubernetes Secret for your docker repository to pull the image during deployment using the below command:

```sh
kubectl create secret generic container-registry-secret --from-file=.dockerconfigjson=./.docker/config.json --type=kubernetes.io/dockerconfigjson -n oracle-database-operator-system
```

This Kubernetes secret will be provided in the .yaml file against the parameter `ordsImagePullSecret` to pull the ORDS Docker Image from your docker repository (if its a private repository).

## Sample Output

[Here](./ords_image.log) is the sample output for docker image created for ORDS version 21.4.3
