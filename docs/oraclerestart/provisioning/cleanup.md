# Cleanup an Oracle Restart Database Deployed using Oracle Database Operator

To delete and clean up the Oracle Restart Database deployed using Oracle Database Operator, run the following commands.

This example uses `oraclerestart_prov.yaml` to clean up an Oracle Restart Database that was initially used for deployment:


1. Use the file `oraclerestart_prov.yaml` to delete an existing deployment:
    ```sh
    kubectl delete -f oraclerestart_prov.yaml
    oraclerestart.database.oracle.com/oraclerestart-sample deleted
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:    
    kubectl get all -n orestart
    ```
3. If your deleted deployment used a software location specified by `hostSwLocation` from the worker node, then to reuse this location in your next deployment, you must clear it at the worker node level.

4. If your deleted deployment used ASM Disks from the worker node, then to reuse the same disks for the next deployment, you must clear the disks at the worker node level using the `dd` command.