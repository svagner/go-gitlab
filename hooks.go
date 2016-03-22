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
	daemon "github.com/svagner/go-gitlab/lib/go-daemon"
	"github.com/svagner/go-gitlab/logger"
	"github.com/svagner/go-gitlab/wsclient"
	//daemon "github.com/sevlyar/go-daemon"
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
	daemonize  = flag.Bool("daemon", false, "Run as daemon")
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
		logger.WarningPrint(err)
	}
	logger.DebugPrint("Get new value: " + string(p))
	result, err := decode(bytes.NewReader(p))
	if err != nil {
		logger.WarningPrint("Error decode hook request: " + err.Error())
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
		logger.WarningPrint("Error sent page for client " + r.Host + ": " + err.Error())
	}
}

func handleWs(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Now a websocket handshake", 400)
		return
	}
	if err != nil {
		logger.WarningPrint("Error init websocket for client " + r.Host + ": " + err.Error())
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
			logger.DebugPrint("Incoming request for repository [" + req.Repository.SshUrl + "], but this repository wasn't found")
			return
		}
		if shortBranchName != git.Repositories[req.Repository.SshUrl+"/"+shortBranchName].Branch {
			logger.DebugPrint("Incoming request for repository [" + req.Repository.SshUrl + "] and branch [" + req.GitRef + "], but branch for this repository wasn't found")
			return
		}
		if !git.Repositories[req.Repository.SshUrl+"/"+shortBranchName].Events.Push {
			logger.DebugPrint("Incoming push request for repository [" + req.Repository.SshUrl + "] and branch [" + req.GitRef + "], but for this repository push requests isn't accepted for this repository")
			return
		}
		if git.Repositories[req.Repository.SshUrl+"/"+shortBranchName].Lock {
			if git.Repositories[req.Repository.SshUrl+"/"+shortBranchName].Events.Notify {
				logger.Skype("Changes from push action need to apply but repository LOCKED. Repository: "+req.Repository.Name+", branch: "+req.GitRef+".", "")
				logger.Slack("Changes from push action need to apply but repository LOCKED. Repository: "+req.Repository.Name+", branch: "+req.GitRef+".", "")
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
			return
		}
		if req.Object.TargetBranch != git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Branch {
			return
		}
		if !git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Events.Merge {
			logger.DebugPrint("Incoming merge request for repository [" + req.Object.Target.Name + "] and branch [" + req.Object.TargetBranch + "], but merge requests isn't accepted for this repository")
			return
		}
		if req.Object.State == "opened" {
			if git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Events.Notify {
				logger.Skype("Merge request from "+req.User.Name+" for merge with repository "+req.Object.Source.Name+". Source branch: "+req.Object.SourceBranch+"; Target branch: "+req.Object.TargetBranch+". Commit: "+req.Object.LastCommit.Url, "")
				logger.Slack("Merge request from "+req.User.Name+" for merge with repository "+req.Object.Source.Name+". Source branch: "+req.Object.SourceBranch+"; Target branch: "+req.Object.TargetBranch+". Commit: "+req.Object.LastCommit.Url, "")
			}
			logger.DebugPrint("Merge request from " + req.User.Name + " for merge with repository " + req.Object.Source.Name + ". Source branch: " + req.Object.SourceBranch + "; Target branch: " + req.Object.TargetBranch + ". Commit: " + req.Object.LastCommit.Url)
			userForSendNotify, err := git.GetUserInfo(req.Object.AssigneeId)
			if err != nil {
				logger.WarningPrint("We have new merge request with assigneeId: " + strconv.Itoa(req.Object.AssigneeId) + ", but get for this user returned: " + err.Error())
				return
			}
			authorInfo, err := git.GetUserInfo(req.Object.AuthorId)
			if err != nil {
				logger.WarningPrint("We have new merge request from author id: " + strconv.Itoa(req.Object.AssigneeId) + ", but get for this user returned: " + err.Error())
				return
			}

			if git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Events.Notify {
				logger.Skype("User "+req.Object.LastCommit.Author.Name+" (skype: "+authorInfo.Skype+")"+" ask you to accept his merge request ("+req.Object.Url+") to the repository "+req.Object.Target.Name+" (branch "+req.Object.TargetBranch+")", userForSendNotify.Skype)
				logger.Slack("User "+req.Object.LastCommit.Author.Name+" ( @"+authorInfo.Website+": )"+" ask you to accept his merge request ("+req.Object.Url+") to the repository "+req.Object.Target.Name+" (branch "+req.Object.TargetBranch+")", userForSendNotify.Website)
			}
		}
		if req.Object.State == "merged" {
			if git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Lock {
				if git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Events.Notify {
					logger.Skype("Changes from merging "+req.Object.Url+" need to apply but repository LOCKED. Repository: "+req.Object.Target.Name+", branch: "+req.Object.TargetBranch+".", "")
					logger.Slack("Changes from merging "+req.Object.Url+" need to apply but repository LOCKED. Repository: "+req.Object.Target.Name+", branch: "+req.Object.TargetBranch+".", "")
				}
				git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].History = append(git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].History, git.UpdateHistory{Url: req.Object.Url, Author: req.User.Name})
				events.Events["pushqueue"].SendToChannel("pushqueue", "add", git.GitOrig2Url(req.Object.Target.SshUrl)+"/"+req.Object.TargetBranch)
			} else {
				git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].History = make([]git.UpdateHistory, 0)
				git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Update <- req.Object.Url
			}
		}
		if req.Object.State == "closed" && req.Object.Action == "close" {
			logger.DebugPrint("Merge request from " + req.User.Name + " for merge with repository " + req.Object.Source.Name + ". Source branch: " + req.Object.SourceBranch + "; Target branch: " + req.Object.TargetBranch + ". Commit: " + req.Object.LastCommit.Url)
			userForSendNotify, err := git.GetUserInfo(req.Object.AuthorId)
			if err != nil {
				logger.WarningPrint("We have changes in merge request with userId: " + strconv.Itoa(req.Object.AuthorId) + ", but get for this user returned: " + err.Error())
				return
			}
			if git.Repositories[req.Object.Target.SshUrl+"/"+req.Object.TargetBranch].Events.Notify {
				logger.Skype("Your merge request "+req.Object.Url+" to the repository "+req.Object.Target.Name+" (branch "+req.Object.TargetBranch+") was closed", userForSendNotify.Skype)
				logger.Slack("Your merge request "+req.Object.Url+" to the repository "+req.Object.Target.Name+" (branch "+req.Object.TargetBranch+") was closed", userForSendNotify.Website)
			}
		}
	}
}

func main() {
	daemon.AddCommand(daemon.StringFlag(sig, "term"), syscall.SIGTERM, cleanup)
	daemon.AddCommand(daemon.StringFlag(sig, "reload"), syscall.SIGHUP, cleanup)
	flag.Parse()

	if *daemonize {
		// Define daemon context
		dmn := &daemon.Context{
			PidFileName: *pidFile,
			PidFilePerm: 0644,
			LogFileName: *logFile,
			LogFilePerm: 0640,
			WorkDir:     "/",
			Umask:       022,
		}

		// Send commands if needed
		if len(daemon.ActiveFlags()) > 0 {
			d, err := dmn.Search()
			if err != nil {
				logger.WarningPrint("Unable send signal to the daemon:", err)
			}
			daemon.SendCommands(d)
			return
		}

		// Process daemon operations - send signal if present flag or daemonize
		//daemon.SetSigHandler(cleanup, os.Interrupt, syscall.SIGTERM, os.Kill)
		child, err := dmn.Reborn()
		if err != nil {
			log.Fatalln(err)
		}
		if child != nil {
			return
		}
		defer dmn.Release()
	}

	// Parse config
	var Config config.Config
	err := Config.ParseConfig(*configFile)
	if err != nil {
		logger.CriticalPrint("Parse config: " + err.Error())
	}

	// signals handle
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.InfoPrint("captured interrupt, exiting..")
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

	logger.Init(Config.Global.Debug, Config.Logger)
	intPort, err := strconv.Atoi(Config.Global.Port)
	if err != nil {
		logger.CriticalPrint(err)
	}
	if intPort > 65564 {
		logger.CriticalPrint("Port is'n correct")
	}
	log.Println(Config)

	// Init git local repositories
	err = git.Init(Config.Git, Config.Repository)
	if err != nil {
		logger.WarningPrint("Init repositories failed: " + err.Error())
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
		logger.CriticalPrint("Error init templates: " + err.Error())
	}

	if apiDir == managementDir {
		logger.CriticalPrint("Error init web interface: [web] Management couldn't equal Api [" + apiDir + "], [" + managementDir + "]")
	}

	events.Init()
	http.HandleFunc(apiDir, func(w http.ResponseWriter, r *http.Request) { gitHooks_process(w, r, Config) })
	http.HandleFunc(managementDir, func(w http.ResponseWriter, r *http.Request) { AdminPage(w, r, Config) })
	http.HandleFunc("/ws", handleWs)
	logger.CriticalPrint(http.ListenAndServe(Config.Global.Host+":"+Config.Global.Port, nil))
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
	logger.InfoPrint("signal " + sig.String() + ": exiting..")
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
					logger.Skype("Changes from merging "+report+" wasn't applied. Repository: "+rep.Name+", branch: "+rep.Branch+". Merging return error: "+err.Error(), "")
					logger.Slack("Changes from merging "+report+" wasn't applied. Repository: "+rep.Name+", branch: "+rep.Branch+". Merging return error: "+err.Error(), "")
				}
				logger.DebugPrint("Changes from merging " + report + " wasn't applied. Repository: " + rep.Name + ", branch: " + rep.Branch + ". Merging return error: " + err.Error())
			} else {
				if rep.Events.Notify {
					logger.Skype("Changes from merging "+report+" was applied. Repository: "+rep.Name+", branch: "+rep.Branch, "")
					logger.Slack("Changes from merging "+report+" was applied. Repository: "+rep.Name+", branch: "+rep.Branch, "")
				}
				logger.DebugPrint("Changes from merging " + report + " was applied. Repository: " + rep.Name + ", branch: " + rep.Branch)
			}
		}
	}
EXIT:
	logger.DebugPrint("Goroutine exiting for repository " + rep.Name + "[" + rep.Path + "]")
	rep.QuitReport <- true
	return
}
