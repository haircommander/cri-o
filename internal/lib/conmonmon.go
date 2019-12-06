package lib

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containers/psgo"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

var sleepTime = 5 * time.Minute

// conmonmon is a struct responsible for monitoring conmons
// it contains a map of containers -> conmonPID, and sleeps on
// a loop, waiting for a conmon to die. if it has, it kills the associated
// container.
type conmonmon struct {
	conmons   map[*oci.Container]int
	mu        sync.RWMutex
	closeChan chan bool
	runtime   *oci.Runtime
	server    *ContainerServer
}

// newConmonmon creates a new conmonmon instance
// given a runtime. It also starts the monitoring routine
func (c *ContainerServer) newConmonmon(r *oci.Runtime) *conmonmon {
	cmm := conmonmon{
		conmons:   make(map[*oci.Container]int),
		runtime:   r,
		server:    c,
		closeChan: make(chan bool),
	}
	go cmm.monitorConmons()
	return &cmm
}

// monitorConmons sits on a loop and sleeps.
// after waking, it signals to the conmons, checking
// if they're still alive.
func (c *conmonmon) monitorConmons() {
	for {
		select {
		case <-time.After(sleepTime):
			c.signalConmons()
		case <-c.closeChan:
			return
		}
	}
}

// signalConmons loops through all available conmons and
// sends a kill 0 to them, checking if they're still alive
// if they're not, the container is killed and we spoof
// an OOM event for the container
func (c *conmonmon) signalConmons() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for ctr, conmonPID := range c.conmons {
		status := ctr.State().Status
		if status == oci.ContainerStateRunning {
			if err := c.verifyConmonValid(ctr.ID(), conmonPID); err != nil {
				logrus.Debugf("conmon pid %d invalid: %v. Killing container %s", conmonPID, err, ctr.ID())
				if err := c.runtime.SignalContainer(ctr, unix.SIGKILL); err != nil {
					logrus.Debugf(err.Error())
				}
				delete(c.conmons, ctr)
				oci.SpoofOOM(ctr)
				if err := c.server.ContainerStateToDisk(ctr); err != nil {
					logrus.Debugf(err.Error())
				}
			}
		}
	}
}

func (c *conmonmon) verifyConmonValid(ctrID string, pid int) error {
	// verify it's still a valid pid
	if err := unix.Kill(pid, 0); err != nil {
		return err
	}

	// verify we are in the same mnt namespace as the pid
	conmonMntNS, err := filepath.EvalSymlinks(fmt.Sprintf("/proc/%d/ns/mnt", pid))
	if err != nil {
		return err
	}
	crioMntNS, err := filepath.EvalSymlinks("/proc/self/ns/mnt")
	if err != nil {
		return err
	}
	if conmonMntNS != crioMntNS {
		return errors.Errorf("pid is running in a different mnt namespace")
	}

	psInfo, err := psgo.ProcessInfoByPids([]string{strconv.Itoa(pid)}, []string{"args"})
	if err != nil {
		return err
	}
	if len(psInfo) != 2 || len(psInfo[1]) != 1 {
		return errors.Errorf("insufficient ps information from pid")
	}

	args := strings.Split(psInfo[1][0], " ")
	if args[0] != oci.ConmonPath(c.runtime) {
		return errors.Errorf("pid is running with a different conmon path %s", args[0])
	}

	if !strings.Contains(psInfo[1][0], ctrID) {
		return errors.Errorf("conmon with pid wasn't called with container ID %s", ctrID)
	}

	return nil
}

// AddConmon adds a container's conmon to map of those watched
func (c *conmonmon) AddConmon(ctr *oci.Container) error {
	// silently return if we are asked to monitor a
	// runtime type that doesn't use conmon
	if runtimeType, err := c.runtime.ContainerRuntimeType(ctr); runtimeType != oci.RuntimeTypeOCI {
		if err != nil {
			logrus.Debugf("error when adding conmon of %s to monitoring loop: %v", ctr.ID(), err)
		}
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, found := c.conmons[ctr]; found {
		return errors.Errorf("container ID: %s already has a registered conmon", ctr.ID())
	}

	if ctr.State().Status != oci.ContainerStateRunning && ctr.State().Status != oci.ContainerStateCreated {
		return nil
	}

	conmonPID, err := oci.ReadConmonPidFile(ctr)
	if err != nil {
		return err
	}

	c.conmons[ctr] = conmonPID

	return nil
}

// RemoveConmon removes a container's conmon to map of those watched
func (c *conmonmon) RemoveConmon(ctr *oci.Container) (lastErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// verify conmon exists
	_, found := c.conmons[ctr]
	if !found {
		// we can be idempotent here, because there are multiple ways a container can
		// not be tracked anymore
		return nil
	}
	// remove from map
	delete(c.conmons, ctr)

	return lastErr
}

// ShutdownConmonmon tells conmonmon to stop sleeping on a loop,
// and to stop monitoring
func (c *conmonmon) ShutdownConmonmon() {
	c.closeChan <- true
}
