# Multitenant Controllers 


Starting from operator version 1.2.0, there are two classes of multitenant controllers: one based on [ORDS](https://www.oracle.com/uk/database/technologies/appdev/rest.html) and another based on a dedicated REST server for the operator, called LREST. In both cases, the overall architecture remains unchanged. A pod running a REST server (either LREST or ORDS) acts as the proxy server connected to the container database (CDB) for all incoming kubectl requests.

## What are the differences

- **ORDS** is a standalone product, while **LREST** is not. **LREST** is an integral component of the Oracle Database Operator.
- Regarding the YAML file, no payload parameters for the existing functionalities have been changed.
- The **CRD** names are different: for controllers based on [ORDS](./ords-based/README.md), we have **PDB** and **CDB**, while for controllers based on [LREST](./lrest-based/README.md), we have **LRPDB** and **LREST**.
- If you use an LREST-based controller, there is no need to manually create the REST server pod. The image is available for download on OCR.
- Controllers based on **LRSET** allow you to manage PDB parameters using kubectl.
- ORDS controllers currently do not support ORDS version 24.1.
