# Setup
```
minikube start --addons=ingress,ingress-dns
```
```
kubectl get pods -n ingress-nginx
```
```
kubectl get svc -n ingress-nginx
```

# Generate ServiceAccount, Role and RoleBinding
```
kubectl create sa ingress-manager-sa -n default --dry-run=client -o yaml > manifests/ingress-manager-sa.yaml
```
```
kubectl create role ingress-manager-role -n default --resource=ingress,service --verb=list,watch,create,update,delete --dry-run=client -o yaml > manifests/ingress-manager-role.yaml
```
```
kubectl create rolebinding ingress-manager-rb -n default --role=ingress-manager-role --serviceaccount=default:ingress-manager-sa --dry-run=client -o yaml > manifests/ingress-manager-rb.yaml
```

# Build and Push Image
```
docker build -t jiaqiyin/ingress-manager:1.0.1 .
```
```
docker push jiaqiyin/ingress-manager:1.0.1
```

# Run Locally

```
kubectl apply -f manifests
```
```
minikube tunnel
```
```
curl --resolve example.com:80:127.0.0.1 http://example.com
```