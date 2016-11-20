package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/svagner/go-gitlab/config"
	"github.com/svagner/go-gitlab/events"
	"github.com/svagner/go-gitlab/git"
	"github.com/svagner/go-gitlab/logger"
	"github.com/svagner/go-gitlab/wsclient"
)

type Record struct {
	Kind         string     `json:"object_kind"`
	User         User       `json:"user"`
	Object       ObjectAttr `json:"object_attributes"`
	Action       string     `json:"action"`
	CommitBefore string     `json:"before"`
	CommitAfter  string     `json:"after"`
	GitRef       string     `json:"ref"`
	GitCoSHA     string     `json:"checkout_sha"`
	UserID       int        `json:"user_id"`
	UserName     string     `json:"user_name"`
	UserEmail    string     `json:"user_email"`
	ProjectID    int        `json:"project_id"`
	Repository   Repository `json:"repository"`
	//Commits      []Commits  `json:"commits"`
	TotalCommits int `json:"total_commits_count"`
}

type User struct {
	Name     string `json:"name"`
	UserName string `json:"username"`
	Avatar   string `json:"avatar_url"`
}

type ObjectAttr struct {
	Id              int                   `json:"id"`
	TargetBranch    string                `json:"target_branch"`
	SourceBranch    string                `json:"source_branch"`
	SourceProjectId int                   `json:"source_project_id"`
	AuthorId        int                   `json:"author_id"`
	AssigneeId      int                   `json:"assignee_id"`
	Title           string                `json:"title"`
	CreatedAt       string                `json:"created_at"`
	UpdatedAt       string                `json:"updated_at"`
	State           string                `json:"state"`
	MergeStatus     string                `json:"merge_status"`
	TargetProgectId int                   `json:"target_project_id"`
	Iid             int                   `json:"iid"`
	Description     string                `json:"description"`
	Position        int                   `json:"position"`
	Source          RepositoryDescription `json:"source"`
	Target          RepositoryDescription `json:"target"`
	LastCommit      Commits               `json:"last_commit"`
	Url             string                `json:"url"`
	Action          string                `json:"action"`
}

type RepositoryDescription struct {
	Name      string `json:"name"`
	Url       string `json:"url"`
	Namespace string `json:"namespace"`
	HttpUrl   string `json:"http_url"`
	SshUrl    string `json:"ssh_url"`
}

type Repository struct {
	Name            string `json:"name"`
	Url             string `json:"url"`
	Description     string `json:"description"`
	Homepage        string `json:"homepage"`
	HttpUrl         string `json:"git_http_url"`
	SshUrl          string `json:"git_ssh_url"`
	VisibilityLevel int    `json:"visibility_level"`
}

type Commits struct {
	Id        string `json:"id"`
	Message   string `json:"message"`
	TimeStamp string `json:"timestamp"`
	Url       string `json:"url"`
	Author    Author `json:"author"`
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

var (
	configFile = flag.String("config", "/etc/githooks.conf", "config file")
	sig        = flag.String("s", "", "send signal")
	logFile    = flag.String("log", "/var/log/githooks.log", "Log file for logger system")
	pidFile    = flag.String("pid", "/var/run/githooks.pid", "Pid file for save pid number")
	templates  *template.Template
)

type AdminPageData struct {
	Repos  map[string]*git.Repository
	Config config.Config
	Title  string
}

func gitHooks_process(w http.ResponseWriter, r *http.Request, cfg config.Config) {
	p := make([]byte, r.ContentLength)
	_, err := r.Body.Read(p)
	if err != nil && err != io.EOF {
		logger.Log.Warning(err)
	}
	logger.Log.Debug("Get new value:", string(p))
	result, err := decode(bytes.NewReader(p))
	if err != nil {
		logger.Log.Warning("Error decode hook request:", err.Error())
		w.Write([]byte("ERROR: " + err.Error()))
		return
	} else {
		result.Process(cfg)
	}
	w.Write([]byte("OK"))
}

func AdminPage(w http.ResponseWriter, r *http.Request, cfg config.Config) {
	err := templates.ExecuteTemplate(w, "AdminPage", &AdminPageData{Config: cfg, Repos: git.Repositories, Title: "Admin repo page"})
	if err != nil {
		logger.Log.Warning("Error sent page for client", r.Host, ":", err.Error())
	}
}

func handleWs(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Now a websocket handshake", 400)
		return
	}
	if err != nil {
		logger.Log.Warning("Error init websocket for client", r.Host, ":", err.Error())
		return
	}
	wsclient.NewClient(ws, r.RemoteAddr, r.UserAgent())
}

func (req *Record) Process(cfg config.Config) {
	switch req.Kind {
	case "push":
		branch := strings.Split(req.GitRef, "/")
		shortBranchName := branch[len(branch)-1]
		if _, ok := git.Repositories[req.Repository.SshUrl+"/"+shortBranchName]; !ok {
			logger.Log.Debug("Incoming request for repository [", req.Repository.SshUrl, "], but this " +
				"repository wasn't found")
			return
		}
		rep := git.Repositories[req.Repository.SshUrl+"/"+shortBranchName]
		if shortBranchName != rep.Branch {
			logger.Log.Debug("Incoming request for repository [", req.Repository.SshUrl, "] and branch [",
				req.GitRef, "], but branch for this repository wasn't found")
			return
		}
		if !rep.Events.Push {
			logger.Log.Debug("Incoming push request for repository [",
				req.Repository.SshUrl, "] and branch [", req.GitRef,
				"], but for this repository push requests isn't accepted for this repository")
			return
		}
		if rep.Lock {
			if git.Repositories[req.Repository.SshUrl+"/"+shortBranchName].Events.Notify {
				rep.SendNotify("Changes from push action need to apply but repository LOCKED. Repository: "+req.Repository.Name+", branch: "+req.GitRef+".")
			}
			git.Repositories[req.Repository.SshUrl+"/"+shortBranchName].History = append(git.Repositories[req.Repository.SshUrl+"/"+shortBranchName].History, git.UpdateHistory{Url: req.CommitAfter, Author: req.UserName})
			events.Events["pushqueue"].SendToChannel("pushqueue", "add", git.GitOrig2Url(req.Repository.SshUrl)+"/"+shortBranchName)
		} else {
			git.Repositories[req.Repository.SshUrl+"/"+shortBranchName].History = make([]git.UpdateHistory, 0)
			git.Repositories[req.Repository.SshUrl+"/"+shortBranchName].Update <- "push request [Last commit: " + req.CommitAfter + "]"
		}
		break
	case "merge_request":
		if _, ok := git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch]; !ok {
			logger.Log.Warning("Request merge_request error. Repository",
				req.Object.Target.SshUrl+"/"+req.Object.TargetBranch,
				"wasn't found")
			return
		}
		rep := git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch]
		if req.Object.TargetBranch != rep.Branch {
			return
		}
		if !rep.Events.Merge {
			logger.Log.Debug("Incoming merge request for repository [", req.Object.Target.Name,
				"] and branch [", req.Object.TargetBranch,
				"], but merge requests isn't accepted for this repository")
			return
		}
		if req.Object.State == "opened" {
			if rep.Events.Notify {
				rep.SendNotify("Merge request from "+req.User.Name+" for merge with repository "+req.Object.Source.Name+". Source branch: "+req.Object.SourceBranch+"; Target branch: "+req.Object.TargetBranch+". Commit: "+req.Object.LastCommit.Url)
			}
			logger.Log.Debug("Merge request from", req.User.Name, "for merge with repository",
				req.Object.Source.Name,
				". Source branch:", req.Object.SourceBranch, "; Target branch:",
				req.Object.TargetBranch, ". Commit:", req.Object.LastCommit.Url)
			userForSendNotify, err := git.GetUserInfo(req.Object.AssigneeId)
			if err != nil {
				logger.Log.Warning("We have new merge request with assigneeId:",
					strconv.Itoa(req.Object.AssigneeId), ", but get for this user returned:",
					err.Error())
				return
			}
			/*authorInfo, err := git.GetUserInfo(req.Object.AuthorId)
			if err != nil {
				logger.Log.Warning("We have new merge request from author id:",
					req.Object.AssigneeId, ", but get for this user returned:",
					err.Error())
				return
			}*/

			if rep.Events.Notify {
				rep.SendNotify("User "+req.Object.LastCommit.Author.Name+" ask you to accept his merge request ("+req.Object.Url+") to the repository "+req.Object.Target.Name+" (branch "+req.Object.TargetBranch+")", userForSendNotify)
			}
		}
		if req.Object.State == "merged" {
			if rep.Lock {
				if git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Events.Notify {
					rep.SendNotify("Changes from merging "+req.Object.Url+" need to apply but repository LOCKED. Repository: "+req.Object.Target.Name+", branch: "+req.Object.TargetBranch+".")
				}
				git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].History = append(git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].History, git.UpdateHistory{Url: req.Object.Url, Author: req.User.Name})
				events.Events["pushqueue"].SendToChannel("pushqueue", "add", git.GitOrig2Url(req.Object.Target.SshUrl)+"/"+req.Object.TargetBranch)
			} else {
				git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].History = make([]git.UpdateHistory, 0)
				git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Update <- req.Object.Url
			}
		}
		if req.Object.State == "closed" && req.Object.Action == "close" {
			logger.Log.Debug("Merge request from " + req.User.Name + " for merge with repository " + req.Object.Source.Name + ". Source branch: " + req.Object.SourceBranch + "; Target branch: " + req.Object.TargetBranch + ". Commit: " + req.Object.LastCommit.Url)
			userForSendNotify, err := git.GetUserInfo(req.Object.AuthorId)
			if err != nil {
				logger.Log.Warning("We have changes in merge request with userId:", strconv.Itoa(req.Object.AuthorId), ", but get for this user returned:", err.Error())
				return
			}

			if rep.Events.Notify {
				rep.SendNotify("Your merge request "+req.Object.Url+" to the repository "+req.Object.Target.Name+" (branch "+req.Object.TargetBranch+") was closed", userForSendNotify)
			}
		}
	}
}

func main() {
	flag.Parse()

	// Parse config
	var Config config.Config
	err := Config.ParseConfig(*configFile)
	if err != nil {
		logger.Log.Critical("Parse config:", err.Error())
	}

	// signals handle
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Log.Info("captured interrupt, exiting..")
		cleanup(sig)
		os.Exit(1)
	}()

	// set user and group
	if Config.Global.User != "" {
		user, err := user.Lookup(Config.Global.User)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		uid, err := strconv.Atoi(user.Uid)
		if err != nil {
			log.Println("Couldn 't convert uid", err)
			os.Exit(1)
		}
		err = syscall.Setuid(uid)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
	}

	logger.InitLogging(Config.Global.Debug, Config.Logger)
	intPort, err := strconv.Atoi(Config.Global.Port)
	if err != nil {
		logger.Log.Critical("Port for listening is invalid:", err.Error())
		return
	}
	if intPort > 65564 {
		logger.Log.Critical("Port is'n correct")
		return
	}

	// Init git local repositories
	err = git.Init(Config)
	if err != nil {
		logger.Log.Critical("Init repositories failed:", err.Error())
		return
	}

	// init git api
	git.InitGitLabApi(Config.Gitlab)

	// channel for updates
	go gitScheduler(Config)

	var apiDir string
	if Config.Web.Api != "" {
		apiDir = Config.Web.Api
	} else {
		apiDir = "/api" // default api page for web hooks
	}

	var managementDir string
	if Config.Web.Management != "" {
		managementDir = Config.Web.Management
	} else {
		managementDir = "/admin" // default admin page
	}

	var templateDir string
	if Config.Web.Templates != "" {
		templateDir = Config.Web.Templates
	} else {
		templateDir = "/www"
	}
	templates, err = template.ParseGlob(templateDir + "/html/*.html")
	if err != nil {
		logger.Log.Critical("Error init templates:" + err.Error())
		return
	}

	if apiDir == managementDir {
		logger.Log.Critical("Error init web interface: [web] Management couldn't equal Api [" + apiDir + "], [" + managementDir + "]")
		return
	}

	events.Init()
	http.HandleFunc(apiDir, func(w http.ResponseWriter, r *http.Request) { gitHooks_process(w, r, Config) })
	http.HandleFunc(managementDir, func(w http.ResponseWriter, r *http.Request) { AdminPage(w, r, Config) })
	http.HandleFunc("/ws", handleWs)
	logger.Log.Critical(http.ListenAndServe(Config.Global.Host+":"+Config.Global.Port, nil))
}

func decode(r io.Reader) (x *Record, err error) {
	x = new(Record)
	err = json.NewDecoder(r).Decode(x)
	return
}

func gitScheduler(cfg config.Config) {
	for _, rep := range git.Repositories {
		go gitEvents(rep)
	}
}

func cleanup(sig os.Signal) (err error) {
	logger.Log.Info("signal " + sig.String() + ": exiting..")
	for _, rep := range git.Repositories {
		rep.Quit <- true
		rep.FileWatchQuit <- true
	}
	for _, rep := range git.Repositories {
		<-rep.QuitReport
	}
	return
}

func gitEvents(rep *git.Repository) {
	for {
		select {
		case <-rep.Quit:
			goto EXIT

		case report := <-rep.Update:
			rep.FileUpdate = true
			err := rep.GetUpdates()
			rep.FileUpdate = false
			if err != nil {
				if rep.Events.Notify {
					rep.SendNotify("Changes from merging "+report+" wasn't applied. Repository: "+rep.Name+", branch: "+rep.Branch+". Merging return error: "+err.Error())
				}
				logger.Log.Debug("Changes from merging", report, "wasn't applied. Repository:" + rep.Name + ", branch:", rep.Branch, ". Merging return error:", err.Error())
			} else {
				if rep.Events.Notify {
					rep.SendNotify("Changes from merging "+report+" was applied. Repository: "+rep.Name+", branch: "+rep.Branch)
				}
				logger.Log.Debug("Changes from merging", report, "was applied. Repository:", rep.Name, ", branch:", rep.Branch)
			}
		}
	}
EXIT:
	logger.Log.Debug("Goroutine exiting for repository", rep.Name, "[", rep.Path, "]")
	rep.QuitReport <- true
	return
}
