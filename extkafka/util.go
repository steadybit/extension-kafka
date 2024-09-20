/*
 * Copyright 2023 steadybit GmbH. All rights reserved.
 */

package extkafka

import (
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"strconv"
	"strings"
)

// resolveStatusCodeExpression resolves the given status code expression into a list of status codes
func resolveStatusCodeExpression(statusCodes string) ([]int, *action_kit_api.ActionKitError) {
	result := make([]int, 0)
	for _, code := range strings.Split(strings.Trim(statusCodes, " "), ";") {
		if strings.Contains(code, "-") {
			rangeParts := strings.Split(code, "-")
			if len(rangeParts) != 2 {
				log.Warn().Msgf("Invalid status code range '%s'", code)
				return nil, &action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Invalid status code range '%s'. Please use '-' for ranges and ';' for enumerations. Example: '200-399;429'", code),
				}
			}
			startCode, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				log.Warn().Msgf("Invalid status code range '%s'", code)
				return nil, &action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Invalid status code range '%s'. Please use '-' for ranges and ';' for enumerations. Example: '200-399;429'", code),
				}
			}
			endCode, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				log.Warn().Msgf("Invalid status code range '%s'", code)
				return nil, &action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Invalid status code range '%s'. Please use '-' for ranges and ';' for enumerations. Example: '200-399;429'", code),
				}
			}
			for i := startCode; i <= endCode; i++ {
				if i < 100 || i > 599 {
					log.Warn().Msgf("Invalid status code '%d'", i)
					return nil, &action_kit_api.ActionKitError{
						Title: fmt.Sprintf("Invalid status code '%d'. Status code should be between 100 and 599.", i),
					}
				}
				result = append(result, i)
			}
		} else {
			if len(code) == 0 {
				log.Error().Msgf("Invalid status code '%s'", code)
				return nil, &action_kit_api.ActionKitError{
					Title: "Status code is required.",
				}
			}
			parsed, err := strconv.Atoi(code)
			if err != nil {
				log.Error().Msgf("Invalid status code '%s'", code)
				return nil, &action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Invalid status code '%s'. Please use '-' for ranges and ';' for enumerations. Example: '200-399;429'", code),
				}
			}
			if parsed < 100 || parsed > 599 {
				log.Error().Msgf("Invalid status code '%d'", parsed)
				return nil, &action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Invalid status code '%d'. Status code should be between 100 and 599.", parsed),
				}
			}
			result = append(result, parsed)
		}
	}
	return result, nil
}
