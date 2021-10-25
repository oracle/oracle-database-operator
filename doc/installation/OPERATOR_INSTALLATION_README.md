# Oracle Database Operator for Kubernetes Development Environment Instructions

Review these instructions to set up your development environment with Oracle Database Operator for Kubernetes.

To set up your development environment to use the operator, complete the following steps

## Check Operating System Requirements

Your system must be using Oracle Linux 7 (7.x), with the Unbreakable Enterprise Kernel Release 5 (UEK R5).

Validate using `uname`. For example: 

  ```sh
  uname -r
  4.14.35-1902.0.18.el7uek.x86_64
  ```

## Download and Set Up the Go Programming Language (GoLang) 
Oracle strongly recommends that you use a higher version of GoLang, at least 1.16 or a later release. 

1. Download the Linux distribution of GoLang from [Go’s official download page](https://golang.org/dl/) and extract it into /usr/local directory

1. Run the following commands:

  ```sh
  tar -C /usr/local -xzf go$VERSION.$OS-$ARCH.tar.gz
  export PATH=$PATH:/usr/local/go/bin
  go version
  ```
1. Add the `/usr/local/go/bin` directory to your `PATH` environment variable. You can do this by adding the a line to your `~/.bash_profile` file.
## Set the GOPATH Environment

Set the `GOPATH` environment variable. This variable specifies the location of your workspace. By default, the location specified by `GOPATH` is assumed to be `$HOME/go`. For example: 
  ```sh
  export GODIR=/scratch/go/projects/
  export GOPATH=$GODIR/go
  ```
The Go workspace is divided into following directoriess:
* `src`: contains Go source files.
* `bin`: contains the binary executables.
* `pkg`: contains Go package archives (`.a`).

## Set Up the OPERATOR Development Environment (Dev Env)

Install the latest version of the Operator software development kit (SDK). For example:
  ```sh
  wget https://github.com/operator-framework/operator-sdk/releases/download/v1.2.0/operator-sdk-v1.2.0-x86_64-linux-gnu
  mv operator-sdk-v1.2.0-x86_64-linux-gnu  /usr/local/bin/operator-sdk
  chmod +x /usr/local/bin/operator-sdk
  ```
If you encounter an issue with these steps, then refer to the more detailed steps given in the following documents:
* [Operator-framework/operator-sdk] (<https://github.com/operator-framework/operator-sdk>)
* [Install the Operator SDK CLI] (<https://sdk.operatorframework.io/docs/installation/>)
* [Quickstart for Go-based Operators] (<https://sdk.operatorframework.io/docs/building-operators/golang/quickstart/>)

## Clone Oracle Database Operator
Clone the [oracle-database-operator](https://github.com/oracle/oracle-database-operator) on your local machine

## Review and modify the Cloned Repository
Make changes as needed to the repository. 

If you want to run the operator locally outside the cluster, then run the following steps:

  ```sh
    cd oracle-database-operator
    make generate
    make manifests
    make install run
  ```
If you want to run the operator inside the cluster, then run the following steps:

  ```sh
  cd oracle-database-operator
  make generate
  make manifests
  make install
  make docker-build IMG=<region-key>.ocir.io/<tenancy-namespace>/<repo-name>/<image-name>:<tag>
  docker push <region-key>.ocir.io/<tenancy-namespace>/<repo-name>/<image-name>:<tag>
  kubectl create namespace oracle-database-operator-system
  kubectl create secret docker-registry container-registry-secret -n oracle-database-operator-system --docker-server=<region-key>.ocir.io --docker-username='<tenancy-namespace>/<oci-username>' --docker-password='<oci-auth-token>' --docker-email='<email-address>'
  make operator-yaml IMG=<region-key>.ocir.io/<tenancy-namespace>/<repo-name>/<image-name>:<tag>
  ```

## Check the YAML File
You should see file `oracle-database-operator.yaml`. This file will perform the following operations:
  * Create CRDs
  * Create Roles and Bindings
  * Operator Deployment

## Run the Quick Install 

Follow the steps mentioned in the `Quick Install of the Operator` section of [README.md](../../README.md#quick-install-the-operator) to install operator with your changes.
