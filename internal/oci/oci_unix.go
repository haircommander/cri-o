//go:build !windows
// +build !windows

package oci

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/containers/storage/pkg/pools"
	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

func Kill(pid int) error {
	err := unix.Kill(pid, unix.SIGKILL)
	if err != nil && err != unix.ESRCH {
		return fmt.Errorf("failed to kill process: %w", err)
	}
	return nil
}

func setSize(fd uintptr, size remotecommand.TerminalSize) error {
	winsize := &unix.Winsize{Row: size.Height, Col: size.Width}
	return unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, winsize)
}

func ttyCmd(execCmd *exec.Cmd, stdin io.Reader, stdout io.WriteCloser, resize <-chan remotecommand.TerminalSize) error {
	p, err := pty.Start(execCmd)
	if err != nil {
		return err
	}
	defer p.Close()

	// make sure to close the stdout stream
	defer stdout.Close()

	kubecontainer.HandleResizing(resize, func(size remotecommand.TerminalSize) {
		if err := setSize(p.Fd(), size); err != nil {
			logrus.Warnf("Unable to set terminal size: %v", err)
		}
	})

	var stdinErr, stdoutErr error
	stdinErrChan := make(chan error, 1)
	stdoutErrChan := make(chan error, 1)
	cmdErrChan := make(chan error, 1)
	if stdin != nil {
		go func() {
			_, stdinErr = pools.Copy(p, stdin)
			stdinErrChan <- stdinErr
		}()
	}

	if stdout != nil {
		go func() {
			_, stdoutErr = pools.Copy(stdout, p)
			stdoutErrChan <- stdoutErr
		}()
	}

	go func() {
		cmdErrChan <- execCmd.Wait()
	}()

	return finishExec(execCmd, cmdErrChan, stdinErrChan, stdoutErrChan, nil)
}

func finishExec(execCmd *exec.Cmd, cmdErrChan, stdinErrChan, stdoutErrChan, stderrErrChan chan error) error {
	var (
		stream string
		err    error
	)
	if stderrErrChan == nil {
		// noop for TTY
		stderrErrChan = make(chan error, 1)
	}
	select {
	case err = <-cmdErrChan:
		return err
	case err = <-stdinErrChan:
		stream = "Stdin"
	case err = <-stdoutErrChan:
		stream = "Stdout"
	case err = <-stderrErrChan:
		stream = "Stderr"
	}
	// We need to always kill and wait on this process.
	// Failing to do so will cause us to leak a process.
	killErr := execCmd.Process.Kill()
	waitErr := <-cmdErrChan
	if killErr != nil {
		err = fmt.Errorf("failed to kill %+v after failing with: %w", killErr, err)
	}
	// Per https://pkg.go.dev/os#ProcessState.ExitCode, the exit code is -1 when the process died because
	// of a signal. We expect this in this case, as we've just killed it with a signal. Don't append the
	// error in this case to reduce noise.
	if exitErr, ok := waitErr.(*exec.ExitError); !ok || exitErr.ExitCode() != -1 {
		err = fmt.Errorf("failed to wait %+v after failing with: %w", waitErr, err)
	}

	if err != nil {
		logrus.Warnf("%s copy error: %v", stream, err)
	}

	return err
}
