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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	testSrcNamespace = "test-src"
	testSrcConfigMap = "test-config"
)

var _ = Describe("ConfigMap Controller", func() {
	var testSrcNS corev1.Namespace
	var testSrcCM corev1.ConfigMap
	BeforeEach(func() {
		By("Create test namespace")
		testSrcNS = corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: testSrcNamespace}}
		ctx := context.Background()
		err := k8sClient.Create(ctx, &testSrcNS)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info("created test source namespace")
		testSrcCM = corev1.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:        testSrcConfigMap,
				Namespace:   testSrcNamespace,
				Annotations: map[string]string{syncKey: "app=myTestApp"},
			},
			Data: map[string]string{"HOST": "https://test-fake-kubed.io/"},
		}

		Eventually(k8sClient.Create(ctx, &testSrcCM)).WithTimeout(time.Second * 10).Should(Succeed())
	})
	// Context("When reconciling a resource", func() {

	// 	It("should successfully reconcile the resource", func() {

	// 		// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
	// 		// Example: If you expect a certain status condition after reconciliation, verify it here.
	// 	})
	// })
	Context("When Namespace contains sync label", func() {
		It("should sync configMap to namespace", func() {
			ctx := context.Background()
			targetNamespace := "test-target"
			err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: targetNamespace}})
			Expect(err).NotTo(HaveOccurred())
			cpConfigMap := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: targetNamespace, Name: testSrcConfigMap}, cpConfigMap)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("synced configMap: %v", cpConfigMap))
		})
	})

})
