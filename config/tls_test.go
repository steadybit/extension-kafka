// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Steadybit GmbH

package config

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type selfSigned struct {
	cert     tls.Certificate
	certFile string
	keyFile  string
}

func newSelfSigned(t *testing.T) selfSigned {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDer, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer}), 0o600))
	return selfSigned{cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, certFile: certFile, keyFile: keyFile}
}

// startTLSListener accepts connections and completes the handshake, so the client's verification result
// is what the dial returns.
func startTLSListener(t *testing.T, server selfSigned) string {
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{server.cert}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.(*tls.Conn).Handshake()
			_ = conn.Close()
		}
	}()
	return listener.Addr().String()
}

func dial(t *testing.T, c *ClusterConfig, address string) error {
	dialer, err := c.TLSDialer()
	require.NoError(t, err)
	require.NotNil(t, dialer)
	conn, err := dialer(context.Background(), "tcp", address)
	if err != nil {
		return err
	}
	return conn.Close()
}

func TestTLSConfig_disabled_without_tls_settings(t *testing.T) {
	c := &ClusterConfig{}

	tlsConfig, err := c.TLSConfig()
	require.NoError(t, err)
	assert.Nil(t, tlsConfig)
	dialer, err := c.TLSDialer()
	require.NoError(t, err)
	assert.Nil(t, dialer)
}

func TestTLSConfig_rejects_self_signed_certificate_by_default(t *testing.T) {
	t.Setenv("STEADYBIT_EXTENSION_INSECURE_SKIP_VERIFY", "")
	address := startTLSListener(t, newSelfSigned(t))

	err := dial(t, &ClusterConfig{KafkaConnectionUseTLS: "true"}, address)

	var unknownAuthority x509.UnknownAuthorityError
	assert.ErrorAs(t, err, &unknownAuthority)
}

func TestTLSConfig_skips_verification_when_enabled(t *testing.T) {
	t.Setenv("STEADYBIT_EXTENSION_INSECURE_SKIP_VERIFY", "true")
	address := startTLSListener(t, newSelfSigned(t))

	assert.NoError(t, dial(t, &ClusterConfig{KafkaConnectionUseTLS: "true"}, address))
}

func TestTLSConfig_trusts_ca_file_without_client_certificate(t *testing.T) {
	t.Setenv("STEADYBIT_EXTENSION_INSECURE_SKIP_VERIFY", "")
	server := newSelfSigned(t)
	address := startTLSListener(t, server)

	// The CA file alone enables TLS, kafkaConnectionUseTLS is not needed.
	c := &ClusterConfig{KafkaClusterCaFile: server.certFile}

	assert.True(t, c.UsesTLS())
	assert.NoError(t, dial(t, c, address))
}

func TestTLSConfig_adds_client_certificate_for_mutual_tls(t *testing.T) {
	server := newSelfSigned(t)
	client := newSelfSigned(t)

	tlsConfig, err := (&ClusterConfig{
		KafkaClusterCaFile:        server.certFile,
		KafkaClusterCertChainFile: client.certFile,
		KafkaClusterCertKeyFile:   client.keyFile,
	}).TLSConfig()

	require.NoError(t, err)
	require.Len(t, tlsConfig.Certificates, 1)
	assert.Equal(t, client.cert.Certificate[0], tlsConfig.Certificates[0].Certificate[0])
	assert.NotNil(t, tlsConfig.RootCAs)
}

func TestTLSConfig_fails_when_only_part_of_the_client_certificate_is_configured(t *testing.T) {
	client := newSelfSigned(t)

	_, errChainOnly := (&ClusterConfig{KafkaClusterCertChainFile: client.certFile}).TLSConfig()
	_, errKeyOnly := (&ClusterConfig{KafkaClusterCertKeyFile: client.keyFile}).TLSConfig()

	assert.ErrorContains(t, errChainOnly, "requires both")
	assert.ErrorContains(t, errKeyOnly, "requires both")
}

func TestTLSConfig_fails_on_invalid_ca_file(t *testing.T) {
	invalid := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(invalid, []byte("not a certificate"), 0o600))

	_, errInvalid := (&ClusterConfig{KafkaClusterCaFile: invalid}).TLSConfig()
	_, errMissing := (&ClusterConfig{KafkaClusterCaFile: filepath.Join(t.TempDir(), "missing.pem")}).TLSConfig()

	assert.ErrorContains(t, errInvalid, "no PEM certificate")
	assert.ErrorContains(t, errMissing, "failed to read CA file")
}
