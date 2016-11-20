package notify

import "gopkg.in/svagner/go-gitlab.v2/git"

const (
	LockedMsg = "lockedMsg"
	MergeSuccessMsg = "mergeSuccessMsg"
	MergePrivateCloseMsg = "mergePrivateCloseMsg"
	MergeErrorMsg = "mergeErrorMsg"
	AskAcceptPrivateMsg = "askAcceptPrivateMsg"
	MergeRequestMsg = "mergeRequestMsg"
	PushAction  = "push"
	MergedAction = "merged"
)

type User struct {
	Name string
	Custom string
	Slack string
}

type Notify struct {
	User User
	Url string
	Repository string
	Branch string
	TargetBranch string
	SourceBranch string
	Action string
	Error string
}

type Notification interface {
	Send(msg string, data interface{}, user... *git.UserInfo) error
	GetType() string
}
