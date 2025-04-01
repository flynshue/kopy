package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

type Kopier interface {
	AddFinalizer() error
	Fetch(req ctrl.Request) error
	GetClient() client.Client
	GetContext() context.Context
	GetObject() client.Object
	LabelSelector() labels.Selector
	MarkedForDeletion() bool
	SyncOptions() bool
	SyncDeletedCopy() error
	SyncSource(namespace string) error
	SourceDeletion() error
}

const (
	syncKey              = "kopy.kot-labs.com/sync"
	sourceLabelName      = "kopy.kot-labs.com/origin.name"
	sourceLabelNamespace = "kopy.kot-labs.com/origin.namespace"
	syncFinalizer        = "kopy.kot-labs.com/finalizer"
)

// KopyReconcile runs the reconcile loop logic for Kopier interface
func KopyReconcile(k Kopier, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(k.GetContext())
	// delete log statement later; using this to debugging reconcile
	// log.Info("Event received")
	if req.Name == "" && req.Namespace == "" {
		return ctrl.Result{}, nil
	}
	if err := k.Fetch(req); err != nil {
		return ctrl.Result{}, err
	}
	if k.MarkedForDeletion() {
		log.Info("Marked for deletion")
		if k.SyncOptions() {
			if err := k.SourceDeletion(); err != nil {
				return ctrl.Result{Requeue: true}, err
			}
			return ctrl.Result{}, nil
		}
		if isNamespaceMarkedForDelete(k.GetContext(), k.GetClient(), req.Namespace) {
			log.Info("namespace marked for deletion")
			ctrlutil.RemoveFinalizer(k.GetObject(), syncFinalizer)
			if err := k.GetClient().Update(k.GetContext(), k.GetObject()); err != nil {
				log.Error(err, "unable to remove the finalizer from object")
				return ctrl.Result{}, err
			}
			log.Info("namespace marked for deletion, removed finalizer from object")
			return ctrl.Result{}, nil
		}
		log.Info("Object is a copy that is marked for deletion; will trigger sync")
		if err := k.SyncDeletedCopy(); err != nil {
			log.Error(err, "unable to sync deleted object")
			return ctrl.Result{}, err
		}
		log.Info("successfully synced")
		return ctrl.Result{}, nil
	}
	if k.SyncOptions() {
		log.Info("source object")
		if err := k.AddFinalizer(); err != nil {
			return ctrl.Result{}, err
		}
		namespaces, err := getSyncNamespaces(k.GetContext(), k.GetClient(), k.LabelSelector())
		if err != nil {
			log.Error(err, "unable to grab list of namespaces with sync key", "namespace.selector", k.LabelSelector().String())
			return ctrl.Result{}, err
		}
		for _, n := range namespaces {
			if err := k.SyncSource(n.Name); err != nil {
				log.Error(err, "unable to sync object")
			}
			log.Info("successfully synced", "target.Namespace", n.Name)
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}
