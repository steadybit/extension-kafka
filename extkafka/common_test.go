// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Steadybit GmbH

package extkafka

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/steadybit/extension-kafka/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

// cm_newCluster starts an in-memory Kafka cluster and returns its seed brokers.
func cm_newCluster(t *testing.T, opts ...kfake.Opt) []string {
	t.Helper()
	c, err := kfake.NewCluster(opts...)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	return c.ListenAddrs()
}

// cm_ping asks the cluster for its broker metadata, which needs a working connection and authentication.
func cm_ping(t *testing.T, client *kgo.Client) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := kadm.NewClient(client).BrokerMetadata(ctx)
	return err
}

// cm_keepLegacyConfig restores the legacy (unindexed) configuration and the cluster map after the test.
func cm_keepLegacyConfig(t *testing.T) {
	t.Helper()
	saved := config.Config
	t.Cleanup(func() {
		config.Config = saved
		config.SetClustersForTest(saved.Clusters)
	})
}

// cm_tlsFixture creates a CA and a broker certificate for 127.0.0.1 signed by it. It returns the broker's
// TLS config and the path of the CA certificate file.
func cm_tlsFixture(t *testing.T) (*tls.Config, string) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "extension-kafka test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)

	caFile := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600))
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, caFile
}

func TestCreateNewClientWithConfig_authenticates_with_each_sasl_mechanism(t *testing.T) {
	for _, mechanism := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		t.Run(mechanism, func(t *testing.T) {
			brokers := cm_newCluster(t, kfake.NumBrokers(1), kfake.EnableSASL(), kfake.Superuser(mechanism, "steadybit", "secret"))

			client, err := createNewClientWithConfig(brokers, &config.ClusterConfig{SaslMechanism: mechanism, SaslUser: "steadybit", SaslPassword: "secret"})
			require.NoError(t, err)
			defer client.Close()
			assert.NoError(t, cm_ping(t, client))

			wrong, err := createNewClientWithConfig(brokers, &config.ClusterConfig{SaslMechanism: mechanism, SaslUser: "steadybit", SaslPassword: "wrong"})
			require.NoError(t, err)
			defer wrong.Close()
			assert.Error(t, cm_ping(t, wrong), "a wrong password must be rejected")
		})
	}
}

func TestCreateNewClient_uses_legacy_sasl_configuration(t *testing.T) {
	for _, mechanism := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		t.Run(mechanism, func(t *testing.T) {
			cm_keepLegacyConfig(t)
			brokers := cm_newCluster(t, kfake.NumBrokers(1), kfake.EnableSASL(), kfake.Superuser(mechanism, "steadybit", "secret"))
			config.Config.SaslMechanism = mechanism
			config.Config.SaslUser = "steadybit"
			config.Config.SaslPassword = "secret"

			admin, err := createNewAdminClient(brokers)
			require.NoError(t, err)
			defer admin.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err = admin.BrokerMetadata(ctx)
			assert.NoError(t, err)
		})
	}
}

func TestCreateNewClientWithConfig_trusts_the_configured_ca(t *testing.T) {
	serverTLS, caFile := cm_tlsFixture(t)
	brokers := cm_newCluster(t, kfake.NumBrokers(1), kfake.TLS(serverTLS))

	trusted, err := createNewClientWithConfig(brokers, &config.ClusterConfig{KafkaConnectionUseTLS: "true", KafkaClusterCaFile: caFile})
	require.NoError(t, err)
	defer trusted.Close()
	assert.NoError(t, cm_ping(t, trusted))

	untrusted, err := createNewClientWithConfig(brokers, &config.ClusterConfig{KafkaConnectionUseTLS: "true"})
	require.NoError(t, err)
	defer untrusted.Close()
	assert.ErrorContains(t, cm_ping(t, untrusted), "certificate", "the broker's certificate is not signed by a trusted CA")
}

func TestCreateNewClient_uses_legacy_tls_configuration(t *testing.T) {
	cm_keepLegacyConfig(t)
	serverTLS, caFile := cm_tlsFixture(t)
	brokers := cm_newCluster(t, kfake.NumBrokers(1), kfake.TLS(serverTLS))
	config.Config.KafkaConnectionUseTLS = "true"
	config.Config.KafkaClusterCaFile = caFile

	client, err := createNewClient(brokers)
	require.NoError(t, err)
	defer client.Close()
	assert.NoError(t, cm_ping(t, client))
}

func TestCreateNewClient_rejects_an_incomplete_client_certificate(t *testing.T) {
	incomplete := &config.ClusterConfig{KafkaClusterCertChainFile: "/does/not/matter.crt"}

	_, err := createNewClientWithConfig([]string{"127.0.0.1:9092"}, incomplete)
	assert.ErrorContains(t, err, "mutual TLS requires both")
	_, err = createNewAdminClientWithConfig([]string{"127.0.0.1:9092"}, incomplete)
	assert.ErrorContains(t, err, "mutual TLS requires both")

	cm_keepLegacyConfig(t)
	config.Config.KafkaClusterCertChainFile = "/does/not/matter.crt"
	_, err = createNewClient([]string{"127.0.0.1:9092"})
	assert.ErrorContains(t, err, "mutual TLS requires both")
	_, err = createNewAdminClient([]string{"127.0.0.1:9092"})
	assert.ErrorContains(t, err, "mutual TLS requires both")
}

func TestCreateNewClientForCluster_reports_unknown_or_invalid_clusters(t *testing.T) {
	cm_keepLegacyConfig(t)
	config.SetClustersForTest(map[string]*config.ClusterConfig{
		"cm-broken": {SeedBrokers: "127.0.0.1:9092", KafkaClusterCertKeyFile: "/only/the/key"},
	})

	_, err := createNewClientForCluster("cm-unknown")
	assert.ErrorContains(t, err, "cluster configuration not found")
	_, err = createNewAdminClientForCluster("cm-unknown")
	assert.ErrorContains(t, err, "cluster configuration not found")
	_, err = createNewAdminClientForCluster("cm-broken")
	assert.ErrorContains(t, err, "mutual TLS requires both")
}

func TestGetClusterNameFromConfig(t *testing.T) {
	brokers := cm_newCluster(t, kfake.NumBrokers(1), kfake.ClusterID("cm-cluster-id"))

	name, err := GetClusterNameFromConfig(&config.ClusterConfig{SeedBrokers: strings.Join(brokers, ",")})
	require.NoError(t, err)
	assert.Equal(t, "cm-cluster-id", name)

	_, err = GetClusterNameFromConfig(&config.ClusterConfig{SeedBrokers: strings.Join(brokers, ","), KafkaClusterCertChainFile: "/only/the/chain"})
	assert.ErrorContains(t, err, "failed to create client")
}

func TestGetClusterNameFromConfig_fails_without_a_cluster_id(t *testing.T) {
	brokers := cm_newCluster(t, kfake.NumBrokers(1), kfake.ClusterID(""))

	_, err := GetClusterNameFromConfig(&config.ClusterConfig{SeedBrokers: strings.Join(brokers, ",")})
	assert.ErrorContains(t, err, "cluster name is empty")
}

func TestDescribeConfig(t *testing.T) {
	cm_keepLegacyConfig(t)
	brokers := cm_newCluster(t, kfake.NumBrokers(1))
	clusterConfig := &config.ClusterConfig{SeedBrokers: strings.Join(brokers, ",")}
	ctx := context.Background()

	value, err := describeConfigInt(ctx, brokers, "message.max.bytes", 0)
	require.NoError(t, err)
	assert.Equal(t, 1048588, value)
	value, err = describeConfigIntWithConfig(ctx, brokers, "message.max.bytes", 0, clusterConfig)
	require.NoError(t, err)
	assert.Equal(t, 1048588, value)

	str, err := describeConfigStrWithConfig(ctx, brokers, "log.message.timestamp.type", 0, clusterConfig)
	require.NoError(t, err)
	assert.Equal(t, "CreateTime", str)
	_, err = describeConfigInt(ctx, brokers, "log.message.timestamp.type", 0)
	assert.Error(t, err, "a non-numeric value cannot be read as int")

	missing, err := describeConfigStr(ctx, brokers, "num.io.threads", 0)
	require.NoError(t, err, "a key the broker does not report is not an error")
	assert.Empty(t, missing)
	_, err = describeConfigIntWithConfig(ctx, brokers, "num.io.threads", 0, clusterConfig)
	assert.Error(t, err, "an empty value cannot be read as int")
}

func TestDescribeConfig_fails_for_invalid_client_configuration(t *testing.T) {
	cm_keepLegacyConfig(t)
	broken := &config.ClusterConfig{KafkaClusterCertKeyFile: "/only/the/key"}
	ctx := context.Background()

	_, err := describeConfigIntWithConfig(ctx, []string{"127.0.0.1:9092"}, "message.max.bytes", 0, broken)
	assert.ErrorContains(t, err, "mutual TLS requires both")
	assert.ErrorContains(t, alterConfigIntWithConfig(ctx, []string{"127.0.0.1:9092"}, "message.max.bytes", 1, 0, broken), "mutual TLS requires both")

	config.Config.KafkaClusterCertKeyFile = "/only/the/key"
	_, err = describeConfigInt(ctx, []string{"127.0.0.1:9092"}, "message.max.bytes", 0)
	assert.ErrorContains(t, err, "mutual TLS requires both")
	assert.ErrorContains(t, alterConfigInt(ctx, []string{"127.0.0.1:9092"}, "message.max.bytes", 1, 0), "mutual TLS requires both")
}

func TestAlterConfig_applies_the_new_value(t *testing.T) {
	cm_keepLegacyConfig(t)
	brokers := cm_newCluster(t, kfake.NumBrokers(1))
	clusterConfig := &config.ClusterConfig{SeedBrokers: strings.Join(brokers, ",")}
	ctx := context.Background()

	require.NoError(t, alterConfigInt(ctx, brokers, "message.max.bytes", 2000000, 0))
	value, err := describeConfigInt(ctx, brokers, "message.max.bytes", 0)
	require.NoError(t, err)
	assert.Equal(t, 2000000, value)

	require.NoError(t, alterConfigIntWithConfig(ctx, brokers, "message.max.bytes", 3000000, 0, clusterConfig))
	value, err = describeConfigIntWithConfig(ctx, brokers, "message.max.bytes", 0, clusterConfig)
	require.NoError(t, err)
	assert.Equal(t, 3000000, value)
}

func TestAlterConfig_reports_an_unknown_broker(t *testing.T) {
	brokers := cm_newCluster(t, kfake.NumBrokers(1))
	clusterConfig := &config.ClusterConfig{SeedBrokers: strings.Join(brokers, ",")}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	assert.Error(t, alterConfigStrWithConfig(ctx, brokers, "message.max.bytes", "2000000", 99, clusterConfig))
	_, err := describeConfigStrWithConfig(ctx, brokers, "message.max.bytes", 99, clusterConfig)
	assert.Error(t, err)
}

func TestAdjustThreads_halves_or_doubles_until_the_target(t *testing.T) {
	cm_keepLegacyConfig(t)
	brokers := cm_newCluster(t, kfake.NumBrokers(1))
	clusterConfig := &config.ClusterConfig{SeedBrokers: strings.Join(brokers, ",")}
	ctx := context.Background()

	// message.max.bytes stands in for the thread counts kfake does not support: the target is a quarter
	// of the current value, which takes two halving steps.
	require.NoError(t, adjustThreads(ctx, brokers, "message.max.bytes", 1048588/4, 0))
	value, err := describeConfigInt(ctx, brokers, "message.max.bytes", 0)
	require.NoError(t, err)
	assert.Equal(t, 1048588/4, value)

	require.NoError(t, adjustThreadsWithConfig(ctx, brokers, "message.max.bytes", 1048588, 0, clusterConfig))
	value, err = describeConfigIntWithConfig(ctx, brokers, "message.max.bytes", 0, clusterConfig)
	require.NoError(t, err)
	assert.Equal(t, 1048588, value)

	assert.Error(t, adjustThreads(ctx, brokers, "num.io.threads", 4, 0), "the broker does not report this config, so the current value is unknown")
	assert.Error(t, adjustThreadsWithConfig(ctx, brokers, "num.io.threads", 4, 0, clusterConfig))
}

func TestIsTransientError(t *testing.T) {
	assert.False(t, isTransientError(nil))
	assert.True(t, isTransientError(&net.OpError{Op: "dial", Err: errors.New("boom")}))
	assert.True(t, isTransientError(errors.New("dial tcp 127.0.0.1:9092: connect: connection refused")))
	assert.True(t, isTransientError(errors.New("unable to dial: i/o timeout")))
	assert.False(t, isTransientError(errors.New("INVALID_CONFIG")))
}

func TestRetryOnTransientError(t *testing.T) {
	t.Run("returns the first success", func(t *testing.T) {
		calls := 0
		v, err := retryOnTransientError(context.Background(), func() (int, error) { calls++; return 42, nil })
		require.NoError(t, err)
		assert.Equal(t, 42, v)
		assert.Equal(t, 1, calls)
	})
	t.Run("does not retry other errors", func(t *testing.T) {
		calls := 0
		_, err := retryOnTransientError(context.Background(), func() (int, error) { calls++; return 0, errors.New("INVALID_CONFIG") })
		assert.EqualError(t, err, "INVALID_CONFIG")
		assert.Equal(t, 1, calls)
	})
	t.Run("retries a transient error", func(t *testing.T) {
		calls := 0
		v, err := retryOnTransientError(context.Background(), func() (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("connection refused")
			}
			return "ok", nil
		})
		require.NoError(t, err)
		assert.Equal(t, "ok", v)
		assert.Equal(t, 2, calls)
	})
	t.Run("stops waiting when the context ends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		_, err := retryOnTransientError(ctx, func() (int, error) {
			calls++
			cancel()
			return 0, errors.New("connection refused")
		})
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, calls)
	})
}

// cm_parseClusters runs config.ParseConfiguration with the given CLUSTER_<i> seed brokers and names, while
// the cluster name lookup fails for the brokers listed in unreachable, so those clusters end up pending.
// Whatever is still pending when the test ends is registered under a throwaway name, which removes it from
// the pending list, so later discoveries do not retry a cluster of this test. That runs before the
// configuration is restored, as cleanups run in reverse order.
func cm_parseClusters(t *testing.T, clusters [][2]string, unreachable ...string) {
	t.Helper()
	require.Empty(t, config.GetPendingClusters(), "an earlier test left a pending cluster behind")
	cm_keepLegacyConfig(t)
	t.Cleanup(func() { config.SetClusterNameGetter(GetClusterNameFromConfig) })
	t.Cleanup(func() {
		for _, p := range config.GetPendingClusters() {
			_ = config.RegisterCluster(fmt.Sprintf("cm-cleanup-%d", p.Index), "cleanup", p.Config, p.Index)
		}
	})
	for i, c := range clusters {
		t.Setenv(fmt.Sprintf("STEADYBIT_EXTENSION_CLUSTER_%d_SEED_BROKERS", i), c[0])
		t.Setenv(fmt.Sprintf("STEADYBIT_EXTENSION_CLUSTER_%d_NAME", i), c[1])
	}
	config.SetClusterNameGetter(func(c *config.ClusterConfig) (string, error) {
		if slices.Contains(unreachable, c.SeedBrokers) {
			return "", errors.New("unreachable")
		}
		return GetClusterNameFromConfig(c)
	})
	config.ParseConfiguration()
	config.SetClusterNameGetter(GetClusterNameFromConfig)
}

func TestRetryPendingClusters_registers_a_cluster_that_became_reachable(t *testing.T) {
	a := strings.Join(cm_newCluster(t, kfake.NumBrokers(1), kfake.ClusterID("cm-a")), ",")
	b := strings.Join(cm_newCluster(t, kfake.NumBrokers(1), kfake.ClusterID("cm-b")), ",")
	cm_parseClusters(t, [][2]string{{a, "cm-first"}, {b, "cm-second"}}, b)
	require.Len(t, config.GetPendingClusters(), 1)
	_, err := config.GetClusterConfig("cm-second")
	require.Error(t, err, "a pending cluster is not usable yet")

	RetryPendingClusters()

	assert.Empty(t, config.GetPendingClusters())
	second, err := config.GetClusterConfig("cm-second")
	require.NoError(t, err)
	assert.Equal(t, "cm-b", second.ClusterID)
	first, err := config.GetClusterConfig("cm-first")
	require.NoError(t, err)
	assert.Equal(t, "cm-a", first.ClusterID)
}

func TestRetryPendingClusters_keeps_a_cluster_it_cannot_resolve_or_register(t *testing.T) {
	a := strings.Join(cm_newCluster(t, kfake.NumBrokers(1), kfake.ClusterID("cm-a")), ",")
	b := strings.Join(cm_newCluster(t, kfake.NumBrokers(1), kfake.ClusterID("cm-b")), ",")
	// CLUSTER_0 resolves on retry, but CLUSTER_1 has taken its name by then. CLUSTER_2 never resolves,
	// because its client configuration is invalid.
	t.Setenv("STEADYBIT_EXTENSION_CLUSTER_2_KAFKA_CLUSTER_CERT_CHAIN_FILE", "/only/the/chain")
	cm_parseClusters(t, [][2]string{{b, "cm-same"}, {a, "cm-same"}, {a, "cm-invalid"}}, b)
	require.Len(t, config.GetPendingClusters(), 2)

	RetryPendingClusters()

	pending := config.GetPendingClusters()
	require.Len(t, pending, 2, "neither pending cluster can be registered")
	assert.ElementsMatch(t, []int{0, 2}, []int{pending[0].Index, pending[1].Index})
	same, err := config.GetClusterConfig("cm-same")
	require.NoError(t, err)
	assert.Equal(t, "cm-a", same.ClusterID, "the name keeps the cluster registered first")
}

func TestRetryPendingClusters_does_nothing_without_pending_clusters(t *testing.T) {
	require.Empty(t, config.GetPendingClusters())
	RetryPendingClusters()
	assert.Empty(t, config.GetPendingClusters())
}
