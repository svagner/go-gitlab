package events

import (
	"github.com/svagner/go-gitlab/convert"
)

func ConnectionListSubscribe(event string, ip string) {
	Events[event].channel <- convert.ConvertToJSON_HTML("New client [" + ip + "] subscribe to event " + event)
}
