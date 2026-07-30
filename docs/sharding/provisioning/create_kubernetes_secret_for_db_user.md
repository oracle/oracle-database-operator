# Create kubernetes secret for db user

Use the following steps to create an encrypted file with a password for the DB User:

- Create a text file that has the password that you want to use for the DB user.
- Create an RSA key pair using `openssl`.
- Encrypt the text file with a password, using `openssl` with the RSA key pair generated earlier.
- Remove the initial text file.
- Create the Kubernetes Secret named `db-user-pass-pkutl` using the encrypted file.

**IMPORTANT:** When creating the encrypted password file, you must use the same OpenSSL algorithm and options shown in this procedure. Using different encryption parameters may cause the password decryption to fail during deployment.

To understand how to create your own file, use the following example:

```sh
# Initialize the variables
PDIR="/tmp"
RSADIR="${PDIR}/pkutl"
PRIVKEY="${RSADIR}/key.pem"
PUBKEY="${RSADIR}/key.pub"

NAMESPACE="shns"
PWDFILE="${RSADIR}/pwdfile.txt"
PWDFILE_ENC="${RSADIR}/pwdfile.enc"
PWDFILE_DEC="${RSADIR}/pwdfile.dec"
SECRET_NAME="db-user-pass-pkutl"

# Create a directory for files for the secret:
mkdir -p "${RSADIR}"
cd "${RSADIR}"

# Generate RSA-3072 keypair if missing (portable across OpenSSL versions)
if [[ ! -f "${PRIVKEY}" || ! -f "${PUBKEY}" ]]; then
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "${PRIVKEY}"
  openssl pkey -in "${PRIVKEY}" -pubout -out "${PUBKEY}"
fi

# Create plaintext without trailing newline
# Replace the string <<Database User Passwrd>> with the actual password you want
printf '%s' '<<Database User Passwrd>>' > "${PWDFILE}"
rm -f "${PWDFILE_ENC}" "${PWDFILE_DEC}"

# Encrypt with explicit secure OAEP settings (version-stable)
openssl pkeyutl -encrypt \
  -pubin -inkey "${PUBKEY}" \
  -in "${PWDFILE}" -out "${PWDFILE_ENC}" \
  -pkeyopt rsa_padding_mode:oaep \
  -pkeyopt rsa_oaep_md:sha256 \
  -pkeyopt rsa_mgf1_md:sha256

# Optional local decrypt verification (must use same options)
# openssl pkeyutl -decrypt \
#  -inkey "${PRIVKEY}" \
#  -in "${PWDFILE_ENC}" -out "${PWDFILE_DEC}" \
#  -pkeyopt rsa_padding_mode:oaep \
#  -pkeyopt rsa_oaep_md:sha256 \
#  -pkeyopt rsa_mgf1_md:sha256

# Remove the initial text file:
rm -f $PWDFILE

# Create Kubernetes secret
kubectl delete secret "${SECRET_NAME}" -n "${NAMESPACE}" --ignore-not-found
kubectl create secret generic "${SECRET_NAME}" \
  --from-file="pwdfile.enc=${PWDFILE_ENC}" \
  --from-file="key.pem=${PRIVKEY}" \
  -n "${NAMESPACE}"

# Get the details of the secret
kubectl get secret "${SECRET_NAME}" -n "${NAMESPACE}"
kubectl describe secret "${SECRET_NAME}" -n "${NAMESPACE}"
```
