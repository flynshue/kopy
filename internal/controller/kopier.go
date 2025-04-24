package controller

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	SyncSource(name, sourceNamespace, targetNamespace string) error
	SourceDeletion() error
	IsCopy() bool
	Logger() logr.Logger
}

const (
	syncKey              = "kopy.kot-labs.com/sync"
	sourceLabelName      = "kopy.kot-labs.com/origin.name"
	sourceLabelNamespace = "kopy.kot-labs.com/origin.namespace"
	syncFinalizer        = "kopy.kot-labs.com/finalizer"
)

// KopyReconcile runs the reconcile loop logic for Kopier interface
func KopyReconcile(k Kopier, req ctrl.Request) (ctrl.Result, error) {
	log := k.Logger().WithValues("name", req.Name, "namespace", req.Namespace)
	// delete log statement later; using this to debugging reconcile
	// log.Info("Event received")
	if req.Name == "" && req.Namespace == "" {
		return ctrl.Result{}, nil
	}
	if err := k.Fetch(req); err != nil {
		return ctrl.Result{}, err
	}
	if ctrlutil.ContainsFinalizer(k.GetObject(), syncFinalizer) {
		log.Info("object contains kopy finalizer")
		if k.MarkedForDeletion() {
			log.Info("object marked for deletion")
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
		sourceNamespace, ok := k.GetObject().GetLabels()[sourceLabelNamespace]
		if ok {
			err := k.SyncSource(req.Name, sourceNamespace, req.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
			log.Info("successfully synced", "sourceNamespace", sourceNamespace, "targetNamespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		if k.SyncOptions() {
			namespaces, err := getSyncNamespaces(k.GetContext(), k.GetClient(), req, k.LabelSelector())
			if err != nil {
				log.Error(err, "unable to grab list of namespaces with sync key", "syncKey", k.LabelSelector().String())
				return ctrl.Result{}, err
			}
			for _, n := range namespaces {
				if err := k.SyncSource(req.Name, req.Namespace, n.Name); err != nil {
					log.Error(err, "unable to sync object", "sourceNamespace", req.Namespace, "targetNamespace", n.Name)
					continue
				}
				log.Info("successfully synced", "sourceNamespace", req.Namespace, "targetNamespace", n.Name)
			}
			return ctrl.Result{}, nil
		}
		// object has a finalizer but doesn't have a source label and doesn't have sync key annotation
		// object was a source that had annotations removed and will need to remove finalizers from copies
		log.Info("sync key annotations were removed from object")
		if err := k.SourceDeletion(); err != nil {
			log.Error(err, "unable to remove finalizers")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if k.SyncOptions() {
		log.Info("new source object")
		if err := k.AddFinalizer(); err != nil {
			return ctrl.Result{}, err
		}
		namespaces, err := getSyncNamespaces(k.GetContext(), k.GetClient(), req, k.LabelSelector())
		if err != nil {
			log.Error(err, "unable to grab list of namespaces with sync key", "syncKey", k.LabelSelector().String())
			return ctrl.Result{}, err
		}
		for _, n := range namespaces {
			if err := k.SyncSource(req.Name, req.Namespace, n.Name); err != nil {
				log.Error(err, "unable to sync object", "sourceNamespace", req.Namespace, "targetNamespace", n.Name)
			}
			log.Info("successfully synced", "sourceNamespace", req.Namespace, "targetNamespace", n.Name)
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}
