package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ Kopier = &KopyConfigMap{}

type KopyConfigMap struct {
	context.Context
	client.Client
	*corev1.ConfigMap
}

// NewKopyConfigMap creates a new instance of KopyConfigMap
func NewKopyConfigMap(ctx context.Context, c client.Client) *KopyConfigMap {
	return &KopyConfigMap{Context: ctx, Client: c, ConfigMap: &corev1.ConfigMap{}}
}

// AddFinalizer adds finalizer to ConfigMap object and updates object in kubernetes cluster
func (ks *KopyConfigMap) AddFinalizer() error {
	ctrlutil.AddFinalizer(ks.ConfigMap, syncFinalizer)
	if err := ks.Update(ks.Context, ks.ConfigMap); err != nil {
		return err
	}
	return nil
}

// Copy takes the ConfigMap Object and creates a copy in the provided target namespace
func (ks *KopyConfigMap) Copy(s *corev1.ConfigMap, namespace string) error {
	copy := &corev1.ConfigMap{
		Data: s.Data,
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: namespace,
			Labels: map[string]string{
				sourceLabelNamespace: s.Namespace,
			},
		},
	}
	ctrlutil.AddFinalizer(copy, syncFinalizer)
	if err := ks.Create(ks.Context, copy); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := ks.Update(ks.Context, copy); err != nil {
				return fmt.Errorf("unable to copy ConfigMap")
			}
			return nil
		}
		return fmt.Errorf("error copying ConfigMap %s in namespace: %s", copy.GetName(), copy.GetNamespace())
	}
	return nil
}

// Fetch uses the event request to retrieve object from the cache
func (ks *KopyConfigMap) Fetch(req ctrl.Request) error {
	if err := ks.Get(ks.Context, req.NamespacedName, ks.ConfigMap); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

// GetClient returns Reconciler client.Client
func (ks *KopyConfigMap) GetClient() client.Client {
	return ks.Client
}

// GetContext returns Reconciler context.Context
func (ks *KopyConfigMap) GetContext() context.Context {
	return ks.Context
}

func (ks *KopyConfigMap) GetObject() client.Object {
	return ks.ConfigMap
}

// LabelSelector parses the sync annotations on ConfigMap to create a label selector
func (ks *KopyConfigMap) LabelSelector() labels.Selector {
	annotations := ks.ConfigMap.GetAnnotations()
	v := annotations[syncKey]
	ls, _ := labels.Parse(v)
	return ls
}

// MarkedForDeletion returns true if the ConfigMap object is marked for deletion and contains the kopy sync finalizer field
func (ks *KopyConfigMap) MarkedForDeletion() bool {
	return ks.ConfigMap.DeletionTimestamp != nil && ctrlutil.ContainsFinalizer(ks.ConfigMap, syncFinalizer)
}

// SyncDeletedCopy uses the labels on the receiver ConfigMap object to grab a copy of the original ConfigMap
// It will Remove the finalizer from the receiver ConfigMap object to allow kubernetes to delete object
// It will verify the receiver ConfigMap Object namespace still contains the sync labels first before syncing the ConfigMap back into namespace
func (ks *KopyConfigMap) SyncDeletedCopy() error {
	log := ctrllog.FromContext(ks.Context)
	originNamespace := ks.Labels[sourceLabelNamespace]
	originConfigMap := &corev1.ConfigMap{}
	if err := ks.Get(ks.Context, types.NamespacedName{Namespace: originNamespace, Name: ks.Name}, originConfigMap); err != nil {
		return err
	}
	ns := &corev1.Namespace{}
	if err := ks.Get(ks.Context, types.NamespacedName{Namespace: ks.Namespace, Name: ks.Namespace}, ns); err != nil {
		return err
	}
	ctrlutil.RemoveFinalizer(ks.ConfigMap, syncFinalizer)
	if err := ks.Update(ks.Context, ks.ConfigMap); err != nil {
		return err
	}
	if namespaceContainsSyncLabel(originConfigMap, ns) {
		return ks.Copy(originConfigMap, ns.Name)
	}
	log.Info("Namespace missing sync labels")
	return nil
}

// SyncOptions returns true if the object annotations contains the sync key to be managed by the controller
func (ks *KopyConfigMap) SyncOptions() bool {
	annotations := ks.GetAnnotations()
	_, ok := annotations[syncKey]
	return ok
}

func (ks *KopyConfigMap) SyncSource(namespace string) error {
	return ks.Copy(ks.ConfigMap, namespace)

}

// SourceDeletion will grab a list objects that are copies of the receiver ConfigMap object and remove the
// finalizer from the copies before removing the finalizer from the receiver ConfigMap object
func (ks *KopyConfigMap) SourceDeletion() error {
	copies := &corev1.ConfigMapList{}
	if err := ks.List(ks.Context, copies, listOptions(ks.ConfigMap)); err != nil {
		return err
	}
	log := ctrllog.FromContext(ks.Context)
	errs := make([]error, 0, len(copies.Items))
	for _, cp := range copies.Items {
		if cp.Name != ks.ConfigMap.Name {
			continue
		}
		if ctrlutil.ContainsFinalizer(&cp, syncFinalizer) {
			log.Info("need to remove finalizer from copy", "copy.ConfigMap", cp.Name, "copy.Namespace", cp.Namespace)
			ctrlutil.RemoveFinalizer(&cp, syncFinalizer)
			if err := ks.Update(ks.Context, &cp); err != nil {
				log.Info("unable to remove finalizer from copy in namespace " + cp.Namespace)
				errs = append(errs, fmt.Errorf("unable to remove finalizer from copy in namespace %s", cp.Namespace))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	log.Info("removed finalizer from source")
	ctrlutil.RemoveFinalizer(ks.ConfigMap, syncFinalizer)
	return ks.Update(ks.Context, ks.ConfigMap)
}
