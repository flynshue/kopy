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
	"fmt"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

var testSrcNamespace corev1.Namespace
var testSrcConfigMap corev1.ConfigMap

const (
	testLabelKey   = "app"
	testLabelValue = "myTestApp"
	timeout        = time.Second * 10
	interval       = time.Millisecond * 250
)

var _ = Describe("ConfigMap Controller", Ordered, func() {
	BeforeAll(setUpSourceEnv)
	AfterAll(cleanUpSourceEnv)
	Context("When Namespace contains sync label", func() {
		It("should sync configMap to namespace", func() {
			By("Creating target namespace that with the sync labels")
			targetNamespace := &corev1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name:   "test-target",
					Labels: map[string]string{testLabelKey: testLabelValue},
				}}
			ctx := context.Background()
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Create(ctx, targetNamespace)).NotTo(HaveOccurred())
			b, _ := yaml.Marshal(targetNamespace)
			log.Println(string(b))

			By("Checking to see if the configmap was synced to target namespace")
			lookupKey := types.NamespacedName{Name: testSrcConfigMap.Name, Namespace: targetNamespace.Name}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(cm)
			log.Println(string(b))

			By("Checking configmap for source label name")
			v, ok := cm.Labels[sourceLabelName]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testSrcConfigMap.Name))

			By("Checking configmap for source label namespace")
			v, ok = cm.Labels[sourceLabelNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testSrcNamespace.Name))

			By("Checking configmap for finalizer")
			Expect(cm.Finalizers).Should(ContainElement(syncFinalizer))

			By("Updating source configMap")
			srcLookupKey := types.NamespacedName{Name: testSrcConfigMap.Name, Namespace: testSrcConfigMap.Namespace}
			Expect(k8sClient.Get(ctx, srcLookupKey, &testSrcConfigMap)).ShouldNot(HaveOccurred())
			data := map[string]string{"HOST": "https://test-fake-kubed.io/foobar"}
			testSrcConfigMap.Data = data
			Expect(k8sClient.Update(ctx, &testSrcConfigMap)).ShouldNot(HaveOccurred())

			By("Checking copy configMap was updated")
			cm = &corev1.ConfigMap{}
			Eventually(func() bool {
				k8sClient.Get(ctx, lookupKey, cm)
				return Expect(cm.Data).To(Equal(data))
			}, timeout, interval)
		})
	})

	Context("When namespace doesn't have sync label", func() {
		It("Copy configmap is not found", func() {
			By("Creating namespace without sync labels")
			ns := corev1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-ns-nolabels",
				},
			}
			err := k8sClient.Create(ctx, &ns)
			Expect(err).ToNot(HaveOccurred())
			By("Looking up source configmap in namespace")
			lookupKey := types.NamespacedName{Name: testSrcConfigMap.Name, Namespace: ns.Name}
			err = k8sClient.Get(ctx, lookupKey, &corev1.ConfigMap{})
			if !apierrors.IsNotFound(err) {
				Fail(err.Error())
			}
		})
	})

	// Context("When source configMap is deleted")

})

func setUpSourceEnv() {
	testSrcNamespace = corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{Name: "test-src"},
	}
	testSrcConfigMap = corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-config",
			Namespace: testSrcNamespace.Name,
			Annotations: map[string]string{
				syncKey: fmt.Sprintf("%s=%s", testLabelKey, testLabelValue),
			},
		},
		Data: map[string]string{"HOST": "https://test-fake-kubed.io/"},
	}
	ctx := context.Background()
	By("Creating test source namespace")
	err := k8sClient.Create(ctx, &testSrcNamespace)
	Expect(err).ToNot(HaveOccurred())
	By("Creating test source configMap")
	err = k8sClient.Create(ctx, &testSrcConfigMap)
	Expect(err).ToNot(HaveOccurred())
}

func cleanUpSourceEnv() {
	By("Cleaning up test source namespace")
	Eventually(func() bool {
		err := k8sClient.Delete(ctx, &testSrcNamespace)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	testSrcNamespace = corev1.Namespace{}
	testSrcConfigMap = corev1.ConfigMap{}
	time.Sleep(time.Second * 5)
}
