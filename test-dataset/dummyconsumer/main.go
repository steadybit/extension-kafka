package main

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"os"
	"strings"
)

func main() {
	// Configure
	seeds, _ := os.LookupEnv("STEADYBIT_DUMMY_SEED_BROKERS")
	if seeds == "" {
		seeds = "kafka-demo.steadybit-demo.svc.cluster.local:9092"
	}

	saslUser, _ := os.LookupEnv("STEADYBIT_DUMMY_SASL_USER")
	if saslUser == "" {
		saslUser = "user1"
	}

	saslPassword, _ := os.LookupEnv("STEADYBIT_DUMMY_SASL_PASSWORD")
	if saslPassword == "" {
		saslPassword = "steadybit"
	}

	topic, _ := os.LookupEnv("STEADYBIT_DUMMY_TOPIC")
	if topic == "" {
		topic = "steadybit-demo"
	}

	consumer, _ := os.LookupEnv("STEADYBIT_DUMMY_CONSUMER_NAME")
	if consumer == "" {
		consumer = "steadybit-demo-consumer"
	}

	// One client can both produce and consume!
	// Consuming can either be direct (no consumer group), or through a group. Below, we use a group.
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(seeds, ",")...),
		kgo.ConsumerGroup(consumer),
		kgo.ConsumeTopics(topic),
		kgo.SASL(plain.Auth{
			User: saslUser,
			Pass: saslPassword,
		}.AsMechanism()),
	)
	log.Info().Msgf("Initiating consumer with kafka config: brokers %s, consumer name %s on topic %s with user %s", seeds, consumer, topic, saslUser)
	if err != nil {
		panic(err)
	}
	defer cl.Close()

	ctx := context.Background()

	// 2.) Consuming messages from a topic
	for {
		fetches := cl.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			// All errors are retried internally when fetching, but non-retriable errors are
			// returned from polls so that users can notice and take action.
			log.Info().Msg(fmt.Sprint(errs))
		}

		// We can iterate through a record iterator...
		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()
			fmt.Printf("%s from an iterator! Offset: %d\n", record.Value, record.Offset)
		}

	}

}
