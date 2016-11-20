package slack

import "gopkg.in/svagner/go-gitlab.v2/notify"

var msgs = map[string]string{
	notify.LockedMsg: "Changes for {{.Action}} for {{.Url}} need to apply but repository LOCKED. Repository: {{.Repository}}, branch: {{.Branch}}.",
	notify.MergeSuccessMsg: "Changes from merging {{.Url}} was applied. Repository: {{.Repository}}, branch: {{.Branch}}",
	notify.MergeErrorMsg: "Changes from merging {{.Url}} wasn't applied. Repository: {{.Repository}}, branch: {{.Branch}}. Merging return error: {{.Error}}",
	notify.MergePrivateCloseMsg: "Your merge request {{.Url}} to the repository {{.Repository}} (branch {{.Branch}}) was closed",
	notify.AskAcceptPrivateMsg: "User {{.User.Name}} @{{.User.Slack}} ask you to accept his merge request ({{.Url}}) to the repository {{.Repository}} (branch {{.Branch}})",
	notify.MergeRequestMsg: "Merge request from {{.User.Name}} for merge with repository {{.Repository}}. Source branch: {{.SourceBranch}}; Target branch: {{.TargetBranch}}. Commit: {{.Url}}",
}

