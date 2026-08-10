# Configure a Custom PDB Service Across Oracle Data Guard Role Transitions

This guide documents a tested procedure for creating a custom PDB service that follows the current primary database during Oracle Data Guard switchovers.

The example uses the custom service `ORCLPDB1_CUST_SERVICE` for PDB `ORCLPDB1`. The service is started on the current primary and must not remain active on the physical standby.

**Important:** Do not use the default service whose name is the same as the PDB. Create and use a separate custom service.

**Important:** This document covers a custom PDB service for reference only.

## Contents

- [Configure a Custom PDB Service Across Oracle Data Guard Role Transitions](#configure-a-custom-pdb-service-across-oracle-data-guard-role-transitions)
  - [Contents](#contents)
  - [Test Environment](#test-environment)
  - [Confirm the Initial Data Guard Configuration](#confirm-the-initial-data-guard-configuration)
  - [Create the Custom PDB Service](#create-the-custom-pdb-service)
  - [Verify the Initial Service State](#verify-the-initial-service-state)
  - [Create the Service Synchronization Procedure](#create-the-service-synchronization-procedure)
  - [Create the Startup and Role-Change Triggers](#create-the-startup-and-role-change-triggers)
  - [Verify the Triggers](#verify-the-triggers)
  - [Test the Procedure Before Switchover](#test-the-procedure-before-switchover)
  - [Perform a Switchover](#perform-a-switchover)
  - [Verify Listener Registration After Switchover](#verify-listener-registration-after-switchover)
    - [New standby: ORCL1](#new-standby-orcl1)
    - [New primary: ORCLS](#new-primary-orcls)
  - [Configure the Client TNS Alias](#configure-the-client-tns-alias)
  - [Test Client Connectivity Across Switchovers](#test-client-connectivity-across-switchovers)
    - [Connect after switching the primary to ORCLS](#connect-after-switching-the-primary-to-orcls)
    - [Switch the primary back to ORCL1](#switch-the-primary-back-to-orcl1)
    - [Switch the primary to ORCLS again](#switch-the-primary-to-orcls-again)
  - [Validation Checklist](#validation-checklist)

## Test Environment

The supplied test case uses the following values:

| Item | Value |
| --- | --- |
| Data Guard configuration | `dg_config` |
| Initial primary database | `ORCL1` |
| Initial physical standby | `ORCLS` |
| PDB | `ORCLPDB1` |
| Custom service | `ORCLPDB1_CUST_SERVICE` |
| Primary client endpoint | `sidb-sample.default:1521` |
| Standby client endpoint | `10.0.10.101:31813` |

Replace the database names, PDB name, service name, hostnames, ports, TNS alias, and password placeholders with values for your environment.

Run the SQL statements as `SYS`. Create the custom service in the PDB, but create the synchronization procedure and database triggers in `CDB$ROOT`.

## Confirm the Initial Data Guard Configuration

From `DGMGRL`, verify the initial configuration:

```text
DGMGRL> show configuration

Configuration - dg_config

  Protection Mode: MaxPerformance
  Members:
  orcl1 - Primary database
    orcls - Physical standby database

Fast-Start Failover:  Disabled

Configuration Status:
SUCCESS   (status updated 28 seconds ago)
```

The remaining steps assume that `ORCL1` is the current primary and `ORCLS` is the physical standby.

## Create the Custom PDB Service

Connect to the current primary database as `SYS`, then switch to `ORCLPDB1`:

```sql
ALTER SESSION SET CONTAINER = ORCLPDB1;
```

Create the custom service:

```sql
EXEC DBMS_SERVICE.CREATE_SERVICE( -
  service_name => 'ORCLPDB1_CUST_SERVICE', -
  network_name => 'ORCLPDB1_CUST_SERVICE' -
);
```

Start the custom service:

```sql
EXEC DBMS_SERVICE.START_SERVICE('ORCLPDB1_CUST_SERVICE');
```

Verify that the service is associated with the correct PDB:

```sql
SET LINES 200
COLUMN container_name FORMAT A20
COLUMN service_name   FORMAT A35
COLUMN network_name   FORMAT A35

SELECT c.name AS container_name,
       s.name AS service_name,
       s.network_name,
       s.con_id
FROM   v$active_services s
       JOIN v$containers c ON c.con_id = s.con_id
WHERE  UPPER(s.name) = 'ORCLPDB1_CUST_SERVICE'
   OR  UPPER(s.network_name) = 'ORCLPDB1_CUST_SERVICE';
```

## Verify the Initial Service State

The desired state is:

- A service definition associated with `ORCLPDB1`.
- `ORCLPDB1_CUST_SERVICE` active only on the current primary.
- No active `ORCLPDB1_CUST_SERVICE` entry on the physical standby.
- No parameter-driven custom service in `SERVICE_NAMES`.

Check dynamic listener registration on both databases:

```bash
lsnrctl status
```

On the current primary, `ORCL1`, the listener should show the custom service:

```text
Service "ORCLPDB1_CUST_SERVICE" has 1 instance(s).
  Instance "ORCL1", status READY, has 1 handler(s) for this service...
```

On the current standby, `ORCLS`, the listener should not show `ORCLPDB1_CUST_SERVICE`.

## Create the Service Synchronization Procedure

Create one procedure that is called after database startup and after a database role change. On the current primary, connect as `SYS`, set the container to `CDB$ROOT`, and create the following procedure:

```sql
ALTER SESSION SET CONTAINER = CDB$ROOT;

CREATE OR REPLACE PROCEDURE sync_orclpdb1_cust_service
AUTHID DEFINER
AS
    l_role       VARCHAR2(30);
    l_open_mode  VARCHAR2(20);
    l_cursor     INTEGER := NULL;
    l_result     INTEGER;
    l_block      VARCHAR2(32767);
BEGIN
    /*
     * Determine the current database role and the PDB open mode.
     */
    SELECT database_role
    INTO   l_role
    FROM   v$database;

    SELECT open_mode
    INTO   l_open_mode
    FROM   v$pdbs
    WHERE  name = 'ORCLPDB1';

    IF l_role = 'PRIMARY' THEN
        /*
         * Ensure that the PDB is open read-write on the primary.
         */
        IF l_open_mode <> 'READ WRITE' THEN
            IF l_open_mode <> 'MOUNTED' THEN
                EXECUTE IMMEDIATE
                    'ALTER PLUGGABLE DATABASE ORCLPDB1 CLOSE IMMEDIATE';
            END IF;

            EXECUTE IMMEDIATE
                'ALTER PLUGGABLE DATABASE ORCLPDB1 OPEN READ WRITE';
        END IF;

        /*
         * Start the service inside the PDB.
         */
        l_block := q'[
DECLARE
    PRAGMA AUTONOMOUS_TRANSACTION;
BEGIN
    BEGIN
        DBMS_SERVICE.START_SERVICE(
            service_name => 'ORCLPDB1_CUST_SERVICE'
        );
    EXCEPTION
        WHEN DBMS_SERVICE.SERVICE_IN_USE THEN
            -- The service is already running.
            NULL;
    END;

    COMMIT;
END;
]';

    ELSE
        /*
         * If the PDB is mounted on the standby, no PDB service can
         * currently accept connections. Refresh listener registration
         * and return.
         */
        IF l_open_mode = 'MOUNTED' THEN
            EXECUTE IMMEDIATE 'ALTER SYSTEM REGISTER';
            RETURN;
        END IF;

        /*
         * Stop the service inside the PDB on the standby. The IMMEDIATE
         * option with a zero drain timeout also disconnects sessions
         * using this service.
         */
        l_block := q'[
DECLARE
    PRAGMA AUTONOMOUS_TRANSACTION;
BEGIN
    BEGIN
        DBMS_SERVICE.STOP_SERVICE(
            service_name  => 'ORCLPDB1_CUST_SERVICE',
            instance_name => NULL,
            stop_option   => DBMS_SERVICE.STOP_OPTION_IMMEDIATE,
            drain_timeout => 0,
            replay        => FALSE
        );
    EXCEPTION
        WHEN DBMS_SERVICE.SERVICE_NOT_RUNNING THEN
            -- The service is already stopped.
            NULL;
    END;

    COMMIT;
END;
]';
    END IF;

    /*
     * DBMS_SERVICE must run in the PDB that owns the service.
     * DBMS_SQL executes the anonymous block in ORCLPDB1 while this
     * procedure remains in CDB$ROOT.
     */
    l_cursor := DBMS_SQL.OPEN_CURSOR;

    DBMS_SQL.PARSE(
        c                          => l_cursor,
        statement                  => l_block,
        language_flag              => DBMS_SQL.NATIVE,
        edition                    => NULL,
        apply_crossedition_trigger => NULL,
        fire_apply_trigger         => TRUE,
        schema                     => 'SYS',
        container                  => 'ORCLPDB1'
    );

    l_result := DBMS_SQL.EXECUTE(l_cursor);

    DBMS_SQL.CLOSE_CURSOR(l_cursor);

    /*
     * Immediately refresh dynamic listener registration.
     */
    EXECUTE IMMEDIATE 'ALTER SYSTEM REGISTER';

EXCEPTION
    WHEN OTHERS THEN
        /*
         * Close the DBMS_SQL cursor, but do not suppress the original
         * error. The error should be visible during testing and in the
         * database diagnostic output.
         */
        BEGIN
            IF DBMS_SQL.IS_OPEN(l_cursor) THEN
                DBMS_SQL.CLOSE_CURSOR(l_cursor);
            END IF;
        EXCEPTION
            WHEN OTHERS THEN
                NULL;
        END;

        RAISE;
END;
/
```

Check the procedure for compilation errors:

```sql
SHOW ERRORS PROCEDURE sync_orclpdb1_cust_service
```

## Create the Startup and Role-Change Triggers

As `SYS` in `CDB$ROOT`, create a trigger that runs after database startup:

```sql
CREATE OR REPLACE TRIGGER dg_pdb_service_startup
AFTER STARTUP ON DATABASE
BEGIN
    SYS.SYNC_ORCLPDB1_CUST_SERVICE;
END;
/
```

Create a second trigger that runs after a database role change:

```sql
CREATE OR REPLACE TRIGGER dg_pdb_service_role_change
AFTER DB_ROLE_CHANGE ON DATABASE
BEGIN
    SYS.SYNC_ORCLPDB1_CUST_SERVICE;
END;
/
```

Check both triggers for compilation errors:

```sql
SHOW ERRORS TRIGGER dg_pdb_service_startup
SHOW ERRORS TRIGGER dg_pdb_service_role_change
```

## Verify the Triggers

Verify the triggers on both databases:

```sql
COLUMN trigger_name     FORMAT A35
COLUMN triggering_event FORMAT A30
COLUMN status           FORMAT A10

SELECT trigger_name,
       triggering_event,
       status
FROM   dba_triggers
WHERE  trigger_name IN (
           'DG_PDB_SERVICE_STARTUP',
           'DG_PDB_SERVICE_ROLE_CHANGE'
       )
ORDER BY trigger_name;
```

Confirm that both triggers are present and enabled.

## Test the Procedure Before Switchover

Run the synchronization procedure manually on the current primary:

```sql
EXEC SYS.SYNC_ORCLPDB1_CUST_SERVICE;
```

Verify that the service is active:

```sql
SET LINES 200
COLUMN container_name FORMAT A20
COLUMN service_name   FORMAT A35
COLUMN network_name   FORMAT A35

SELECT c.name AS container_name,
       s.name AS service_name,
       s.network_name,
       s.con_id
FROM   v$active_services s
       JOIN v$containers c
         ON c.con_id = s.con_id
WHERE  UPPER(s.name) = 'ORCLPDB1_CUST_SERVICE'
   OR  UPPER(s.network_name) = 'ORCLPDB1_CUST_SERVICE';
```

Before proceeding, confirm again that the custom service is registered only on the current primary listener.

## Perform a Switchover

From `DGMGRL`, switch the primary role to `ORCLS`:

```text
DGMGRL> switchover to orcls;
```

Verify the Data Guard configuration:

```text
DGMGRL> show configuration

Configuration - dg_config

  Protection Mode: MaxPerformance
  Members:
  orcls - Primary database
    orcl1 - Physical standby database

Fast-Start Failover:  Disabled

Configuration Status:
SUCCESS   (status updated 34 seconds ago)
```

After the switchover:

- `ORCLS` is the new primary.
- `ORCL1` is the new physical standby.
- The `AFTER DB_ROLE_CHANGE` trigger runs the synchronization procedure.
- The custom service should be active on `ORCLS` and absent from `ORCL1`.

## Verify Listener Registration After Switchover

Run the following command on each database host or pod:

```bash
lsnrctl status
```

### New standby: ORCL1

The listener output must not contain `ORCLPDB1_CUST_SERVICE`. The supplied test case showed the default and Data Guard services, but no custom service:

```text
Service "ORCL1" has 1 instance(s).
  Instance "ORCL1", status READY, has 1 handler(s) for this service...
Service "ORCL1XDB" has 1 instance(s).
  Instance "ORCL1", status READY, has 1 handler(s) for this service...
Service "ORCL1_CFG" has 1 instance(s).
  Instance "ORCL1", status READY, has 1 handler(s) for this service...
Service "ORCL1_DGMGRL" has 1 instance(s).
  Instance "ORCL1", status UNKNOWN, has 1 handler(s) for this service...
Service "orclpdb1" has 1 instance(s).
  Instance "ORCL1", status READY, has 1 handler(s) for this service...
```

### New primary: ORCLS

The listener output should include the custom service registered against `ORCLS`:

```text
Service "ORCLPDB1_CUST_SERVICE" has 1 instance(s).
  Instance "ORCLS", status READY, has 1 handler(s) for this service...
```

The supplied test case also showed the following services on the new primary:

```text
Service "57359ffd1f160eb0e0632e0a000ad362" has 1 instance(s).
  Instance "ORCLS", status READY, has 1 handler(s) for this service...
Service "ORCL1XDB" has 1 instance(s).
  Instance "ORCLS", status READY, has 1 handler(s) for this service...
Service "ORCL1_CFG" has 1 instance(s).
  Instance "ORCLS", status READY, has 1 handler(s) for this service...
Service "ORCLS" has 1 instance(s).
  Instance "ORCLS", status READY, has 1 handler(s) for this service...
Service "ORCLS_DGMGRL" has 1 instance(s).
  Instance "ORCLS", status UNKNOWN, has 1 handler(s) for this service...
Service "orclpdb1" has 1 instance(s).
  Instance "ORCLS", status READY, has 1 handler(s) for this service...
```

## Configure the Client TNS Alias

On the client, add the following entry to `tnsnames.ora`. The two addresses provide connection failover between the database endpoints:

```text
ORCLPDB1_CUST_SERVICE =
  (DESCRIPTION =
    (ADDRESS_LIST =
      (FAILOVER = ON)
      (LOAD_BALANCE = OFF)
      (ADDRESS = (PROTOCOL = TCP)(HOST = sidb-sample.default)(PORT = 1521))
      (ADDRESS = (PROTOCOL = TCP)(HOST = 10.0.10.101)(PORT = 31813))
    )
    (CONNECT_DATA =
      (SERVER = DEDICATED)
      (SERVICE_NAME = ORCLPDB1_CUST_SERVICE)
    )
  )
```

Replace the hostnames and ports with endpoints that are reachable from your client.

## Test Client Connectivity Across Switchovers

### Connect after switching the primary to ORCLS

Connect through the custom service:

```bash
sqlplus sys/<password>@ORCLPDB1_CUST_SERVICE as sysdba
```

Confirm the connected database role and open mode:

```sql
SELECT db_unique_name,
       database_role,
       open_mode
FROM   v$database;
```

Expected result after switching to `ORCLS`:

```text
DB_UNIQUE_NAME                 DATABASE_ROLE     OPEN_MODE
------------------------------ ---------------- --------------------
ORCLS                          PRIMARY           READ WRITE
```

### Switch the primary back to ORCL1

From `DGMGRL`:

```text
DGMGRL> switchover to orcl1;
```

Verify the configuration:

```text
DGMGRL> show configuration

Configuration - dg_config

  Protection Mode: MaxPerformance
  Members:
  orcl1 - Primary database
    orcls - Physical standby database

Fast-Start Failover:  Disabled

Configuration Status:
SUCCESS   (status updated 8 seconds ago)
```

Reconnect through the same client alias:

```bash
sqlplus sys/<password>@ORCLPDB1_CUST_SERVICE as sysdba
```

Run the database-role query again:

```sql
SELECT db_unique_name,
       database_role,
       open_mode
FROM   v$database;
```

Expected result:

```text
DB_UNIQUE_NAME                 DATABASE_ROLE     OPEN_MODE
------------------------------ ---------------- --------------------
ORCL1                          PRIMARY           READ WRITE
```

### Switch the primary to ORCLS again

Perform one more switchover:

```text
DGMGRL> switchover to orcls;
```

Reconnect using the same TNS alias:

```bash
sqlplus sys/<password>@ORCLPDB1_CUST_SERVICE as sysdba
```

Verify the database role:

```sql
SELECT db_unique_name,
       database_role,
       open_mode
FROM   v$database;
```

Expected result:

```text
DB_UNIQUE_NAME                 DATABASE_ROLE     OPEN_MODE
------------------------------ ---------------- --------------------
ORCLS                          PRIMARY           READ WRITE
```

The repeated tests confirm that the same client alias connects to whichever database currently holds the primary role.

## Validation Checklist

After each startup or Data Guard role transition, verify all of the following:

- The Data Guard configuration status is `SUCCESS`.
- `ORCLPDB1` is open `READ WRITE` on the current primary.
- `ORCLPDB1_CUST_SERVICE` is present in `v$active_services` on the current primary.
- The current primary listener registers `ORCLPDB1_CUST_SERVICE` with status `READY`.
- The physical standby listener does not register `ORCLPDB1_CUST_SERVICE`.
- The startup and role-change triggers are enabled.
- The client TNS alias connects to the current primary after each switchover.
- The connected database reports `DATABASE_ROLE = PRIMARY` and `OPEN_MODE = READ WRITE`.
