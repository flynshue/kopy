package controller

import (
	"context"
	"fmt"
	"log"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNameLength(t *testing.T) {
	test1 := generateRandomName()
	test2 := generateRandomName()
	log.Println(test1)
	log.Println(test2)
	if test1 == test2 {
		t.Fail()
	}
}

type syncLabel struct {
	key   string
	value string
}

func generateRandomName() string {
	// return faker.Username(fakeopts.WithRandomStringLength(128))
	// rand.SafeEncodeString()
	return rand.String(253)
	// return ""
}

func createNamespace(name string, label *syncLabel) (*corev1.Namespace, error) {
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

func getObj(objName, objNamespace string, obj client.Object) error {
	return k8sClient.Get(context.Background(), types.NamespacedName{Name: objName, Namespace: objNamespace}, obj)
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

func (tc testClient) UpdateConfigMap(cm *corev1.ConfigMap) error {
	return k8sClient.Update(tc.ctx, cm)
}

func (tc testClient) DeleteConfigmap(cm *corev1.ConfigMap) error {
	return k8sClient.Delete(tc.ctx, cm)
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
