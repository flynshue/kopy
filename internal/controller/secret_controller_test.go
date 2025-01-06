package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/yaml"
)

var (
	testSrcSecretNS *corev1.Namespace
	testSrcSecret   *corev1.Secret
	tc              testClient
)

var _ = Describe("Secret Controller\n", func() {
	Context("Namespace contains sync label", func() {
		It("Should sync source secret to target namespace", func() {
			By("Create source namespace")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				secret    *corev1.Secret
			}{
				name: "test-src-secret-00", namespace: "test-src-secret-ns-00",
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(src.namespace, &corev1.Namespace{})
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Create source secret")
			data := map[string][]byte{"key1": []byte("supersecret")}
			src.secret, err = tc.CreateSecret(src.name, src.namespace, &syncLabel{key: testLabelKey, value: testLabelValue}, data)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				err = tc.GetSecret(src.name, src.namespace, src.secret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(src.secret)
			GinkgoWriter.Println(string(b))

			By("Create target namespace with sync labels")
			targetNamespace, err := tc.CreateNamespace("test-secret-target-00", &syncLabel{key: testLabelKey, value: testLabelValue})
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(targetNamespace.Name, targetNamespace)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(targetNamespace)
			GinkgoWriter.Println(string(b))

			By("Checking target namespace for synced secret")
			copy := &corev1.Secret{}
			Eventually(func() bool {
				err := tc.GetSecret(src.name, targetNamespace.Name, copy)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(copy)
			GinkgoWriter.Println(string(b))

		})
	})
	Context("Namespace doesn't doesn't contain sync label", func() {
		It("Should not sync source secret", func() {
			By("Create source namespace")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				secret    *corev1.Secret
			}{
				name: "test-src-secret-01", namespace: "test-src-secret-ns-01",
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(src.namespace, &corev1.Namespace{})
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Create source secret")
			data := map[string][]byte{"key1": []byte("supersecret")}
			src.secret, err = tc.CreateSecret(src.name, src.namespace, &syncLabel{key: testLabelKey, value: testLabelValue}, data)
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				err = tc.GetSecret(src.name, src.namespace, src.secret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(src.secret)
			GinkgoWriter.Println(string(b))

			By("Creating Namespace without sync labels")
			targetNamespace, err := tc.CreateNamespace("test-secret-target-01", nil)
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
				namespace: "test-src-secret-ns-02",
			}
			srcNamespace, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(src.namespace, srcNamespace), timeout, interval).Should(Succeed())

			By("Creating new source secret with 253 characters")
			src.name = rand.String(253)
			label := &syncLabel{key: src.namespace, value: "testSecretLongNames"}
			data := map[string][]byte{"foo.bar": []byte("anothersupersecret")}
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetSecret(src.name, src.namespace, src.secret), timeout, interval).Should(Succeed())
			b, _ := yaml.Marshal(src.secret)
			GinkgoWriter.Println(string(b))

			By("Creating new target namespace")
			targetNamespace, err := tc.CreateNamespace("test-target-secret-ns-02", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(targetNamespace.Name, targetNamespace), timeout, interval).Should(Succeed())
			b, _ = yaml.Marshal(targetNamespace)
			GinkgoWriter.Println(string(b))

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
})
