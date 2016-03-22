package git

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"path/filepath"
	"sort"

	"github.com/howeyc/fsnotify"
	"github.com/svagner/go-gitlab/config"
	"github.com/svagner/go-gitlab/logger"
	git2go "gopkg.in/libgit2/git2go.v22"
	sshutil "sourcegraph.com/sourcegraph/go-vcs/vcs/ssh"
)

var standardKnownHosts sshutil.KnownHosts

type SSHConfig struct {
	PublicKey  []byte
	PrivateKey []byte
}

type UpdateHistory struct {
	Author string
	Url    string
}

type GitCommitLog struct {
	Type        git2go.ObjectType
	Id          *git2go.Oid
	IdStr       string
	Author      GitAuthor
	Commiter    GitAuthor
	ParentCount uint
	TreeId      *git2go.Oid
	Message     string
}

type GitBlobLog struct {
	Type  git2go.ObjectType
	Id    *git2go.Oid
	IdStr string
	Size  int64
}

type GitTreeLog struct {
	Type       git2go.ObjectType
	Id         *git2go.Oid
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
	Link           *git2go.Repository
	Callback       *git2go.RemoteCallbacks
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
	cb := createRemoteCallbacks(cfg)

	for _, rep := range repos {
		var branch string
		if rep.Branch != "" {
			branch = rep.Branch
		} else {
			branch = DEFAULT_BRANCH
		}
		gitOptions := git2go.CloneOptions{RemoteCallbacks: cb, CheckoutBranch: branch}
		log.Println(rep.Remote)
		logger.DebugPrint("Try to open repository " + rep.Remote + ": " + rep.Path)
		gitH, err := git2go.OpenRepository(rep.Path)
		if err != nil {
			logger.DebugPrint("Init new repository (clone) copy for " + rep.Remote + ": " + rep.Path)
			gitH, err = git2go.Clone(rep.Remote, rep.Path, &gitOptions)
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
		Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch] = &Repository{
			Link:          gitH,
			Callback:      cb,
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
		logger.DebugPrint("Get commits for " + rep.Remote + ": " + rep.Path)
		odb, err := gitH.Odb()
		if err != nil {
			logger.WarningPrint("Get commits for " + rep.Remote + ": " + rep.Path + " returned error: " + err.Error())
			return err
		}
		ccb := git2go.OdbForEachCallback(func(oid *git2go.Oid) error {
			obj, err := gitH.Lookup(oid)
			if err != nil {
				logger.WarningPrint("Lookup commits for " + rep.Remote + ": " + rep.Path + " returned error: " + err.Error())
				return err
			}

			switch obj := obj.(type) {
			default:
			case *git2go.Blob:
				break
				Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].BlobLog =
					append(Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].BlobLog,
						GitBlobLog{
							Type:  obj.Type(),
							Id:    obj.Id(),
							IdStr: obj.Id().String(),
							Size:  obj.Size(),
						})
			case *git2go.Commit:
				author := obj.Author()
				committer := obj.Committer()
				Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].CommitLog =
					append(Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].CommitLog,
						GitCommitLog{
							Type:  obj.Type(),
							Id:    obj.Id(),
							IdStr: obj.Id().String(),
							Author: GitAuthor{
								User:    author.Name,
								Email:   author.Email,
								Date:    author.When,
								DateStr: author.When.String(),
							},
							Commiter: GitAuthor{
								User:    committer.Name,
								Email:   committer.Email,
								Date:    committer.When,
								DateStr: author.When.String(),
							},
							ParentCount: obj.ParentCount(),
							TreeId:      obj.TreeId(),
							Message:     strings.Replace(obj.Message(), "\n", "\n        ", -1),
						})
			case *git2go.Tree:
				break
				Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].TreeLog =
					append(Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].TreeLog,
						GitTreeLog{
							Type:       obj.Type(),
							Id:         obj.Id(),
							IdStr:      obj.Id().String(),
							EntryCount: obj.EntryCount(),
						})
			}
			return nil
		})
		odb.ForEach(ccb)
		sort.Reverse(Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].CommitLog)
		if len(Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].CommitLog) > 10 {
			Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].CommitLog = Repositories[GitUrl2Orig(rep.Remote)+"/"+rep.Branch].CommitLog[0:10]
		}
		logger.DebugPrint("Commits was recieved for " + rep.Remote + ": " + rep.Path)
	}
	return nil
}

func (rep *Repository) GetUpdates() error {
	remotes, err := rep.Link.ListRemotes()
	if err != nil {
		return err
	} else {
		origin, err := rep.Link.LookupRemote(remotes[0])
		origin.SetCallbacks(rep.Callback)
		if err != nil {
			return err
		} else {
			refspec := make([]string, 0)
			err = origin.Fetch(refspec, nil, "")
			if err != nil {
				return err
			}
		}
	}

	// merge
	/*i, err := rep.Link.NewReferenceNameIterator()
	for r, err := i.Next(); err == nil; r, err = i.Next() {
		log.Println(r)
	}
	refLocal := "refs/heads/developments"
	ref := "refs/remotes/origin/developments"
	remote, err := rep.Link.LookupReference(ref)
	if err != nil {
		return err
	}
	local, err := rep.Link.LookupReference(refLocal)
	if err != nil {
		return err
	}
	log.Println(ref)
	log.Println(refLocal)
	log.Println(remote.Target())
	log.Println(remote.IsRemote())
	log.Println(local.Target())
	log.Println(local.IsRemote())
	if err != nil {
		return err
	}*/
	// FEXME: How can I make merge with git2go library???
	rep.StopFSWatch()
	res, err := gitMerge(rep.Path, rep.Branch)
	if err != nil {
		rep.StartFSWatch()
		return err
	} else {
		logger.DebugPrint("Git merge command for repository " + rep.Name + " returned: " + string(res))
	}
	err = rep.commitLog()
	if err != nil {
		logger.WarningPrint("Get commits for " + rep.Path + " return error code: " + err.Error())
	}
	rep.StartFSWatch()
	return nil
}

func md5String(md5Sum [16]byte) string {
	md5Str := fmt.Sprintf("% x", md5Sum)
	md5Str = strings.Replace(md5Str, " ", ":", -1)
	return md5Str
}

func createRemoteCallbacks(cfg config.GitConfig) *git2go.RemoteCallbacks {
	cb := &git2go.RemoteCallbacks{}

	cb.CredentialsCallback = git2go.CredentialsCallback(func(url string, usernameFromURL string, allowedTypes git2go.CredType) (git2go.ErrorCode, *git2go.Cred) {
		err, cred := git2go.NewCredSshKey(usernameFromURL, cfg.PublicKey, cfg.PrivateKey, cfg.Passphrase)
		return git2go.ErrorCode(err), &cred
	})
	cb.CertificateCheckCallback = git2go.CertificateCheckCallback(func(cert *git2go.Certificate, valid bool, hostname string) git2go.ErrorCode {
		return git2go.ErrOk
	})
	return cb
}

func gitMerge(path, branch string) (res []byte, err error) {
	os.Chdir(path)
	cmd := exec.Command("git", "merge", "origin/"+branch)
	res, err = cmd.Output()
	os.Chdir("/")
	return
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
func (rep *Repository) commitLog() error {
	// get rep log
	rep.CommitLog = make([]GitCommitLog, 0)
	logger.DebugPrint(rep)
	odb, err := rep.Link.Odb()
	if err != nil {
		return err
	}
	ccb := git2go.OdbForEachCallback(func(oid *git2go.Oid) error {
		obj, err := rep.Link.Lookup(oid)
		if err != nil {
			return err
		}

		switch obj := obj.(type) {
		default:
		case *git2go.Blob:
			break
			rep.BlobLog =
				append(rep.BlobLog,
					GitBlobLog{
						Type:  obj.Type(),
						Id:    obj.Id(),
						IdStr: obj.Id().String(),
						Size:  obj.Size(),
					})
		case *git2go.Commit:
			author := obj.Author()
			committer := obj.Committer()
			rep.CommitLog =
				append(rep.CommitLog,
					GitCommitLog{
						Type:  obj.Type(),
						Id:    obj.Id(),
						IdStr: obj.Id().String(),
						Author: GitAuthor{
							User:    author.Name,
							Email:   author.Email,
							Date:    author.When,
							DateStr: author.When.String(),
						},
						Commiter: GitAuthor{
							User:    committer.Name,
							Email:   committer.Email,
							Date:    committer.When,
							DateStr: author.When.String(),
						},
						ParentCount: obj.ParentCount(),
						TreeId:      obj.TreeId(),
						Message:     strings.Replace(obj.Message(), "\n", "\n        ", -1),
					})
		case *git2go.Tree:
			break
			rep.TreeLog =
				append(rep.TreeLog,
					GitTreeLog{
						Type:       obj.Type(),
						Id:         obj.Id(),
						IdStr:      obj.Id().String(),
						EntryCount: obj.EntryCount(),
					})
		}
		return nil
	})
	odb.ForEach(ccb)
	sort.Reverse(rep.CommitLog)
	if len(rep.CommitLog) > 10 {
		rep.CommitLog = rep.CommitLog[0:10]
	}
	return nil
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
