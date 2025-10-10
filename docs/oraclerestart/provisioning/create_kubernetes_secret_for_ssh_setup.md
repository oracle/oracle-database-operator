# Create Kubernetes secret for db user using an SSH Key Pair

Generate an SSH Key Pair using `ssh-keygen`. Then create a Kubernetes secret named `ssh-key-secret` using this key pair.

```sh
mkdir /tmp/.secrets/ssh

# Generate a private and public key
ssh-keygen -t rsa -C "your_email@example.info" -f /tmp/.secrets/ssh/id_rsa

# Deleting the exisitng secret
kubectl delete secret ssh-key-secret -n orestart

# Create the Kubernetes secret in namespace "orestart"
kubectl create secret generic ssh-key-secret --from-file=ssh-privkey=/tmp/.secrets/ssh/id_rsa --from-file=ssh-pubkey=/tmp/.secrets/ssh/id_rsa.pub -n orestart

# Check the secret details 
kubectl get secret -n orestart
```
