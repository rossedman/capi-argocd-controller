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
	"fmt"

	corev1 "k8s.io/api/core/v1"
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

	// TODO: retrieve cluster kubeconfig from cluster-api and create client

	// create clientset from kubeconfig
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

	// retrieve cluster secret
	secret, err := clientset.CoreV1().
		Secrets(cluster.Namespace).
		Get(context.TODO(), fmt.Sprintf("%s-kubeconfig", cluster.Name), v1.GetOptions{})
	if err != nil {
		log.Error(err, "unable to retrieve secret")
		return ctrl.Result{Requeue: true}, nil
	}

	// Connect to target cluster and create serviceaccount and rolebinding
	clientConf, err := clientcmd.RESTConfigFromKubeConfig(secret.Data["value"])
	if err != nil {
		log.Error(err, "unable to load kubeconfig for target cluster")
		return ctrl.Result{}, err
	}
	clientClusterSet, err := kubernetes.NewForConfig(clientConf)
	if err != nil {
		log.Error(err, "unable to create client config")
		return ctrl.Result{}, err
	}

	// Create serviceaccount in target cluster
	// TODO: Do CreateOrUpdate if serviceaccount already exists
	_, err = clientClusterSet.CoreV1().ServiceAccounts("kube-system").Create(context.TODO(), &corev1.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "argocd-ctrl-test",
			Namespace: "kube-system",
		},
	}, v1.CreateOptions{})
	if err != nil {
		log.Error(err, "unable to create serviceaccount")
		return ctrl.Result{}, err
	}

	// TODO: create argocd cluster secret in proper namespace

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
