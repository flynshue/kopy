/*
Copyright 2024.

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

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ConfigMapReconciler reconciles a ConfigMap object
type ConfigMapReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	syncKey              = "flynshue.io/sync"
	sourceLabelName      = "flynshue.io/origin.name"
	sourceLabelNamespace = "flynshue.io/origin.namespace"
	syncFinalizer        = "flynshue.io/finalizer"
)

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ConfigMap object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	if req.Name == "" && req.Namespace == "" {
		return ctrl.Result{}, nil
	}
	// delete log statement later; using this to debugging reconcile
	// log.Info("configMap event received")
	var configMap corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to to get configmap")
		return ctrl.Result{}, err
	}
	selector, isSource := hasSyncOptions(&configMap)

	if configMap.DeletionTimestamp != nil && ctrlutil.ContainsFinalizer(&configMap, syncFinalizer) {
		log.Info("configmap marked for deletion")
		if isSource {
			if err := r.sourceDeletion(ctx, &configMap); err != nil {
				return ctrl.Result{Requeue: true}, err
			}
			return ctrl.Result{}, nil
		}
		if r.isNamespaceMarkedForDelete(ctx, req.Namespace) {
			log.Info("namespace marked for deletion")
			ctrlutil.RemoveFinalizer(&configMap, syncFinalizer)
			if err := r.Update(ctx, &configMap); err != nil {
				log.Error(err, "unable to remove the finalizer from configMap")
				return ctrl.Result{}, err
			}
			log.Info("namespace marked for deletion, removed finalizer from configMap")
			return ctrl.Result{}, nil
		}
		log.Info("configMap was marked for deletion but was a copy; will trigger sync")
		if err := r.syncDeletedConfigmap(ctx, configMap); err != nil {
			log.Error(err, "unable to sync deleted configmap")
			return ctrl.Result{}, err
		}
		log.Info("successfully synced configmap")
		return ctrl.Result{}, nil

	}
	if isSource {
		ctrlutil.AddFinalizer(&configMap, syncFinalizer)
		if err := r.Update(ctx, &configMap); err != nil {
			return ctrl.Result{}, err
		}
		namespaces, err := r.getSyncNamespaces(ctx, selector)
		if err != nil {
			log.Error(err, "unable to grab list of namespaces with sync key", "namespace.selector", selector.String())
			return ctrl.Result{}, err
		}
		for _, n := range namespaces {
			cm := copyConfigMap(&configMap, n.Name)
			if err := r.syncConfigMap(ctx, cm); err != nil {
				log.Error(err, "unable to sync configmap")
			}
			log.Info("successfully synced configmap", "target.Namespace", n.Name)
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *ConfigMapReconciler) sourceDeletion(ctx context.Context, cm *corev1.ConfigMap) error {
	set := labels.Set(map[string]string{sourceLabelName: cm.Name, sourceLabelNamespace: cm.Namespace})
	opts := &client.ListOptions{LabelSelector: set.AsSelector()}
	copies, err := r.listConfigMaps(ctx, opts)
	if err != nil {
		return fmt.Errorf("unable to find list of the copies for the source configmap")
	}
	log := ctrllog.FromContext(ctx)
	errs := make([]error, 0, len(copies))
	for _, cp := range copies {
		log.Info("need to remove finalizer from copy", "copy.ConfigMap", cp.Name, "copy.Namespace", cp.Namespace)
		ctrlutil.RemoveFinalizer(&cp, syncFinalizer)
		if err := r.Update(ctx, &cp); err != nil {
			errs = append(errs, fmt.Errorf("unable to remove finalizer from copy in namespace %s", cp.Namespace))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	ctrlutil.RemoveFinalizer(cm, syncFinalizer)
	return r.Update(ctx, cm)
}

func (r *ConfigMapReconciler) syncDeletedConfigmap(ctx context.Context, cm corev1.ConfigMap) error {
	srcNamespace := cm.Labels[sourceLabelNamespace]
	srcName := cm.Labels[sourceLabelName]
	srcConfigMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: srcNamespace, Name: srcName}, srcConfigMap); err != nil {
		return err
	}
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: cm.Namespace, Name: cm.Namespace}, ns); err != nil {
		return err
	}
	ctrlutil.RemoveFinalizer(&cm, syncFinalizer)
	if err := r.Update(ctx, &cm); err != nil {
		return err
	}
	if namespaceContainsSyncLabel(srcConfigMap, ns) {
		cp := copyConfigMap(srcConfigMap, ns.Name)
		return r.syncConfigMap(ctx, cp)
	}
	return fmt.Errorf("namespace: %s is missing the sync labels", ns.Name)
}

func hasSyncOptions(cm *corev1.ConfigMap) (labels.Selector, bool) {
	v, ok := cm.Annotations[syncKey]
	if !ok {
		return nil, false
	}
	ls, err := labels.Parse(v)
	if err != nil {
		return nil, false
	}
	return ls, true
}

func (r *ConfigMapReconciler) getSyncNamespaces(ctx context.Context, selector labels.Selector) ([]corev1.Namespace, error) {
	namespaceList := &corev1.NamespaceList{}
	opts := &client.ListOptions{LabelSelector: selector}
	if err := r.List(ctx, namespaceList, opts); err != nil {
		return nil, fmt.Errorf("unable to list namespaces")
	}
	namespaces := make([]corev1.Namespace, len(namespaceList.Items))
	for i, ns := range namespaceList.Items {
		if ns.DeletionTimestamp == nil {
			namespaces[i] = ns
		}
	}
	return namespaces, nil
}

func copyConfigMap(src *corev1.ConfigMap, targetNamespace string) *corev1.ConfigMap {
	dstCM := &corev1.ConfigMap{
		Data:       src.Data,
		BinaryData: src.BinaryData,
		ObjectMeta: metav1.ObjectMeta{
			Name:      src.Name,
			Namespace: targetNamespace,
			Labels: map[string]string{
				sourceLabelNamespace: src.Namespace,
			},
		},
	}
	ctrlutil.AddFinalizer(dstCM, syncFinalizer)
	return dstCM
}

func (r *ConfigMapReconciler) syncConfigMap(ctx context.Context, duplicate *corev1.ConfigMap) error {
	if err := r.Create(ctx, duplicate); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := r.Update(ctx, duplicate); err != nil {
				return fmt.Errorf("unable to update configmap copy")
			}
			return nil
		}
		return fmt.Errorf("syncConfigMap(); error creating configmap: %s in namespace: %s", duplicate.Name, duplicate.Namespace)
	}
	return nil
}

func (r *ConfigMapReconciler) watchNamespaces(ctx context.Context, namespace client.Object) []reconcile.Request {
	log := ctrllog.FromContext(ctx)
	if r.isNamespaceMarkedForDelete(ctx, namespace.GetName()) {
		return nil
	}
	configMaps, err := r.listConfigMaps(ctx, nil)
	if err != nil {
		log.Info("unable to grab a list of configmaps")
		return nil
	}
	req := make([]reconcile.Request, len(configMaps))
	for i, cm := range configMaps {
		v, ok := cm.Annotations[syncKey]
		if !ok {
			continue
		}
		syncLabel := strings.Split(v, "=")
		labelKey := syncLabel[0]
		labelValue := syncLabel[1]
		nsLabels := namespace.GetLabels()
		if nsLabels[labelKey] == labelValue {
			req[i] = reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: cm.GetNamespace(),
				Name:      cm.GetName(),
			}}
			log.Info("need to add reconile", "source.configMap", cm.GetName(), "source.Namespace", cm.GetNamespace(), "target.Namespace", namespace.GetName())
		}

	}
	return req
}

func namespaceContainsSyncLabel(cm *corev1.ConfigMap, namespace client.Object) bool {
	v, ok := cm.Annotations[syncKey]
	if !ok {
		return false
	}
	label := strings.Split(v, "=")
	key := label[0]
	value := label[1]
	return namespace.GetLabels()[key] == value
}

func (r *ConfigMapReconciler) isNamespaceMarkedForDelete(ctx context.Context, namespace string) bool {
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: namespace, Namespace: namespace}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return true
		}
	}
	if ns.Status.Phase == corev1.NamespaceTerminating {
		return true
	}
	return false
}

func (r *ConfigMapReconciler) listConfigMaps(ctx context.Context, opts client.ListOption) ([]corev1.ConfigMap, error) {
	configMapList := &corev1.ConfigMapList{}
	if opts == nil {
		if err := r.List(ctx, configMapList); err != nil {
			return nil, err
		}
		return configMapList.Items, nil
	}
	if err := r.List(ctx, configMapList, opts); err != nil {
		return nil, err
	}
	return configMapList.Items, nil
}

var p = predicate.Funcs{
	DeleteFunc: func(e event.DeleteEvent) bool {
		return !e.DeleteStateUnknown

	},
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Watches(&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.watchNamespaces),
			// builder.WithPredicates(p),
		).
		Complete(r)
}
