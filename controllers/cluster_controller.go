/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// TODO: secrets need to be scoped to the 'argocd' namespace as well as a way to
// scope to the cluster-api namespaces where kubeconfigs are stored

//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters/status,verbs=get
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	log := log.Log.WithValues("cluster", req.NamespacedName)

	// retrieve the cluster object
	var cluster capi.Cluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		log.Error(err, "unable to fetch cluster")
		return ctrl.Result{}, err
	}

	// if control plane is not ready, return and requeue
	if !cluster.Status.ControlPlaneReady {
		log.Info(fmt.Sprintf("cluster %s is not ready", cluster.Name))
		return ctrl.Result{Requeue: true}, nil
	}

	// check status of cluster controlplane
	// if controlplane is ready let's do some stuff
	log.Info(fmt.Sprintf("cluster %s is ready", cluster.Name))

	// create clientset from kubeconfig for management cluster
	conf, err := ctrl.GetConfig()
	if err != nil {
		log.Error(err, "unable to get config")
		return ctrl.Result{}, err
	}
	clientset, err := kubernetes.NewForConfig(conf)
	if err != nil {
		log.Error(err, "unable to create client")
		return ctrl.Result{}, err
	}

	// retrieve target cluster secret
	secret, err := clientset.CoreV1().
		Secrets(cluster.Namespace).
		Get(context.TODO(), fmt.Sprintf("%s-kubeconfig", cluster.Name), v1.GetOptions{})
	if err != nil {
		log.Error(err, "unable to retrieve secret")
		return ctrl.Result{Requeue: true}, nil
	}

	// connect to target cluster
	// TODO: check that value isn't empty
	targetConf, err := clientcmd.RESTConfigFromKubeConfig(secret.Data["value"])
	if err != nil {
		log.Error(err, "unable to load kubeconfig for target cluster")
		return ctrl.Result{}, err
	}
	targetClusterConf, err := kubernetes.NewForConfig(targetConf)
	if err != nil {
		log.Error(err, "unable to create client config")
		return ctrl.Result{}, err
	}

	// create serviceaccount in target cluster
	// TODO: do CreateOrUpdate if serviceaccount already exists

	serviceAccount, err := targetClusterConf.CoreV1().ServiceAccounts("kube-system").Get(context.TODO(), "argocd-manager", v1.GetOptions{})
	if err != nil {
		// TODO: feels like a code smell since its setting values on something nested
		// that is depended on elsewhere
		serviceAccount, err = targetClusterConf.CoreV1().ServiceAccounts("kube-system").Create(context.TODO(), &corev1.ServiceAccount{
			ObjectMeta: v1.ObjectMeta{
				Name:      "argocd-manager",
				Namespace: "kube-system",
			},
		}, v1.CreateOptions{})
		if err != nil {
			log.Error(err, "unable to create serviceaccount")
			return ctrl.Result{Requeue: true}, err
		}
	}

	// retrieve the service account bearer token
	var bearerToken []byte // we need this later on for ArgoCD
	s, err := targetClusterConf.CoreV1().Secrets("kube-system").Get(context.TODO(), serviceAccount.Secrets[0].Name, v1.GetOptions{})
	if err != nil {
		log.Error(err, "unable to retrieve secret")
		return ctrl.Result{Requeue: true}, err
	}
	bearerToken = s.Data["token"]

	// create clusterrole
	// TODO: check if already exists
	if _, err = targetClusterConf.RbacV1().ClusterRoles().Get(context.TODO(), "argocd-manager-role", v1.GetOptions{}); err != nil {
		_, err = targetClusterConf.RbacV1().ClusterRoles().Create(context.TODO(), &rbacv1.ClusterRole{
			ObjectMeta: v1.ObjectMeta{
				Name:      "argocd-manager-role",
				Namespace: "kube-system",
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"*"},
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
				{
					NonResourceURLs: []string{"*"},
					Verbs:           []string{"*"},
				},
			},
		}, v1.CreateOptions{})
		if err != nil {
			log.Error(err, "unable to create clusterrole")
			return ctrl.Result{Requeue: true}, err
		}
	}

	// create clusterrolebinding
	// TODO: check if needs to be updated or is out of sync
	if _, err = targetClusterConf.RbacV1().ClusterRoleBindings().Get(context.TODO(), "argocd-manager-role-binding", v1.GetOptions{}); err != nil {
		_, err = targetClusterConf.RbacV1().ClusterRoleBindings().Create(context.TODO(), &rbacv1.ClusterRoleBinding{
			ObjectMeta: v1.ObjectMeta{
				Name:      "argocd-manager-role-binding",
				Namespace: "kube-system",
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     "argocd-manager-role",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "argocd-manager",
					Namespace: "kube-system",
				},
			},
		}, v1.CreateOptions{})
		if err != nil {
			log.Error(err, "unable to create clusterrolebinding")
			return ctrl.Result{}, err
		}
	}

	// TODO: create argocd cluster secret structure
	// we need a `config` key that follows this structure:
	// 		https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#clusters
	// for our other clusters it seems we just have tlsClientConfig.caData set and insecure set

	// create config structure for argocd
	clusterConfig := ClusterConfig{
		BearerToken: string(bearerToken),
		TLSClientConfig: TLSClientConfig{
			Insecure: false,
			CAData:   targetConf.TLSClientConfig.CAData,
		},
	}
	// marshall config to json
	data, err := json.Marshal(clusterConfig)
	if err != nil {
		log.Error(err, "unable to marshall cluster config")
		return ctrl.Result{}, err
	}

	// create cluster secret
	log.Info("cluster", "servername", cluster.Name, "host", targetConf.Host)
	if _, err = clientset.CoreV1().Secrets(cluster.Namespace).Get(context.TODO(), fmt.Sprintf("cluster-%s", cluster.Name), v1.GetOptions{}); err != nil {
		_, err = clientset.CoreV1().Secrets("argocd").Create(context.TODO(), &corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      fmt.Sprintf("cluster-%s", cluster.Name),
				Namespace: "argocd",
				Annotations: map[string]string{
					"managed-by": "argocd.argoproj.io",
				},
				Labels: map[string]string{
					"argocd.argoproj.io/secret-type": "cluster",
				},
			},
			Type: "Opaque",
			Data: map[string][]byte{
				"config": data,
				"name":   []byte(cluster.Name),
				"server": []byte(targetConf.Host),
			},
		}, v1.CreateOptions{})
		if err != nil {
			log.Error(err, "unable to create argocd secret")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

// In true go fashion, I spent most of my time trying to resolve go mod issues
// so that I could import this one thing, I am now including it here directly
// for the time being. This is sourced from ArgoCD codebase located here:
// 		https://github.com/argoproj/argo-cd/blob/master/pkg/apis/application/v1alpha1/types.go
type ClusterConfig struct {
	// Server requires Basic authentication
	Username string `json:"username,omitempty" protobuf:"bytes,1,opt,name=username"`
	Password string `json:"password,omitempty" protobuf:"bytes,2,opt,name=password"`

	// Server requires Bearer authentication. This client will not attempt to use
	// refresh tokens for an OAuth2 flow.
	// TODO: demonstrate an OAuth2 compatible client.
	BearerToken string `json:"bearerToken,omitempty" protobuf:"bytes,3,opt,name=bearerToken"`

	// TLSClientConfig contains settings to enable transport layer security
	TLSClientConfig `json:"tlsClientConfig" protobuf:"bytes,4,opt,name=tlsClientConfig"`
}

// TLSClientConfig contains settings to enable transport layer security
type TLSClientConfig struct {
	// Insecure specifies that the server should be accessed without verifying the TLS certificate. For testing only.
	Insecure bool `json:"insecure" protobuf:"bytes,1,opt,name=insecure"`
	// ServerName is passed to the server for SNI and is used in the client to check server
	// certificates against. If ServerName is empty, the hostname used to contact the
	// server is used.
	ServerName string `json:"serverName,omitempty" protobuf:"bytes,2,opt,name=serverName"`
	// CertData holds PEM-encoded bytes (typically read from a client certificate file).
	// CertData takes precedence over CertFile
	CertData []byte `json:"certData,omitempty" protobuf:"bytes,3,opt,name=certData"`
	// KeyData holds PEM-encoded bytes (typically read from a client certificate key file).
	// KeyData takes precedence over KeyFile
	KeyData []byte `json:"keyData,omitempty" protobuf:"bytes,4,opt,name=keyData"`
	// CAData holds PEM-encoded bytes (typically read from a root certificates bundle).
	// CAData takes precedence over CAFile
	CAData []byte `json:"caData,omitempty" protobuf:"bytes,5,opt,name=caData"`
}
