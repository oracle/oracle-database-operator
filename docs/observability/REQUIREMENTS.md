# Observability Controller Requirements
To properly deploy an Observability custom resource, complete the following steps below.

## Install and configure Prometheus

The Observability controller creates multiple resources that include the Prometheus `servicemonitor`.
These `servicemonitors` are used to define monitoring for a set of services by scraping
metrics from within Kubernetes. In order for the controller to create ServiceMonitors, the
ServiceMonitor custom resource must exist.

### Checking if the Custom Resource Exists
To check if the custom resource already exists, run:

```bash
kubectl api-resources | grep monitoring
```
You should see the below entry listed among others included with Prometheus
```bash
# result
...
servicemonitors    smon   monitoring.coreos.com/v1   true   ServiceMonitor
...
```

### Installing Prometheus
If you do not have Prometheus installed, you can install Prometheus using different ways. Below
shows two ways of installing Prometheus

#### Using the Prometheus Operator
Using the Prometheus Operator for an example, run the following command which installs the Prometheus Operator into
the Kubernetes cluster. This bundle includes all Prometheus CRDs (including the required _servicemonitor_).
```bash
kubectl create -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/master/bundle.yaml
```

#### Using Helm
Alternatively, you can install Prometheus using helm. [Helm](https://helm.sh/docs/) must be installed beforehand.
Using helm, run the following command to add the Prometheus repository as follows:
```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
```
Run the update command to retrieve the latest from the Prometheus chart repository.
```bash
helm repo update
```
Run the installation command to install prometheus without a custom template.
```bash
helm install prometheus prometheus-community/prometheus
```


## Provision or set up your Database

The Observability Controller does not include the provisioning of databases by itself. Before creating
the Observability custom resource, you will need to prepare your Oracle Database and the required secrets containing
database credentials. The database user must have access to Database Performance (V$) views.