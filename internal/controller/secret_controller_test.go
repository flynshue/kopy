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

var (
	testSrcSecretNS *corev1.Namespace
	testSrcSecret   *corev1.Secret
)

var _ = Describe("Secret Controller", Ordered, func() {
	BeforeAll(setUpSecretSourceEnv)
	AfterAll(cleanUpSecretSourceEnv)
	Context("When Namespace contains sync label", func() {
		It("should sync secret to namespace", func() {
			By("Creating target namespace that with the sync labels")
			tc := NewTestClient(context.Background())
			targetNamespace, err := tc.CreateNamespace("test-secret-target-00", &syncLabel{key: testLabelKey, value: testLabelValue})
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(targetNamespace.Name, targetNamespace)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(targetNamespace)
			GinkgoWriter.Println(string(b))

			By("Checking to see if the secret was synced to target namespace")
			secret := &corev1.Secret{}
			srcNamespace := testSrcSecret.Namespace
			srcName := testSrcSecret.Name
			Eventually(func() bool {
				err := tc.GetSecret(srcName, targetNamespace.Name, secret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(secret)
			GinkgoWriter.Println(string(b))

			By("Checking secret for source label namespace")
			v, ok := secret.Labels[sourceLabelNamespace]
			Expect(ok).Should(BeTrue())
			GinkgoWriter.Printf("secret label: %v\n", secret.Labels)
			GinkgoWriter.Printf("origin ns: %v\n", testSrcSecretNS)
			Expect(v).Should(Equal(testSrcSecretNS.Name))

			By("Checking secret for finalizer")
			Expect(secret.Finalizers).Should(ContainElement(syncFinalizer))

			By("Updating source secret")
			err = tc.GetSecret(srcName, srcNamespace, testSrcSecret)
			Expect(err).ShouldNot(HaveOccurred())
			data := map[string][]byte{"key1": []byte("newsupersecret")}
			testSrcSecret.Data = data
			err = tc.UpdateSecret(testSrcSecret)
			Expect(err).ShouldNot(HaveOccurred())
			b, _ = yaml.Marshal(testSrcSecret)
			GinkgoWriter.Println(string(b))
			Eventually(func() bool {
				tc.GetSecret(srcName, srcNamespace, testSrcSecret)
				return Expect(testSrcSecret.Data).Should(Equal(data))
			}, timeout, interval).Should(BeTrue())

			By("Checking copy secret was updated")
			Eventually(func() bool {
				tc.GetSecret(testSrcSecret.Name, testSrcSecretNS.Name, secret)
				return reflect.DeepEqual(secret.Data, data)
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(secret)
			GinkgoWriter.Println(string(b))
		})
	})

	Context("When namespace doesn't have sync label", func() {
		It("Copy secret is not found", func() {
			By("Creating namespace without sync labels")
			tc := NewTestClient(context.Background())
			ns, err := tc.CreateNamespace("test-secret-ns-nolabels", nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(ns.Name, ns)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(ns)
			GinkgoWriter.Println(string(b))

			By("Looking up source secret in namespace")
			Consistently(func() bool {
				err := tc.GetSecret(testSrcSecretNS.Name, ns.Name, &corev1.Secret{})
				return apierrors.IsNotFound(err)
			}, time.Second*2, interval).Should(BeTrue())
		})
	})

	Context("When source secret name is 253 characters", func() {
		It("Should successfully sync secret", func() {
			By("Creating new source secret with 253 characters")
			tc := NewTestClient(context.Background())
			srcName := rand.String(253)
			srcNamespace := testSrcSecretNS.Name
			label := &syncLabel{key: srcNamespace, value: "testSecretLongNames"}
			// data := map[string]string{"FOO": "bar"}
			data := map[string][]byte{"foo.bar": []byte("anothersupersecret")}
			srcSecret, err := tc.CreateSecret(srcName, srcNamespace, label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetSecret(srcName, srcNamespace, srcSecret), timeout, interval).Should(Succeed())

			By("Creating new target namespace")
			ns, err := tc.CreateNamespace("test-secret-target-03", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(ns.Name, ns), timeout, interval).Should(Succeed())

			By("Checking for copy of secret")
			Eventually(func() bool {
				s := &corev1.Secret{}
				err := tc.GetSecret(srcName, ns.Name, s)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Table tests; source secret is deleted", func() {
		It("secret copies should have finalizers removed", func() {
			By("Creating source secret to be deleted")
			src := struct {
				namespace string
				name      string
				label     *syncLabel
				obj       *corev1.Secret
			}{namespace: testSrcSecretNS.Name, name: "deleteme-secret-01",
				label: &syncLabel{key: "kopy-sync", value: "deleteme"},
			}
			// data := map[string]string{"DELETEME": "true"}
			data := map[string][]byte{"password": []byte("deleteme")}
			c := NewTestClient(context.Background())
			var err error
			src.obj, err = c.CreateSecret(src.name, src.namespace, src.label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(c.GetSecret(src.name, src.namespace, src.obj), timeout, interval).Should(Succeed())
			b, _ := yaml.Marshal(src.obj)

			By("Creating target namespaces for table tests")
			GinkgoWriter.Println(string(b))
			testCases := []struct {
				namespaceName string
				ns            *corev1.Namespace
				obj           *corev1.Secret
			}{
				{namespaceName: "tt-secret-target-01", ns: &corev1.Namespace{}, obj: &corev1.Secret{}},
				{namespaceName: "tt-secret-target-02", ns: &corev1.Namespace{}, obj: &corev1.Secret{}},
			}
			for i, tc := range testCases {
				var err error
				tc.ns, err = c.CreateNamespace(tc.namespaceName, src.label)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					err := c.GetNamespace(tc.namespaceName, tc.ns)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				testCases[i] = tc
			}
			By("Verifying target namespaces in table tests")
			for _, tc := range testCases {
				b, _ := yaml.Marshal(tc.ns)
				GinkgoWriter.Println(string(b))
			}

			By("Checking target namespace for secret copy")
			for i, tc := range testCases {
				Eventually(func() bool {
					err := c.GetSecret(src.name, tc.namespaceName, tc.obj)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				testCases[i] = tc
			}
			for _, tc := range testCases {
				b, _ := yaml.Marshal(tc.obj)
				GinkgoWriter.Println(string(b))
			}

			By("Deleting src secret")
			Expect(c.DeleteSecret(src.obj)).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := c.GetSecret(src.name, src.namespace, &corev1.Secret{})
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Checking copy secret for finalizers")
			for _, tc := range testCases {
				Eventually(func() bool {
					err := c.GetSecret(src.name, tc.namespaceName, tc.obj)
					if err != nil {
						return false
					}
					return !slices.Contains(tc.obj.Finalizers, syncFinalizer)
				}, timeout, interval).Should(BeTrue())
				b, _ := yaml.Marshal(tc.obj)
				GinkgoWriter.Println(string(b))
			}
		})
	})
	if useKind {
		Context("When namespace that contains copy is deleted", func() {
			It("The namespace should be deleted properly", func() {
				By("Creating new namespace for sync")
				c := NewTestClient(context.Background())
				targetNS, err := c.CreateNamespace("test-secret-target-04", nil)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(func() bool {
					err := c.GetNamespace("test-secret-target-04", targetNS)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				b, _ := yaml.Marshal(targetNS)
				GinkgoWriter.Println(string(b))

				By("Creating test secret")
				targetSecret, err := c.CreateSecret("test-target-04-secret", targetNS.Name, nil, map[string][]byte{"key": []byte("fakesecret")})
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(func() bool {
					err := c.GetSecret(targetSecret.Name, targetNS.Name, targetSecret)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				b, _ = yaml.Marshal(targetSecret)
				GinkgoWriter.Println(string(b))

				By("Deleting target namespace")
				err = c.DeleteNamespace(targetNS)
				Expect(err).ShouldNot(HaveOccurred())
				time.Sleep(time.Second * 2)
				c.GetNamespace(targetNS.Name, targetNS)
				b, _ = yaml.Marshal(targetNS)
				GinkgoWriter.Println(string(b))

				time.Sleep(time.Second * 5)
				c.GetSecret(targetSecret.Name, targetNS.Name, targetSecret)
				b, _ = yaml.Marshal(targetSecret)
				GinkgoWriter.Println(string(b))

			})
		})
	}

})

func setUpSecretSourceEnv() {
	ctx := context.Background()
	By("Creating test source namespace")
	tc := NewTestClient(ctx)
	ns := "test-secret-src"
	var err error
	testSrcSecretNS, err = tc.CreateNamespace(ns, nil)
	Expect(err).ShouldNot(HaveOccurred())
	Eventually(func() bool {
		err := tc.GetNamespace(ns, &corev1.Namespace{})
		return err == nil
	}, timeout, interval).Should(BeTrue())

	By("Creating test source secret")
	data := map[string][]byte{"key1": []byte("supersecret")}
	testSrcSecret, err = tc.CreateSecret("test-secret", ns, &syncLabel{key: testLabelKey, value: testLabelValue}, data)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		err := tc.GetSecret("test-secret", ns, testSrcSecret)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	b, _ := yaml.Marshal(testSrcSecret)
	GinkgoWriter.Println(string(b))
}

func cleanUpSecretSourceEnv() {
	By("Cleaning up test source namespace")
	tc := NewTestClient(context.Background())
	err := tc.GetNamespace(testSrcSecretNS.Name, testSrcSecretNS)
	Expect(err).ShouldNot(HaveOccurred())
	Eventually(func() bool {
		err := k8sClient.Delete(ctx, testSrcSecretNS)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	testSrcSecretNS = &corev1.Namespace{}
	testSrcSecret = &corev1.Secret{}
}
