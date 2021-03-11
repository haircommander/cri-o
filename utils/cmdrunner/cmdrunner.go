package cmdrunner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

type CommandRunner interface {
	ExecCmd(args ...string) (string, error)
	ExecCmdWithStdin(stdin io.Reader, args ...string) (string, error)
}

type RealCommandRunner struct {
	command    string
	prefixArgs []string
	env        []string
}

func NewWithEnv(command string, env []string, prefixArgs ...string) *RealCommandRunner {
	return &RealCommandRunner{
		command:    command,
		env:        env,
		prefixArgs: prefixArgs,
	}
}

func New(command string, prefixArgs ...string) *RealCommandRunner {
	return &RealCommandRunner{
		command:    command,
		prefixArgs: prefixArgs,
	}
}

// ExecCmd executes a command with args and returns its output as a string along
// with an error, if any
func (e *RealCommandRunner) ExecCmd(args ...string) (string, error) {
	return e.ExecCmdWithStdin(nil, args...)
}

// ExecCmd executes a command with args and returns its output as a string along
// with an error, if any
func (e *RealCommandRunner) ExecCmdWithStdin(stdin io.Reader, args ...string) (string, error) {
	allArgs := append(e.prefixArgs, args...)
	cmd := exec.Command(e.command, allArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != nil {
		cmd.Stdin = stdin
	}
	for _, envVar := range e.env {
		if v, found := os.LookupEnv(envVar); found {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", envVar, v))
		}
	}

	err := cmd.Run()
	if err != nil {
		return "", errors.Wrapf(err, "`%v %v` failed: %v %v", e.command, strings.Join(allArgs, " "), stderr.String(), stdout.String())
	}

	return stdout.String(), nil
}
