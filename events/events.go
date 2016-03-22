package events

import (
	"errors"

	"github.com/svagner/go-gitlab/convert"
	"github.com/svagner/go-gitlab/git"
)

type clientChan struct {
	c  chan string
	ip string
}

type chanList []clientChan

func (self chanList) Remove(clchan clientChan) (res chanList) {
	for _, cn := range self {
		if cn != clchan {
			res = append(res, cn)
		}
	}
	return res
}

type Event struct {
	genEvent    func(string, string)
	channel     chan string
	subscribers chanList
}

type ResCmd struct {
	Channel string
	Command string
	Data    interface{}
}

var Events = make(map[string]*Event)

func (self *Event) Notifier() {
	for {
		select {
		case data := <-self.channel:
			for _, cc := range self.subscribers {
				cc.c <- data
			}
		}
	}
}

func (ev *Event) SendToChannel(channel string, command string, data string) {
	res := ResCmd{Channel: channel, Command: command, Data: data}
	ev.channel <- convert.ConvertToJSON_HTML(res)
}

func (self *Event) AddUser(out chan string, ip string) {
	self.subscribers = append(self.subscribers, clientChan{c: out, ip: ip})
}

func Init() {
	Events["blocker"] = &Event{ConnectionListSubscribe, make(chan string), make(chanList, 0)}
	go Events["blocker"].Notifier()
	Events["pushqueue"] = &Event{ConnectionListSubscribe, make(chan string), make(chanList, 0)}
	go Events["pushqueue"].Notifier()
	Events["addcommit"] = &Event{ConnectionListSubscribe, make(chan string), make(chanList, 0)}
	go Events["addcommit"].Notifier()
	Events["error"] = &Event{ConnectionListSubscribe, make(chan string), make(chanList, 0)}
	go Events["error"].Notifier()
}

func Unsubscribe(event string, out chan string, ip string) error {
	Events[event].subscribers = Events[event].subscribers.Remove(clientChan{c: out, ip: ip})
	return nil
}

func Subscribe(event string, out chan string, ip string) error {
	if _, ok := Events[event]; !ok {
		return errors.New("Channel wasn't found")
	}
	Events[event].AddUser(out, ip)
	if Events[event].genEvent != nil {
		Events[event].genEvent(event, ip)
	}
	return nil
}

func Lock(data string, co chan string, ip string) error {
	if _, ok := git.Repositories[git.GitUrl2Orig(data)]; !ok {
		return errors.New("Repository " + data + " wasn't found")
	}
	git.Repositories[git.GitUrl2Orig(data)].Lock = true
	res := ResCmd{Channel: "blocker", Command: "lock", Data: data}
	Events["blocker"].channel <- convert.ConvertToJSON_HTML(res)
	return nil
}

func UnLock(data string, co chan string, ip string) error {
	if _, ok := git.Repositories[git.GitUrl2Orig(data)]; !ok {
		return errors.New("Repository " + data + " wasn't found")
	}
	git.Repositories[git.GitUrl2Orig(data)].Lock = false

	if len(git.Repositories[git.GitUrl2Orig(data)].History) > 0 {
		var urls string
		for _, rep := range git.Repositories[git.GitUrl2Orig(data)].History {
			urls = urls + " " + rep.Url
		}
		git.Repositories[git.GitUrl2Orig(data)].Update <- urls
		git.Repositories[git.GitUrl2Orig(data)].History = make([]git.UpdateHistory, 0)
		res := ResCmd{Channel: "pushqueue", Command: "clean", Data: data}
		Events["pushqueue"].channel <- convert.ConvertToJSON_HTML(res)
	}

	res := ResCmd{Channel: "blocker", Command: "unlock", Data: data}
	Events["blocker"].channel <- convert.ConvertToJSON_HTML(res)
	return nil
}
