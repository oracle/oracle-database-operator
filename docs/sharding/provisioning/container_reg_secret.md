# Create Kubernetes Secret to pull image from Oracle Container Registry
In order to pull an image from Oracle Container Registry, you need to create Kubernetes Secret with the Oracle Container Registry credential. This Kubernetes Secret will be used during the deployment to pull the corresponding Container Image. You need to following steps to create the Kuberetes Secret:

- Log into Oracle Container Registry and accept the license agreement for the container image you want to pull from Oracle Container Registry. You can ignore this step if you have accepted the license agreement already.

## Create a Secret by providing credentials on the command line

Create an image pull secret `ocr-reg-cred` for the Oracle Container Registry by providing the credentials on the command line as below:

```
$ kubectl create secret docker-registry ocr-reg-cred --docker-server=container-registry.oracle.com --docker-username='<Your Username for Oracle Container Registry Login>' --docker-password='<container-registry-auth-token for your account>' --docker-email='<Email address for your account>' -n <namespace>
```
**Note:** Generate the auth token from user profile section on top right of the page after logging into `container-registry.oracle.com`

## Create a Secret based on existing credentials 

A Kubernetes cluster uses the Secret of `kubernetes.io/dockerconfigjson` type to authenticate with a container registry to pull a private image.

If you already ran docker login, you can copy that credential into Kubernetes:
So, after a successful login to Oracle Container Registry, the Kubernetes secret can also be created from the docker config.json or from podman auth.json as below:
```
docker login container-registry.oracle.com
kubectl create secret generic ocr-reg-cred --from-file=.dockerconfigjson=.docker/config.json --type=kubernetes.io/dockerconfigjson -n <namespace>
```
or

```
podman login container-registry.oracle.com
kubectl create secret generic ocr-reg-cred --from-file=.dockerconfigjson=${XDG_RUNTIME_DIR}/containers/auth.json --type=kubernetes.io/ dockerconfigjson -n <namespace>
```

**Note:** Before running the above commands, you need to confirm the file path for `--from-file=.dockerconfigjson` in your environment.