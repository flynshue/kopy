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
	"reflect"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/yaml"
)

var testSrcNamespace *corev1.Namespace
var testSrcConfigMap *corev1.ConfigMap

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
			tc := NewTestClient(context.Background())
			targetNamespace, err := tc.CreateNamespace("test-target", &syncLabel{key: testLabelKey, value: testLabelValue})
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace("test-target", targetNamespace)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(targetNamespace)
			GinkgoWriter.Println(string(b))

			By("Checking to see if the configmap was synced to target namespace")
			cm := &corev1.ConfigMap{}
			srcNamespace := testSrcConfigMap.Namespace
			srcName := testSrcConfigMap.Name
			Eventually(func() bool {
				err := tc.GetConfigMap(srcName, targetNamespace.Name, cm)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(cm)
			GinkgoWriter.Println(string(b))

			By("Checking configmap for source label namespace")
			v, ok := cm.Labels[sourceLabelNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testSrcNamespace.Name))

			By("Checking configmap for finalizer")
			Expect(cm.Finalizers).Should(ContainElement(syncFinalizer))

			By("Updating source configMap")
			err = tc.GetConfigMap(srcName, srcNamespace, testSrcConfigMap)
			Expect(err).ShouldNot(HaveOccurred())
			data := map[string]string{"HOST": "https://test-fake-kubed.io/foobar"}
			testSrcConfigMap.Data = data
			err = tc.UpdateConfigMap(testSrcConfigMap)
			Expect(err).ShouldNot(HaveOccurred())
			b, _ = yaml.Marshal(testSrcConfigMap)
			GinkgoWriter.Println(string(b))
			Eventually(func() bool {
				tc.GetConfigMap(srcName, srcNamespace, testSrcConfigMap)
				return Expect(testSrcConfigMap.Data).Should(Equal(data))
			}, timeout, interval).Should(BeTrue())

			By("Checking copy configMap was updated")
			Eventually(func() bool {
				tc.GetConfigMap(testSrcConfigMap.Name, targetNamespace.Name, cm)
				return reflect.DeepEqual(cm.Data, data)
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(cm)
			GinkgoWriter.Println(string(b))
		})
	})

	Context("When namespace doesn't have sync label", func() {
		It("Copy configmap is not found", func() {
			By("Creating namespace without sync labels")
			tc := NewTestClient(context.Background())
			ns, err := tc.CreateNamespace("test-ns-nolabels", nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(ns.Name, ns)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(ns)
			GinkgoWriter.Println(string(b))

			By("Looking up source configmap in namespace")
			Consistently(func() bool {
				err := tc.GetConfigMap(testSrcConfigMap.Name, ns.Name, &corev1.ConfigMap{})
				return apierrors.IsNotFound(err)
			}, time.Second*2, interval).Should(BeTrue())
		})
	})

	Context("When source configMap is deleted", func() {
		It("Copy of configMap should remain in the target namespace", func() {
			By("Create new source configMap")
			srcNamespace := testSrcNamespace.Name
			srcName := "deleteme-config"
			data := map[string]string{"DELETEME": "true"}
			tc := NewTestClient(context.Background())
			label := &syncLabel{key: srcNamespace, value: srcName}
			srccm, err := tc.CreateConfigMap(srcName, srcNamespace, label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetConfigMap(srcName, srcNamespace, srccm), timeout, interval).Should(Succeed())
			b, _ := yaml.Marshal(srccm)
			GinkgoWriter.Println(string(b))

			By("Creating new target namespace with matching labels")
			ns, err := tc.CreateNamespace("test-target-02", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(ns.Name, ns), timeout, interval).Should(Succeed())
			b, _ = yaml.Marshal(ns)
			GinkgoWriter.Println(string(b))

			By("Checking target namespace for configMap copy")
			var dstConfig corev1.ConfigMap
			Eventually(func() bool {
				err := tc.GetConfigMap(srcName, ns.Name, &dstConfig)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(&dstConfig)
			GinkgoWriter.Println(string(b))

			By("Deleting source configMap")
			Expect(tc.DeleteConfigmap(srccm)).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetConfigMap(srcName, srcNamespace, &corev1.ConfigMap{})
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Checking copy configMap for finalizers")
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				tc.GetConfigMap(dstConfig.Name, ns.Name, cm)
				return slices.Contains(cm.Finalizers, syncFinalizer)
			}, timeout, interval).Should(BeTrue())
		})
	})
	Context("When source configMap name is 253 characters", func() {
		It("Should successfully sync configMap", func() {
			By("Creating new source configMap with 253 characters")
			tc := NewTestClient(context.Background())
			srcName := rand.String(253)
			srcNamespace := testSrcNamespace.Name
			label := &syncLabel{key: srcNamespace, value: "testLongNames"}
			data := map[string]string{"FOO": "bar"}
			srccm, err := tc.CreateConfigMap(srcName, srcNamespace, label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetConfigMap(srcName, srcNamespace, srccm), timeout, interval).Should(Succeed())

			By("Creating new target namespace")
			ns, err := tc.CreateNamespace("test-target-03", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(ns.Name, ns), timeout, interval).Should(Succeed())

			By("Checking for copy of configMap")
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := tc.GetConfigMap(srcName, ns.Name, cm)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		})
	})

})

func setUpSourceEnv() {
	ctx := context.Background()
	By("Creating test source namespace")
	tc := NewTestClient(ctx)
	var err error
	testSrcNamespace, err = tc.CreateNamespace("test-src", nil)
	Expect(err).ShouldNot(HaveOccurred())
	Eventually(func() bool {
		err := tc.GetNamespace("test-src", &corev1.Namespace{})
		return err == nil
	}, timeout, interval).Should(BeTrue())

	By("Creating test source configMap")
	data := map[string]string{"HOST": "https://test-fake-kubed.io/"}
	testSrcConfigMap, err = tc.CreateConfigMap("test-config", "test-src", &syncLabel{key: testLabelKey, value: testLabelValue}, data)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		err := tc.GetConfigMap("test-config", "test-src", testSrcConfigMap)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	b, _ := yaml.Marshal(testSrcConfigMap)
	GinkgoWriter.Println(string(b))
}

func cleanUpSourceEnv() {
	By("Cleaning up test source namespace")
	tc := NewTestClient(context.Background())
	err := tc.GetNamespace(testSrcNamespace.Name, testSrcNamespace)
	Expect(err).ShouldNot(HaveOccurred())
	Eventually(func() bool {
		err := k8sClient.Delete(ctx, testSrcNamespace)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	testSrcNamespace = &corev1.Namespace{}
	testSrcConfigMap = &corev1.ConfigMap{}
}
