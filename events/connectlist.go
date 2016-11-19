package events

import (
	"gopkg.in/svagner/go-gitlab.v2/convert"
)

func ConnectionListSubscribe(event string, ip string) {
	Events[event].channel <- convert.ConvertToJSON_HTML("New client [" + ip + "] subscribe to event " + event)
}
