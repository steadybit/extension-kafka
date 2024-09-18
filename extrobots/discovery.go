/*
 * Copyright 2023 steadybit GmbH. All rights reserved.
 */

package extrobots

import (
	"context"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_commons"
	"github.com/steadybit/discovery-kit/go/discovery_kit_sdk"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/steadybit/extension-scaffold/config"
	"time"
)

type robotDiscovery struct {
}

var (
	_ discovery_kit_sdk.TargetDescriber    = (*robotDiscovery)(nil)
	_ discovery_kit_sdk.AttributeDescriber = (*robotDiscovery)(nil)
)

func NewRobotDiscovery() discovery_kit_sdk.TargetDiscovery {
	discovery := &robotDiscovery{}
	return discovery_kit_sdk.NewCachedTargetDiscovery(discovery,
		discovery_kit_sdk.WithRefreshTargetsNow(),
		discovery_kit_sdk.WithRefreshTargetsInterval(context.Background(), 1*time.Minute),
	)
}

func (d *robotDiscovery) Describe() discovery_kit_api.DiscoveryDescription {
	return discovery_kit_api.DiscoveryDescription{
		Id: TargetType,
		Discover: discovery_kit_api.DescribingEndpointReferenceWithCallInterval{
			CallInterval: extutil.Ptr("1m"),
		},
	}
}

func (d *robotDiscovery) DescribeTarget() discovery_kit_api.TargetDescription {
	return discovery_kit_api.TargetDescription{
		Id:      TargetType,
		Version: extbuild.GetSemverVersionStringOrUnknown(),
		Icon:    extutil.Ptr(targetIcon),

		// Labels used in the UI
		Label: discovery_kit_api.PluralLabel{One: "Robot", Other: "Robots"},

		// Category for the targets to appear in
		Category: extutil.Ptr("example"),

		// Specify attributes shown in table columns and to be used for sorting
		Table: discovery_kit_api.Table{
			Columns: []discovery_kit_api.Column{
				{Attribute: "steadybit.label"},
				{Attribute: "robot.reportedBy"},
			},
			OrderBy: []discovery_kit_api.OrderBy{
				{
					Attribute: "steadybit.label",
					Direction: "ASC",
				},
			},
		},
	}
}

func (d *robotDiscovery) DescribeAttributes() []discovery_kit_api.AttributeDescription {
	return []discovery_kit_api.AttributeDescription{
		{
			Attribute: "robot.reportedBy",
			Label: discovery_kit_api.PluralLabel{
				One:   "Reported by",
				Other: "Reported by",
			},
		},
	}
}

func (d *robotDiscovery) DiscoverTargets(_ context.Context) ([]discovery_kit_api.Target, error) {
	targets := make([]discovery_kit_api.Target, len(config.Config.RobotNames))
	for i, name := range config.Config.RobotNames {
		needsMaintenance := "true"
		if name == "Bender" {
			needsMaintenance = "false"
		}
		targets[i] = discovery_kit_api.Target{
			Id:         name,
			TargetType: TargetType,
			Label:      name,
			Attributes: map[string][]string{
				"steadybit.label":         {name},
				"robot.reportedBy":        {"extension-scaffold"},
				"robot.tags.firstTag":     {"just a tag"},
				"robot.needs.maintenance": {needsMaintenance},
			},
		}
	}
	return discovery_kit_commons.ApplyAttributeExcludes(targets, config.Config.DiscoveryAttributesExcludesRobot), nil
}
