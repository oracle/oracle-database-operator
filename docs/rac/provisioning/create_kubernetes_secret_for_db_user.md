# Create Kubernetes secret for db user

Create a Kubernetes secret named `db-user-pass` using a password in a text file, and then encrypt it using an `openssl` key. The text file will be removed after the secret is created. Worker nodes must have `openssl` installed.

```sh
mkdir /tmp/.secrets/

# Generate a random openssl key
echo Oracle_23ai > /tmp/.secrets/pwdfile.txt
openssl genrsa -out /tmp/.secrets/key.pem
openssl rsa -in /tmp/.secrets/key.pem -out /tmp/.secrets/key.pub -pubout

# Encrypt the password file
openssl pkeyutl -in /tmp/.secrets/pwdfile.txt -out /tmp/.secrets/pwdfile.enc -pubin -inkey /tmp/.secrets/key.pub -encrypt
rm -rf /tmp/.secrets/pwdfile.txt

# Deleting the exisitng secret
kubectl delete secret db-user-pass-pkutl -n rac

# Create the Kubernetes secret in namespace "rac"
kubectl create secret generic db-user-pass-pkutl --from-file=/tmp/.secrets/pwdfile.enc --from-file=/tmp/.secrets/key.pem -n rac

# Check the secret details 
kubectl get secret -n rac
```
