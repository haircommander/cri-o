// +build test
// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package oci

import (
	runnerMock "github.com/cri-o/cri-o/test/mocks/cmdrunner"
)

// SetState sets the container state
func (c *Container) SetState(state *ContainerState) {
	c.state = state
}

// SetStateAndSpoofPid sets the container state
// as well as configures the ProcessInformation to succeed
// useful for tests that don't care about pid handling
func (c *Container) SetStateAndSpoofPid(state *ContainerState) {
	// we do this hack because most of the tests
	// don't care to set a Pid.
	// but rely on calling Pid()
	if state.Pid == 0 {
		state.Pid = 1
		state.SetInitPid(state.Pid) // nolint:errcheck
	}
	c.state = state
}

func (r *Runtime) MockImplForContainer(id string, runner *runnerMock.MockCommandRunner) {
	r.runtimeImplMapMutex.Lock()
	r.runtimeImplMap[id] = &runtimeOCI{
		Runtime: r,
		path:    "command",
		root:    "runRoot",
		runner:  runner,
	}
	r.runtimeImplMapMutex.Unlock()
}
