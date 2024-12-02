package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type syncLabel struct {
	key   string
	value string
}

type testClient struct {
	ctx context.Context
}

func NewTestClient(ctx context.Context) testClient {
	return testClient{ctx: ctx}
}

func (tc testClient) GetNamespace(name string, ns *corev1.Namespace) error {
	return k8sClient.Get(tc.ctx, types.NamespacedName{Name: name}, ns)
}

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

func (tc testClient) GetConfigMap(name, namespace string, cm *corev1.ConfigMap) error {
	return k8sClient.Get(tc.ctx, types.NamespacedName{Name: name, Namespace: namespace}, cm)
}

func (tc testClient) GetSecret(name, namespace string, s *corev1.Secret) error {
	return k8sClient.Get(tc.ctx, types.NamespacedName{Name: name, Namespace: namespace}, s)
}

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

func (tc testClient) CreateSecret(name, namespace string, label *syncLabel, data map[string][]byte) (*corev1.Secret, error) {
	s := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
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

func (tc testClient) UpdateConfigMap(cm *corev1.ConfigMap) error {
	return k8sClient.Update(tc.ctx, cm)
}

func (tc testClient) UpdateSecret(s *corev1.Secret) error {
	return k8sClient.Update(tc.ctx, s)
}

func (tc testClient) DeleteConfigmap(cm *corev1.ConfigMap) error {
	return k8sClient.Delete(tc.ctx, cm)
}

func (tc testClient) DeleteSecret(s *corev1.Secret) error {
	return k8sClient.Delete(tc.ctx, s)
}

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

func (tc testClient) DeleteNamespace(ns *corev1.Namespace) error {
	return k8sClient.Delete(tc.ctx, ns)
}
