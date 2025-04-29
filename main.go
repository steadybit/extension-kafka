/*
 * Copyright 2024 steadybit GmbH. All rights reserved.
 */

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "github.com/KimMachineGun/automemlimit" // By default, it sets `GOMEMLIMIT` to 90% of cgroup's memory limit.
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/advice-kit/go/advice_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_sdk"
	"github.com/steadybit/event-kit/go/event_kit_api"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kafka/extkafka"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/exthealth"
	"github.com/steadybit/extension-kit/exthttp"
	"github.com/steadybit/extension-kit/extlogging"
	"github.com/steadybit/extension-kit/extruntime"
	"github.com/steadybit/extension-kit/extsignals"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	_ "go.uber.org/automaxprocs" // Importing automaxprocs automatically adjusts GOMAXPROCS.
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	// Most Steadybit extensions leverage zerolog. To encourage persistent logging setups across extensions,
	// you may leverage the extlogging package to initialize zerolog. Among others, this package supports
	// configuration of active log levels and the log format (JSON or plain text).
	//
	// Example
	//  - to activate JSON logging, set the environment variable STEADYBIT_LOG_FORMAT="json"
	//  - to set the log level to debug, set the environment variable STEADYBIT_LOG_LEVEL="debug"
	extlogging.InitZeroLog()

	// Build information is set at compile-time. This line writes the build information to the log.
	// The information is mostly handy for debugging purposes.
	extbuild.PrintBuildInformation()
	extruntime.LogRuntimeInformation(zerolog.DebugLevel)

	// Most extensions require some form of configuration. These calls exist to parse and validate the
	// configuration obtained from environment variables.
	config.ParseConfiguration()
	config.ValidateConfiguration()
	testBrokerConnection()

	//This will start /health/liveness and /health/readiness endpoints on port 8081 for use with kubernetes
	//The port can be configured using the STEADYBIT_EXTENSION_HEALTH_PORT environment variable
	exthealth.SetReady(false)
	exthealth.StartProbes(8084)

	ctx, cancel := SignalCanceledContext()

	registerHandlers(ctx)

	extsignals.AddSignalHandler(extsignals.SignalHandler{
		Handler: func(signal os.Signal) {
			cancel()
		},
		Order: extsignals.OrderStopCustom,
		Name:  "custom-extension-kafka",
	})
	extsignals.ActivateSignalHandlers()

	//This will register the coverage endpoints for the extension (used by action_kit_test)
	action_kit_sdk.RegisterCoverageEndpoints()

	//This will switch the readiness state of the application to true.
	exthealth.SetReady(true)

	exthttp.Listen(exthttp.ListenOpts{
		// This is the default port under which your extension is accessible.
		// The port can be configured externally through the
		// STEADYBIT_EXTENSION_PORT environment variable.
		// We suggest that you keep port 8080 as the default.
		Port: 8083,
	})
}

// ExtensionListResponse exists to merge the possible root path responses supported by the
// various extension kits. In this case, the response for ActionKit, DiscoveryKit and EventKit.
type ExtensionListResponse struct {
	action_kit_api.ActionList       `json:",inline"`
	discovery_kit_api.DiscoveryList `json:",inline"`
	event_kit_api.EventListenerList `json:",inline"`
	advice_kit_api.AdviceList       `json:",inline"`
}

func registerHandlers(ctx context.Context) {
	discovery_kit_sdk.Register(extkafka.NewKafkaBrokerDiscovery(ctx))
	discovery_kit_sdk.Register(extkafka.NewKafkaTopicDiscovery(ctx))
	discovery_kit_sdk.Register(extkafka.NewKafkaConsumerGroupDiscovery(ctx))
	action_kit_sdk.RegisterAction(extkafka.NewProduceMessageActionPeriodically())
	action_kit_sdk.RegisterAction(extkafka.NewProduceMessageActionFixedAmount())
	action_kit_sdk.RegisterAction(extkafka.NewConsumerGroupCheckAction())
	action_kit_sdk.RegisterAction(extkafka.NewConsumerGroupLagCheckAction())
	action_kit_sdk.RegisterAction(extkafka.NewKafkaBrokerElectNewLeaderAttack())
	action_kit_sdk.RegisterAction(extkafka.NewDeleteRecordsAttack())
	action_kit_sdk.RegisterAction(extkafka.NewAlterMaxMessageBytesAttack())
	action_kit_sdk.RegisterAction(extkafka.NewAlterNumberIOThreadsAttack())
	action_kit_sdk.RegisterAction(extkafka.NewAlterNumberNetworkThreadsAttack())
	action_kit_sdk.RegisterAction(extkafka.NewAlterLimitConnectionCreateRateAttack())
	action_kit_sdk.RegisterAction(extkafka.NewKafkaConsumerDenyAccessAttack())
	action_kit_sdk.RegisterAction(extkafka.NewPartitionsCheckAction())
	action_kit_sdk.RegisterAction(extkafka.NewBrokersCheckAction())

	exthttp.RegisterHttpHandler("/", exthttp.GetterAsHandler(getExtensionList))
}

func SignalCanceledContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, func() {
		signal.Stop(c)
		cancel()
	}
}

func getExtensionList() ExtensionListResponse {
	return ExtensionListResponse{
		// See this document to learn more about the action list:
		// https://github.com/steadybit/action-kit/blob/main/docs/action-api.md#action-list
		ActionList: action_kit_sdk.GetActionList(),

		// See this document to learn more about the discovery list:
		// https://github.com/steadybit/discovery-kit/blob/main/docs/discovery-api.md#index-response
		DiscoveryList: discovery_kit_sdk.GetDiscoveryList(),
	}
}

func testBrokerConnection() {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(config.Config.SeedBrokers, ",")...),
		kgo.DefaultProduceTopic("steadybit"),
		kgo.ClientID("steadybit"),
	}

	if config.Config.SaslMechanism != "" {
		switch saslMechanism := config.Config.SaslMechanism; saslMechanism {
		case kadm.ScramSha256.String():
			opts = append(opts, []kgo.Opt{
				kgo.SASL(scram.Auth{
					User: config.Config.SaslUser,
					Pass: config.Config.SaslPassword,
				}.AsSha256Mechanism()),
			}...)
		case kadm.ScramSha512.String():
			opts = append(opts, []kgo.Opt{
				kgo.SASL(scram.Auth{
					User: config.Config.SaslUser,
					Pass: config.Config.SaslPassword,
				}.AsSha512Mechanism()),
			}...)
		default:
			opts = append(opts, []kgo.Opt{
				kgo.SASL(plain.Auth{
					User: config.Config.SaslUser,
					Pass: config.Config.SaslPassword,
				}.AsMechanism()),
			}...)
		}
	}

	if config.Config.KafkaClusterCaFile != "" && config.Config.KafkaClusterCertKeyFile != "" && config.Config.KafkaClusterCertChainFile != "" {
		tlsConfig, err := newTLSConfig(config.Config.KafkaClusterCertChainFile, config.Config.KafkaClusterCertKeyFile, config.Config.KafkaClusterCaFile)
		if err != nil {
			log.Fatal().Err(err).Msgf("failed to create tls config: %s", err.Error())
		}

		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	} else if config.Config.KafkaConnectionUseTLS == "true" {
		tlsDialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}}
		opts = append(opts, kgo.Dialer(tlsDialer.DialContext))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	err = client.Ping(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to reach brokers: %s", err.Error())
	}
	log.Info().Msg("Successfully reached the brokers.")
	//initTestData(client)
}

//func initTestData(client *kgo.Client) {
//	admin := kadm.NewClient(client)
//	defer admin.Close()
//
//	//Step 3: Define topic parameters
//	topicName := "steadybit"
//
//	// Step 4: Create the topic using the Admin client
//	ctx := context.Background()
//	result, err := admin.CreateTopics(ctx, -1, -1, nil, topicName)
//	if err != nil {
//		log.Fatal().Err(err).Msgf("Failed to create topic: %v", err)
//	}
//
//	// Step 5: Check the result of the topic creation
//	for _, res := range result {
//		if res.Err != nil {
//			fmt.Printf("Failed to create topic %s: %v\n", res.Topic, res.Err)
//		} else {
//			fmt.Printf("Topic %s created successfully\n", res.Topic)
//		}
//	}
//
//	// Create ACL for dummy client
//	acl := kadm.NewACLs().
//		ResourcePatternType(kadm.ACLPatternLiteral).
//		Topics("*").
//		Groups("dummy").
//		Operations(kadm.OpRead, kadm.OpWrite, kadm.OpDescribe).
//		Allow("User:consumer")
//	_, err = admin.CreateACLs(ctx, acl)
//	if err != nil {
//		return
//	}
//}

func newTLSConfig(clientCertFile, clientKeyFile, caCertFile string) (*tls.Config, error) {
	tlsConfig := tls.Config{}

	// Load client cert
	cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		return &tlsConfig, err
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	// Load CA cert
	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		return &tlsConfig, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig.RootCAs = caCertPool

	return &tlsConfig, err
}
