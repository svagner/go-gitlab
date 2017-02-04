package gitter

import (
	"log"
	"os"
	"os/exec"
	"gopkg.in/svagner/go-gitlab.v1/config"
	"gopkg.in/svagner/go-gitlab.v1/logger"
	"fmt"
	"bytes"
	"bufio"
	"strings"
	"errors"
)

// Gitter performs git operations. Works through os/exec.
type Gitter struct {
	git string
	environment string
}



// New returns a configured *Gitter or an error.
func New(cfg config.GitConfig, rep *config.GitRepository) (*Gitter, error) {
	g, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	s, err := exec.LookPath("ssh")
	if err != nil {
		return nil, err
	}
	return &Gitter{
		git: g,
		environment: fmt.Sprintf("GIT_SSH_COMMAND=%s -o StrictHostKeyChecking=no -i %s -p %d", s, cfg.PrivateKey, rep.Port),
	}, nil
}

// Clone clones a git repo to a name
func (g *Gitter) Clone(repo, branch, name string) error {
	var (
		sout bytes.Buffer
		serr bytes.Buffer
	)
	stdout := bufio.NewWriter(&sout)
	stderr := bufio.NewWriter(&serr)
	if _, err := os.Stat(name+"/.git"); err != nil {
		if os.IsNotExist(err) {
			logger.DebugPrint(fmt.Sprintf("Execute process: %s %s %s %s %s %s %s %s", g.environment, g.git, "clone", repo, "--branch", branch, "--single-branch", name))
			cmd := &exec.Cmd{Path: g.git, Args: []string{g.git, "clone", repo, "--branch", branch, "--single-branch", name}, Env: []string{g.environment}}
			cmd.Stderr = stderr
			cmd.Stdout = stdout
			cm_err := cmd.Run()
			logger.DebugPrint("STDOUT:", strings.TrimSpace(sout.String()))
			if serr.String() != "" {
				logger.WarningPrint("STDERR:", strings.TrimSpace(serr.String()))
			}
			return cm_err
		} else {
			return err

		}
	} else {
		curdir, err := os.Getwd()
		if err != nil {
			return err
		}
		if err = os.Chdir(name); err != nil {
			return err
		}
		defer func() {
			if err = os.Chdir(curdir); err != nil {
				log.Fatal(err)
			}
		}()
		logger.DebugPrint(fmt.Sprintf("Execute process: %s %s %s %s %s", g.environment, g.git, "remote", "get-url", "origin"))
		cmd := &exec.Cmd{Path: g.git, Args: []string{g.git, "remote", "get-url", "origin"}, Env: []string{g.environment}}
		cmd.Stderr = stderr
		cmd.Stdout = stdout
		cm_err := cmd.Run()
		logger.DebugPrint("Data from get origin:", strings.TrimSpace(sout.String()), "| data from config:", repo)
		if strings.TrimSpace(sout.String()) != repo {
			return errors.New("Path "+ name + " has a invalid data")
		}
		logger.DebugPrint("STDOUT:", strings.TrimSpace(sout.String()))
		if serr.String() != "" {
			logger.WarningPrint("STDERR:", strings.TrimSpace(serr.String()))
		}
		return cm_err
	}
	return nil
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
	var (
		sout bytes.Buffer
		serr bytes.Buffer
	)
	stdout := bufio.NewWriter(&sout)
	stderr := bufio.NewWriter(&serr)
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
	logger.DebugPrint(fmt.Sprintf("Execute process: %s %s %s", g.environment, g.git, "pull"))
	cmd := &exec.Cmd{Path: g.git, Args: []string{g.git, "pull"}, Env: []string{g.environment}}
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	cm_err := cmd.Run()
	logger.DebugPrint("STDOUT:", strings.TrimSpace(sout.String()))
	if serr.String() != "" {
		logger.WarningPrint("STDERR:", strings.TrimSpace(serr.String()))
	}
	return cm_err
}
