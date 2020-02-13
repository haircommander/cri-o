package lib

import (
	"fmt"
	"os"
	"sync"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// conmonmon is a struct responsible for monitoring conmons
// it contains a map of containers -> info on conmons, and sleeps on
// a loop, waiting for a conmon to die. if it has, it kills the associated
// container.
type conmonmon struct {
	ctrToConmons   map[*oci.Container]*conmonInfo
	oomControlToConmons map[string]*conmonInfo
	closeChan chan bool
	runtime   *oci.Runtime
	server    *ContainerServer
	lock      sync.Mutex
	watcher   *fsnotify.Watcher
}

// conmonInfo contains all necessary state to verify
// the conmon the container was originally spawned from is still running
type conmonInfo struct {
	ctr *oci.Container
	conmonPID int
	oomControlPath string
}

// newConmonmon creates a new conmonmon instance given a runtime.
// It also starts the monitoring routine
func (c *ContainerServer) newConmonmon(r *oci.Runtime) (*conmonmon, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create new watch: %v", err)
	}

	cmm := conmonmon{
		ctrToConmons:   make(map[*oci.Container]*conmonInfo),
		oomControlToConmons:   make(map[string]*conmonInfo),
		runtime:   r,
		server:    c,
		closeChan: make(chan bool, 2),
		watcher:   watcher,
	}

	go cmm.monitorConmons()

	return &cmm, nil
}

func (c *conmonmon) monitorConmons() {
	for {
		select {
		case event := <-c.watcher.Events:
			fmt.Fprintf(os.Stderr, "event: %v\n", event)
			//if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
				fmt.Fprintf(os.Stderr, "file written to %s\n", event.Name)

				c.lock.Lock()
				ci, found := c.oomControlToConmons[event.Name]
				if !found {
					logrus.Errorf("invalid oom control file %s\n", event.Name)
					// TODO FIXME should this be that?
					c.lock.Unlock()
					return
				}
				c.lock.Unlock()

				fmt.Fprintf(os.Stderr, "associated conmon %s\n", ci.conmonPID)
				c.oomKillContainer(ci.ctr)
			//}
		case err := <-c.watcher.Errors:
			//errorCh <- fmt.Errorf("watch error for container log reopen %v: %v", c.ID(), err)
			fmt.Fprintln(os.Stderr, "error found in monitoring %v\n", err)
			return
		}
	}
}

// MonitorConmon adds a container's conmon to map of those watched
func (c *conmonmon) MonitorConmon(ctr *oci.Container) error {
	// silently return if we are asked to monitor a
	// runtime type that doesn't use conmon
	if runtimeType, err := c.runtime.ContainerRuntimeType(ctr); runtimeType == config.RuntimeTypeVM {
		if err != nil {
			logrus.Debugf("error when adding conmon of %s to monitoring loop: %v", ctr.ID(), err)
		}
		fmt.Fprintf(os.Stderr, "WHAT\n")
		return nil
	}

	// TODO FIXME how do I make sure it wasn't stopped?
	//status := ctr.State().Status
	//if status != oci.ContainerStateRunning && status != oci.ContainerStateCreated {
	//	return nil
	//}

	conmonPID, err := oci.ReadConmonPidFile(ctr)
	if err != nil {
		return err
	}

	ci := &conmonInfo{
		conmonPID: conmonPID,
		ctr:       ctr,
	}

	fmt.Fprintf(os.Stderr, "registering conmon %d with fsnotify\n", conmonPID)
	if err := c.registerConmon(ci, false); err != nil {
		return errors.Wrapf(err, "failed to register conmon %d with epoll watcher", conmonPID)
	}

	return nil
}

// StopMonitoringConmon removes a container's conmon to map of those watched
func (c *conmonmon) StopMonitoringConmon(ctr *oci.Container) {
	c.lock.Lock()
	defer c.lock.Unlock()
	// we can be idempotent here, because there are multiple ways a container can
	// not be tracked anymore
	ci, ok := c.ctrToConmons[ctr]
	if !ok {
		return
	}

	fmt.Fprintf(os.Stderr, "deregistering conmon %d with fsnotify\n", ci.conmonPID)
	c.deregisterConmon(ci)

	delete(c.ctrToConmons, ctr)
	delete(c.oomControlToConmons, ci.oomControlPath)
}

// ShutdownConmonmon tells conmonmon to stop sleeping on a loop,
// and to stop monitoring
func (c *conmonmon) ShutdownConmonmon() {
	c.closeChan <- true
	c.watcher.Close()
}
