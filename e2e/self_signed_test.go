// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Steadybit GmbH

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steadybit/action-kit/go/action_kit_test/e2e"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/extension-kit/extlogging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	tlsKafkaRelease       = "my-kafka-tls"
	tlsKafkaSecret        = "kafka-tls-certs"
	selfSignedCaConfigMap = "kafka-self-signed-ca"
	tlsSeedBroker         = tlsKafkaRelease + ".default.svc.cluster.local:9092"
	untrustedCertificate  = "certificate signed by unknown authority"
)

// tlsClusterArgs configures the extension for the TLS Kafka, without trusting its certificate.
var tlsClusterArgs = []string{
	"--set", "logging.level=debug",
	"--set", "kafka.seedBrokers=" + tlsSeedBroker,
	"--set", "kafka.auth.saslMechanism=PLAIN",
	"--set", "kafka.auth.saslUser=user1",
	"--set", "kafka.auth.saslPassword=steadybit", //NOSONAR go:S2068
	"--set-string", "kafka.auth.useTLS=true",
}

// TestWithMinikubeSelfSignedCertificate runs the extension against a Kafka whose client listener uses a
// certificate signed by a self-signed CA. The extension must accept it once the operator either skips
// verification or mounts the CA, and refuse it otherwise.
//
// A rejected certificate makes the extension exit at startup (it cannot reach any cluster), so the harness
// cannot start it in that state. The rejection is therefore checked last in the skip-verification cluster,
// by redeploying without skip and reading the failing pod's logs.
func TestWithMinikubeSelfSignedCertificate(t *testing.T) {
	extlogging.InitZeroLog()
	certs := generateKafkaCertificates(t)

	scenarios := []struct {
		name  string
		args  []string
		tests []e2e.WithMinikubeTestCase
	}{
		{
			name: "insecureSkipVerify",
			args: []string{"--set", "kafka.insecureSkipVerify=true"},
			tests: []e2e.WithMinikubeTestCase{
				{Name: "accepts self-signed certificate with insecureSkipVerify", Test: testTLSBrokerDiscovered},
				{Name: "rejects self-signed certificate by default", Test: testRejectsUntrustedCertificate},
			},
		},
		{
			name: "mounted CA",
			args: []string{
				"--set", "extraEnv[0].name=SSL_CERT_DIR",
				"--set", "extraEnv[0].value=/etc/ssl/extra-certs:/etc/ssl/certs",
				"--set", "extraVolumeMounts[0].name=extra-certs",
				"--set", "extraVolumeMounts[0].mountPath=/etc/ssl/extra-certs",
				"--set", "extraVolumeMounts[0].readOnly=true",
				"--set", "extraVolumes[0].name=extra-certs",
				"--set", "extraVolumes[0].configMap.name=" + selfSignedCaConfigMap,
			},
			tests: []e2e.WithMinikubeTestCase{
				{Name: "accepts self-signed certificate with mounted CA", Test: testTLSBrokerDiscovered},
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			extFactory := e2e.HelmExtensionFactory{
				Name: "extension-kafka",
				Port: 8083,
				ExtraArgs: func(m *e2e.Minikube) []string {
					return append(append([]string{}, tlsClusterArgs...), scenario.args...)
				},
			}
			e2e.WithMinikube(t, e2e.DefaultMinikubeOpts().AfterStart(certs.installTLSKafka), &extFactory, scenario.tests)
		})
	}
}

func testTLSBrokerDiscovered(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	target, err := e2e.PollForTarget(ctx, e, "com.steadybit.extension_kafka.broker", func(target discovery_kit_api.Target) bool {
		hosts := target.Attributes["kafka.broker.host"]
		return len(hosts) > 0 && strings.Contains(hosts[0], tlsKafkaRelease+"-controller")
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"9092"}, target.Attributes["kafka.broker.port"])
}

// testRejectsUntrustedCertificate redeploys the extension without insecureSkipVerify and asserts that it
// fails because it does not trust the broker's certificate.
func testRejectsUntrustedCertificate(t *testing.T, m *e2e.Minikube, _ *e2e.Extension) {
	args := append([]string{
		"upgrade", "--install",
		"--kube-context", m.Profile,
		"--namespace=default",
		"--set", "image.name=docker.io/library/extension-kafka",
		"--set", "image.tag=latest",
		"--set", "image.pullPolicy=Never",
	}, tlsClusterArgs...)
	args = append(args, "extension-kafka", "../charts/steadybit-extension-kafka")
	out, err := exec.Command("helm", args...).CombinedOutput() //NOSONAR go:S4036
	require.NoError(t, err, string(out))

	assert.Eventually(t, func() bool {
		return extensionLogsContain(t, m, untrustedCertificate)
	}, 3*time.Minute, 5*time.Second, "the extension must fail with %q", untrustedCertificate)
}

// extensionLogsContain checks the current and previous container logs of every extension pod, as the pod
// that fails to connect restarts.
func extensionLogsContain(t *testing.T, m *e2e.Minikube, text string) bool {
	pods, err := m.ListPods(t.Context(), "default", "app.kubernetes.io/name=steadybit-extension-kafka")
	if err != nil {
		return false
	}
	for _, pod := range pods {
		for _, previous := range []bool{false, true} {
			stream, err := m.GetClient().CoreV1().Pods("default").GetLogs(pod.Name, &corev1.PodLogOptions{Previous: previous}).Stream(t.Context())
			if err != nil {
				continue
			}
			logs, _ := io.ReadAll(stream)
			_ = stream.Close()
			if strings.Contains(string(logs), text) {
				return true
			}
		}
	}
	return false
}

type kafkaCertificates struct {
	caFile  string
	caPEM   []byte
	certPEM []byte
	keyPEM  []byte
}

// generateKafkaCertificates creates a self-signed CA and a broker certificate it signs, valid for the
// broker service and the per-broker addresses the controllers advertise.
func generateKafkaCertificates(t *testing.T) kafkaCertificates {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "steadybit-e2e-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	headless := tlsKafkaRelease + "-controller-headless"
	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: tlsKafkaRelease},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{
			tlsKafkaRelease, tlsKafkaRelease + ".default", tlsKafkaRelease + ".default.svc", tlsKafkaRelease + ".default.svc.cluster.local",
			"*." + headless, "*." + headless + ".default", "*." + headless + ".default.svc", "*." + headless + ".default.svc.cluster.local",
			"localhost",
		},
	}
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	der, err := x509.CreateCertificate(rand.Reader, &template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	certs := kafkaCertificates{
		caFile:  filepath.Join(t.TempDir(), "self-signed-ca.crt"),
		caPEM:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	}
	require.NoError(t, os.WriteFile(certs.caFile, certs.caPEM, 0o600))
	return certs
}

// installTLSKafka deploys a single-broker Kafka whose client listener uses SASL_SSL with the generated
// certificate, plus the CA as a ConfigMap for the extension to mount.
func (c kafkaCertificates) installTLSKafka(m *e2e.Minikube) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: tlsKafkaSecret, Namespace: "default"},
		Data:       map[string][]byte{"ca.crt": c.caPEM, "tls.crt": c.certPEM, "tls.key": c.keyPEM},
	}
	if _, err := m.GetClient().CoreV1().Secrets("default").Create(context.Background(), secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create kafka TLS secret: %w", err)
	}
	if err := m.CreateConfigMap("default", selfSignedCaConfigMap, c.caFile); err != nil {
		return fmt.Errorf("failed to create CA config map: %w", err)
	}

	out, err := exec.Command("helm", "repo", "add", "bitnami", "https://charts.bitnami.com/bitnami").CombinedOutput() //NOSONAR go:S4036
	if err != nil {
		return fmt.Errorf("failed to add bitnami helm repo: %s: %s", err, out)
	}
	out, err = exec.Command("helm", //NOSONAR go:S4036
		"upgrade", "--install",
		"--kube-context", m.Profile,
		"--set", "sasl.client.passwords=steadybit",
		"--set", "image.repository=bitnamilegacy/kafka",
		"--set", "image.tag=4.0.0-debian-12-r10",
		"--set", "global.security.allowInsecureImages=true",
		"--set", "controller.replicaCount=1",
		"--set", "controller.resources.limits.memory=1Gi",
		"--set", "listeners.client.protocol=SASL_SSL",
		"--set", "listeners.client.sslClientAuth=none",
		"--set", "tls.type=PEM",
		"--set", "tls.autoGenerated.enabled=false",
		"--set", "tls.existingSecret="+tlsKafkaSecret,
		"--namespace=default",
		"--timeout=15m0s",
		tlsKafkaRelease, "bitnami/kafka", "--wait").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install TLS kafka helm chart: %s: %s", err, out)
	}
	return nil
}
