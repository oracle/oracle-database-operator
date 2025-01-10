drop user MONGO cascade
create user MONGO identified by "My_Password1!";
grant soda_app, create session, create table, create view, create sequence, create procedure, create job,
unlimited tablespace to MONGO;  
conn MONGO/My_Password1!@FREEPDB1
exec ords.enable_schema;



