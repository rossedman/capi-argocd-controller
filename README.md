# capi-argocd-controller

This controller adds clusters to ArgoCD when created by CAPI/CAPA.

## Setup

I used [this blog](https://kubernetes.io/blog/2021/06/21/writing-a-controller-for-pod-labels/) for reference

```
brew install operator-sdk
```

Below, I'm creating a controller and referencing types that exist in CAPI. I am also skipping the creation of those resources because CAPI creates them for us.

```
operator-sdk init \
    --domain=x-k8s.io \
    --repo=github.com/project-mimosa/capi-argocd-controller

operator-sdk create api \
    --group=cluster \
    --version=v1beta1 \
    --kind=Cluster \
    --resource=false \
    --controller=true
```

Once we have a controller bootstrapped we need to import the types from `cluster-api` so we can reference them

```
go get sigs.k8s.io/cluster-api/api/v1beta1
```
