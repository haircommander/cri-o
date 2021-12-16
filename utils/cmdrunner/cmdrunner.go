package cmdrunner

import (
	"os/exec"
)

// Use a singleton instance because there are many modules that may want access
// and having it all go through the config is cumbersome.
var commandRunner CommandRunner

type CommandRunner interface {
	command(string, ...string) *exec.Cmd
	combinedOutput(string, ...string) ([]byte, error)
}

type RealCommandRunner struct {
	prependCmd  string
	prependArgs []string
}

// Set updates the singleton object with the configured prepended cmd and args
func Set(prependCmd string, prependArgs ...string) {
	commandRunner = &RealCommandRunner{
		prependCmd:  prependCmd,
		prependArgs: prependArgs,
	}
}

func CombinedOutput(command string, args ...string) ([]byte, error) {
	if commandRunner == nil {
		return exec.Command(command, args...).CombinedOutput()
	}
	return commandRunner.combinedOutput(command, args...)
}

func (c *RealCommandRunner) combinedOutput(command string, args ...string) ([]byte, error) {
	return c.command(command, args...).CombinedOutput()
}

func Command(cmd string, args ...string) *exec.Cmd {
	if commandRunner == nil {
		return exec.Command(cmd, args...)
	}
	return commandRunner.command(cmd, args...)
}

func (c *RealCommandRunner) command(cmd string, args ...string) *exec.Cmd {
	realCmd := cmd
	realArgs := args
	if c.prependCmd != "" {
		realCmd = c.prependCmd
		realArgs = append(c.prependArgs, cmd)
		realArgs = append(realArgs, args...)
	}
	return exec.Command(realCmd, realArgs...)
}
