package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func isNamespaceMarkedForDelete(ctx context.Context, c client.Client, namespace string) bool {
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: namespace, Namespace: namespace}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return true
		}
	}
	if ns.Status.Phase == corev1.NamespaceTerminating {
		return true
	}
	return false
}

func namespaceContainsSyncLabel(o client.Object, namespace client.Object) bool {
	annotations := o.GetAnnotations()
	v, ok := annotations[syncKey]
	if !ok {
		return false
	}
	label := strings.Split(v, "=")
	key := label[0]
	value := label[1]
	return namespace.GetLabels()[key] == value
}

func getSyncNamespaces(ctx context.Context, c client.Client, req ctrl.Request, selector labels.Selector) ([]corev1.Namespace, error) {
	namespaceList := &corev1.NamespaceList{}
	opts := &client.ListOptions{LabelSelector: selector}
	if err := c.List(ctx, namespaceList, opts); err != nil {
		return nil, fmt.Errorf("unable to list namespaces")
	}
	namespaces := make([]corev1.Namespace, 0, len(namespaceList.Items))
	for _, ns := range namespaceList.Items {
		if ns.Name == req.Namespace {
			continue
		}
		if ns.DeletionTimestamp == nil {
			namespaces = append(namespaces, ns)
		}
	}
	return namespaces, nil
}

func listOptions(o client.Object) *client.ListOptions {
	set := labels.Set(map[string]string{sourceLabelNamespace: o.GetNamespace()})
	return &client.ListOptions{LabelSelector: set.AsSelector()}
}
