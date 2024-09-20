package extkafka

import (
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kgo"
	"net/url"
	"time"
)

const (
	kafkaBrokerTargetId  = "com.steadybit.extension_kafka.broker"
	TargetIDPeriodically = "com.steadybit.extension_http.check.periodically"
	TargetIDFixedAmount  = "com.steadybit.extension_http.check.fixed_amount"
)

const (
	kafkaIcon                = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZlcnNpb249IjEuMSIgdmlld0JveD0iMCAwIDI0IDI0Ij4KICA8cGF0aAogICAgZD0iTTE1LjksMTMuMmMtLjksMC0xLjYuNC0yLjIsMWwtMS4zLTFjLjEtLjQuMi0uOC4yLTEuM3MwLS45LS4yLTEuMmwxLjMtLjljLjUuNiwxLjMsMSwyLjEsMSwxLjYsMCwyLjktMS4zLDIuOS0yLjlzLTEuMy0yLjktMi45LTIuOS0yLjksMS4zLTIuOSwyLjksMCwuNi4xLjhsLTEuMy45Yy0uNi0uNy0xLjQtMS4yLTIuMy0xLjN2LTEuNmMxLjMtLjMsMi4zLTEuNCwyLjMtMi44LDAtMS42LTEuMy0yLjktMi45LTIuOXMtMi45LDEuMy0yLjksMi45LDEsMi41LDIuMiwyLjh2MS42Yy0xLjcuMy0zLjEsMS44LTMuMSwzLjZzMS4zLDMuNCwzLjEsMy42djEuN2MtMS4zLjMtMi4zLDEuNC0yLjMsMi44czEuMywyLjksMi45LDIuOSwyLjktMS4zLDIuOS0yLjktMS0yLjUtMi4zLTIuOHYtMS43Yy45LS4xLDEuNy0uNiwyLjMtMS4zbDEuNCwxYzAsLjMtLjEuNS0uMS44LDAsMS42LDEuMywyLjksMi45LDIuOXMyLjktMS4zLDIuOS0yLjktMS4zLTIuOS0yLjktMi45aDBaTTE1LjksNi41Yy44LDAsMS40LjYsMS40LDEuNHMtLjYsMS40LTEuNCwxLjQtMS40LS42LTEuNC0xLjQuNi0xLjQsMS40LTEuNGgwWk03LjUsMy45YzAtLjguNi0xLjQsMS40LTEuNHMxLjQuNiwxLjQsMS40LS42LDEuNC0xLjQsMS40LTEuNC0uNi0xLjQtMS40aDBaTTEwLjMsMjAuMWMwLC44LS42LDEuNC0xLjQsMS40cy0xLjQtLjYtMS40LTEuNC42LTEuNCwxLjQtMS40LDEuNC42LDEuNCwxLjRaTTguOSwxMy45Yy0xLjEsMC0xLjktLjktMS45LTEuOXMuOS0xLjksMS45LTEuOSwxLjkuOSwxLjksMS45LS45LDEuOS0xLjksMS45Wk0xNS45LDE3LjRjLS44LDAtMS40LS42LTEuNC0xLjRzLjYtMS40LDEuNC0xLjQsMS40LjYsMS40LDEuNC0uNiwxLjQtMS40LDEuNFoiCiAgICBmaWxsPSJjdXJyZW50Q29sb3IiIC8+Cjwvc3ZnPg=="
	kafkaMessagePeriodically = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZlcnNpb249IjEuMSIgdmlld0JveD0iMCAwIDI0IDI0Ij4KICA8cGF0aAogICAgZD0iTTE1LjksMTMuMmMtLjksMC0xLjYuNC0yLjIsMWwtMS4zLTFjLjEtLjQuMi0uOC4yLTEuM3MwLS45LS4yLTEuMmwxLjMtLjljLjUuNiwxLjMsMSwyLjEsMSwxLjYsMCwyLjktMS4zLDIuOS0yLjlzLTEuMy0yLjktMi45LTIuOS0yLjksMS4zLTIuOSwyLjksMCwuNi4xLjhsLTEuMy45Yy0uNi0uNy0xLjQtMS4yLTIuMy0xLjN2LTEuNmMxLjMtLjMsMi4zLTEuNCwyLjMtMi44LDAtMS42LTEuMy0yLjktMi45LTIuOXMtMi45LDEuMy0yLjksMi45LDEsMi41LDIuMiwyLjh2MS42Yy0xLjcuMy0zLjEsMS44LTMuMSwzLjZzMS4zLDMuNCwzLjEsMy42djEuN2MtMS4zLjMtMi4zLDEuNC0yLjMsMi44czEuMywyLjksMi45LDIuOSwyLjktMS4zLDIuOS0yLjktMS0yLjUtMi4zLTIuOHYtMS43Yy45LS4xLDEuNy0uNiwyLjMtMS4zbDEuNCwxYzAsLjMtLjEuNS0uMS44LDAsMS42LDEuMywyLjksMi45LDIuOXMyLjktMS4zLDIuOS0yLjktMS4zLTIuOS0yLjktMi45aDBaTTE1LjksNi41Yy44LDAsMS40LjYsMS40LDEuNHMtLjYsMS40LTEuNCwxLjQtMS40LS42LTEuNC0xLjQuNi0xLjQsMS40LTEuNGgwWk03LjUsMy45YzAtLjguNi0xLjQsMS40LTEuNHMxLjQuNiwxLjQsMS40LS42LDEuNC0xLjQsMS40LTEuNC0uNi0xLjQtMS40aDBaTTEwLjMsMjAuMWMwLC44LS42LDEuNC0xLjQsMS40cy0xLjQtLjYtMS40LTEuNC42LTEuNCwxLjQtMS40LDEuNC42LDEuNCwxLjRaTTguOSwxMy45Yy0xLjEsMC0xLjktLjktMS45LTEuOXMuOS0xLjksMS45LTEuOSwxLjkuOSwxLjksMS45LS45LDEuOS0xLjksMS45Wk0xNS45LDE3LjRjLS44LDAtMS40LS42LTEuNC0xLjRzLjYtMS40LDEuNC0xLjQsMS40LjYsMS40LDEuNC0uNiwxLjQtMS40LDEuNFoiCiAgICBmaWxsPSJjdXJyZW50Q29sb3IiIC8+Cjwvc3ZnPg=="
	kafkaMessageFixedAmount  = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZlcnNpb249IjEuMSIgdmlld0JveD0iMCAwIDI0IDI0Ij4KICA8cGF0aCBmaWxsPSJjdXJyZW50Q29sb3IiCiAgICBkPSJNMTguNTYsMTQuMjZjLTIuNSwwLTQuNiwyLjEtNC42LDQuNnMyLjEsNC42LDQuNiw0LjYsNC42LTIuMSw0LjYtNC42LTIuMS00LjYtNC42LTQuNlpNMTguNTYsMjIuMjZjLTEuOSwwLTMuNC0xLjUtMy40LTMuNHMxLjUtMy40LDMuNC0zLjQsMy40LDEuNSwzLjQsMy40LTEuNSwzLjQtMy40LDMuNFpNMjAuNDYsMTYuNjZoLS44bC0uMiwxLjJoLTEuMWwuMi0xLjJoLS44bC0uMiwxLjJoLS45di43aC43bC0uMi45aC0uOXYuOGguN2wtLjIsMS4yaC44bC4yLTEuMmgxLjFsLS4yLDEuMmguOHYtLjFsLjItMS4yaC45di0uN2gtLjdsLjItLjloLjl2LS43aC0uN2wuMi0xLjJaTTE5LjA2LDE5LjM2aC0xLjFsLjItLjloMS4xbC0uMi45Wk0xNS4zNiwxNS4wNmMtLjUxLTEuMDUtMS41Ny0xLjc3LTIuODEtMS43Ny0uOTQsMC0xLjc3LjQyLTIuMzQsMS4wOGwtMS42Mi0xLjAzYy4xNy0uNDQuMjctLjkyLjI3LTEuNDMsMC0uNDQtLjA5LS44Ni0uMjItMS4yNWwxLjU1LTEuMDFjLjU3LjY2LDEuNDEsMS4wOSwyLjM1LDEuMDksMS43MywwLDMuMTMtMS40LDMuMTMtMy4xM3MtMS40LTMuMTMtMy4xMy0zLjEzLTMuMTMsMS40LTMuMTMsMy4xM2MwLC4zMy4wNy42NC4xNi45NGwtMS41MS45OGMtLjYtLjgyLTEuNS0xLjM5LTIuNTQtMS41N3YtMS43OGMxLjQxLS4zLDIuNDgtMS41NSwyLjQ4LTMuMDYsMC0xLjczLTEuNC0zLjEzLTMuMTMtMy4xM1MxLjc1LDEuNCwxLjc1LDMuMTNjMCwxLjUxLDEuMDcsMi43NiwyLjQ4LDMuMDZ2MS43OGMtMS45Mi4zLTMuMzksMS45NS0zLjM5LDMuOTVzMS40NywzLjY1LDMuMzksMy45NXYxLjk1Yy0xLjQyLjI5LTIuNSwxLjU1LTIuNSwzLjA2LDAsMS43MywxLjQsMy4xMywzLjEzLDMuMTNzMy4xMy0xLjQsMy4xMy0zLjEzYzAtMS41LTEuMDYtMi43NS0yLjQ2LTMuMDV2LTEuOTZjLjk1LS4xNiwxLjc3LS42NiwyLjM3LTEuMzZsMS42NywxLjA2Yy0uMDguMjgtLjE0LjU2LS4xNC44NiwwLDEuNzMsMS40LDMuMTMsMy4xMywzLjEzLjM3LDAsLjcxLS4wOCwxLjA0LS4xOS0uMDEtLjE1LS4wNS0uMy0uMDUtLjQ2LDAtMS41NS43MS0yLjkxLDEuODEtMy44M1pNMTIuNDcsNmMuODIsMCwxLjQ5LjY3LDEuNDksMS40OXMtLjY3LDEuNDktMS40OSwxLjQ5LTEuNDktLjY3LTEuNDktMS40OS42Ny0xLjQ5LDEuNDktMS40OVpNMy4yOCwzLjFjMC0uODIuNjctMS40OSwxLjQ5LTEuNDlzMS40OS42NywxLjQ5LDEuNDktLjY3LDEuNDktMS40OSwxLjQ5LTEuNDktLjY3LTEuNDktMS40OVpNNi4zMywyMC44OGMwLC44Mi0uNjcsMS40OS0xLjQ5LDEuNDlzLTEuNDktLjY3LTEuNDktMS40OS42Ny0xLjQ5LDEuNDktMS40OSwxLjQ5LjY3LDEuNDksMS40OVpNNC44NSwxNC4xNmMtMS4xOCwwLTIuMTMtLjk1LTIuMTMtMi4xM3MuOTUtMi4xMywyLjEzLTIuMTMsMi4xMy45NSwyLjEzLDIuMTMtLjk1LDIuMTMtMi4xMywyLjEzWk0xMi40NSwxNy44OGMtLjgyLDAtMS40OS0uNjctMS40OS0xLjQ5cy42Ny0xLjQ5LDEuNDktMS40OSwxLjQ5LjY3LDEuNDksMS40OS0uNjcsMS40OS0xLjQ5LDEuNDlaIiAvPgo8L3N2Zz4="
)

type KafkaBrokerAttackState struct {
	NodeID                   string
	Topic                    string
	Interval                 time.Duration
	ExpectedErrorCodes       []int
	DelayBetweenRequestsInMS int64
	Timeout                  time.Time
	ResponsesContains        string
	SuccessRate              int
	ResponseTimeMode         string
	ResponseTime             *time.Duration
	MaxConcurrent            int
	NumberOfRequests         uint64
	RequestSizeMb            uint64
	ReadTimeout              time.Duration
	ExecutionID              uuid.UUID
	Body                     string
	URL                      url.URL
	Method                   string
	Headers                  map[string]string
	ConnectionTimeout        time.Duration
	FollowRedirects          bool
}

var KafkaClient *kgo.Client

var (
	requestDefinition = action_kit_api.ActionParameter{
		Name:  "requestDefinition",
		Label: "Request Definition",
		Type:  action_kit_api.Header,
		Order: extutil.Ptr(0),
	}
	method = action_kit_api.ActionParameter{
		Name:         "method",
		Label:        "HTTP Method",
		Description:  extutil.Ptr("The HTTP method to use."),
		Type:         action_kit_api.String,
		DefaultValue: extutil.Ptr("GET"),
		Required:     extutil.Ptr(true),
		Order:        extutil.Ptr(1),
		Options: extutil.Ptr([]action_kit_api.ParameterOption{
			action_kit_api.ExplicitParameterOption{
				Label: "GET",
				Value: "GET",
			},
			action_kit_api.ExplicitParameterOption{
				Label: "POST",
				Value: "POST",
			},
			action_kit_api.ExplicitParameterOption{
				Label: "PUT",
				Value: "PUT",
			},
			action_kit_api.ExplicitParameterOption{
				Label: "PATCH",
				Value: "PATCH",
			},
			action_kit_api.ExplicitParameterOption{
				Label: "HEAD",
				Value: "HEAD",
			},
			action_kit_api.ExplicitParameterOption{
				Label: "DELETE",
				Value: "DELETE",
			},
		}),
	}
	urlParameter = action_kit_api.ActionParameter{
		Name:        "url",
		Label:       "Target URL",
		Description: extutil.Ptr("The URL to check."),
		Type:        action_kit_api.Url,
		Required:    extutil.Ptr(true),
		Order:       extutil.Ptr(2),
	}
	body = action_kit_api.ActionParameter{
		Name:        "body",
		Label:       "HTTP Body",
		Description: extutil.Ptr("The HTTP Body."),
		Type:        action_kit_api.Textarea,
		Order:       extutil.Ptr(3),
	}
	headers = action_kit_api.ActionParameter{
		Name:        "headers",
		Label:       "HTTP Headers",
		Description: extutil.Ptr("The HTTP Headers."),
		Type:        action_kit_api.KeyValue,
		Order:       extutil.Ptr(4),
	}
	repetitionControl = action_kit_api.ActionParameter{
		Name:  "repetitionControl",
		Label: "Repetition Control",
		Type:  action_kit_api.Header,
		Order: extutil.Ptr(6),
	}
	duration = action_kit_api.ActionParameter{
		Name:         "duration",
		Label:        "Duration",
		Description:  extutil.Ptr("In which timeframe should the specified requests be executed?"),
		Type:         action_kit_api.Duration,
		DefaultValue: extutil.Ptr("10s"),
		Required:     extutil.Ptr(true),
		Order:        extutil.Ptr(8),
	}
	resultVerification = action_kit_api.ActionParameter{
		Name:  "resultVerification",
		Label: "Result Verification",
		Type:  action_kit_api.Header,
		Order: extutil.Ptr(10),
	}
	successRate = action_kit_api.ActionParameter{
		Name:         "successRate",
		Label:        "Required Success Rate",
		Description:  extutil.Ptr("How many percent of the Request must be at least successful (in terms of the following response verifications) to continue the experiment execution? The result will be evaluated and the end of the given duration."),
		Type:         action_kit_api.Percentage,
		DefaultValue: extutil.Ptr("100"),
		Required:     extutil.Ptr(true),
		Order:        extutil.Ptr(11),
		MinValue:     extutil.Ptr(0),
		MaxValue:     extutil.Ptr(100),
	}
	statusCode = action_kit_api.ActionParameter{
		Name:         "statusCode",
		Label:        "Response status codes",
		Description:  extutil.Ptr("Which HTTP-Status code should be considered as success? This field supports ranges with '-' and multiple codes delimited by ';' for example '200-399;429'."),
		Type:         action_kit_api.String,
		DefaultValue: extutil.Ptr("200-299"),
		Required:     extutil.Ptr(true),
		Order:        extutil.Ptr(12),
	}
	responsesContains = action_kit_api.ActionParameter{
		Name:        "responsesContains",
		Label:       "Responses contains",
		Description: extutil.Ptr("The Responses needs to contain the given string, otherwise the experiment will fail. The responses will be evaluated and the end of the given duration."),
		Type:        action_kit_api.String,
		Required:    extutil.Ptr(false),
		Order:       extutil.Ptr(13),
	}
	responsesTimeMode = action_kit_api.ActionParameter{
		Name:         "responseTimeMode",
		Label:        "Response Time Verification Mode",
		Description:  extutil.Ptr("Should the Response Time be shorter or longer than the given duration?"),
		Type:         action_kit_api.String,
		Required:     extutil.Ptr(false),
		Order:        extutil.Ptr(14),
		DefaultValue: extutil.Ptr("NO_VERIFICATION"),
		Options: extutil.Ptr([]action_kit_api.ParameterOption{
			action_kit_api.ExplicitParameterOption{
				Label: "no verification",
				Value: "NO_VERIFICATION",
			},
			action_kit_api.ExplicitParameterOption{
				Label: "shorter than",
				Value: "SHORTER_THAN",
			},
			action_kit_api.ExplicitParameterOption{
				Label: "longer than",
				Value: "LONGER_THAN",
			},
		}),
	}
	responseTime = action_kit_api.ActionParameter{
		Name:         "responseTime",
		Label:        "Response Time",
		Description:  extutil.Ptr("The value for the response time verification."),
		Type:         action_kit_api.Duration,
		Required:     extutil.Ptr(false),
		Order:        extutil.Ptr(15),
		DefaultValue: extutil.Ptr("500ms"),
	}
	maxConcurrent = action_kit_api.ActionParameter{
		Name:         "maxConcurrent",
		Label:        "Max concurrent requests",
		Description:  extutil.Ptr("Maximum count on parallel running requests. (min 1, max 10)"),
		Type:         action_kit_api.Integer,
		DefaultValue: extutil.Ptr("5"),
		Required:     extutil.Ptr(true),
		Advanced:     extutil.Ptr(true),
		Order:        extutil.Ptr(16),
	}
	clientSettings = action_kit_api.ActionParameter{
		Name:     "clientSettings",
		Label:    "HTTP Client Settings",
		Type:     action_kit_api.Header,
		Advanced: extutil.Ptr(true),
		Order:    extutil.Ptr(17),
	}
	followRedirects = action_kit_api.ActionParameter{
		Name:        "followRedirects",
		Label:       "Follow Redirects?",
		Description: extutil.Ptr("Should Redirects be followed?"),
		Type:        action_kit_api.Boolean,
		Required:    extutil.Ptr(true),
		Advanced:    extutil.Ptr(true),
		Order:       extutil.Ptr(18),
	}
	connectTimeout = action_kit_api.ActionParameter{
		Name:         "connectTimeout",
		Label:        "Connection Timeout",
		Description:  extutil.Ptr("Connection Timeout for a single Call in seconds. Should be between 1 and 10 seconds."),
		Type:         action_kit_api.Duration,
		DefaultValue: extutil.Ptr("5s"),
		Required:     extutil.Ptr(true),
		Advanced:     extutil.Ptr(true),
		Order:        extutil.Ptr(19),
	}
	readTimeout = action_kit_api.ActionParameter{
		Name:         "readTimeout",
		Label:        "Read Timeout",
		Description:  extutil.Ptr("Read Timeout for a single Call in seconds. Should be between 1 and 10 seconds."),
		Type:         action_kit_api.Duration,
		DefaultValue: extutil.Ptr("5s"),
		Required:     extutil.Ptr(true),
		Advanced:     extutil.Ptr(true),
		Order:        extutil.Ptr(20),
	}
)
