#!/bin/bash

date +"%Y-%m-%d %H:%M:%S"
echo "=== ORDS start ==="
echo "ORDS_CONFIG: ${ORDS_CONFIG}"

unset APEX_IMAGES

# old path, until ORDS image 24.1.1
if [[ !(-z ${APEX_BASE}) && !(-z ${APEX_VER}) && (-d ${APEX_BASE}/${APEX_VER}/images) ]]; then
        APEX_IMAGES=${APEX_BASE}/${APEX_VER}/images
fi

# downloaded image path
if [[ -d /opt/oracle/apex/images ]]; then
	APEX_IMAGES=/opt/oracle/apex/images
fi

if [[ -z ${APEX_IMAGES} ]]; then
	echo "APEX_IMAGES not found"
	ords --config ${ORDS_CONFIG} serve
else
	echo "APEX_IMAGES: ${APEX_IMAGES}"
	ords --config ${ORDS_CONFIG} serve --apex-images ${APEX_IMAGES}
fi	

