package extkafka

import (
	"context"
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"testing"
)

func TestBrokerUserDeny_Start(t *testing.T) {
	// Set Sasl config
	config.Config.SaslUser = "consumer"
	config.Config.SaslPassword = "password"
	config.Config.SaslMechanism = "PLAIN"

	c, err := kfake.NewCluster(
		kfake.Ports(9092, 9093, 9094),
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(3),
		kfake.EnableSASL(),
		kfake.Superuser("PLAIN", "consumer", "password"),
	)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	seeds := []string{"localhost:9092"}
	// One client can both produce and consume!
	// Consuming can either be direct (no consumer group), or through a group. Below, we use a group.
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.ConsumerGroup("steadybit"),
		kgo.SASL(plain.Auth{
			User: "consumer",
			Pass: "password",
		}.AsMechanism()),
		kgo.DefaultProduceTopic("steadybit"),
		kgo.ConsumeTopics("steadybit"),
	)
	if err != nil {
		panic(err)
	}
	defer cl.Close()

	clAdm := kadm.NewClient(cl)

	// produce messages for lags
	for i := 0; i < 10; i++ {
		cl.ProduceSync(context.TODO(), &kgo.Record{Key: []byte("steadybit"), Value: []byte("test")})
	}

	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *KafkaDenyUserState
	}{
		{
			name: "Should return status ok",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.consumer-group.name": {"steadybit"},
					},
				},
				Config: map[string]interface{}{
					"duration": 5000,
					"topic":    "steadybit",
					"user":     "consumer",
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &KafkaDenyUserState{
				Topic:         "steadybit",
				ConsumerGroup: "steadybit",
				User:          "consumer",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := KafkaConsumerDenyAccessAttack{}
			state := KafkaDenyUserState{}
			request := tt.requestBody
			//When
			_, errPrepare := action.Prepare(context.TODO(), &state, request)
			_, errStatus := action.Start(context.TODO(), &state)
			acl := kadm.NewACLs().
				ResourcePatternType(kadm.ACLPatternLiteral).
				Topics(state.Topic).
				Groups(state.ConsumerGroup).
				Operations(kadm.OpRead, kadm.OpWrite, kadm.OpDescribe).
				Deny(state.User).DenyHosts()

			result, err := clAdm.DescribeACLs(context.TODO(), acl)
			assert.NotNil(t, result)
			assert.Nil(t, err)

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.User, state.User)
				assert.Equal(t, tt.wantedState.Topic, state.Topic)
				assert.Equal(t, tt.wantedState.ConsumerGroup, state.ConsumerGroup)
				//assert.Nil(t, statusResult.Error)
			}

		})
	}
}
