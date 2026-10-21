// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Steadybit GmbH

package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/steadybit/extension-kit/exttls"
)

const tlsDialTimeout = 10 * time.Second

// UsesTLS reports whether connections to the cluster are encrypted: either explicitly requested, or
// implied by a configured certificate file.
func (c *ClusterConfig) UsesTLS() bool {
	return c.KafkaConnectionUseTLS == "true" || c.KafkaClusterCaFile != "" || c.KafkaClusterCertChainFile != "" || c.KafkaClusterCertKeyFile != ""
}

// TLSConfig builds the TLS configuration for the cluster, or returns nil when it does not use TLS.
//
// It starts from exttls, so STEADYBIT_EXTENSION_INSECURE_SKIP_VERIFY applies and, without a CA file, the
// system pool is used (which honours SSL_CERT_DIR). A CA file replaces the system pool. A client
// certificate is added for mutual TLS when both the certificate chain and the key are configured.
func (c *ClusterConfig) TLSConfig() (*tls.Config, error) {
	if !c.UsesTLS() {
		return nil, nil
	}
	tlsConfig := exttls.ClientConfig()

	if (c.KafkaClusterCertChainFile == "") != (c.KafkaClusterCertKeyFile == "") {
		return nil, errors.New("mutual TLS requires both the certificate chain file and the certificate key file, only one is configured")
	}
	if c.KafkaClusterCertChainFile != "" {
		cert, err := tls.LoadX509KeyPair(c.KafkaClusterCertChainFile, c.KafkaClusterCertKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if c.KafkaClusterCaFile != "" {
		caCert, err := os.ReadFile(c.KafkaClusterCaFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("no PEM certificate found in CA file %s", c.KafkaClusterCaFile)
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// TLSDialer returns the function to dial the cluster's brokers over TLS, for kgo.Dialer, or nil when
// the cluster does not use TLS.
func (c *ClusterConfig) TLSDialer() (func(ctx context.Context, network, host string) (net.Conn, error), error) {
	tlsConfig, err := c.TLSConfig()
	if err != nil || tlsConfig == nil {
		return nil, err
	}
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: tlsDialTimeout}, Config: tlsConfig}
	return dialer.DialContext, nil
}
