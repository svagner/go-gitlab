package logger

import (
	"bytes"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"gopkg.in/svagner/go-gitlab.v1/config"
)

var (
	debug        bool
	logFile      os.File
	skypeUrl     string
	skypeDst     string
	slackUrl     string
	slackToken   string
	slackChannel string
)

func Init(dbg bool, cfg config.LogConfig) error {
	if cfg.Log != "" {
		logFile, err := os.OpenFile(cfg.Log, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		log.SetOutput(logFile)
	}
	skypeUrl = cfg.SkypeUrl
	skypeDst = cfg.SkypeDistination
	slackUrl = cfg.SlackUrl
	slackToken = cfg.SlackToken
	slackChannel = cfg.SlackChannel
	log.Println(dbg)
	debug = dbg
	log.Println(debug)
	return nil
}

func DebugPrint(text ...interface{}) {
	if debug {
		log.Println("[DEBUG] ", text)
	}
}

func InfoPrint(text ...interface{}) {
	log.Println("[INFO] ", text)
}

func WarningPrint(text ...interface{}) {
	log.Println("[WARNING] ", text)
}

func CriticalPrint(text ...interface{}) {
	log.Fatalln("[Critical] ", text)
}

func Delete() {
	logFile.Close()
}

func Skype(msg string, user string) {
	if skypeUrl == "" {
		return
	}
	var (
		Url    *url.URL
		sendTo string
	)

	if user == "" {
		sendTo = skypeDst
	} else {
		sendTo = user
	}

	Url, err := url.Parse(skypeUrl)
	if err != nil {
		WarningPrint("Skype url parse error: " + err.Error())
		return
	}
	parameters := url.Values{}
	parameters.Add("user", sendTo)
	parameters.Add("msg", msg)
	Url.RawQuery = parameters.Encode()
	timeout := time.Duration(1 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(Url.String())
	DebugPrint("Try to send skype message: " + Url.String())
	if err != nil {
		WarningPrint("Skype url get error: " + err.Error())
		return
	}
	DebugPrint("Send skype message response: ", resp.StatusCode)
	if resp.StatusCode != 200 {
		WarningPrint("Skype get wrong response: ", resp.StatusCode)
	}
}

func Slack(msg, user string) {
	var (
		Url    *url.URL
		sendTo string
	)
	if slackUrl == "" {
		return
	}
	if user == "" {
		sendTo = slackChannel
	} else {
		sendTo = user
	}
	Url, err := url.Parse(slackUrl)
	parameters := url.Values{}
	parameters.Add("token", slackToken)
	parameters.Add("channel", "@"+sendTo)
	Url.RawQuery = parameters.Encode()
	timeout := time.Duration(1 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Post("POST", Url.String(), bytes.NewBufferString(msg))
	DebugPrint("Try to send slack message: " + Url.String())
	if err != nil {
		WarningPrint("Slack url get error: " + err.Error())
		return
	}
	DebugPrint("Send slack message response: ", resp.StatusCode)
	if resp.StatusCode != 200 {
		WarningPrint("Slack get wrong response: ", resp.StatusCode)
	}

}
