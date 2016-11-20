package slack

import (
	"net/url"
	"time"
	"bytes"
	"errors"
	"net/http"
	"gopkg.in/svagner/go-gitlab.v2/logger"
	"gopkg.in/svagner/go-gitlab.v2/config"
	"strconv"
	"gopkg.in/svagner/go-gitlab.v2/git"
)

const typeNotify  = "slack"

type Slack struct {
	url *url.URL
	channel string
	token string
}

func Create(cfg *config.SlackConfig) (s Slack, err error) {
	if cfg.Url == "" {
		err = errors.New("Url for slack notify wasn't set")
		return
	}
	if cfg.Token == "" {
		err = errors.New("Token for slack notify wasn't set")
		return
	}
	if cfg.Channel == "" {
		err = errors.New("Default channel for slack notify wasn't set")
		return
	}
	s.url, err = url.Parse(cfg.Url)
	if err != nil {
		return
	}
	s.token = cfg.Token
	s.channel = cfg.Channel
	return
}

func (s Slack) GetType() string {
	return typeNotify
}

func (s Slack) Send(msg string, user... *git.UserInfo) error {
	var (
		Url    *url.URL
		sendTo string
	)
	if s.url == nil {
		return errors.New("Url for slack notify wasn't set")
	}
	if len(user) == 0 {
		sendTo = s.channel
	} else {
		sendTo = user[0].Website
	}
	parameters := url.Values{}
	parameters.Add("token", s.token)
	parameters.Add("channel", "@"+sendTo)
	Url.RawQuery = parameters.Encode()
	timeout := time.Duration(1 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Post("POST", Url.String(), bytes.NewBufferString(msg))
	logger.Log.Debug("Try to send slack message: " + Url.String())
	if err != nil {
		logger.Log.Warning("Slack url get error: " + err.Error())
		return err
	}
	logger.Log.Debug("Send slack message response: ", resp.StatusCode)
	if resp.StatusCode != 200 {
		logger.Log.Warning("Slack get wrong response: ", resp.StatusCode)
		return errors.New("Slack get wrong response: "+strconv.Itoa(resp.StatusCode))
	}
	return nil
}
