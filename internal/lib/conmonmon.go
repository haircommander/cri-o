package lib

import (
	"strconv"
	"time"

	"github.com/containers/psgo"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

var sleepTime = 5 * time.Minute

type conmonInfo struct {
	ctr       *oci.Container
	conmonPID int
	startTime string
}

// conmonmon is a struct responsible for monitoring conmons
// it contains a map of containers -> conmonPID, and sleeps on
// a loop, waiting for a conmon to die. if it has, it kills the associated
// container.
type conmonmon struct {
	conmons   cmap.ConcurrentMap
	closeChan chan bool
	runtime   *oci.Runtime
	server    *ContainerServer
}

// newConmonmon creates a new conmonmon instance
// given a runtime. It also starts the monitoring routine
func (c *ContainerServer) newConmonmon(r *oci.Runtime) *conmonmon {
	cmm := conmonmon{
		conmons:   cmap.New(),
		runtime:   r,
		server:    c,
		closeChan: make(chan bool, 2),
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
	for item := range c.conmons.IterBuffered() {
		ctr := item.Val.(*conmonInfo).ctr
		conmonPID := item.Val.(*conmonInfo).conmonPID
		startTime := item.Val.(*conmonInfo).startTime

		ctrID := item.Key
		status := ctr.State().Status

		if status == oci.ContainerStateRunning {
			if err := c.verifyConmonValid(startTime, conmonPID); err != nil {
				logrus.Debugf("conmon pid %d invalid: %v. Killing container %s", conmonPID, err, ctrID)
				c.conmons.Remove(ctrID)
				go func() {
					if err := c.runtime.SignalContainer(ctr, unix.SIGKILL); err != nil {
						logrus.Debugf(err.Error())
					}
					oci.SpoofOOM(ctr)
					if err := c.server.ContainerStateToDisk(ctr); err != nil {
						logrus.Debugf(err.Error())
					}
				}()
			}
		}
	}
}

func (c *conmonmon) verifyConmonValid(savedStart string, pid int) error {
	// verify it's still a valid pid
	if err := unix.Kill(pid, 0); err != nil {
		return err
	}

	startTime, err := getProcessStartTime(pid)
	if err != nil {
		return err
	}

	if startTime != savedStart {
		return errors.Errorf("pids found to differ in stime: recoreded %s found %s", savedStart, startTime)
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

	if c.conmons.Has(ctr.ID()) {
		return errors.Errorf("container ID: %s already has a registered conmon", ctr.ID())
	}

	if ctr.State().Status != oci.ContainerStateRunning && ctr.State().Status != oci.ContainerStateCreated {
		return nil
	}

	conmonPID, err := oci.ReadConmonPidFile(ctr)
	if err != nil {
		return err
	}

	startTime, err := getProcessStartTime(conmonPID)
	if err != nil {
		return err
	}

	ci := &conmonInfo{
		conmonPID: conmonPID,
		ctr:       ctr,
		startTime: startTime,
	}
	c.conmons.Set(ctr.ID(), ci)

	return nil
}

func getProcessStartTime(pid int) (string, error) {
	psInfo, err := psgo.ProcessInfoByPids([]string{strconv.Itoa(pid)}, []string{"stime"})
	if err != nil {
		return "", err
	}
	if len(psInfo) != 2 || len(psInfo[1]) != 1 {
		return "", errors.Errorf("insufficient ps information from pid")
	}

	return psInfo[1][0], nil
}

// RemoveConmon removes a container's conmon to map of those watched
func (c *conmonmon) RemoveConmon(ctr *oci.Container) {
	// verify conmon exists
	if !c.conmons.Has(ctr.ID()) {
		// we can be idempotent here, because there are multiple ways a container can
		// not be tracked anymore
		return
	}
	// remove from map
	c.conmons.Remove(ctr.ID())
}

// ShutdownConmonmon tells conmonmon to stop sleeping on a loop,
// and to stop monitoring
func (c *conmonmon) ShutdownConmonmon() {
	c.closeChan <- true
}