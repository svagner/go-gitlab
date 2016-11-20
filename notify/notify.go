package notify

import "gopkg.in/svagner/go-gitlab.v2/git"

type Notification interface {
	Send(msg string, user... *git.UserInfo) error
	GetType() string
}
