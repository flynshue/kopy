package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SecretReconciler reconciles a Secret object
type SecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Secret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	if req.Name == "" && req.Namespace == "" {
		return ctrl.Result{}, nil
	}
	// delete log statement later; using this to debugging reconcile
	// log.Info("configMap event received")
	var secret corev1.Secret
	if err := r.Get(ctx, req.NamespacedName, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to to get secret")
		return ctrl.Result{}, err
	}
	selector, isSource := secretSyncOptions(&secret)

	if secret.DeletionTimestamp != nil && ctrlutil.ContainsFinalizer(&secret, syncFinalizer) {
		log.Info("secret marked for deletion")
		if isSource {
			if err := r.sourceDeletion(ctx, &secret); err != nil {
				return ctrl.Result{Requeue: true}, err
			}
			return ctrl.Result{}, nil
		}
		if r.isNamespaceMarkedForDelete(ctx, req.Namespace) {
			log.Info("namespace marked for deletion")
			ctrlutil.RemoveFinalizer(&secret, syncFinalizer)
			if err := r.Update(ctx, &secret); err != nil {
				log.Error(err, "unable to remove the finalizer from secret")
				return ctrl.Result{}, err
			}
			log.Info("namespace marked for deletion, removed finalizer from secret")
			return ctrl.Result{}, nil
		}
		log.Info("secret was marked for deletion but was a copy; will trigger sync")
		if err := r.syncDeletedSecret(ctx, secret); err != nil {
			log.Error(err, "unable to sync deleted configmap")
			return ctrl.Result{}, err
		}
		log.Info("successfully synced configmap")
		return ctrl.Result{}, nil

	}
	if isSource {
		log.Info("source secret")
		ctrlutil.AddFinalizer(&secret, syncFinalizer)
		if err := r.Update(ctx, &secret); err != nil {
			return ctrl.Result{}, err
		}
		namespaces, err := r.getSyncNamespaces(ctx, selector)
		if err != nil {
			log.Error(err, "unable to grab list of namespaces with sync key", "namespace.selector", selector.String())
			return ctrl.Result{}, err
		}
		for _, n := range namespaces {
			cm := copySecret(&secret, n.Name)
			if err := r.syncSecret(ctx, cm); err != nil {
				log.Error(err, "unable to sync secret")
			}
			log.Info("successfully synced secret", "target.Namespace", n.Name)
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *SecretReconciler) sourceDeletion(ctx context.Context, s *corev1.Secret) error {
	set := labels.Set(map[string]string{sourceLabelNamespace: s.Namespace})
	opts := &client.ListOptions{LabelSelector: set.AsSelector()}
	copies, err := r.listSecrets(ctx, opts)
	if err != nil {
		return fmt.Errorf("unable to find list of the copies for the source secret")
	}
	log := ctrllog.FromContext(ctx)
	errs := make([]error, 0, len(copies))
	for _, cp := range copies {
		if cp.Name != s.Name {
			continue
		}
		if ctrlutil.ContainsFinalizer(&cp, syncFinalizer) {
			log.Info("need to remove finalizer from copy", "copy.Secret", cp.Name, "copy.Namespace", cp.Namespace)
			ctrlutil.RemoveFinalizer(&cp, syncFinalizer)
			if err := r.Update(ctx, &cp); err != nil {
				errs = append(errs, fmt.Errorf("unable to remove finalizer from copy in namespace %s", cp.Namespace))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	ctrlutil.RemoveFinalizer(s, syncFinalizer)
	return r.Update(ctx, s)
}

func (r *SecretReconciler) listSecrets(ctx context.Context, opts client.ListOption) ([]corev1.Secret, error) {
	secretList := &corev1.SecretList{}
	if opts == nil {
		if err := r.List(ctx, secretList); err != nil {
			return nil, err
		}
		return secretList.Items, nil
	}
	if err := r.List(ctx, secretList, opts); err != nil {
		return nil, err
	}
	return secretList.Items, nil
}

func (r *SecretReconciler) isNamespaceMarkedForDelete(ctx context.Context, namespace string) bool {
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

func (r *SecretReconciler) syncDeletedSecret(ctx context.Context, s corev1.Secret) error {
	srcNamespace := s.Labels[sourceLabelNamespace]
	srcName := s.Labels[sourceLabelName]
	srcSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: srcNamespace, Name: srcName}, srcSecret); err != nil {
		return err
	}
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: s.Namespace}, ns); err != nil {
		return err
	}
	ctrlutil.RemoveFinalizer(&s, syncFinalizer)
	if err := r.Update(ctx, &s); err != nil {
		return err
	}
	if secretNamespaceContainsSyncLabel(srcSecret, ns) {
		cp := copySecret(srcSecret, ns.Name)
		return r.syncSecret(ctx, cp)
	}
	return fmt.Errorf("namespace: %s is missing the sync labels", ns.Name)
}

func (r *SecretReconciler) syncSecret(ctx context.Context, duplicate *corev1.Secret) error {
	if err := r.Create(ctx, duplicate); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := r.Update(ctx, duplicate); err != nil {
				return fmt.Errorf("unable to update secret copy")
			}
			return nil
		}
		return fmt.Errorf("syncSecret(); error creating secret: %s in namespace: %s", duplicate.Name, duplicate.Namespace)
	}
	return nil
}

func (r *SecretReconciler) getSyncNamespaces(ctx context.Context, selector labels.Selector) ([]corev1.Namespace, error) {
	log := ctrllog.FromContext(context.Background())
	log.Info("Get namespaces", "syncLabel", selector.String())
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
	log.Info("Found Namespaces", "namespaces", namespaces)
	return namespaces, nil
}

func secretNamespaceContainsSyncLabel(s *corev1.Secret, namespace client.Object) bool {
	v, ok := s.Annotations[syncKey]
	if !ok {
		return false
	}
	label := strings.Split(v, "=")
	key := label[0]
	value := label[1]
	return namespace.GetLabels()[key] == value
}

func copySecret(src *corev1.Secret, targetNamespace string) *corev1.Secret {
	dstSecret := &corev1.Secret{
		Data:       src.Data,
		StringData: src.StringData,
		ObjectMeta: metav1.ObjectMeta{
			Name:      src.Name,
			Namespace: targetNamespace,
			Labels: map[string]string{
				sourceLabelNamespace: src.Namespace,
			},
		},
	}
	ctrlutil.AddFinalizer(dstSecret, syncFinalizer)
	return dstSecret
}

func (r *SecretReconciler) watchNamespaces(ctx context.Context, namespace client.Object) []reconcile.Request {
	log := ctrllog.FromContext(ctx)
	if r.isNamespaceMarkedForDelete(ctx, namespace.GetName()) {
		return nil
	}
	secrets, err := r.listSecrets(ctx, nil)
	if err != nil {
		log.Info("unable to grab a list of configmaps")
		return nil
	}
	req := make([]reconcile.Request, len(secrets))
	for i, s := range secrets {
		v, ok := s.Annotations[syncKey]
		if !ok {
			continue
		}
		syncLabel := strings.Split(v, "=")
		labelKey := syncLabel[0]
		labelValue := syncLabel[1]
		nsLabels := namespace.GetLabels()
		if nsLabels[labelKey] == labelValue {
			req[i] = reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: s.GetNamespace(),
				Name:      s.GetName(),
			}}
			log.Info("need to add reconile", "source.configMap", s.GetName(), "source.Namespace", s.GetNamespace(), "target.Namespace", namespace.GetName())
		}

	}
	return req
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		Watches(&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.watchNamespaces),
			// builder.WithPredicates(p),
		).
		Complete(r)
}

func secretSyncOptions(s *corev1.Secret) (labels.Selector, bool) {
	v, ok := s.Annotations[syncKey]
	if !ok {
		return nil, false
	}
	ls, err := labels.Parse(v)
	if err != nil {
		return nil, false
	}
	return ls, true
}

func printSecret(s *corev1.Secret) {
	b, _ := yaml.Marshal(s)
	fmt.Println(string(b))
}
