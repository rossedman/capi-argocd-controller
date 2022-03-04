## Usage

[Helm](https://helm.sh) must be installed to use the charts. Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

  helm repo add capi-argocd-controller https://rossedman.github.io/capi-argocd-controller

If you had already added this repo earlier, run `helm repo update` to retrieve
the latest versions of the packages.  You can then run `helm search repo
capi-argocd-controller` to see the charts.

To install the capi-argocd-controller chart:

    helm install my-capi-argocd-controller capi-argocd-controller/capi-argocd-controller

To uninstall the chart:

    helm delete my-capi-argocd-controller