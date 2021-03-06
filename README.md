# capi-argocd-controller

This controller adds clusters to ArgoCD when created by CAPI/CAPA.

## Prerequisites

This controller makes a few assumptions right now

- CAPI is installed in the cluster this is being deployed to
- ArgoCD is installed in the cluster this is being deploy to
- ArgoCD is installed in the `argocd` namespace

## Quickstart

To release a new image to the registry

```sh
echo $GITHUB_TOKEN | docker login ghcr.io -u $GITHUB_USERNAME --password-stdin
make docker-build docker-push
```

This will release an image to `ghcr.io/rossedman/capi-argocd-controller`. Once the image is created, then it can be deployed. To deploy the controller, you must be pointed at the correct cluster context you want to deploy to and then run these commands

```sh
kubectl config use-context $CLUSTER_NAME
make docker-build docker-push deploy
```

## Setup

Below I am setting up a controller that will watch for cluster-api resources using the `operator-sdk` framework. This provides us with a scaffolding and testing framework to ensure our controllers work as expected and generates tons of boilerplate code for us. First, let's install the `operator-sdk` tool

```
brew install operator-sdk
```

Below, I'm creating a controller and referencing types that exist in CAPI. I am also skipping the creation of those resources because CAPI creates them for us. This is generating a controller without CRDs.

```
operator-sdk init \
    --domain=x-k8s.io \
    --repo=github.com/rossedman/capi-argocd-controller

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
go get github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1
```

Once this is downloaded, run these commands

```
go mod tidy
go mod download
```

Also important to note, to track resources that are not part of the core Kubernetes API or a CRD we are generating, we need to declare them in the scheme for the controller. This was added in `main.go` to track cluster api resources

```
utilruntime.Must(capi.AddToScheme(scheme))
```

