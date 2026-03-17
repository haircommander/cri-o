package oci

import (
	"fmt"
	"sync"
	"time"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// ContainerState represents the status of a container.
type ContainerState struct {
	specs.State

	Created       time.Time `json:"created"`
	Started       time.Time `json:"started"`
	Finished      time.Time `json:"finished"`
	ExitCode      *int32    `json:"exitCode,omitempty"`
	OOMKilled     bool      `json:"oomKilled,omitempty"`
	SeccompKilled bool      `json:"seccompKilled,omitempty"`
	Error         string    `json:"error,omitempty"`
	InitPid       int       `json:"initPid,omitempty"`
	// The unix start time of the container's init PID.
	// This is used to track whether the PID we have stored
	// is the same as the corresponding PID on the host.
	InitStartTime string `json:"initStartTime,omitempty"`
	// Checkpoint/Restore related states
	CheckpointedAt time.Time `json:"checkpointedTime"`
	// ContainerMonitorProcess is used to check the liveness of the container monitor.
	// This is supposed to be immutable once set.
	ContainerMonitorProcess *ContainerMonitorProcess `json:"containerMonitorProcess,omitempty"`

	l sync.RWMutex
}

func (c *ContainerState) Status() specs.ContainerState {
	c.l.RLock()
	defer c.l.RUnlock()
	return c.Status
}

// SetInitPid initializes the InitPid and InitStartTime for the container state
// given a PID.
// These values should be set once, and not changed again.
func (c *ContainerState) SetInitPid(pid int) error {
	c.l.Lock()
	defer c.l.Unlock()

	if c.InitPid != 0 || c.InitStartTime != "" {
		return fmt.Errorf("pid and start time already initialized: %d %s", cstate.InitPid, cstate.InitStartTime)
	}

	c.InitPid = pid

	startTime, err := getPidStartTime(pid)
	if err != nil {
		return err
	}

	c.InitStartTime = startTime

	return nil
}

type stateModifier func(*ContainerState)

func (c *ContainerState) ModifyState(modify stateModifier) {
	c.l.Lock()
	defer c.l.Unlock()

	modify(c)
}

func (c *ContainerState) SetFinished(t time.Time) {
	c.l.Lock()
	defer c.l.Unlock()

	c.Finished = t
}

func (c *ContainerState) OOMKilled() bool {
	return c.OOMKilled
}

func (c *ContainerState) SetExitCode(ec *int32) {
	c.ExitCode = ec
}
