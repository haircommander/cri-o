package lib

import (
	"bufio"
	"fmt"
	"strings"
	"time"
	"os"

	epoll "github.com/mailru/easygo/netpoll"
	"github.com/gxed/eventfd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

var cgroupRoot string = "/sys/fs/cgroup"

func (c *conmonmon) registerConmon(info *conmonInfo, cgroupv2 bool) error {
	eventControl, err := processCgroupSubsystemPath(info.conmonPID, cgroupv2, "event_control")
	if err != nil {
		return errors.Wrapf(err, "failed to get event_control file for pid %d", info.conmonPID)
	}

	oomControl, err := processCgroupSubsystemPath(info.conmonPID, cgroupv2, "oom_control")
	if err != nil {
		return errors.Wrapf(err, "failed to get oom_control file for pid %d", info.conmonPID)
	}

	efd, err := eventfd.New()
	if err != nil {
		return errors.Wrapf(err, "failed to open event fd to listen for conmon OOMs")
	}
	defer efd.Close()

	cfd, err := os.OpenFile(eventControl, os.O_WRONLY, 0755)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s", eventControl)
	}
	defer cfd.Close()

	// not closed here, closed when conmon is removed
	ofd, err := os.Open(oomControl)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s", oomControl)
	}

	content := fmt.Sprintf("%d %d", efd.Fd(), ofd.Fd())
	if _, err := cfd.WriteString(content); err != nil {
		return errors.Wrapf(err, "failed to write %s to %s", content, eventControl)
	}

	c.ep.Add(efd.Fd(), epoll.EPOLLIN, info.oomKillContainer)

	time.Sleep(100*time.Second)

	return nil
}

// oomKillContainer does everything required to pretend as though the container OOM'd
// this includes killing, setting its state, and writing that state to disk
func (ci *conmonInfo) oomKillContainer(epoll.EpollEvent) {
	if err := ci.cmm.runtime.SignalContainer(ci.ctr, unix.SIGKILL); err != nil {
		// in all likelihood, we'd get here because the container was killed or stopped after we made the last state check,
		// but before we called $runtime kill. We should probably log it just to be sure.
		logrus.Errorf("Failed to spoof OOM of container %s: %v. This could be expected.", ci.ctr.ID(), err)
		return
	}
	ci.cmm.runtime.SpoofOOM(ci.ctr)
	if err := ci.cmm.server.ContainerStateToDisk(ci.ctr); err != nil {
		logrus.Errorf("Failed to save spoofed OOM state of container %s: %v", err)
	}
}


func (c *conmonmon) deregisterConmon(info *conmonInfo) {
	c.ep.Del(info.eventControlFd)
	info.oomControl.Close()
}

// TODO FIXME test with cgroup v1 and cgroup v2
func processCgroupSubsystemPath(pid int, cgroupv2 bool, subsystem string) (string, error) {
	cgroupFilePath := fmt.Sprintf("/proc/%d/cgroup", pid)
	cgroupFile, err := os.Open(cgroupFilePath)
	if err != nil {
		return "", err
	}
	defer cgroupFile.Close()

	scanner := bufio.NewScanner(cgroupFile)
	for scanner.Scan() {
		lineFields := strings.Split(scanner.Text(), ":")
		if len(lineFields) != 3 {
			return "", errors.Errorf("reading cgroup file %s resulted in invalid line %v", cgroupFilePath, lineFields)
		}
		if cgroupv2 {
			return fmt.Sprintf("%s%s", cgroupRoot, lineFields[2]), nil
		}
		subsystems := strings.Split(lineFields[1], ",")
		for _, subsystemInFile := range subsystems {
			if subsystemInFile == subsystem {
				subpathInFile := ""
				if idx := strings.Index(subsystemInFile, "="); idx > -1 {
					subpathInFile = subsystemInFile[idx:]
				}
				return fmt.Sprintf("%s/%s%s", cgroupRoot, subpathInFile, lineFields[2]), nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.Errorf("unable to find subsystem path %s in file %s", cgroupFilePath, subsystem)
}
