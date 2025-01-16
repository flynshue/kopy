package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testLabelKey   = "app"
	testLabelValue = "myTestApp"
	timeout        = time.Second * 10
	interval       = time.Millisecond * 250
)

var (
	tc testClient
)

type syncLabel struct {
	key   string
	value string
}

type testClient struct {
	ctx context.Context
}

// NewTestClient creates a new testClient will be used for running test scenarios against cluster
func NewTestClient(ctx context.Context) testClient {
	return testClient{ctx: ctx}
}

// GetNamespace gets namespace object using name and stores it ns
func (tc testClient) GetNamespace(name string, ns *corev1.Namespace) error {
	return k8sClient.Get(tc.ctx, types.NamespacedName{Name: name}, ns)
}

// CreateNamespace will namespace object in cluster using name and will use label as kopy sync label
// Returns corev1.Namespace object
func (tc testClient) CreateNamespace(name string, label *syncLabel) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
	}
	if label != nil {
		ns.Labels = map[string]string{label.key: label.value}
	}
	if err := k8sClient.Create(context.Background(), ns); err != nil {
		return nil, err
	}
	return ns, nil
}

// GetConfigMap retrieves ConfigMap object and stores it cm
func (tc testClient) GetConfigMap(name, namespace string, cm *corev1.ConfigMap) error {
	return k8sClient.Get(tc.ctx, types.NamespacedName{Name: name, Namespace: namespace}, cm)
}

// GetSecret retrieves secret object and stores it s
func (tc testClient) GetSecret(name, namespace string, s *corev1.Secret) error {
	return k8sClient.Get(tc.ctx, types.NamespacedName{Name: name, Namespace: namespace}, s)
}

// CreateConfigMap testing helper function that creates ConfigMap object with name in supplied namespace using the label to create sync annotations
func (tc testClient) CreateConfigMap(name, namespace string, label *syncLabel, data map[string]string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	if label != nil {
		cm.Annotations = map[string]string{syncKey: fmt.Sprintf("%s=%s", label.key, label.value)}
	}
	if data != nil {
		cm.Data = data
	}
	if err := k8sClient.Create(tc.ctx, cm); err != nil {
		return nil, err
	}
	return cm, nil
}

// CreateSecret testing helper function that creates secret object with name in supplied namespace using the label to create sync annotations
func (tc testClient) CreateSecret(name, namespace string, label *syncLabel, data map[string][]byte, t corev1.SecretType) (*corev1.Secret, error) {
	s := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: t,
	}
	if label != nil {
		s.Annotations = map[string]string{syncKey: fmt.Sprintf("%s=%s", label.key, label.value)}
	}
	if data != nil {
		s.Data = data
	}
	if err := k8sClient.Create(tc.ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateConfigMap updates cm configmap object against cluster
func (tc testClient) UpdateConfigMap(cm *corev1.ConfigMap) error {
	return k8sClient.Update(tc.ctx, cm)
}

// UpdateSecret updates s secret object against cluster
func (tc testClient) UpdateSecret(s *corev1.Secret) error {
	return k8sClient.Update(tc.ctx, s)
}

// DeleteConfigmap deletes cm configmap object from cluster
func (tc testClient) DeleteConfigmap(cm *corev1.ConfigMap) error {
	return k8sClient.Delete(tc.ctx, cm)
}

// DeleteConfigmap deletes s secret object from cluster
func (tc testClient) DeleteSecret(s *corev1.Secret) error {
	return k8sClient.Delete(tc.ctx, s)
}

// ListConfigMaps returns a list of ConfigMap objects from namespace
func (tc testClient) ListConfigMaps(namespace string) ([]corev1.ConfigMap, error) {
	opts := &client.ListOptions{
		Namespace: namespace,
	}
	configMapList := &corev1.ConfigMapList{}
	err := k8sClient.List(tc.ctx, configMapList, opts)
	if err != nil {
		return nil, err
	}
	return configMapList.Items, nil
}

// DeleteNamespace deletes ns namespace object from cluster
func (tc testClient) DeleteNamespace(ns *corev1.Namespace) error {
	return k8sClient.Delete(tc.ctx, ns)
}
