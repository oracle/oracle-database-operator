<span style="font-family:Liberation mono; font-size:0.8em; line-height: 1.2em">

# Build ORDS Docker Image

In the below steps, we are building an ORDS Docker Image for ORDS Software. The image built can be later pushed to a local repository to be used later for a deployment.

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

1. Clone the software using git:
```sh
 git clone git@orahub.oci.oraclecorp.com:rac-docker-dev/oracle-database-operator.git
 cd oracle-database-operator/ords/
```

2. Login to the registry: container-registry.oracle.com

**NOTE:** To login to this registry, you will need to the URL https://container-registry.oracle.com , Sign in, then click on "Java" and then accept the agreement.

```bash
docker login container-registry.oracle.com
``` 

3. Login to a repo where you want to push your docker image (if needed) to pull during deployment in your environment.

```bash
docker login <repo where you want to push the created docker ORDS image>
```

4. Build the docker image by using below command:

```bash
docker build -t oracle/ords-dboper:ords-latest .
```

5. Check the docker image details using:

```bash
docker images
```

6. Tag and push the image to your docker repository.

NOTE: We have the repo as `phx.ocir.io/<repo_name>/oracle/ords:latest`. Please change as per your environment.

```bash
docker tag oracle/ords-dboper:ords-latest phx.ocir.io/<repo_name>/oracle/ords:latest
docker push phx.ocir.io/<repo_name>/oracle/ords:latest
```

7. Verify the image pushed to your docker repository.

You can refer to below sample output for above steps as well.

8. Create a Kubernetes Secret for your docker repository to pull the image during deployment using the below command:

```bash
kubectl create secret generic container-registry-secret --from-file=.dockerconfigjson=./.docker/config.json --type=kubernetes.io/dockerconfigjson -n oracle-database-operator-system
```

This Kubernetes secret will be provided in the .yaml file against the parameter `ordsImagePullSecret` to pull the ORDS Docker Image from your docker repository (if its a private repository).

## Sample Output

[Here](./ords_image.log) is the sample output for docker image created for ORDS  latest version
