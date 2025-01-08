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

var _ = Describe("Secret Controller\n", func() {
	Context("Namespace contains sync label", func() {
		It("Should sync source secret to target namespace", func() {
			By("Create source namespace and secret")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				secret    *corev1.Secret
			}{
				name: "test-secret-00", namespace: "test-src-secret-ns-00", secret: &corev1.Secret{},
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(src.namespace, &corev1.Namespace{})
				return err == nil
			}, timeout, interval).Should(BeTrue())

			data := map[string][]byte{"password": []byte("supersecret")}
			label := &syncLabel{key: testLabelKey, value: src.name}
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				err = tc.GetSecret(src.name, src.namespace, src.secret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(src.secret)
			GinkgoWriter.Println(string(b))

			By("Create target namespace with sync labels")
			targetNamespace, err := tc.CreateNamespace("test-target-secret-ns-00", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(targetNamespace.Name, targetNamespace)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(targetNamespace)
			GinkgoWriter.Println(string(b))

			By("Check target namespace for synced secret")
			copy := &corev1.Secret{}
			Eventually(func() bool {
				err := tc.GetSecret(src.name, targetNamespace.Name, copy)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(copy)
			GinkgoWriter.Println(string(b))

			By("Update source secret data")
			Expect(tc.GetSecret(src.name, src.namespace, src.secret)).ShouldNot(HaveOccurred())
			src.secret.Data = map[string][]byte{"password": []byte("newsupersecret")}
			Expect(tc.UpdateSecret(src.secret)).ShouldNot(HaveOccurred())
			b, _ = yaml.Marshal(src.secret)
			GinkgoWriter.Println(string(b))

			By("Verify secret copy in target namespace was updated")
			Eventually(func() bool {
				tc.GetSecret(src.name, targetNamespace.Name, copy)
				return reflect.DeepEqual(copy.Data, src.secret.Data)
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(copy)
			GinkgoWriter.Println(string(b))

		})
	})
	Context("Namespace doesn't doesn't contain sync label", func() {
		It("Should not sync source secret", func() {
			By("Create source namespace and secret")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				secret    *corev1.Secret
			}{
				name: "test-secret-01", namespace: "test-src-secret-ns-01", secret: &corev1.Secret{},
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(src.namespace, &corev1.Namespace{})
				return err == nil
			}, timeout, interval).Should(BeTrue())

			data := map[string][]byte{"password": []byte(src.name)}
			label := &syncLabel{key: testLabelKey, value: src.name}
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				err = tc.GetSecret(src.name, src.namespace, src.secret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(src.secret)
			GinkgoWriter.Println(string(b))

			By("Creating Namespace without sync labels")
			targetNamespace, err := tc.CreateNamespace("test-target-secret-ns-01", nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(targetNamespace.Name, targetNamespace)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(targetNamespace)
			GinkgoWriter.Println(string(b))

			By("Verifying Namespace doesn't contain secret")
			Consistently(func() bool {
				err := tc.GetSecret(src.secret.Name, targetNamespace.Name, &corev1.Secret{})
				return apierrors.IsNotFound(err)
			}, time.Second*2, interval).Should(BeTrue())
		})
	})
	Context("When source secret name is 253 characters", func() {
		It("Should successfully sync secret", func() {
			By("Create source namespace")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				secret    *corev1.Secret
			}{
				namespace: "test-src-secret-ns-02", secret: &corev1.Secret{},
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			By("Creating new source secret with 253 characters")
			src.name = rand.String(253)
			label := &syncLabel{key: src.namespace, value: "testSecretLongNames"}
			data := map[string][]byte{"password": []byte("anothersupersecret")}
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetSecret(src.name, src.namespace, src.secret), timeout, interval).Should(Succeed())
			b, _ := yaml.Marshal(src.secret)
			GinkgoWriter.Println(string(b))

			By("Creating new target namespace")
			targetNamespace, err := tc.CreateNamespace("test-target-secret-ns-02", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(targetNamespace.Name, targetNamespace), timeout, interval).Should(Succeed())

			By("Checking for target namespace for copy of secret")
			targetSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := tc.GetSecret(src.name, targetNamespace.Name, targetSecret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(targetSecret)
			GinkgoWriter.Println(string(b))
		})
	})
	Context("When source secret is deleted", func() {
		It("Should remove the finalizer from the copies", func() {
			By("Creating Source Namespace and secret")
			src := struct {
				name      string
				namespace string
				secret    *corev1.Secret
			}{
				name: "test-secret-03", namespace: "test-src-secret-ns-03", secret: &corev1.Secret{},
			}
			tc = NewTestClient(context.Background())
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ToNot(HaveOccurred())
			Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			label := &syncLabel{key: testLabelKey, value: src.name}
			data := map[string][]byte{"password": []byte("deleteme")}
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data)
			Expect(err).ToNot(HaveOccurred())
			Eventually(tc.GetSecret(src.name, src.namespace, src.secret), timeout, interval).Should(Succeed())

			By("Creating target namespaces and checking for copies")
			testCases := []struct {
				name      string
				namespace *corev1.Namespace
				secret    *corev1.Secret
			}{
				{name: "target-tt-secret-01", namespace: &corev1.Namespace{}, secret: &corev1.Secret{}},
				{name: "target-tt-secret-02", namespace: &corev1.Namespace{}, secret: &corev1.Secret{}},
				{name: "target-tt-secret-03", namespace: &corev1.Namespace{}, secret: &corev1.Secret{}},
			}
			for _, t := range testCases {
				t.namespace, err = tc.CreateNamespace(t.name, label)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(tc.GetNamespace(t.name, t.namespace), timeout, interval).Should(Succeed())
				Eventually(func() bool {
					tc.GetSecret(src.name, t.name, t.secret)
					return reflect.DeepEqual(src.secret.Data, t.secret.Data)
				}, timeout, interval).Should(BeTrue())
			}

			By("Deleting source secret")
			err = tc.DeleteSecret(src.secret)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetSecret(src.name, src.namespace, src.secret)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Verifying finalizer has been removed from copies")
			for _, t := range testCases {
				Eventually(func() bool {
					tc.GetSecret(src.name, t.name, t.secret)
					return !slices.Contains(t.secret.Finalizers, syncFinalizer)
				}, timeout, interval).Should(BeTrue())
			}

		})
	})
	Context("When copy secret is deleted", func() {
		It("Should resync the copy to the appropriate namespace", func() {
			By("Creating Source Namespace and secret")
			src := struct {
				name      string
				namespace string
				secret    *corev1.Secret
			}{
				name: "test-src-secret-04", namespace: "test-src-secret-ns-04", secret: &corev1.Secret{},
			}
			tc = NewTestClient(context.Background())
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			label := &syncLabel{key: "kopy-sync", value: src.name}
			data := map[string][]byte{"password": []byte("deleteme")}
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetSecret(src.name, src.namespace, src.secret)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Creating target namespace")
			targetNamespace, err := tc.CreateNamespace("test-target-secret-ns-04", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			By("Verifying copy secret synced")
			targetSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := tc.GetSecret(src.name, targetNamespace.Name, targetSecret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(targetSecret)
			GinkgoWriter.Println(string(b))

			By("Deleting secret copy from target namespace")
			Expect(tc.DeleteSecret(targetSecret)).ShouldNot(HaveOccurred())

			By("Verifying that secret was recreated in the target namespace")
			Eventually(func() bool {
				newSecret := &corev1.Secret{}
				tc.GetSecret(src.name, targetNamespace.Name, newSecret)
				return targetSecret.UID != newSecret.UID
			}, timeout, interval).Should(BeTrue())

		})
	})
	if useKind {
		Context("When namespace that contains copy is deleted", func() {
			It("The namespace should be deleted properly", func() {
				By("Creating new source namespace and secret")
				tc = NewTestClient(context.Background())
				src := struct {
					name      string
					namespace string
					secret    *corev1.Secret
				}{
					name: "test-src-secret-05", namespace: "test-src-secret-ns-05", secret: &corev1.Secret{},
				}
				_, err := tc.CreateNamespace(src.namespace, nil)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())
				label := &syncLabel{key: testLabelKey, value: src.name}
				data := map[string][]byte{"password": []byte(src.name)}

				src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(func() bool {
					err := tc.GetSecret(src.name, src.namespace, src.secret)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				By("Creating new target namespace")
				targetNamespace, err := tc.CreateNamespace("test-target-secret-ns-05", label)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(func() bool {
					err := tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{})
					return err == nil
				}, timeout, interval).Should(BeTrue())

				By("Checking target namespace for copy")
				copy := &corev1.Secret{}
				Eventually(func() bool {
					err := tc.GetSecret(src.name, targetNamespace.Name, copy)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				b, _ := yaml.Marshal(copy)
				GinkgoWriter.Println(string(b))

				By("Deleting target namespace")
				Expect(tc.DeleteNamespace(targetNamespace)).ShouldNot(HaveOccurred())

				By("Verify finalizer was removed from copy")
				Eventually(func() bool {
					copy = &corev1.Secret{}
					tc.GetSecret(src.name, targetNamespace.Name, copy)
					return !slices.Contains(copy.Finalizers, syncFinalizer)
				}, timeout, interval).Should(BeTrue())

				By("Verifying namespace was deleted")
				Eventually(func() bool {
					err := tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{})
					return apierrors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())

			})
		})
		Context("When namespace with source secret is deleted", func() {
			It("Should remove the finalizer from the copies and delete namespace", func() {
				By("Creating Source Namespace and secret")
				src := struct {
					name      string
					namespace string
					secret    *corev1.Secret
				}{
					name: "test-secret-06", namespace: "test-src-secret-ns-06", secret: &corev1.Secret{},
				}
				tc = NewTestClient(context.Background())
				_, err := tc.CreateNamespace(src.namespace, nil)
				Expect(err).ToNot(HaveOccurred())
				Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())

				label := &syncLabel{key: testLabelKey, value: src.name}
				data := map[string][]byte{"password": []byte("deleteme")}
				src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data)
				Expect(err).ToNot(HaveOccurred())
				Eventually(tc.GetSecret(src.name, src.namespace, src.secret), timeout, interval).Should(Succeed())
				b, _ := yaml.Marshal(src.secret)
				GinkgoWriter.Println(string(b))

				By("Creating target namespace and check for copy")
				targetNamespace, err := tc.CreateNamespace("test-target-secret-ns-06", label)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{}), timeout, interval).Should(Succeed())
				copy := &corev1.Secret{}
				Eventually(func() bool {
					tc.GetSecret(src.name, targetNamespace.Name, copy)
					return reflect.DeepEqual(src.secret.Data, copy.Data)
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
					copy = &corev1.Secret{}
					tc.GetSecret(src.name, targetNamespace.Name, copy)
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
