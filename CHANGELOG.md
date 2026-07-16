# Changelog

## v1.2.17

- Add a "Fail early" option to the broker, consumer group, partition and topic lag checks. When enabled, the check fails as soon as a deviating event is observed; when disabled, it keeps collecting events for the whole duration and only fails at the end of the step. The broker/consumer-group/partition checks default to fail-early (matching their previous behavior); the topic lag check defaults to fail-at-end (matching its previous behavior). Only affects the "All the time" mode of the mode-based checks.
- Merge pull request #123 from steadybit/feat/check-fail-early-option
- build(deps): bump github.com/steadybit/event-kit/go/event_kit_api
- build(deps): bump github.com/twmb/franz-go from 1.21.3 to 1.21.5
- build(deps): bump golang.org/x/crypto in /test-dataset/dummyconsumer
- chore(deps): bump go to 1.26.5 (#125)
- chore(deps): bump go-openapi/swag/loading to fix go mod tidy (#128)
- chore(deps): update dependencies
- chore: add Claude Code workflows (#122)
- chore: normalize dependabot-auto-merge workflow to the standard version (#124)
- chore: silence SonarQube finding on secrets: inherit in Claude workflows
- ci: skip build on .trivyignore.yml-only changes [skip ci]
- refactor: register extension index via exthttp.RegisterRevisionedHandler (#127)

## v1.2.16

- build(deps): bump actions/checkout from 6 to 7
- build(deps): bump github.com/steadybit/extension-kit
- chore(deps): bump golang.org/x/net to v0.55.0 (CVE-2026-39821) (#117)

## v1.2.15

- build(deps): bump github.com/twmb/franz-go from 1.21.2 to 1.21.3
- chore: update to go 1.26.4
- feat: add weekly auto patch-release workflow

## v1.2.14

- Support discovery group attribute via `STEADYBIT_EXTENSION_DISCOVERY_GROUP` env var (or `discovery.group` Helm value) — when set, the extension adds `steadybit.group=<value>` to every discovered target
- Update dependencies

## v1.2.13

- Bump Go to 1.26.3
- Update dependencies

## v1.2.12

- Bump Go to 1.26.2
- Update alpine packages in Docker image

## v1.2.11

- Bump Go to 1.25.9
- Update dependencies

## v1.2.9

- Support if-none-match for the extension list endpoint
- Fix Kafka admin client connection leak in broker config describe operations
- Retry on transient connection errors in broker config operations
- feat(chart): split image.name into image.registry + image.name
- Support global.priorityClassName
- Update dependencies

## v1.2.8

- Update dependencies

## v1.2.7

- Support multi cluster for configuration
- Breaking change: now Check Brokers needs broker targets.

## v1.2.6

- Update dependencies

## v1.2.5

- Update dependencies

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
