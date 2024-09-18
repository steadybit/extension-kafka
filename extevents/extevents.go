// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 Steadybit GmbH

package extevents

import (
	"encoding/json"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/event-kit/go/event_kit_api"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/exthttp"
	"net/http"
)

const eventsBasePath = "/events/all"

func RegisterEventListenerHandlers() {
	exthttp.RegisterHttpHandler(eventsBasePath, onEvent)
}

func GetEventListenerList() event_kit_api.EventListenerList {
	return event_kit_api.EventListenerList{
		EventListeners: []event_kit_api.EventListener{
			{
				Method:   "POST",
				Path:     eventsBasePath,
				ListenTo: []string{"*"},
			},
		},
	}
}

func onEvent(w http.ResponseWriter, r *http.Request, body []byte) {
	var event event_kit_api.EventRequestBody
	err := json.Unmarshal(body, &event)
	if err != nil {
		exthttp.WriteError(w, extension_kit.ToError("Failed to decode event request body", err))
		return
	}

	log.Info().Msgf("Received event %s", event.EventName)

	exthttp.WriteBody(w, event_kit_api.ListenResult{})
}
