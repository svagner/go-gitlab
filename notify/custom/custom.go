package custom

import (
	"net/url"
	"time"
	"gopkg.in/svagner/go-gitlab.v2/config"
	"errors"
	"net/http"
	"github.com/svagner/go-gitlab/logger"
	"strconv"
	"gopkg.in/svagner/go-gitlab.v2/git"
)

const typeNotify = "custom"

type Custom struct {
	url *url.URL
	distanation string
}

func Create(cfg *config.CustomConfig) (c Custom, err error) {
	if cfg.Url == "" {
		err = errors.New("Url for sustom notify wasn't set")
		return
	}
	if cfg.Distanation == "" {
		err = errors.New("Default distination for custom notify wasn't set")
		return
	}
	c.url, err = url.Parse(cfg.Url)
	if err != nil {
		return
	}
	c.distanation = cfg.Distanation
	return
}

func (c Custom) GetType() string {
	return typeNotify
}

func (c Custom) Send(msg string, user... *git.UserInfo) (err error) {
	var (
		sendTo string
		Url    *url.URL
	)
	if c.url == nil {
		return
	}
	if len(user) == 0 {
		sendTo = c.distanation
	} else {
		sendTo = user[0].Skype
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
	logger.Log.Debug("Try to send custom message:", Url.String())
	if err != nil {
		logger.Log.Warning("Custom url get error:", err.Error())
		return
	}
	logger.Log.Debug("Send custom message response:", resp.StatusCode)
	if resp.StatusCode != 200 {
		logger.Log.Warning("Custom notify get wrong response:", resp.StatusCode)
		return errors.New("Costom notify get wrong response: "+strconv.Itoa(resp.StatusCode))
	}
	return
}
