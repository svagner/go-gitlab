package git

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"path/filepath"

	"github.com/howeyc/fsnotify"
	"gopkg.in/svagner/go-gitlab.v1/logger"
	"gopkg.in/svagner/go-gitlab.v1/config"
	"gopkg.in/svagner/go-gitlab.v1/lib/gitter"
)


type SSHConfig struct {
	PublicKey  []byte
	PrivateKey []byte
}

type UpdateHistory struct {
	Author string
	Url    string
}

type GitCommitLog struct {
	IdStr       string
	Author      GitAuthor
	Commiter    GitAuthor
	ParentCount uint
	Message     string
}

type GitBlobLog struct {
	IdStr string
	Size  int64
}

type GitTreeLog struct {
	IdStr      string
	EntryCount uint64
}

type GitAuthor struct {
	User    string
	Email   string
	Date    time.Time
	DateStr string
}

type GitEvents struct {
	Push   bool
	Merge  bool
	Notify bool
}

type Repository struct {
	Git	gitter.Gitter
	Path           string
	Branch         string
	Update         chan string
	Quit           chan bool
	QuitReport     chan bool
	Name           string
	Url            string
	Lock           bool
	FileWatchQuit  chan bool
	fileWatcher    *fsnotify.Watcher
	FileUpdate     bool
	Error          bool
	LastError      string
	History        []UpdateHistory
	BlobLog        []GitBlobLog
	TreeLog        []GitTreeLog
	CommitLog      GitCommit
	Events         GitEvents
	SubDirectories []string
}

const (
	DEFAULT_BRANCH = "master"
)

var (
	Repositories = make(map[string]*Repository, 0)
)

type GitCommit []GitCommitLog

// Forward request for length
func (p GitCommit) Len() int {
	return len(p)
}

// Define compare
func (p GitCommit) Less(i, j int) bool {
	return p[i].Commiter.Date.Before(p[j].Author.Date)
}

// Define swap over an array
func (p GitCommit) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func Init(cfg config.GitConfig, repos map[string]*config.GitRepository) error {

	for _, rep := range repos {
		var branch string
		if rep.Branch != "" {
			branch = rep.Branch
		} else {
			branch = DEFAULT_BRANCH
		}

		log.Println(rep.Remote)
		logger.DebugPrint("Init new repository (clone) copy for " + rep.Remote + ": " + rep.Path)
		git, err := gitter.New(cfg)
		if err != nil {
			logger.WarningPrint("Error while init git binary: " + err.Error())
			return err
		}
		err = git.Clone(rep.Remote, branch, rep.Path)
		if err != nil {
			logger.WarningPrint(fmt.Sprintf("Error while clone git repository %s: %s", rep.Remote, err.Error()))
			return err
		}
		chanQuit := make(chan bool)
		chanUpdate := make(chan string)
		chanQuitAccept := make(chan bool)
		fileWatchQ := make(chan bool)
		updHist := make([]UpdateHistory, 0)
		blobLog := make([]GitBlobLog, 0)
		treeLog := make([]GitTreeLog, 0)
		cmtLog := make([]GitCommitLog, 0)
		subDirs := make([]string, 0)
		Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch] = &Repository{
			Git:		git,
			Path:          rep.Path,
			Branch:        branch,
			Name:          rep.Remote,
			Url:           GitOrig2Http(rep.Remote),
			Quit:          chanQuit,
			QuitReport:    chanQuitAccept,
			Update:        chanUpdate,
			History:       updHist,
			BlobLog:       blobLog,
			TreeLog:       treeLog,
			CommitLog:     cmtLog,
			FileWatchQuit: fileWatchQ,
			Events: GitEvents{
				Push:   rep.PushRequests,
				Merge:  rep.MergeRequests,
				Notify: rep.Notifications,
			},
			SubDirectories: subDirs,
		}
		go Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].InitFSWatch()

		// get rep log
		logger.DebugPrint(rep)
	}
	return nil
}

func (rep *Repository) GetUpdates() error {
	rep.StopFSWatch()
	err := rep.Git.Pull(rep.Path)
	if err != nil {
		rep.StartFSWatch()
		return err
	}
	rep.StartFSWatch()
	return nil
}

func GitUrl2Orig(url string) string {
	repo := strings.SplitN(strings.TrimLeft(url, "ssh://"), "/", 2)
	return repo[0] + ":" + repo[1]
}

func GitOrig2Http(url string) string {
	withoutUser := strings.SplitN(strings.TrimLeft(url, "ssh://"), "@", 2)
	return "http://" + strings.TrimSuffix(withoutUser[1], ".git")
}

// File watcher

func (rep *Repository) InitFSWatch() {
	var err error
	rep.fileWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		logger.CriticalPrint("Filed to initialize file system watcher for <" + rep.Path + ">:" + err.Error())
	}

	go rep.fsEvent(rep.fileWatcher)
	<-rep.FileWatchQuit
	rep.fileWatcher.Close()
}

func (rep *Repository) StartFSWatch() {
	if len(rep.SubDirectories) != 0 {
		logger.WarningPrint("Couldn't remove all subdirectories from watcher. Check it!")
		return
	}
	filepath.Walk(rep.Path, func(pathStr string, info os.FileInfo, err error) error {
		dir, err := directoryChooser(pathStr, info, err)
		if err != nil {
			return err
		}
		if dir != "" {
			rep.SubDirectories = append(rep.SubDirectories, dir)
		}
		return nil
	})

	for _, path := range rep.SubDirectories {
		err := rep.fileWatcher.Watch(path)
		logger.DebugPrint("Add directory for watch: " + path)
		if err != nil {
			logger.WarningPrint("FS Monitor error monitor path [" +
				path + "]: " + err.Error())
		}
	}
}

func (rep *Repository) StopFSWatch() {
	rmCounter := 0
	for _, path := range rep.SubDirectories {
		err := rep.fileWatcher.RemoveWatch(path)
		if err != nil {
			logger.WarningPrint("Remove directory from watching [" + path +
				"]: " + err.Error())
		} else {
			logger.DebugPrint("Remove directory from watching: " + path)
			rmCounter++
		}
	}
	if rmCounter == len(rep.SubDirectories) {
		rep.SubDirectories = make([]string, 0)
	}
}

func (rep *Repository) fsEvent(watcher *fsnotify.Watcher) {
	rep.StartFSWatch()
	for {
		select {
		case ev := <-watcher.Event:
			if !rep.FileUpdate {
				rep.Error = true
				rep.LastError = ev.String()
				logger.WarningPrint("ALARM! Change repository git without version control! Repository: " + rep.Name + ", Branch: " + rep.Branch + ". Event: " + ev.String())
				logger.Skype("ALARM! Change repository git without version control! Repository: "+rep.Name+", Branch: "+rep.Branch+". Event: "+ev.String(), "")
				logger.Slack("ALARM! Change repository git without version control! Repository: "+rep.Name+", Branch: "+rep.Branch+". Event: "+ev.String(), "")
			}
		case err := <-watcher.Error:
			if !rep.FileUpdate {
				logger.WarningPrint("File watcher exitting... Repository: " + rep.Name + ", Branch: " + rep.Branch + ". Quit: " + err.Error())
				return
			}
		}
	}
}

func directoryChooser(pathStr string, info os.FileInfo, err error) (string, error) {
	if !info.IsDir() {
		return "", nil
	}
	if info.Name() == ".git" {
		return "", filepath.SkipDir
	}
	return pathStr, nil
}
