# Known Issues

This document lists any known issues when the Oracle Restart Database is provisioned using the Oracle Restart controller.

- If you describe the resource `oraclerestarts.database.oracle.com/oraclerestart-sample` in the namespace used for Oracle Restart Database provisioning, sometimes it may report the `State` under `Service Details` as `FAILED` while the Service is up and running. Similarly, the `Instance State` may report as `NOTAVAILABLE` while the Oracle Restart Database Instance is up and running fine.