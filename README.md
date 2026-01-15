# Steadybit extension-kafka

A [Steadybit](https://www.steadybit.com/) extension to integrate [Kafka](https://kafka.apache.org/) into Steadybit.

Learn about the capabilities of this extension in
our [Reliability Hub](https://hub.steadybit.com/extension/com.steadybit.extension_kafka).

## Prerequisites

The extension-kafka is using these capacities, thus may need elevated rights on kafka side :

- List brokers / topics / consumer groups / offsets
- Elect leaders for partitions
- Alter broker configuration
- Create / Delete ACLs
- Delete Records

## Configuration

### Single Cluster Configuration

For connecting to a single Kafka cluster, use the following configuration:

| Environment Variable                                                | Helm value                               | Meaning                                                                                                                                 | Required | Default |
|---------------------------------------------------------------------|------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|----------|---------|
| `STEADYBIT_EXTENSION_SEED_BROKERS`                                  | `kafka.seedBrokers`                      | Brokers hosts (without scheme) with port separated by comma (example: "localhost:9092,localhost:9093"                                   | yes      |         |
| `STEADYBIT_EXTENSION_SASL_MECHANISM`                                | `kafka.auth.saslMechanism`               | PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512                                                                                                  | no       |         |
| `STEADYBIT_EXTENSION_SASL_USER`                                     | `kafka.auth.saslUser`                    | Sasl User                                                                                                                               | no       |         |
| `STEADYBIT_EXTENSION_SASL_PASSWORD`                                 | `kafka.auth.saslPassword`                | Sasl Password                                                                                                                           | no       |         |
| `STEADYBIT_EXTENSION_KAFKA_CLUSTER_CERT_CHAIN_FILE`                 | `kafka.auth.kafkaClusterCertChainFile`   | The client certificate in PEM format.                                                                                                   | no       |         |
| `STEADYBIT_EXTENSION_KAFKA_CLUSTER_CERT_KEY_FILE`                   | `kafka.auth.kafkaClusterCertKeyFile`     | The private key associated with the client certificate.                                                                                 | no       |         |
| `STEADYBIT_EXTENSION_KAFKA_CLUSTER_CA_FILE`                         | `kafka.auth.kafkaClusterCaFile`          | The Certificate Authority (CA) certificate in PEM format.                                                                               | no       |         |
| `STEADYBIT_EXTENSION_KAFKA_CONNECTION_USE_TLS`                      | `kafka.auth.useTLS`                      | Switch to "true" to use a TLS connection with default system certs, fill the certs fields above if you want to tune the tls connection. | no       |         |
| `STEADYBIT_EXTENSION_DISCOVERY_ATTRIBUTES_EXCLUDES_BROKERS`         | `discovery.attributes.excludes.broker`   | List of Broker Attributes which will be excluded during discovery. Checked by key equality and supporting trailing "*"                  | no       |         |
| `STEADYBIT_EXTENSION_DISCOVERY_ATTRIBUTES_EXCLUDES_TOPICS`          | `discovery.attributes.excludes.topic`    | List of Broker Attributes which will be excluded during discovery. Checked by key equality and supporting trailing "*"                  | no       |         |
| `STEADYBIT_EXTENSION_DISCOVERY_ATTRIBUTES_EXCLUDES_CONSUMER_GROUPS` | `discovery.attributes.excludes.consumer` | List of Broker Attributes which will be excluded during discovery. Checked by key equality and supporting trailing "*"                  | no       |         |

### Multi-Cluster Configuration

The extension supports connecting to multiple Kafka clusters simultaneously. When using multi-cluster configuration, all clusters are discovered in parallel and each target includes a `kafka.cluster.name` attribute to identify which cluster it belongs to.

**Important:** When `kafka.clusters` is defined, the single-cluster configuration (`kafka.seedBrokers`, `kafka.auth.*`) is ignored.

#### Helm Values

Configure multiple clusters using the `kafka.clusters` array:

```yaml
kafka:
  clusters:
    - name: production                              # Descriptive name (for your reference)
      seedBrokers: "broker1:9092,broker2:9092"      # Required: comma-separated broker list
      auth:
        saslMechanism: "SCRAM-SHA-256"              # Optional: PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512
        saslUser: "prod-user"                       # Optional: SASL username
        saslPassword: "prod-pass"                   # Optional: SASL password
        useTLS: "true"                              # Optional: enable TLS
        caFile: "/path/to/ca.pem"                   # Optional: CA certificate path
        certChainFile: "/path/to/cert.pem"          # Optional: client certificate path
        certKeyFile: "/path/to/key.pem"             # Optional: client key path

    - name: staging
      seedBrokers: "staging-broker:9092"
      auth:
        existingSecret: "staging-kafka-secret"      # Optional: use existing K8s secret for auth
```

#### Using Existing Secrets

For each cluster, you can reference an existing Kubernetes secret containing the authentication credentials. The secret should contain the following keys:

- `sasl-mechanism` - SASL mechanism (PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512)
- `sasl-user` - SASL username
- `sasl-password` - SASL password

Example secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: staging-kafka-secret
type: Opaque
stringData:
  sasl-mechanism: "SCRAM-SHA-256"
  sasl-user: "staging-user"
  sasl-password: "staging-password"
```

#### Environment Variables (Alternative)

Multi-cluster configuration can also be done via indexed environment variables. Replace `X` with the cluster index (0, 1, 2, ...):

| Environment Variable                                      | Meaning                                      |
|-----------------------------------------------------------|----------------------------------------------|
| `STEADYBIT_EXTENSION_CLUSTER_X_SEED_BROKERS`              | Comma-separated broker list                  |
| `STEADYBIT_EXTENSION_CLUSTER_X_SASL_MECHANISM`            | PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512       |
| `STEADYBIT_EXTENSION_CLUSTER_X_SASL_USER`                 | SASL username                                |
| `STEADYBIT_EXTENSION_CLUSTER_X_SASL_PASSWORD`             | SASL password                                |
| `STEADYBIT_EXTENSION_CLUSTER_X_KAFKA_CONNECTION_USE_TLS`  | Enable TLS (true/false)                      |
| `STEADYBIT_EXTENSION_CLUSTER_X_KAFKA_CLUSTER_CA_FILE`     | CA certificate path                          |
| `STEADYBIT_EXTENSION_CLUSTER_X_KAFKA_CLUSTER_CERT_CHAIN_FILE` | Client certificate path                  |
| `STEADYBIT_EXTENSION_CLUSTER_X_KAFKA_CLUSTER_CERT_KEY_FILE`   | Client key path                          |

Example:

```bash
# First cluster
STEADYBIT_EXTENSION_CLUSTER_0_SEED_BROKERS=prod-broker1:9092,prod-broker2:9092
STEADYBIT_EXTENSION_CLUSTER_0_SASL_MECHANISM=SCRAM-SHA-256
STEADYBIT_EXTENSION_CLUSTER_0_SASL_USER=prod-user
STEADYBIT_EXTENSION_CLUSTER_0_SASL_PASSWORD=prod-pass

# Second cluster
STEADYBIT_EXTENSION_CLUSTER_1_SEED_BROKERS=staging-broker:9092
STEADYBIT_EXTENSION_CLUSTER_1_SASL_MECHANISM=PLAIN
STEADYBIT_EXTENSION_CLUSTER_1_SASL_USER=staging-user
STEADYBIT_EXTENSION_CLUSTER_1_SASL_PASSWORD=staging-pass
```

The extension supports all environment variables provided
by [steadybit/extension-kit](https://github.com/steadybit/extension-kit#environment-variables).

## Installation

### Using Docker

```sh
docker run \
  --rm \
  -p 8080 \
  --name steadybit-extension-kafka \
  --env STEADYBIT_EXTENSION_SEED_BROKERS="localhost:9092" \
  ghcr.io/steadybit/extension-kafka:latest
```

### Using Helm in Kubernetes

```sh
helm repo add steadybit-extension-kafka https://steadybit.github.io/extension-kafka
helm repo update
helm upgrade steadybit-extension-kafka \
    --install \
    --wait \
    --timeout 5m0s \
    --create-namespace \
    --namespace steadybit-agent \
    --set kafka.seedBrokers="localhost:9092" \
    steadybit-extension-kafka/steadybit-extension-kafka
```

## Register the extension

Make sure to register the extension on the Steadybit platform. Please refer to
the [documentation](https://docs.steadybit.com/integrate-with-steadybit/extensions/extension-installation) for more
information.

## Generate pem files from truststore and keystore

### Prerequisites

- **Keystore file**: `kafka.keystore.jks` (contains the client certificate and private key).
- **Truststore file**: `kafka.truststore.jks` (contains the CA certificate).
- **Tools Required**: `keytool` and `openssl` must be installed.

---

### Steps to Generate PEM Files

1. **Export the CA Certificate (`ca-cert.pem`)**
	 Extract the CA certificate from the truststore using the following command:

```bash
keytool -exportcert \
 -keystore kafka.truststore.jks \
 -alias CARoot \
 -storepass <truststore-password> \
 -rfc -file ca-cert.pem
```

• Replace <truststore-password> with the password for the truststore.
• The output file ca-cert.pem will contain the CA certificate in PEM format.

2. **Convert the Keystore to PKCS12 Format**

Convert the keystore to a PKCS12 file to facilitate extracting the certificate and private key:

```bash
keytool -importkeystore \
-srckeystore kafka.keystore.jks \
-srcstorepass <keystore-password> \
-srcalias kafka \
-destkeystore kafka-keystore.p12 \
-deststoretype PKCS12 \
-deststorepass <p12-password>
```

• Replace <keystore-password> with the password for the keystore.
• Replace <p12-password> with a new password for the PKCS12 file.
• This will generate the file kafka-keystore.p12, which contains both the client certificate and private key.

3. **Extract the Private Key (client-key.pem)**
	 Use the following command to extract the private key from the PKCS12 file:

```bash
 openssl pkcs12 -in kafka-keystore.p12 \
 -nocerts -nodes -out client-key.pem \
 -passin pass:<p12-password>
```

• Replace <p12-password> with the password set for the PKCS12 file.
• This will generate the file client-key.pem, which contains the private key in PEM format.

4. **Extract the Client Certificate (client-cert.pem)**
	 Use the following command to extract the client certificate from the PKCS12 file:

```bash
 openssl pkcs12 -in kafka-keystore.p12 \
 -clcerts -nokeys -out client-cert.pem \
 -passin pass:<p12-password>
```

• Replace <p12-password> with the password set for the PKCS12 file.
• This will generate the file client-cert.pem, which contains the client certificate in PEM format.

5. **(Optional) Verifying the Generated PEM Files**

```bash
openssl x509 -in ca-cert.pem -text -noout
openssl rsa -in client-key.pem -check
openssl x509 -in client-cert.pem -text -noout
```

Ensure that:
• The CA certificate includes the correct issuer and validity period.
• The private key matches the client certificate.

#### Issue: “Alias not found”

Verify the contents of the keystore and truststore:

```bash
keytool -list -v -keystore kafka.keystore.jks -storepass <keystore-password>
keytool -list -v -keystore kafka.truststore.jks -storepass <truststore-password>
```

#### Notes

1. The private key (client-key.pem) must be kept secure. Unauthorized access to this file can compromise the client.
2. Ensure the Kafka broker’s hostname or IP address matches the Subject Alternative Name (SAN) in the server’s
	 certificate.
3. Always use strong passwords for your keystore, truststore, and PKCS12 files.

## Version and Revision

The version and revision of the extension:

- are printed during the startup of the extension
- are added as a Docker label to the image
- are available via the `version.txt`/`revision.txt` files in the root of the image
