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

// var (
// 	testSrcNamespace *corev1.Namespace
// 	testSrcConfigMap *corev1.ConfigMap
// )

var _ = Describe("ConfigMap Controller\n", func() {
	Context("Namespace contains sync label", func() {
		It("Should sync source configmap to target namespace", func() {
			By("Create source namespace and configmap")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				configMap *corev1.ConfigMap
			}{
				name: "test-config-00", namespace: "test-src-config-ns-00", configMap: &corev1.ConfigMap{},
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(src.namespace, &corev1.Namespace{})
				return err == nil
			}, timeout, interval).Should(BeTrue())

			data := map[string]string{"HOST": "https://test-kopy.io/foobar"}
			label := &syncLabel{key: testLabelKey, value: src.name}
			src.configMap, err = tc.CreateConfigMap(src.name, src.namespace, label, data)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				err = tc.GetConfigMap(src.name, src.namespace, src.configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(src.configMap)
			GinkgoWriter.Println(string(b))

			By("Create target namespace with sync labels")
			targetNamespace, err := tc.CreateNamespace("test-target-config-ns-00", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(targetNamespace.Name, targetNamespace)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(targetNamespace)
			GinkgoWriter.Println(string(b))

			By("Check target namespace for synced configMap")
			copy := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := tc.GetConfigMap(src.name, targetNamespace.Name, copy)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(copy)
			GinkgoWriter.Println(string(b))

			By("Update source configMap data")
			Expect(tc.GetConfigMap(src.name, src.namespace, src.configMap)).ShouldNot(HaveOccurred())
			src.configMap.Data = map[string]string{"HOST": "https://test-kopy.io/"}
			Expect(tc.UpdateConfigMap(src.configMap)).ShouldNot(HaveOccurred())
			b, _ = yaml.Marshal(src.configMap)
			GinkgoWriter.Println(string(b))

			By("Verify configMap copy in target namespace was updated")
			Eventually(func() bool {
				tc.GetConfigMap(src.name, targetNamespace.Name, copy)
				return reflect.DeepEqual(copy.Data, src.configMap.Data)
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(copy)
			GinkgoWriter.Println(string(b))

		})
	})
	Context("Namespace doesn't doesn't contain sync label", func() {
		It("Should not sync source configMap", func() {
			By("Create source namespace and configMap")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				configMap *corev1.ConfigMap
			}{
				name: "test-config-01", namespace: "test-src-config-ns-01", configMap: &corev1.ConfigMap{},
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(src.namespace, &corev1.Namespace{})
				return err == nil
			}, timeout, interval).Should(BeTrue())

			data := map[string]string{"HOST": "https://test-kopy.io/"}
			label := &syncLabel{key: testLabelKey, value: src.name}
			src.configMap, err = tc.CreateConfigMap(src.name, src.namespace, label, data)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				err = tc.GetConfigMap(src.name, src.namespace, src.configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(src.configMap)
			GinkgoWriter.Println(string(b))

			By("Creating Namespace without sync labels")
			targetNamespace, err := tc.CreateNamespace("test-target-config-ns-01", nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(targetNamespace.Name, targetNamespace)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(targetNamespace)
			GinkgoWriter.Println(string(b))

			By("Verifying Namespace doesn't contain configMap")
			Consistently(func() bool {
				err := tc.GetConfigMap(src.configMap.Name, targetNamespace.Name, &corev1.ConfigMap{})
				return apierrors.IsNotFound(err)
			}, time.Second*2, interval).Should(BeTrue())
		})
	})
	Context("When source configMap name is 253 characters", func() {
		It("Should successfully sync secret", func() {
			By("Create source namespace")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				configMap *corev1.ConfigMap
			}{
				namespace: "test-src-config-ns-02", configMap: &corev1.ConfigMap{},
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			By("Creating new source configMap with 253 characters")
			src.name = rand.String(253)
			label := &syncLabel{key: src.namespace, value: "testSecretLongNames"}
			data := map[string]string{"HOST": "https://test-kopy.io/"}
			src.configMap, err = tc.CreateConfigMap(src.name, src.namespace, label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetConfigMap(src.name, src.namespace, src.configMap), timeout, interval).Should(Succeed())
			b, _ := yaml.Marshal(src.configMap)
			GinkgoWriter.Println(string(b))

			By("Creating new target namespace")
			targetNamespace, err := tc.CreateNamespace("test-target-config-ns-02", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(targetNamespace.Name, targetNamespace), timeout, interval).Should(Succeed())

			By("Checking for target namespace for copy of configMap")
			targetConfigMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := tc.GetConfigMap(src.name, targetNamespace.Name, targetConfigMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(targetConfigMap)
			GinkgoWriter.Println(string(b))
		})
	})
	Context("When source configMap is deleted", func() {
		It("Should remove the finalizer from the copies", func() {
			By("Creating Source Namespace and configMap")
			src := struct {
				name      string
				namespace string
				configMap *corev1.ConfigMap
			}{
				name: "test-config-03", namespace: "test-src-config-ns-03", configMap: &corev1.ConfigMap{},
			}
			tc = NewTestClient(context.Background())
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ToNot(HaveOccurred())
			Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			label := &syncLabel{key: testLabelKey, value: src.name}
			data := map[string]string{"HOST": "https://test-kopy.io/"}
			src.configMap, err = tc.CreateConfigMap(src.name, src.namespace, label, data)
			Expect(err).ToNot(HaveOccurred())
			Eventually(tc.GetConfigMap(src.name, src.namespace, src.configMap), timeout, interval).Should(Succeed())

			By("Creating target namespaces and checking for copies")
			testCases := []struct {
				name      string
				namespace *corev1.Namespace
				configMap *corev1.ConfigMap
			}{
				{name: "target-tt-config-01", namespace: &corev1.Namespace{}, configMap: &corev1.ConfigMap{}},
				{name: "target-tt-config-02", namespace: &corev1.Namespace{}, configMap: &corev1.ConfigMap{}},
				{name: "target-tt-config-03", namespace: &corev1.Namespace{}, configMap: &corev1.ConfigMap{}},
			}
			for _, t := range testCases {
				t.namespace, err = tc.CreateNamespace(t.name, label)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(tc.GetNamespace(t.name, t.namespace), timeout, interval).Should(Succeed())
				Eventually(func() bool {
					tc.GetConfigMap(src.name, t.name, t.configMap)
					return reflect.DeepEqual(src.configMap.Data, t.configMap.Data)
				}, timeout, interval).Should(BeTrue())
			}

			By("Deleting source configMap")
			err = tc.DeleteConfigmap(src.configMap)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetConfigMap(src.name, src.namespace, src.configMap)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Verifying finalizer has been removed from copies")
			for _, t := range testCases {
				Eventually(func() bool {
					tc.GetConfigMap(src.name, t.name, t.configMap)
					return !slices.Contains(t.configMap.Finalizers, syncFinalizer)
				}, timeout, interval).Should(BeTrue())
			}

		})
	})
	Context("When copy configMap is deleted", func() {
		It("Should resync the copy to the appropriate namespace", func() {
			By("Creating Source Namespace and configMap")
			src := struct {
				name      string
				namespace string
				configMap *corev1.ConfigMap
			}{
				name: "test-src-config-04", namespace: "test-src-config-ns-04", configMap: &corev1.ConfigMap{},
			}
			tc = NewTestClient(context.Background())
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			label := &syncLabel{key: "kopy-sync", value: src.name}
			data := map[string]string{"HOST": "https://test-kopy.io/"}
			src.configMap, err = tc.CreateConfigMap(src.name, src.namespace, label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetConfigMap(src.name, src.namespace, src.configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Creating target namespace")
			targetNamespace, err := tc.CreateNamespace("test-target-config-ns-04", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			By("Verifying copy configMap synced")
			targetConfigMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := tc.GetConfigMap(src.name, targetNamespace.Name, targetConfigMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(targetConfigMap)
			GinkgoWriter.Println(string(b))

			By("Deleting configMap copy from target namespace")
			Expect(tc.DeleteConfigmap(targetConfigMap)).ShouldNot(HaveOccurred())

			// failed test
			By("Verifying that configMap was recreated in the target namespace")
			Eventually(func() bool {
				newConfig := &corev1.ConfigMap{}
				tc.GetConfigMap(src.name, targetNamespace.Name, newConfig)
				return targetConfigMap.UID != newConfig.UID
			}, timeout, interval).Should(BeTrue())

		})
	})
	if useKind {
		Context("When namespace that contains copy is deleted", func() {
			It("The namespace should be deleted properly", func() {
				By("Creating new source namespace and configMap")
				tc = NewTestClient(context.Background())
				src := struct {
					name      string
					namespace string
					configMap *corev1.ConfigMap
				}{
					name: "test-src-config-05", namespace: "test-src-config-ns-05", configMap: &corev1.ConfigMap{},
				}
				_, err := tc.CreateNamespace(src.namespace, nil)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())
				label := &syncLabel{key: testLabelKey, value: src.name}
				data := map[string]string{"HOST": "https://test-kopy.io/"}

				src.configMap, err = tc.CreateConfigMap(src.name, src.namespace, label, data)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(func() bool {
					err := tc.GetConfigMap(src.name, src.namespace, src.configMap)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				By("Creating new target namespace")
				targetNamespace, err := tc.CreateNamespace("test-target-config-ns-05", label)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(func() bool {
					err := tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{})
					return err == nil
				}, timeout, interval).Should(BeTrue())

				By("Checking target namespace for copy")
				copy := &corev1.ConfigMap{}
				Eventually(func() bool {
					err := tc.GetConfigMap(src.name, targetNamespace.Name, copy)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				b, _ := yaml.Marshal(copy)
				GinkgoWriter.Println(string(b))

				By("Deleting target namespace")
				Expect(tc.DeleteNamespace(targetNamespace)).ShouldNot(HaveOccurred())

				By("Verify finalizer was removed from copy")
				Eventually(func() bool {
					copy = &corev1.ConfigMap{}
					tc.GetConfigMap(src.name, targetNamespace.Name, copy)
					return !slices.Contains(copy.Finalizers, syncFinalizer)
				}, timeout, interval).Should(BeTrue())

				By("Verifying namespace was deleted")
				Eventually(func() bool {
					err := tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{})
					return apierrors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())

			})
		})
		Context("When namespace with source configMap is deleted", func() {
			It("Should remove the finalizer from the copies and delete namespace", func() {
				By("Creating Source Namespace and configMap")
				src := struct {
					name      string
					namespace string
					configMap *corev1.ConfigMap
				}{
					name: "test-config-06", namespace: "test-src-config-ns-06", configMap: &corev1.ConfigMap{},
				}
				tc = NewTestClient(context.Background())
				_, err := tc.CreateNamespace(src.namespace, nil)
				Expect(err).ToNot(HaveOccurred())
				Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())

				label := &syncLabel{key: testLabelKey, value: src.name}
				data := map[string]string{"HOST": "https://test-kopy.io/"}
				src.configMap, err = tc.CreateConfigMap(src.name, src.namespace, label, data)
				Expect(err).ToNot(HaveOccurred())
				Eventually(tc.GetConfigMap(src.name, src.namespace, src.configMap), timeout, interval).Should(Succeed())
				b, _ := yaml.Marshal(src.configMap)
				GinkgoWriter.Println(string(b))

				By("Creating target namespace and check for copy")
				targetNamespace, err := tc.CreateNamespace("test-target-config-ns-06", label)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{}), timeout, interval).Should(Succeed())
				copy := &corev1.ConfigMap{}
				Eventually(func() bool {
					tc.GetConfigMap(src.name, targetNamespace.Name, copy)
					return reflect.DeepEqual(src.configMap.Data, copy.Data)
				}, timeout, interval).Should(BeTrue())
				b, _ = yaml.Marshal(copy)
				GinkgoWriter.Println(string(b))

				By("Deleting source namespace")
				ns := &corev1.Namespace{}
				err = tc.GetNamespace(src.namespace, ns)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(tc.DeleteNamespace(ns)).ShouldNot(HaveOccurred())

				By("Verify finalizer has been removed from copy")
				Eventually(func() bool {
					copy = &corev1.ConfigMap{}
					tc.GetConfigMap(src.name, targetNamespace.Name, copy)
					return !slices.Contains(copy.Finalizers, syncFinalizer)
				}, timeout, interval).Should(BeTrue())
				b, _ = yaml.Marshal(copy)
				GinkgoWriter.Println(string(b))

				By("Verify source namespace has been deleted")
				Eventually(func() bool {
					err := tc.GetNamespace(src.namespace, &corev1.Namespace{})
					return err != nil
				}, timeout, interval).Should(BeTrue())
			})
		})
	}
})
