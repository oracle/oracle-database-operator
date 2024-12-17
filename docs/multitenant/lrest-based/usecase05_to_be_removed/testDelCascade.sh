#!/bin/bash


#kubectl delete lrpdb lrpdb1 -n pdbnamespace 2>/dev/null
make deldbop && make dboperator

kubectl apply -f create_lrest_pod.yaml
RUN=0
sleep 2
while [ $RUN -eq 0 ]
do
        RUN=`kubectl get pods -n cdbnamespace | grep Running|wc -l`
        sleep 1
        echo $RUN
done
echo "LREST RUNNING"

kubectl apply -f create_lrpdb1_resource.yaml
sleep 1
kubectl apply -f open_lrpdb1_resource.yaml

#kubectl delete lrest cdb-dev -n cdbnamespace


