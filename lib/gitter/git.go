package gitter

import (
	"log"
	"os"
	"os/exec"
	"gopkg.in/svagner/go-gitlab.v1/config"
	"fmt"
)

// Gitter performs git operations. Works through os/exec.
type Gitter struct {
	git string
	environment string
}



// New returns a configured *Gitter or an error.
func New(cfg config.GitConfig) (*Gitter, error) {
	g, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	return &Gitter{
		git: g,
		environment: fmt.Sprintf("GIT_SSH=\"/usr/bin/ssh -o StrictHostKeyChecking=no -i %s", cfg.PrivateKey),
	}, nil
}

// Clone clones a git repo to a name
func (g *Gitter) Clone(repo, branch, name string) error {
	cmd := &exec.Cmd{Path: g.git, Args: []string{"clone", repo, "--branch", branch, "--single-branch", name}, Env: []string{g.environment}}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CheckoutBranch cd's into repoPath and checks out the branch.
func (g *Gitter) CheckoutBranch(repoPath, branch string) error {
	curdir, err := os.Getwd()
	if err != nil {
		return err
	}
	if err = os.Chdir(repoPath); err != nil {
		return err
	}
	defer func() {
		if err = os.Chdir(curdir); err != nil {
			log.Print(err)
		}
	}()
	cmd := exec.Command(g.git, "checkout", branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *Gitter) Pull(repoPath string) error {
	curdir, err := os.Getwd()
	if err != nil {
		return err
	}
	if err = os.Chdir(repoPath); err != nil {
		return err
	}
	defer func() {
		if err = os.Chdir(curdir); err != nil {
			log.Fatal(err)
		}
	}()
	cmd := &exec.Cmd{Path: g.git, Args: []string{"pull"}, Env: []string{g.environment}}
	cmd := exec.Command(g.git, "pull")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
