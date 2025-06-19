package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
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

var _ Kopier = &KopySecret{}

type KopySecret struct {
	context.Context
	client.Client
	*corev1.Secret
}

// NewKopySecret creates a new instance of KopySecret
func NewKopySecret(ctx context.Context, c client.Client) *KopySecret {
	return &KopySecret{Context: ctx, Client: c, Secret: &corev1.Secret{}}
}

// AddFinalizer adds finalizer to secret object and updates object in kubernetes cluster
func (ks *KopySecret) AddFinalizer() error {
	ctrlutil.AddFinalizer(ks.Secret, syncFinalizer)
	if err := ks.Update(ks.Context, ks.Secret); err != nil {
		return err
	}
	return nil
}

// Copy takes the Secret Object and creates a copy in the provided target namespace
func (ks *KopySecret) Copy(s *corev1.Secret, namespace string) error {
	copy := &corev1.Secret{
		Data:       s.Data,
		StringData: s.StringData,
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: namespace,
			Labels: map[string]string{
				sourceLabelNamespace: s.Namespace,
			},
		},
		Type: s.Type,
	}
	ctrlutil.AddFinalizer(copy, syncFinalizer)
	if err := ks.Create(ks.Context, copy); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := ks.Update(ks.Context, copy); err != nil {
				return fmt.Errorf("unable to copy secret")
			}
			return nil
		}
		return fmt.Errorf("error copying secret %s in namespace: %s", copy.GetName(), copy.GetNamespace())
	}
	return nil
}

// Fetch uses the event request to retrieve object from the cache
func (ks *KopySecret) Fetch(req ctrl.Request) error {
	if err := ks.Get(ks.Context, req.NamespacedName, ks.Secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

// GetClient returns Reconciler client.Client
func (ks *KopySecret) GetClient() client.Client {
	return ks.Client
}

// GetContext returns Reconciler context.Context
func (ks *KopySecret) GetContext() context.Context {
	return ks.Context
}

func (ks *KopySecret) GetObject() client.Object {
	return ks.Secret
}

// LabelSelector parses the sync annotations on Secret to create a label selector
func (ks *KopySecret) LabelSelector() labels.Selector {
	annotations := ks.Secret.GetAnnotations()
	v := annotations[syncKey]
	ls, _ := labels.Parse(v)
	return ls
}

// MarkedForDeletion returns true if the Secret object is marked for deletion and contains the kopy sync finalizer field
func (ks *KopySecret) MarkedForDeletion() bool {
	return ks.Secret.DeletionTimestamp != nil && ctrlutil.ContainsFinalizer(ks.Secret, syncFinalizer)
}

// SyncDeletedCopy uses the labels on the receiver Secret object to grab a copy of the original Secret
// It will Remove the finalizer from the receiver Secret object to allow kubernetes to delete object
// It will verify the receiver Secret Object namespace still contains the sync labels first before syncing the Secret back into namespace
func (ks *KopySecret) SyncDeletedCopy() error {
	log := ks.Logger()
	originNamespace := ks.Labels[sourceLabelNamespace]
	originSecret := &corev1.Secret{}
	if err := ks.Get(ks.Context, types.NamespacedName{Namespace: originNamespace, Name: ks.Name}, originSecret); err != nil {
		return err
	}
	ns := &corev1.Namespace{}
	if err := ks.Get(ks.Context, types.NamespacedName{Namespace: ks.Namespace, Name: ks.Namespace}, ns); err != nil {
		return err
	}
	ctrlutil.RemoveFinalizer(ks.Secret, syncFinalizer)
	if err := ks.Update(ks.Context, ks.Secret); err != nil {
		return err
	}
	if namespaceContainsSyncLabel(originSecret, ns) {
		return ks.Copy(originSecret, ns.Name)
	}
	log.Info("Namespace missing sync labels")
	return nil
}

// SyncOptions returns true if the object annotations contains the sync key to be managed by the controller
func (ks *KopySecret) SyncOptions() bool {
	annotations := ks.GetAnnotations()
	_, ok := annotations[syncKey]
	return ok
}

func (ks *KopySecret) SyncSource(name, sourceNamespace, targetNamespace string) error {
	sourceSecret := &corev1.Secret{}
	req := types.NamespacedName{Namespace: sourceNamespace, Name: name}
	if err := ks.Client.Get(ks.Context, req, sourceSecret); err != nil {
		return err
	}
	// Verify that there are no other sources
	req.Namespace = targetNamespace
	targetSecret := &corev1.Secret{}
	err := ks.Client.Get(ks.Context, req, targetSecret)
	// if secret doesn't exist in targetNamespace yet, copy
	if apierrors.IsNotFound(err) {
		return ks.Copy(sourceSecret, targetNamespace)
	}
	// secret exists in the targetNamespace, need to verify if it contains labels "kopy.kot-labs.com/origin.namespace"
	origin, ok := targetSecret.Labels[sourceLabelNamespace]
	// if "kopy.kot-labs.com/origin.namespace" doesn't exist on the target secret, overwrite it
	if !ok {
		return ks.Copy(sourceSecret, targetNamespace)
	}
	if origin != sourceNamespace {
		return fmt.Errorf("%s has a different source in namespace %s", name, origin)
	}
	return ks.Copy(sourceSecret, targetNamespace)
}

// SourceDeletion will grab a list objects that are copies of the receiver Secret object and remove the
// finalizer from the copies before removing the finalizer from the receiver Secret object
func (ks *KopySecret) SourceDeletion() error {
	copies := &corev1.SecretList{}
	if err := ks.List(ks.Context, copies, listOptions(ks.Secret)); err != nil {
		return err
	}
	log := ks.Logger()
	errs := make([]error, 0, len(copies.Items))
	for _, cp := range copies.Items {
		if cp.Name != ks.Secret.Name {
			continue
		}
		if ctrlutil.ContainsFinalizer(&cp, syncFinalizer) {
			log.Info("need to remove finalizer from copy", "name", cp.Name, "namespace", cp.Namespace)
			ctrlutil.RemoveFinalizer(&cp, syncFinalizer)
			delete(cp.Labels, sourceLabelNamespace)
			log.Info("remove labels from copy", "name", cp.Name, "namespace", cp.Namespace)
			if err := ks.Update(ks.Context, &cp); err != nil {
				log.Info("unable to remove finalizer from copy in namespace " + cp.Namespace)
				errs = append(errs, fmt.Errorf("unable to remove finalizer from copy in namespace %s", cp.Namespace))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	log.Info("removing finalizer from source", "name", ks.Secret.Name)
	ctrlutil.RemoveFinalizer(ks.Secret, syncFinalizer)
	return ks.Update(ks.Context, ks.Secret)
}

func (ks *KopySecret) IsCopy() bool {
	_, ok := ks.Secret.Labels[sourceLabelNamespace]
	ctrlutil.ContainsFinalizer(ks.Secret, syncFinalizer)
	return ok && ctrlutil.ContainsFinalizer(ks.Secret, syncFinalizer)
}

func (ks *KopySecret) Logger() logr.Logger {
	return ctrllog.Log.WithValues("controller", "secret")
}
