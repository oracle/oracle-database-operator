# Create kubernetes secret for db user

Create a Kubernetes secret named "db-user-pass" using a password in a text file and then encrypt it using an `openssl` key. The text file will be removed after secret is created.

```sh
mkdir /tmp/.secrets/

# Generate a random openssl key
openssl rand -hex 64 -out /tmp/.secrets/pwd.key

# Use a password you want and add it to a text file
echo ORacle_21c > /tmp/.secrets/common_os_pwdfile

# Encrypt the file with the password with the random openssl key generated above
openssl enc -aes-256-cbc -md md5 -salt -in /tmp/.secrets/common_os_pwdfile -out /tmp/.secrets/common_os_pwdfile.enc -pass file:/tmp/.secrets/pwd.key

# Remove the password text file
rm -f /tmp/.secrets/common_os_pwdfile

# Create the Kubernetes secret in namespace "shns"
kubectl create secret generic db-user-pass --from-file=/tmp/.secrets/common_os_pwdfile.enc --from-file=/tmp/.secrets/pwd.key -n shns

# Check the secret details 
kubectl get secret -n shns
```
