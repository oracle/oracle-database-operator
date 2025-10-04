# Cleanup an Oracle Restart Database Deployed using Oracle Database Operator

In order to delete and cleanup the Oracle Restart Database deployed using Oracle Database Operator, run below command.

This example uses `oraclerestart_prov.yaml` to cleanup an Oracle Restart Database which was initially used for deployment:


1. Use the `oraclerestart_prov.yaml` file to delete an existing deployment:
    ```sh
    kubectl delete -f oraclerestart_prov.yaml
    oraclerestart.database.oracle.com/oraclerestart-sample deleted
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:    
    kubectl get all -n orestart
    ```
3. If your deleted deployment used software location specified by `hostSwLocation` from worker node, then in order to reuse this location in next deployment, you will need to clear it at the worker node level.

4. If your deleted deployment used ASM Disks from the worker node, then in order to reuse the same disks for the next deployment, you will need to clear the disks at the worker node level using `dd` command.