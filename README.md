# Steadybit extension-kafka

A [Steadybit](https://www.steadybit.com/) extension to integrate [Kafka](https://kafka.apache.org/) into Steadybit.

Learn about the capabilities of this extension in our [Reliability Hub](https://hub.steadybit.com/extension/com.steadybit.extension_kafka).

## Prerequisites

The extension-kafka is using these capacities, thus may need elevated rights on kafka side :
- List brokers / topics / consumer groups / offsets
- Elect leaders for partitions
- Alter broker configuration
- Create / Delete ACLs
- Delete Records

## Configuration

| Environment Variable                                                | Helm value                               | Meaning                                                                                                                | Required | Default |
|---------------------------------------------------------------------|------------------------------------------|------------------------------------------------------------------------------------------------------------------------|----------|---------|
| `STEADYBIT_EXTENSION_SEED_BROKERS`                                  | `kafka.seedBrokers`                      | Brokers hosts (without scheme) with port separated by comma (example: "localhost:9092,localhost:9093"                  | yes      |         |
| `STEADYBIT_EXTENSION_SASL_MECHANISM`                                | `kafka.auth.saslMechanism`               | PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512                                                                                 | no       |         |
| `STEADYBIT_EXTENSION_SASL_USER`                                     | `kafka.auth.saslUser`                    | Sasl User                                                                                                              | no       |         |
| `STEADYBIT_EXTENSION_SASL_PASSWORD`                                 | `kafka.auth.saslPassword`                | Sasl Password                                                                                                          | no       |         |
| `STEADYBIT_EXTENSION_CERT_CHAIN_FILE`                               | `kafka.auth.certChainFile`               | The client certificate in PEM format.                                                                                  | no       |         |
| `STEADYBIT_EXTENSION_CERT_KEY_FILE`                                 | `kafka.auth.certKeyFile`                 | The private key associated with the client certificate.                                                                | no       |         |
| `STEADYBIT_EXTENSION_CA_FILE`                                       | `kafka.auth.caFile`                      | The Certificate Authority (CA) certificate in PEM format.                                                              | no       |         |
| `STEADYBIT_EXTENSION_DISCOVERY_ATTRIBUTES_EXCLUDES_BROKERS`         | `discovery.attributes.excludes.broker`   | List of Broker Attributes which will be excluded during discovery. Checked by key equality and supporting trailing "*" | no       |         |
| `STEADYBIT_EXTENSION_DISCOVERY_ATTRIBUTES_EXCLUDES_TOPICS`          | `discovery.attributes.excludes.topic`    | List of Broker Attributes which will be excluded during discovery. Checked by key equality and supporting trailing "*" | no       |         |
| `STEADYBIT_EXTENSION_DISCOVERY_ATTRIBUTES_EXCLUDES_CONSUMER_GROUPS` | `discovery.attributes.excludes.consumer` | List of Broker Attributes which will be excluded during discovery. Checked by key equality and supporting trailing "*" | no       |         |


The extension supports all environment variables provided by [steadybit/extension-kit](https://github.com/steadybit/extension-kit#environment-variables).

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

Make sure to register the extension on the Steadybit platform. Please refer to the [documentation](https://docs.steadybit.com/integrate-with-steadybit/extensions/extension-installation) for more information.

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

•	Replace <truststore-password> with the password for the truststore.
•	The output file ca-cert.pem will contain the CA certificate in PEM format.

2.	**Convert the Keystore to PKCS12 Format**

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
•	Replace <keystore-password> with the password for the keystore.
•	Replace <p12-password> with a new password for the PKCS12 file.
•	This will generate the file kafka-keystore.p12, which contains both the client certificate and private key.

3. **Extract the Private Key (client-key.pem)**
Use the following command to extract the private key from the PKCS12 file:
```bash
 openssl pkcs12 -in kafka-keystore.p12 \
 -nocerts -nodes -out client-key.pem \
 -passin pass:<p12-password>
```
•	Replace <p12-password> with the password set for the PKCS12 file.
•	This will generate the file client-key.pem, which contains the private key in PEM format.
4. **Extract the Client Certificate (client-cert.pem)**
Use the following command to extract the client certificate from the PKCS12 file:
```bash
 openssl pkcs12 -in kafka-keystore.p12 \
 -clcerts -nokeys -out client-cert.pem \
 -passin pass:<p12-password>
```
•	Replace <p12-password> with the password set for the PKCS12 file.
•	This will generate the file client-cert.pem, which contains the client certificate in PEM format.

5. **(Optional) Verifying the Generated PEM Files**
```bash
openssl x509 -in ca-cert.pem -text -noout
openssl rsa -in client-key.pem -check
openssl x509 -in client-cert.pem -text -noout
```
Ensure that:
•	The CA certificate includes the correct issuer and validity period.
•	The private key matches the client certificate.


#### Issue: “Alias not found”
Verify the contents of the keystore and truststore:
```bash
keytool -list -v -keystore kafka.keystore.jks -storepass <keystore-password>
keytool -list -v -keystore kafka.truststore.jks -storepass <truststore-password>
```

#### Notes
1.	The private key (client-key.pem) must be kept secure. Unauthorized access to this file can compromise the client.
2.	Ensure the Kafka broker’s hostname or IP address matches the Subject Alternative Name (SAN) in the server’s certificate.
3.	Always use strong passwords for your keystore, truststore, and PKCS12 files.
