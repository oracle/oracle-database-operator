#!/bin/bash 

kubectl apply -f  open_lrpdb1_resource.yaml
kubectl get lrpdb lrpdb1 -n pdbnamespace --watch
