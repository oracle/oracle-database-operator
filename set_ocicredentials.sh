#!/bin/bash
# Copyright (c) 2022, Oracle and/or its affiliates. 
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.

# Parse command line arguments
POSITIONAL=()
while [[ $# -gt 0 ]]; do
  option_key="$1"

  case $option_key in
      run)
      COMMAND="$1"
      shift # past argument
      ;;
      -configmap)
      CONFIGMAP_NAME="$2"
      shift # past argument
      shift # past value
      ;;
      -secret)
      SECRET_NAME="$2"
      shift # past argument
      shift # past value
      ;;
      -n|-namespace)
      NAMESPACE="$2"
      shift # past argument
      shift # past value
      ;;
      -path)
      OCI_PATH="$2"
      shift # past argument
      shift # past value
      ;;
      -profile)
      PROFILE="$2"
      shift # past argument
      shift # past value
      ;;
      -h|-help)
      cat <<EOF
Set up OCI credentials as secrets in Kubernetes.
Usage:
  set_ocicredentials run [options]

Options:
  -configmap    Name of the ConfigMap which contains OCI credentials. Default is "oci-cred"
  -secret       Name of the Secret of OCI privatekey. Default is "oci-privatekey"
  -n|-namespace Namespace of the ConfigMap and Secret. Default is "default".
  -path         Path of the OCI config file. Default is ~/.oci/config.
  -profile      Specify which profile in the config file should be used.
EOF
      exit 0
      ;;
      *)    # unknown command
      echo "Unknown command. Use [set_ocicredentials -h] for help."
      exit 1
      shift # past argument
      ;;
  esac
done
set -- "${POSITIONAL[@]}" # restore positional parameters

# Verify command before we start
if [[ -z "$COMMAND"  ]] || [[ "$COMMAND" -ne "run" ]] ; then
  echo "Unknown command. Check [set_ocicredentials -h] for usage."
  exit 0
fi

init() {
  # Set ConfigMap name
  if [ -z $CONFIGMAP_NAME ]; then
    CONFIGMAP_NAME="oci-cred"
  fi

  # Set Secret name
  if [ -z $SECRET_NAME ]; then
    SECRET_NAME="oci-privatekey"
  fi

  # Set OCI config path
  if [ -z $OCI_PATH ]; then
    OCI_PATH=$HOME/.oci/config
  fi

  # Set profile
  if [ -z $PROFILE ]; then
    PROFILE="DEFAULT"
  fi
}; init

echo "Parsing OCI config file $OCI_PATH..."
echo ""
echo "Using profile $PROFILE"
echo ""
echo "Tenancy, user, fingerprint, region and passphrase will be stored in the \"$CONFIGMAP_NAME\" ConfigMap; privatekey will be stored in the \"$SECRET_NAME\" Secret."
echo ""

parse() {
  # Start parsing the oci config file
  file=$OCI_PATH
  found=false

  # While loop to read line by line
  while IFS= read -r line; do

    # Find the target PROFILE section and parse the data
    if [[ "$line" =~ ^"[$PROFILE]" ]] ; then
      found=true
      while IFS= read -r line; do

        # Exit if reaches the next profile section
        if [[ "$line" =~ ^\[(.*)\] ]]; then
          break
        fi

        IFS="=" read key value <<< "$line"
				# trim both the key and value:
        key="${key/ }"
				value="${value/ }"

        case "$key" in
          "tenancy") tenancy="$value";;
          "user") user="$value";;
          "fingerprint") fingerprint="$value";;
          "region") region="$value";;
          "key_file") key_file="$value";;
          "passphrase") passphrase="$value";;
        esac
      done

      # Return when exit from the inner loop
      break
    fi
  done < "$file"

  if [[ $found != true ]]; then
    echo "Profile $PROFILE not found; exit the program."
    exit 1
  fi
}; parse

create_namespace() {
  if [ ! -z $NAMESPACE ]; then
    # Create the namespace if doesn't exist
    operator_namespace=`kubectl get namespaces | grep $NAMESPACE | tr -s ' ' | cut -d' ' -f1`
    if [ -z "$operator_namespace" ] ; then
      echo "Namespace/$NAMESPACE not found; exit the program."
      exit 1
    fi
  fi
}; create_namespace

create_configmap() {
  echo "Generating Kubernetes ConfigMap \"$CONFIGMAP_NAME\"$cat_str. Next, when you populate the .yaml file for the AutonomousDatabase, use this value for the ociConfig.configMapName attribute."
  echo ""

  # Generate command that creates a ConfigMap
  cmd="kubectl create configmap $CONFIGMAP_NAME \
  --from-literal=tenancy=$tenancy \
  --from-literal=user=$user \
  --from-literal=fingerprint=$fingerprint \
  --from-literal=region=$region"

  # Concat the passphrase if exists
  if [ ! -z $passphrase ]; then
    cmd="$cmd \
    --from-literal=passphrase=$passphrase"
  fi

  # Concat the namespace if exists
  if [ ! -z $NAMESPACE ]; then
    cmd="$cmd \
    -n $NAMESPACE"

    cat_str=" with namespace \"$NAMESPACE\""
  fi

  eval $cmd

  echo ""
}; create_configmap

create_secret() {
  echo "Generating Kubernetes Secret \"$SECRET_NAME\"$cat_str. Next, when you populate the .yaml file for the AutonomousDatabase, use this value for the ociConfig.secretName attribute."
  echo ""

  # Replace tilde(~) with $HOME in key_file path so that the script can recognize
  key_file="${key_file//\~/$HOME}"

  # Generate command that creates a Secret
  cmd="kubectl create secret generic $SECRET_NAME \
  --from-file=privatekey=$key_file"

  # Concat the namespace if exists
  if [ ! -z $NAMESPACE ]; then
    cmd="$cmd \
    -n $NAMESPACE"

    cat_str=" with namespace \"$NAMESPACE\""
  fi

  eval $cmd

  echo ""
}; create_secret
