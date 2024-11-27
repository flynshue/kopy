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
			targetNamespace, err := tc.CreateNamespace("test-target-00", &syncLabel{key: testLabelKey, value: testLabelValue})
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetNamespace(targetNamespace.Name, targetNamespace)
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
			data := map[string]string{"HOST": "https://test-kopy.io/foobar"}
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

	Context("Table tests; source configMap is deleted", func() {
		It("configMap copies should have finalizers removed", func() {
			By("Creating source configMap to be deleted")
			src := struct {
				namespace string
				name      string
				label     *syncLabel
				cmObj     *corev1.ConfigMap
			}{namespace: testSrcNamespace.Name, name: "deleteme-config-01",
				label: &syncLabel{key: "kopy-sync", value: "deleteme"},
			}
			data := map[string]string{"DELETEME": "true"}
			c := NewTestClient(context.Background())
			var err error
			src.cmObj, err = c.CreateConfigMap(src.name, src.namespace, src.label, data)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(c.GetConfigMap(src.name, src.namespace, src.cmObj), timeout, interval).Should(Succeed())
			b, _ := yaml.Marshal(src.cmObj)

			By("Creating target namespaces for table tests")
			GinkgoWriter.Println(string(b))
			testCases := []struct {
				namespaceName string
				nsObj         *corev1.Namespace
				cmObj         *corev1.ConfigMap
			}{
				{namespaceName: "tt-target-01", nsObj: &corev1.Namespace{}, cmObj: &corev1.ConfigMap{}},
				{namespaceName: "tt-target-02", nsObj: &corev1.Namespace{}, cmObj: &corev1.ConfigMap{}},
			}
			for i, tc := range testCases {
				var err error
				tc.nsObj, err = c.CreateNamespace(tc.namespaceName, src.label)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() bool {
					err := c.GetNamespace(tc.namespaceName, tc.nsObj)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				testCases[i] = tc
			}
			By("Verifying target namespaces in table tests")
			for _, tc := range testCases {
				b, _ := yaml.Marshal(tc.nsObj)
				GinkgoWriter.Println(string(b))
			}

			By("Checking target namespace for configMap copy")
			for i, tc := range testCases {
				Eventually(func() bool {
					err := c.GetConfigMap(src.name, tc.namespaceName, tc.cmObj)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				testCases[i] = tc
			}
			for _, tc := range testCases {
				b, _ := yaml.Marshal(tc.cmObj)
				GinkgoWriter.Println(string(b))
			}

			By("Deleting src configMap")
			Expect(c.DeleteConfigmap(src.cmObj)).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := c.GetConfigMap(src.name, src.namespace, &corev1.ConfigMap{})
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Checking copy configMap for finalizers")
			for _, tc := range testCases {
				Eventually(func() bool {
					err := c.GetConfigMap(src.name, tc.namespaceName, tc.cmObj)
					if err != nil {
						return false
					}
					return !slices.Contains(tc.cmObj.Finalizers, syncFinalizer)
				}, timeout, interval).Should(BeTrue())
				b, _ := yaml.Marshal(tc.cmObj)
				GinkgoWriter.Println(string(b))
			}
		})
	})
	if useKind {
		Context("When namespace that contains copy is deleted", func() {
			It("The namespace should be deleted properly", func() {
				By("Creating new namespace for sync")
				c := NewTestClient(context.Background())
				targetNS, err := c.CreateNamespace("test-cm-target-04", nil)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(func() bool {
					err := c.GetNamespace("test-cm-target-04", targetNS)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				b, _ := yaml.Marshal(targetNS)
				GinkgoWriter.Println(string(b))

				By("Creating test configmap")
				targetCm, err := c.CreateConfigMap("test-target-04-config", targetNS.Name, nil, map[string]string{"host": "https://fakehost.us"})
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(func() bool {
					err := c.GetConfigMap(targetCm.Name, targetNS.Name, targetCm)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				b, _ = yaml.Marshal(targetCm)
				GinkgoWriter.Println(string(b))

				By("Deleting target namespace")
				err = c.DeleteNamespace(targetNS)
				Expect(err).ShouldNot(HaveOccurred())
				time.Sleep(time.Second * 2)
				c.GetNamespace(targetNS.Name, targetNS)
				b, _ = yaml.Marshal(targetNS)
				GinkgoWriter.Println(string(b))

				time.Sleep(time.Second * 5)
				c.GetConfigMap(targetCm.Name, targetNS.Name, targetCm)
				b, _ = yaml.Marshal(targetCm)
				GinkgoWriter.Println(string(b))

			})
		})
	}

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
	data := map[string]string{"HOST": "https://test-kopy.io/"}
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
