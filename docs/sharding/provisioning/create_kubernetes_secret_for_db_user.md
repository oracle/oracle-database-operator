# Create kubernetes secret for db user

Use the following steps to create an encrypted file with a password for the DB User:

- Create a text file that has the password that you want to use for the DB user.
- Create an RSA key pair using `openssl`.
- Encrypt the text file with a password, using `openssl` with the RSA key pair generated earlier.
- Remove the initial text file.
- Create the Kubernetes Secret named `db-user-pass-rsa` using the encrypted file.

**IMPORTANT:** Make sure the version of `openssl` in the Oracle Database and Oracle GSM images is compatible with the `openssl` version on the machine where you will run the openssl commands to generated the encrypted password file during the deployment.

To understand how to create your own file, use the following example:

```sh
# Create a directory for files for the secret:
rm -rf /tmp/.secrets/ 
mkdir /tmp/.secrets/

# Create directories and initialize the variables
RSADIR="/tmp/.secrets"
PRIVKEY="${RSADIR}"/"key.pem"
PUBKEY="${RSADIR}"/"key.pub"
NAMESPACE="shns"
PWDFILE="${RSADIR}"/"pwdfile.txt"
PWDFILE_ENC="${RSADIR}"/"pwdfile.enc"
SECRET_NAME="db-user-pass-rsa"

# Generate the RSA Key
openssl genrsa -out "${RSADIR}"/key.pem
openssl rsa -in "${RSADIR}"/key.pem -out "${RSADIR}"/key.pub -pubout

# Create a text file with the password
rm -f $PWDFILE_ENC
echo ORacle_23c > ${RSADIR}/pwdfile.txt

# Create encrypted file from the text file using the RSA key
openssl pkeyutl -in $PWDFILE -out $PWDFILE_ENC -pubin -inkey $PUBKEY -encrypt

# Remove the initial text file:
rm -f $PWDFILE

# Deleting the existing secret if existing
kubectl delete secret $SECRET_NAME -n  $NAMESPACE

# Create the Kubernetes secret in namespace "NAMESPACE"
kubectl create secret generic $SECRET_NAME --from-file=$PWDFILE_ENC --from-file=${PRIVKEY} -n $NAMESPACE
```
