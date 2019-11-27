package lib

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	epoll "github.com/mailru/easygo/netpoll"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

type conmonmon struct {
	// ctrID to conmon
	conmons map[string]*conmonPidAndFds
	mu      sync.RWMutex
	ep      *epoll.Epoll
	server  *ContainerServer // TODO FIXME maybe I just need store
}

func (c *ContainerServer) newConmonmon() (*conmonmon, error) {
	config := epoll.EpollConfig{
		OnWaitError: epollOnError,
	}
	ep, err := epoll.EpollCreate(&config)
	if err != nil {
		return nil, err
	}

	cmm := conmonmon{
		conmons: make(map[string]*conmonPidAndFds),
		ep:      ep,
		server:  c,
	}
	return &cmm, nil
}

func epollOnError(err error) {
	logrus.Debugf(err.Error())
}

func (c *conmonmon) addConmon(conmonPID int, ctrID string) error {
	// verify container state running
	c.mu.RLock()
	if _, found := c.conmons[ctrID]; found {
		c.mu.RUnlock()
		return errors.Errorf("container ID: %s already has a registered conmon", ctrID)
	}
	c.mu.RUnlock()

	// get cgroup location of oom event
	cgroupMemoryControllerPath, err := getConmonCgroupMemoryPath(conmonPID)
	if err != nil {
		return err
	}

	oomCtlFd, eventFd, err := configureEpollFiles(cgroupMemoryControllerPath)
	if err != nil {
		return err
	}

	// open oom file at that location
	// set callback for removing container after conmon ooms
	kcb := killCB{
		ctrID:  ctrID,
		server: c.server,
	}

	conmonInfo := conmonPidAndFds{
		pid:      conmonPID,
		oomCtlFd: oomCtlFd,
		eventFd:  eventFd,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.conmons[ctrID] = &conmonInfo

	// return with epollctl
	return c.ep.Add(eventFd, epoll.EPOLLIN, kcb.callback)
}

func getConmonCgroupMemoryPath(conmonPID int) ([]byte, error) {
	const CGROUP_ROOT = "/sys/fs/cgroup"
	memoryPath := []byte{}

	cgroupPath := fmt.Sprintf("/proc/%d/cgroup", conmonPID)
	cgroupFile, err := os.Open(cgroupPath)
	if err != nil {
		return memoryPath, err
	}

	defer cgroupFile.Close()

	reader := bufio.NewReader(cgroupFile)
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			// TODO FIXME what do
			return memoryPath, errors.Errorf("Could not find the cgroup memory controller location")
		}
		parts := bytes.Split(line, []byte(":"))
		if len(parts) < 3 {
			return memoryPath, errors.Errorf("Invalid cgroup line %s in file %s", line, cgroupFile)
		}
		if string(parts[1]) != "memory" {
			continue
		}
		memoryPath = []byte(fmt.Sprintf("%s%s", CGROUP_ROOT, parts[2]))
		break
	}
	return memoryPath, nil
}

func configureEpollFiles(cgroupMemoryControllerPath []byte) (int, int, error) {
	oomFilePath := fmt.Sprintf("%s/%s", cgroupMemoryControllerPath, "memory.oom_control")
	oomFile, err := os.Open(oomFilePath)
	if err != nil {
		return -1, -1, err
	}
	// TODO FIXME cleanup on error

	eventFd, err := unix.Eventfd(0, unix.EFD_CLOEXEC)
	if err != nil {
		return -1, -1, err
	}
	// TODO FIXME cleanup on error

	eventControlPath := fmt.Sprintf("%s/cgroup.event_control")
	eventControlFile, err := os.OpenFile(eventControlPath, unix.O_WRONLY|unix.O_CLOEXEC, 0644)
	if err != nil {
		return -1, -1, err
	}
	defer eventControlFile.Close()

	if _, err := eventControlFile.Write([]byte(fmt.Sprintf("%d %d", eventFd, oomFile.Fd()))); err != nil {
		return -1, -1, err
	}

	return int(oomFile.Fd()), eventFd, nil
}

func (c *conmonmon) removeConmon(ctrID string) (lastErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// verify conmon exists
	pidAndFds, found := c.conmons[ctrID]
	if !found {
		return errors.Errorf("couldn't find associated conmon associated with container %s", ctrID)
	}
	// remove from map
	delete(c.conmons, ctrID)

	if err := c.ep.Del(pidAndFds.eventFd); err != nil {
		logrus.Debugf("error removing eventFd for %s from epoll: %v", ctrID, err)
		lastErr = err
	}

	if err := pidAndFds.closeFds(); err != nil {
		lastErr = err
	}
}

func (c *conmonmon) restore() (lastErr error) {
	defer func() {
		if lastErr == nil {
			return
		}
		if err := c.Shutdown(); err != nil {
			logrus.Debugf("An error occurred when shutting down conmonmon after a failed restore: %v", err)
		}
	}()
	// loop through containers
	for _, ctr := range c.listContainers() {
		// add container to map and register each container's conmon with the epoll instance
		if err := c.AddConmon(ctr.ID()); err != nil {
			logrus.Debugf("encountered error adding container %s to conmonmon: %v", ctr.ID(), err)
			lastErr = err
			return
		}
	}
}

func (c *conmonmon) shutdown() (lastErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ep.Close(); err != nil {
		logrus.Debugf("error closing epoll instance %v", err)
		lastErr = err
	}

	for _, conmon := range c.conmons {
		if err := conmon.closeFds(); err != nil {
			lastErr = err
		}
	}
}

type conmonPidAndFds struct {
	pid      int
	oomCtlFd int
	eventFd  int
}

func (c *conmonPidAndFds) closeFds() (lastErr error) {
	if err := syscall.Close(pidAndFds.eventFd); err != nil {
		logrus.Debugf("error closing eventFd for %s: %v", ctrID, err)
		lastErr = err
	}
	if err := syscall.Close(pidAndFds.oomCtlFd); err != nil {
		logrus.Debugf("error closing oomCtlFd for %s: %v", ctrID, err)
		lastErr = err
	}
}

type killCB struct {
	ctrID  string
	server *ContainerServer
}

func (k *killCB) callback(events epoll.EpollEvent) {
	// TODO FIXME make sure this is as much as we're supposed to listen to
	if events|epoll.EPOLLIN != 1 {
		return
	}
	// write oom file
	if err := k.createOOMFile(); err != nil {
		logrus.Debugf(err.Error())
	}
	// kill container
	k.server.ContainerKill(k.ctrID, unix.SIGKILL)
}

func (k *killCB) createOOMFile() error {
	containerPath, err := k.server.store.ContainerRunDirectory(k.ctrID)
	if err != nil {
		return err
	}
	oomFd, err := os.Create(filepath.Join(containerPath, "oom"))
	if err != nil {
		return err
	}
	oomFd.Close()
	return nil
}
