/*
 * Copyright 2023 steadybit GmbH. All rights reserved.
 */

package main

import (
	"context"
	"fmt"
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
	"github.com/steadybit/extension-kafka/extadvice/robot_maintenance"
	"github.com/steadybit/extension-kafka/extevents"
	"github.com/steadybit/extension-kafka/extkafka"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/exthealth"
	"github.com/steadybit/extension-kit/exthttp"
	"github.com/steadybit/extension-kit/extlogging"
	"github.com/steadybit/extension-kit/extruntime"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	_ "go.uber.org/automaxprocs" // Importing automaxprocs automatically adjusts GOMAXPROCS.
	_ "net/http/pprof"           //allow pprof
	"os"
	"os/signal"
	"syscall"
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
	initKafkaClient()

	//This will start /health/liveness and /health/readiness endpoints on port 8081 for use with kubernetes
	//The port can be configured using the STEADYBIT_EXTENSION_HEALTH_PORT environment variable
	exthealth.SetReady(false)
	exthealth.StartProbes(8081)

	ctx, cancel := SignalCanceledContext()

	registerHandlers(ctx)

	action_kit_sdk.InstallSignalHandler(func(o os.Signal) {
		cancel()
	})

	//This will register the coverage endpoints for the extension (used by action_kit_test)
	action_kit_sdk.RegisterCoverageEndpoints()

	//This will switch the readiness state of the application to true.
	exthealth.SetReady(true)

	exthttp.Listen(exthttp.ListenOpts{
		// This is the default port under which your extension is accessible.
		// The port can be configured externally through the
		// STEADYBIT_EXTENSION_PORT environment variable.
		// We suggest that you keep port 8080 as the default.
		Port: 8080,
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
	action_kit_sdk.RegisterAction(extkafka.NewKafkaBrokerElectNewLeaderAttack())
	action_kit_sdk.RegisterAction(extkafka.NewConsumerGroupCheckAction())

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

		// See this document to learn more about the event listener list:
		// https://github.com/steadybit/event-kit/blob/main/docs/event-api.md#event-listeners-list
		EventListenerList: extevents.GetEventListenerList(),

		// See this document to learn more about the advice list:
		// https://github.com/steadybit/advice-kit/blob/main/docs/advice-api.md#index-response
		AdviceList: advice_kit_api.AdviceList{
			Advice: getAdviceRefs(),
		},
	}
}

func getAdviceRefs() []advice_kit_api.DescribingEndpointReference {
	var refs []advice_kit_api.DescribingEndpointReference
	refs = make([]advice_kit_api.DescribingEndpointReference, 0)
	for _, adviceId := range config.Config.ActiveAdviceList {
		// Maintenance advice
		if adviceId == "*" || adviceId == robot_maintenance.RobotMaintenanceID {
			refs = append(refs, advice_kit_api.DescribingEndpointReference{
				Method: "GET",
				Path:   "/advice/robot-maintenance",
			})
		}
	}
	return refs
}

func initKafkaClient() {
	opts := []kgo.Opt{
		kgo.SeedBrokers(config.Config.SeedBrokers),
		kgo.DefaultProduceTopic("steadybit"),
		kgo.ClientID("steadybit"),
	}

	var err error
	extkafka.KafkaClient, err = kgo.NewClient(opts...)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to initialize kafka client: %s", err.Error())
	}
	defer extkafka.KafkaClient.Close()

	err = extkafka.KafkaClient.Ping(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to reach brokers: %s", err.Error())
	}

	// Create a test topic
	// Step 2: Create Admin Client for Topic Management
	admin := kadm.NewClient(extkafka.KafkaClient)
	defer admin.Close()

	// Step 3: Define topic parameters
	topicName := "steadybit"

	// Step 4: Create the topic using the Admin client
	ctx := context.Background()
	result, err := admin.CreateTopics(ctx, -1, -1, nil, topicName)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to create topic: %v", err)
	}

	// Step 5: Check the result of the topic creation
	for _, res := range result {
		if res.Err != nil {
			fmt.Printf("Failed to create topic %s: %v\n", res.Topic, res.Err)
		} else {
			fmt.Printf("Topic %s created successfully\n", res.Topic)
		}
	}

	//// Generate a large payload (e.g., 1 MB)
	//payload := make([]byte, 0, 1*1024*1024)
	//_, err = rand.Read(payload)
	//if err != nil {
	//	log.Fatal().Err(err).Msgf("Failed to produce records: %s", err.Error())
	//}
	//
	//record := &kgo.Record{
	//	Topic: "steadybit",
	//	Value: payload,
	//}
	//results := extkafka.KafkaClient.ProduceSync(context.TODO(), record)
	//if results.FirstErr() != nil {
	//	log.Fatal().Err(err).Msgf("Failed to produce records: %s", results.FirstErr())
	//}
}
