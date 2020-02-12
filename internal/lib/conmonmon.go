package lib

import (
	"os"
	"sync"
	"time"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
	epoll "github.com/mailru/easygo/netpoll"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// the time between checking the registered conmons
var sleepTime = 5 * time.Minute

// conmonmon is a struct responsible for monitoring conmons
// it contains a map of containers -> info on conmons, and sleeps on
// a loop, waiting for a conmon to die. if it has, it kills the associated
// container.
type conmonmon struct {
	conmons   map[*oci.Container]*conmonInfo
	closeChan chan bool
	runtime   *oci.Runtime
	server    *ContainerServer
	lock      sync.Mutex
	ep        *epoll.Epoll
}

// conmonInfo contains all necessary state to verify
// the conmon the container was originally spawned from is still running
type conmonInfo struct {
	conmonPID int
	oomControl *os.File
	eventControlFd int
	ctr *oci.Container
	cmm *conmonmon
}

// newConmonmon creates a new conmonmon instance given a runtime.
// It also starts the monitoring routine
func (c *ContainerServer) newConmonmon(r *oci.Runtime) (*conmonmon, error) {
	// create epoll
	ep, err := epoll.EpollCreate(nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create epoll to listen for conmon OOMs")
	}

	cmm := conmonmon{
		conmons:   make(map[*oci.Container]*conmonInfo),
		runtime:   r,
		server:    c,
		closeChan: make(chan bool, 2),
		ep:		   ep,
	}

	return &cmm, nil
}

// MonitorConmon adds a container's conmon to map of those watched
func (c *conmonmon) MonitorConmon(ctr *oci.Container) error {
	// silently return if we are asked to monitor a
	// runtime type that doesn't use conmon
	if runtimeType, err := c.runtime.ContainerRuntimeType(ctr); runtimeType == config.RuntimeTypeVM {
		if err != nil {
			logrus.Debugf("error when adding conmon of %s to monitoring loop: %v", ctr.ID(), err)
		}
		return nil
	}

	status := ctr.State().Status
	if status != oci.ContainerStateRunning && status != oci.ContainerStateCreated {
		return nil
	}

	conmonPID, err := oci.ReadConmonPidFile(ctr)
	if err != nil {
		return err
	}

	ci := &conmonInfo{
		conmonPID: conmonPID,
		ctr:       ctr,
		cmm: c,
	}

	if err := c.registerConmon(ci, false); err != nil {
		return errors.Wrapf(err, "failed to register conmon %d with epoll watcher", conmonPID)
	}

	c.lock.Lock()
	if _, found := c.conmons[ctr]; found {
		c.lock.Unlock()
		return errors.Errorf("container ID: %s already has a registered conmon", ctr.ID())
	}
	c.conmons[ctr] = ci
	c.lock.Unlock()

	return nil
}

// StopMonitoringConmon removes a container's conmon to map of those watched
func (c *conmonmon) StopMonitoringConmon(ctr *oci.Container) {
	c.lock.Lock()
	defer c.lock.Unlock()
	// we can be idempotent here, because there are multiple ways a container can
	// not be tracked anymore
	if _, ok := c.conmons[ctr]; !ok {
		return
	}

	delete(c.conmons, ctr)
}

// ShutdownConmonmon tells conmonmon to stop sleeping on a loop,
// and to stop monitoring
func (c *conmonmon) ShutdownConmonmon() {
	c.closeChan <- true
	c.ep.Close()
}
