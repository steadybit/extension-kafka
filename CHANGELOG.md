# Changelog

## v1.2.4

- Support changing IO and network thread count values with huge increments or decrements
- Update dependencies

## v1.2.3

- Bump app version

## v1.2.2

- Bump app version

## v1.2.1

- Add cluster name to topic and consumergroup targets

## v1.2.0

- Add cluster name to broker target attributes
- Better target ID for brokers in case of multiple clusters
- Add min/max validations
- Update dependencies

## v1.1.1

- Make extension-kafka compatible with AWS MSK SCRAM-SHA-512 Auth
- Add TLS for compatibility with SASL_SSL security protocol
- Update to go 1.24
- Update dependencies

## v1.1.0

- Fix log line for check error
- Change metric colors behavior
- Change name of kafka config for certs

## v1.0.9

- Fix log line for check error
- Fix metric ID for broker check

## v1.0.8

- Add pod and container enrichment

## v1.0.7

- Fix action ID

## v1.0.6

- Add controller information to target attributes
- Add new broker check
- Add TLS connection support
- Update dependencies

## v1.0.5

- Use uid instead of name for user statement in Dockerfile
- Fix data race issue
- Update dependencies

## v1.0.0

 - Initial release
