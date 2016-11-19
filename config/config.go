package config

import (
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/gcfg.v1"
)

const (
	defaultContent = `[global]
port = 8189
host = 127.0.0.1
allowFrom = 127.0.0.1
debug = false
log = /var/log/gitlabHooks.log
`
)

type GitConfig struct {
	PublicKey  string
	PrivateKey string
	User       string
	Passphrase string
}

type GitRepository struct {
	Path          string
	Alias 		string
	Branch        string
	Remote        string
	PushRequests  bool
	MergeRequests bool
	Notifications bool
}

type GitLab struct {
	Host   string
	Scheme string
	User   string
	Token  string
}

type LogConfig struct {
	SkypeUrl         string
	SkypeDistination string
	SlackUrl         string
	SlackToken       string
	SlackChannel     string
	File      string
	BufioFile bool
	Syslog    bool
	Console   bool
}

type WebConfig struct {
	Api        string
	Management string
	Templates  string
}

type Config struct {
	Global struct {
		Port      string
		Host      string
		AllowFrom string
		Debug     bool
		PidFile   string
		User      string
	}
	Web        WebConfig
	Logger     LogConfig
	Gitlab     GitLab
	Git        GitConfig
	Repository map[string]*GitRepository
}

func (self *Config) ParseConfig(file string) error {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		log.Printf("Creating default config file %s", file)
		if err = createDefault(file); err != nil {
			log.Fatalln("Couldn't create config file ", file, err.Error())
		}
	}
	if err := gcfg.ReadFileInto(self, file); err != nil {
		return err
	}
	return nil
}

func createDefault(file string) error {
	err := ioutil.WriteFile(file, []byte(defaultContent), 0700)
	return err
}
