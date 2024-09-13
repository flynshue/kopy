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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

var _ = Describe("ConfigMap Controller", func() {
	Context("When Namespace contains sync label", func() {
		ctx := context.Background()
		const (
			testLabelKey   = "app"
			testLabelValue = "myTestApp"
			timeout        = time.Second * 10
			interval       = time.Millisecond * 250
		)
		testSrcNamespace := corev1.Namespace{
			ObjectMeta: v1.ObjectMeta{Name: "test-src"},
		}
		testSrcConfigMap := corev1.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-config",
				Namespace: testSrcNamespace.Name,
				Annotations: map[string]string{
					syncKey: fmt.Sprintf("%s=%s", testLabelKey, testLabelValue),
				},
			},
			Data: map[string]string{"HOST": "https://test-fake-kubed.io/"},
		}
		BeforeEach(func() {
			By("Creating test source namespace")
			err := k8sClient.Create(ctx, &testSrcNamespace)
			Expect(err).ToNot(HaveOccurred())
			By("Creating test source configMap")
			err = k8sClient.Create(ctx, &testSrcConfigMap)
			Expect(err).ToNot(HaveOccurred())
		})
		It("should sync configMap to namespace", func() {
			By("Creating target namespace that with the sync labels")
			// targetNamespace := "test-target"
			targetNamespace := &corev1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name:   "test-target",
					Labels: map[string]string{testLabelKey: testLabelValue},
				}}
			ctx := context.Background()
			cpConfigMap := &corev1.ConfigMap{}
			Expect(k8sClient.Create(ctx, targetNamespace)).NotTo(HaveOccurred())
			b, _ := yaml.Marshal(targetNamespace)
			log.Println(string(b))

			By("Checking to see if the configmap was synced to target namespace")
			lookupKey := types.NamespacedName{Name: testSrcConfigMap.Name, Namespace: targetNamespace.Name}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cpConfigMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(cpConfigMap)
			log.Println(string(b))

			By("Checking configmap for source label name")
			v, ok := cpConfigMap.Labels[sourceLabelName]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testSrcConfigMap.Name))

			By("Checking configmap for source label namespace")
			v, ok = cpConfigMap.Labels[sourceLabelNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testSrcNamespace.Name))

			By("Checking configmap for finalizer")
			Expect(cpConfigMap.Finalizers).Should(ContainElement(syncFinalizer))
		})
	})

})
