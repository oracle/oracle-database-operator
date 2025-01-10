#!/bin/bash
NAMESPACE=${1:-"ordsnamespace"}
KUBECTL=/usr/bin/kubectl
for _pod in `${KUBECTL} get pods  --no-headers -o custom-columns=":metadata.name" --no-headers -n ${NAMESPACE}`
do
	for _podinit in   `${KUBECTL} get pod ${_pod} -n ${NAMESPACE} -o="custom-columns=INIT-CONTAINERS:.spec.initContainers[*].name" --no-headers`
	do
        echo "DUMPINIT ${_pod}:${_podinit}"
	${KUBECTL} logs -f --since=0 ${_pod} -n ${NAMESPACE} -c ${_podinit}
        done
done
