/*
Copyright 2025.

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
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math"
	"math/big"
	"net"
	"reflect"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/yaml"

	cryptorand "crypto/rand"
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
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data, corev1.SecretTypeOpaque)
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
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data, corev1.SecretTypeOpaque)
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
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data, corev1.SecretTypeOpaque)
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
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data, corev1.SecretTypeOpaque)
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
			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data, corev1.SecretTypeOpaque)
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
	Context("When secret is type dockerconfigjson", func() {
		It("Should sync the secret to the target namespace", func() {
			By("Creating new source namespace and secret")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				secret    *corev1.Secret
			}{
				name: "test-src-secret-07", namespace: "test-src-secret-ns-07", secret: &corev1.Secret{},
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())
			label := &syncLabel{key: testLabelKey, value: src.name}
			configJson := `{"auths":{"https://registry.kopy.io":{"username":"kopy","password":"kopysecret"}}}`
			data := map[string][]byte{corev1.DockerConfigJsonKey: []byte(configJson)}

			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data, corev1.SecretTypeDockerConfigJson)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetSecret(src.name, src.namespace, src.secret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(src.secret)
			GinkgoWriter.Println(string(b))

			By("Creating target namespace")
			targetNamespace, err := tc.CreateNamespace("test-target-secret-ns-07", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			By("Verifying copy secret synced")
			targetSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := tc.GetSecret(src.name, targetNamespace.Name, targetSecret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(targetSecret)
			GinkgoWriter.Println(string(b))
		})
	})
	Context("When secret is type tls", func() {
		It("Should sync the secret to the target namespace", func() {
			By("Creating new source namespace and secret")
			tc = NewTestClient(context.Background())
			src := struct {
				name      string
				namespace string
				secret    *corev1.Secret
			}{
				name: "test-src-secret-08", namespace: "test-src-secret-ns-08", secret: &corev1.Secret{},
			}
			_, err := tc.CreateNamespace(src.namespace, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(src.namespace, &corev1.Namespace{}), timeout, interval).Should(Succeed())
			label := &syncLabel{key: testLabelKey, value: src.name}
			certs, key, err := generateSelfSignedCert("k8s.kopy.io")
			Expect(err).ShouldNot(HaveOccurred())
			data := map[string][]byte{
				corev1.TLSCertKey:       certs,
				corev1.TLSPrivateKeyKey: key,
			}

			src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data, corev1.SecretTypeTLS)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				err := tc.GetSecret(src.name, src.namespace, src.secret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ := yaml.Marshal(src.secret)
			GinkgoWriter.Println(string(b))
			originalCert, err := decodePemCert(src.secret.Data[corev1.TLSCertKey])
			Expect(err).ShouldNot(HaveOccurred())

			By("Creating target namespace")
			targetNamespace, err := tc.CreateNamespace("test-target-secret-ns-08", label)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(tc.GetNamespace(targetNamespace.Name, &corev1.Namespace{}), timeout, interval).Should(Succeed())

			By("Verifying copy secret synced")
			targetSecret := &corev1.Secret{}
			Eventually(func() bool {
				err := tc.GetSecret(src.name, targetNamespace.Name, targetSecret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			b, _ = yaml.Marshal(targetSecret)
			GinkgoWriter.Println(string(b))

			By("Update source secret tls certs")
			Expect(tc.GetSecret(src.name, src.namespace, src.secret)).ShouldNot(HaveOccurred())
			certs, key, err = generateSelfSignedCert("new.k8s.kopy.io")
			Expect(err).ShouldNot(HaveOccurred())
			updatedSecret := src.secret
			data = map[string][]byte{
				corev1.TLSCertKey:       certs,
				corev1.TLSPrivateKeyKey: key,
			}
			updatedSecret.Data = data
			Expect(tc.UpdateSecret(updatedSecret)).ShouldNot(HaveOccurred())
			Expect(tc.GetSecret(src.name, src.namespace, updatedSecret)).ShouldNot(HaveOccurred())
			newCert, err := decodePemCert(updatedSecret.Data[corev1.TLSCertKey])
			Expect(err).ShouldNot(HaveOccurred())
			Expect(newCert.Equal(originalCert)).Should(BeFalse())

			By("Verify target namespace secret was updated")
			Eventually(func() bool {
				copy := &corev1.Secret{}
				tc.GetSecret(src.name, targetNamespace.Name, copy)
				copyCert, _ := decodePemCert(copy.Data[corev1.TLSCertKey])
				return copyCert.Equal(newCert)
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

				src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data, corev1.SecretTypeOpaque)
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
				src.secret, err = tc.CreateSecret(src.name, src.namespace, label, data, corev1.SecretTypeOpaque)
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

// generateSelfSignedCert helper function used to help generate new selfsigned certs for the test cases
func generateSelfSignedCert(host string) (certificate, key []byte, err error) {
	validFrom := time.Now().Add(-time.Hour) // valid an hour earlier to avoid flakes due to clock skew
	maxAge := time.Hour * 24 * 365          // one year self-signed certs
	caKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	// returns a uniform random value in [0, max-1), then add 1 to serial to make it a uniform random value in [1, max).
	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64-1))
	if err != nil {
		return nil, nil, err
	}
	serial = new(big.Int).Add(serial, big.NewInt(1))
	caTemplate := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("%s-ca@%d", host, time.Now().Unix()),
		},
		NotBefore: validFrom,
		NotAfter:  validFrom.Add(maxAge),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	caCertificate, err := x509.ParseCertificate(caDERBytes)
	if err != nil {
		return nil, nil, err
	}
	ip := net.ParseIP("127.0.0.1")
	priv, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	// returns a uniform random value in [0, max-1), then add 1 to serial to make it a uniform random value in [1, max).
	serial, err = cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64-1))
	if err != nil {
		return nil, nil, err
	}
	serial = new(big.Int).Add(serial, big.NewInt(1))
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("%s@%d", host, time.Now().Unix()),
		},
		NotBefore: validFrom,
		NotAfter:  validFrom.Add(maxAge),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	template.IPAddresses = append(template.IPAddresses, ip)
	derBytes, err := x509.CreateCertificate(cryptorand.Reader, &template, caCertificate, &priv.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	certBuffer := bytes.Buffer{}
	if err := pem.Encode(&certBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, nil, err
	}
	keyBuffer := bytes.Buffer{}
	if err := pem.Encode(&keyBuffer, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return nil, nil, err
	}
	return certBuffer.Bytes(), keyBuffer.Bytes(), nil
}

// decodePemCert will return the first certificate from the pem block.  This is only used for the test cases.
func decodePemCert(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("data does not contain valid certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}
