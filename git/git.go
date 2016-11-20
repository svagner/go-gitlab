package git

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"path/filepath"
	"sort"

	"github.com/howeyc/fsnotify"
	"gopkg.in/svagner/go-gitlab.v2/config"
	"gopkg.in/svagner/go-gitlab.v2/logger"
	"gopkg.in/svagner/go-gitlab.v2/notify"
	customNotify "gopkg.in/svagner/go-gitlab.v2/notify/custom"
	slackNotify "gopkg.in/svagner/go-gitlab.v2/notify/slack"
	git "gopkg.in/src-d/go-git.v4"
	gitCommon "gopkg.in/svagner/go-git.v4/plumbing/client/common"
	ssh_client "gopkg.in/svagner/go-git.v4/plumbing/client/ssh"
	"gopkg.in/svagner/go-git.v4/plumbing"
	"io"
	"io/ioutil"
	"golang.org/x/crypto/ssh"
	"errors"
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
	Type       plumbing.ObjectType
	Id         plumbing.Hash
	IdStr       string
	Author      GitAuthor
	Commiter    GitAuthor
	ParentCount int
	TreeId      plumbing.Hash
	Message     string
}

type GitBlobLog struct {
	Type  plumbing.ObjectType
	Id    plumbing.Hash
	IdStr string
}

type GitTreeLog struct {
	Type       plumbing.ObjectType
	Id         plumbing.Hash
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
	Link *git.Repository
	Auth gitCommon.AuthMethod
	User string
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
	Notify []notify.Notification

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

func Init(cfg config.Config) error {
	for _, rep := range cfg.Repository {
		var (
			branch,
			repName string
		)
		if rep.Branch != "" {
			branch = rep.Branch
		} else {
			branch = DEFAULT_BRANCH
		}

		if rep.Alias != "" {
			repName = rep.Alias + "/" + rep.Branch
		} else {
			repName = GitUrl2Orig(rep.Remote)+"/"+rep.Branch
		}

		r, err := git.NewFilesystemRepository(rep.Path)
		if err != nil {
			return err
		}


		sshKey, err := makeSigner(cfg.Git.PrivateKey)
		if err != nil {
			log.Fatalln("SSH error >>", err.Error())
		}

		if empty, err :=  r.IsEmpty(); empty || err != nil {
			err = r.Clone(&git.CloneOptions{
				URL: rep.Remote,
				Auth: &ssh_client.PublicKeys{User: cfg.Git.User, Signer: sshKey},
			})
			if err != nil {
				return err
			}
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
		Repositories[repName] = &Repository{
			Link: r,
			User: cfg.Git.User,
			Auth: &ssh_client.PublicKeys{User: cfg.Git.User, Signer: sshKey},
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

		// Init notification transports
		logger.Log.Debug("Start init notifications for repository", repName)
		Repositories[repName].Notify = make([]notify.Notification, 0)
		if _, ok := cfg.Customnotify[rep.CustomNotify]; rep.CustomNotify != "" && ok {
			custom, err := customNotify.Create(cfg.Customnotify)
			if err != nil {
				logger.Log.Warning("Error while init notification with type custom for" +
					"repository", repName)
			} else {
				Repositories[repName].Notify = append(Repositories[repName].Notify, custom)
			}
		}
		if _, ok := cfg.Slacknotify[rep.SlackNotify]; rep.SlackNotify != "" && ok {
			custom, err := slackNotify.Create(cfg.Slacknotify)
			if err != nil {
				logger.Log.Warning("Error while init notification with type slack for" +
					"repository", repName)
			} else {
				Repositories[repName].Notify = append(Repositories[repName].Notify, custom)
			}
		}

		ref, err := r.Head()
		if err != nil {
			return err
		}
		commit, err := r.Commit(ref.Hash())
		if err != nil {
			return err
		}

		// get rep log
		files, err := commit.Files()
		if err != nil {
			return err
		}

		// ... now we iterate the files to save to disk
		err = files.ForEach(func(f *git.File) error {
			abs := filepath.Join(rep.Path, f.Name)
			dir := filepath.Dir(abs)

			os.MkdirAll(dir, 0777)
			file, err := os.Create(abs)
			if err != nil {
				return err
			}

			defer file.Close()
			r, err := f.Reader()
			if err != nil {
				return err
			}

			defer r.Close()

			if err := file.Chmod(f.Mode); err != nil {
				return err
			}

			_, err = io.Copy(file, r)
			return err
		})
		go Repositories[repName].InitFSWatch()

		ccb := func(commit *git.Commit) error {
			switch commit.Type() {
			default:
			case plumbing.BlobObject:
				break
				Repositories[repName].BlobLog =
					append(Repositories[repName].BlobLog,
						GitBlobLog{
							Type:  commit.Type(),
							Id:    commit.ID(),
							IdStr: commit.ID().String(),
						})
			case plumbing.CommitObject:
				tree, err := commit.Tree()
				if err != nil {
					return err
				}
				Repositories[repName].CommitLog =
					append(Repositories[repName].CommitLog,
						GitCommitLog{
							Type:  commit.Type(),
							Id:    commit.ID(),
							IdStr: commit.ID().String(),
							Author: GitAuthor{
								User:    commit.Author.Name,
								Email:   commit.Author.Email,
								Date:    commit.Author.When,
								DateStr: commit.Author.When.String(),
							},
							Commiter: GitAuthor{
								User:    commit.Committer.Name,
								Email:   commit.Committer.Email,
								Date:    commit.Committer.When,
								DateStr: commit.Committer.When.String(),
							},
							ParentCount: commit.NumParents(),
							TreeId:      tree.ID(),
							Message:     strings.Replace(commit.Message, "\n", "\n        ", -1),
						})
			case plumbing.AnyObject:
				break
			}
			return nil
		}
		// lockup commits
		commits, err := r.Commits()
		if err != nil {
			logger.Log.Warning("Lookup commits for " + rep.Remote + ": " + rep.Path + " returned error: " + err.Error())
			return err
		}
		commits.ForEach(ccb)
		sort.Reverse(Repositories[repName].CommitLog)
		if len(Repositories[repName].CommitLog) > 10 {
			Repositories[repName].CommitLog = Repositories[repName].CommitLog[0:10]
		}
		logger.Log.Debug("Commits was recieved for " + rep.Remote + ": " + rep.Path)
	}
	return nil
}

func (rep *Repository) SendNotify(msg string, user... *UserInfo) error {
	for _, ntf := range rep.Notify {
		err := ntf.Send(msg, user)
		if err != nil {
			logger.Log.Warning("Error while send notification for repo", rep.Name, "with transport", ntf.GetType())
		}
	}
	return nil
}

func (rep *Repository) GetUpdates(c string) error {
	err := rep.Link.Pull(&git.PullOptions{
		RemoteName: "origin",
		Auth: &ssh_client.PublicKeys{User: rep.User, Signer: rep.Auth},
		ReferenceName: "refs/heads/"+rep.Branch,
		SingleBranch: true,
		Depth: 0})
	if err != nil {
		logger.Log.Warning(err.Error())
	}
	ref, _ := rep.Link.Head()
	// ... retrieving the commit object
	commit, err := rep.Link.Commit(ref.Hash())
	if err != nil {
		return err
	}
	logger.Log.Debug("Got commit", c, ref.Hash().String())
	files,_ := commit.Files()

	// ... now we iterate the files to save to disk
	err = files.ForEach(func(f *git.File) error {
		logger.Log.Debug("Commit", c, "file", rep.Path, f.Name)
		abs := filepath.Join(rep.Path, f.Name)
		dir := filepath.Dir(abs)

		os.MkdirAll(dir, 0777)
		file, err := os.Create(abs)
		if err != nil {
			return err
		}

		defer file.Close()
		r, err := f.Reader()
		if err != nil {
			return err
		}

		defer r.Close()

		if err := file.Chmod(f.Mode); err != nil {
			return err
		}

		_, err = io.Copy(file, r)
		return err
	})
	return nil
}

func md5String(md5Sum [16]byte) string {
	md5Str := fmt.Sprintf("% x", md5Sum)
	md5Str = strings.Replace(md5Str, " ", ":", -1)
	return md5Str
}

func GitUrl2Orig(url string) string {
	repo := strings.SplitN(strings.TrimLeft(url, "ssh://"), "/", 2)
	return repo[0] + ":" + repo[1]
}

func GitOrig2Url(url string) string {
	repo := strings.SplitN(url, ":", 2)
	return "ssh://" + repo[0] + "/" + repo[1]
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
		logger.Log.Critical("Filed to initialize file system watcher for <" + rep.Path + ">:" + err.Error())
	}

	go rep.fsEvent(rep.fileWatcher)
	<-rep.FileWatchQuit
	rep.fileWatcher.Close()
}

func (rep *Repository) StartFSWatch() {
	if len(rep.SubDirectories) != 0 {
		logger.Log.Warning("Couldn't remove all subdirectories from watcher. Check it!")
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
		logger.Log.Debug("Add directory for watch: " + path)
		if err != nil {
			logger.Log.Warning("FS Monitor error monitor path [" +
				path + "]: " + err.Error())
		}
	}
}

func (rep *Repository) StopFSWatch() {
	rmCounter := 0
	for _, path := range rep.SubDirectories {
		err := rep.fileWatcher.RemoveWatch(path)
		if err != nil {
			logger.Log.Warning("Remove directory from watching [" + path +
				"]: " + err.Error())
		} else {
			logger.Log.Debug("Remove directory from watching: " + path)
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
				logger.Log.Warning("ALARM! Change repository git without version control! Repository: " + rep.Name + ", Branch: " + rep.Branch + ". Event: " + ev.String())
				//logger.Skype("ALARM! Change repository git without version control! Repository: "+rep.Name+", Branch: "+rep.Branch+". Event: "+ev.String(), "")
				//logger.Slack("ALARM! Change repository git without version control! Repository: "+rep.Name+", Branch: "+rep.Branch+". Event: "+ev.String(), "")
			}
		case err := <-watcher.Error:
			if !rep.FileUpdate {
				logger.Log.Warning("File watcher exitting... Repository: " + rep.Name + ", Branch: " + rep.Branch + ". Quit: " + err.Error())
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

func makeSigner(keyname string) (signer ssh.Signer, err error) {
	key, err := ioutil.ReadFile(keyname)
	if err != nil {
		return
	}
	signer, err = ssh.ParsePrivateKey([]byte(key))
	if err != nil {
		return
	}
	return
}