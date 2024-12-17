echo enter password for matteo.malvezzi@oracle.com@container-registry.oracle.com 
read -s scpwd 
/usr/local/go/bin/kubectl create secret docker-registry oracle-container-registry-secret --docker-server=container-registry.oracle.com --docker-username=matteo.malvezzi@oracle.com --docker-password=$scpwd --docker-email=matteo.malvezzi@oracle.com -n oracle-database-operator-system 
/usr/local/go/bin/kubectl create secret docker-registry oracle-container-registry-secret --docker-server=container-registry.oracle.com --docker-username=matteo.malvezzi@oracle.com --docker-password=$scpwd --docker-email=matteo.malvezzi@oracle.com -n ordsnamespace 
