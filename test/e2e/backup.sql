set cloudconfig -proxy=&1 &2
connect ADMIN/&3@&4
ALTER DATABASE PROPERTY SET default_backup_bucket='&5';

BEGIN
DBMS_CLOUD.DROP_CREDENTIAL( credential_name => 'DEF_CRED_NAME' );
END;
/

BEGIN
  DBMS_CLOUD.CREATE_CREDENTIAL(
    credential_name => 'DEF_CRED_NAME',
    username => '&6',
    password => '&7'
);
END;
/

ALTER DATABASE PROPERTY SET DEFAULT_CREDENTIAL = 'ADMIN.DEF_CRED_NAME';
exit