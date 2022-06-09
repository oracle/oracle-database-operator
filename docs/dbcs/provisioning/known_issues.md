# Known Issues - Oracle DB Operator DBCS Controller 

Below are the known issues using the Oracle DB Operator DBCS Controller:

1. There is a known issue related to the DB Version 19c, 12c and 11g when used with the Oracle DB Operator DBCS Controller. DB Version 21c and 18c work with the controller.
2. In order to scale up storage of an existing DBCS system, the steps will be:
    * Bind the existing DBCS System to DBCS Controller.
    * Apply the change to scale up its storage.
   This causes issue. The actual real step sequence that work is
    * Bind 
    * Apply Shape change 
    * Apply scale storage change 
