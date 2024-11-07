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

| Environment Variable                 | Helm value                 | Meaning                                                                                               | Required | Default |
|--------------------------------------|----------------------------|-------------------------------------------------------------------------------------------------------|----------|---------|
| `STEADYBIT_EXTENSION_SEED_BROKERS`   | `kafka.seedBrokers`        | Brokers hosts (without scheme) with port separated by comma (example: "localhost:9092,localhost:9093" | yes      |         |
| `STEADYBIT_EXTENSION_SASL_MECHANISM` | `kafka.auth.saslMechanism` | PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512                                                                | no       |         |
| `STEADYBIT_EXTENSION_SASL_USER`      | `kafka.auth.saslUser`      | Sasl User                                                                                             | no       |         |
| `STEADYBIT_EXTENSION_SASL_PASSWORD`  | `kafka.auth.saslPassword`  | Sasl Password                                                                                         | no       |         |


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
helm repo add steadybit-extension-grafana https://steadybit.github.io/extension-kafka
helm repo update
helm upgrade steadybit-extension-kafka \
    --install \
    --wait \
    --timeout 5m0s \
    --create-namespace \
    --namespace steadybit-agent \
    --set kafka.seedBrokers="localhost:9092" \
    steadybit-extension-grafana/steadybit-extension-grafana
```

## Register the extension

Make sure to register the extension on the Steadybit platform. Please refer to the [documentation](https://docs.steadybit.com/integrate-with-steadybit/extensions/extension-installation) for more information.

## FAQ

### The extension-grafana is unauthorized to fetch data from grafana (status code 401)

Do you provide the service account token to the extension ? Does the token still exists on Grafana ?

_warning: If you want the service account token to survive a Grafana pod deletion or restart, you need to [persist the Grafana data in a DB](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/#database)._
