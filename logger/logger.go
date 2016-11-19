package logger

import (
	"github.com/op/go-logging"
	"log/syslog"
	"gopkg.in/svagner/go-gitlab.v2/config"
	"os"
)

var (
	Log = logging.MustGetLogger("go-gitlab")

	format = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.8s} %{id:03x}%{color:reset} %{message}`,
	)
	simple_format = logging.MustStringFormatter(
		`%{time:15:04:05.000} %{shortfunc} >> %{level:.8s} %{message}`,
	)
)

func InitLogging(debug bool, conf *config.LogConfig) {
	backends := make([]logging.Backend, 0)
	if conf.Console {
		backend := logging.NewLogBackend(os.Stderr, "", 0)
		//backend2 := logging.NewLogBackend(os.Stderr, "", 0)
		//backend2Formatter := logging.NewBackendFormatter(backend2, format)
		backendFormatter := logging.NewBackendFormatter(backend, format)
		backendLeveled := logging.AddModuleLevel(backendFormatter)
		if debug {
			backendLeveled.SetLevel(logging.DEBUG, "")
		} else {
			backendLeveled.SetLevel(logging.INFO, "")
		}
		//backends = append(backends, backendLeveled)
		backends = append(backends, backendLeveled)
	}
	if conf.File != "" {
		var (
			logFile *os.File
			err     error
		)
		if conf.BufioFile {
			logFile, err = os.OpenFile(conf.File, os.O_WRONLY|os.O_CREATE, 0666)
			if err != nil {
				panic(err)
			}
		} else {
			logFile, err = os.OpenFile(conf.File, os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0666)
			if err != nil {
				panic(err)
			}
		}
		backend := logging.NewLogBackend(logFile, "", 0)
		//backend2 := logging.NewLogBackend(os.Stderr, "", 0)
		//backend2Formatter := logging.NewBackendFormatter(backend2, format)
		backendFormatter := logging.NewBackendFormatter(backend, simple_format)
		backendLeveled := logging.AddModuleLevel(backendFormatter)
		if debug {
			backendLeveled.SetLevel(logging.DEBUG, "")
		} else {
			backendLeveled.SetLevel(logging.INFO, "")
		}
		//backends = append(backends, backendLeveled)
		backends = append(backends, backendLeveled)
	}
	if conf.Syslog {
		backend, err := logging.NewSyslogBackendPriority("[tcp-transport-map]", syslog.LOG_MAIL)
		if err != nil {
			panic(err)
		}
		backendFormatter := logging.NewBackendFormatter(backend, simple_format)
		backendLeveled := logging.AddModuleLevel(backendFormatter)
		if debug {
			backendLeveled.SetLevel(logging.DEBUG, "")
		} else {
			backendLeveled.SetLevel(logging.INFO, "")
		}
		//backends = append(backends, backendLeveled)
		backends = append(backends, backendLeveled)
	}

	//logging.SetBackend(backend1Leveled, backend2Formatter)
	logging.SetBackend(backends...)
}
