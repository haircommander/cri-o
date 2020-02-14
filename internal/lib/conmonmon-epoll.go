package lib

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"strings"
	"os"
	"path/filepath"

	epoll "github.com/mailru/easygo/netpoll"
	"github.com/gxed/eventfd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

var cgroupRoot string = "/sys/fs/cgroup"

func (c *conmonmon) registerConmon(info *conmonInfo, cgroupv2 bool) error {
	fmt.Fprintf(os.Stderr, "finding cgroup files for %d\n", info.conmonPID)
	cgroupMemoryPath, err := processCgroupSubsystemPath(info.conmonPID, cgroupv2, "memory")
	if err != nil {
		return errors.Wrapf(err, "failed to get event_control file for pid %d", info.conmonPID)
	}

	fmt.Fprintf(os.Stderr, "found cgroup subsystem path %s for %d\n", cgroupMemoryPath, info.conmonPID)

	oomControl := filepath.Join(cgroupMemoryPath, "memory.oom_control")
	eventControl := filepath.Join(cgroupMemoryPath, "cgroup.event_control")

	fmt.Fprintf(os.Stderr, "got %s %s\n", oomControl, eventControl)

	// not closed here, closed when conmon is removed
	efd, err := eventfd.New()
	if err != nil {
		return errors.Wrapf(err, "failed to open event fd to listen for conmon OOMs")
	}

	cfd, err := os.OpenFile(eventControl, os.O_WRONLY, 0755)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s", eventControl)
	}
	defer cfd.Close()

	// not closed here, closed when conmon is removed
	// TODO FIXME if we error this should be closed
	ofd, err := os.Open(oomControl)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s", oomControl)
	}

	content := fmt.Sprintf("%d %d", efd.Fd(), ofd.Fd())
	fmt.Fprintf(os.Stderr, "writing content: %s to %s\n", content, cfd.Name())
	if _, err := cfd.WriteString(content); err != nil {
		return errors.Wrapf(err, "failed to write %s to %s", content, eventControl)
	}

	fmt.Fprintf(os.Stderr, "adding %d to epoll\n", efd.Fd())

	go func() {
		val, err := efd.ReadEvents()
		fmt.Fprintf(os.Stderr, "%d: %v", val, err)
	}()
	//if err := c.ep.Add(efd.Fd(), epoll.EPOLLIN | epoll.EPOLLERR | epoll.EPOLLONESHOT, info.oomKillContainer); err != nil {
	//	return errors.Wrapf(err, "failed to register %d with epoll", efd.Fd())
	//}

	info.eventFD = efd
	info.oomControl = ofd

	return nil
}

// oomKillContainer does everything required to pretend as though the container OOM'd
// this includes killing, setting its state, and writing that state to disk
func (ci *conmonInfo) oomKillContainer(e epoll.EpollEvent) {
	fmt.Fprintf(os.Stderr, "oom killing container %v\n", e)

    b, err := ioutil.ReadAll(ci.oomControl)
    if err != nil {
		logrus.Errorf("can't read file %s to check if oom happened: %v", ci.oomControl.Name(), err)
		return
    }

	if !strings.Contains(string(b), "oom_kill 1") {
		// TODO FIXME probably debug
		logrus.Errorf("caught %v on control file, but no OOM happened %s", e, string(b))
		return
	}

	fmt.Fprintf(os.Stderr, "found oom_kill 1\n")
	if err := ci.cmm.runtime.SignalContainer(ci.ctr, unix.SIGKILL); err != nil {
		// in all likelihood, we'd get here because the container was killed or stopped after we made the last state check,
		// but before we called $runtime kill. We should probably log it just to be sure.
		logrus.Errorf("Failed to spoof OOM of container %s: %v. This could be expected.", ci.ctr.ID(), err)
		return
	}
	fmt.Fprintf(os.Stderr, "signaled container\n")

	ci.cmm.runtime.SpoofOOM(ci.ctr)

	fmt.Fprintf(os.Stderr, "spoofed oom\n")

	if err := ci.cmm.server.ContainerStateToDisk(ci.ctr); err != nil {
		logrus.Errorf("Failed to save spoofed OOM state of container %s: %v", err)
	}
	fmt.Fprintf(os.Stderr, "wrote state to disk\n")

	ci.cmm.deregisterConmon(ci)
}

// TODO FIXME test with cgroup v1 and cgroup v2
func processCgroupSubsystemPath(pid int, cgroupv2 bool, subsystem string) (string, error) {
	cgroupFilePath := fmt.Sprintf("/proc/%d/cgroup", pid)
	cgroupFile, err := os.Open(cgroupFilePath)
	if err != nil {
		return "", err
	}
	defer cgroupFile.Close()
	fmt.Fprintf(os.Stdin, "finding path off of %s\n", cgroupFilePath)

	scanner := bufio.NewScanner(cgroupFile)
	for scanner.Scan() {
		lineFields := strings.Split(scanner.Text(), ":")
		if len(lineFields) != 3 {
			return "", errors.Errorf("reading cgroup file %s resulted in invalid line %v", cgroupFilePath, lineFields)
		}
		if cgroupv2 {
			subsystemPath := filepath.Join(cgroupRoot, subsystem, lineFields[2])
			fmt.Fprintf(os.Stdin, "found %s\n", subsystemPath)
			return subsystemPath, nil
		}
		subsystems := strings.Split(lineFields[1], ",")
		for _, subsystemInFile := range subsystems {
			if subsystemInFile == subsystem {
				subpathInFile := ""
				if idx := strings.Index(subsystemInFile, "="); idx > -1 {
					subpathInFile = subsystemInFile[idx:]
				}
				subsystemPath := filepath.Join(cgroupRoot, subsystem, subpathInFile, lineFields[2])
				fmt.Fprintf(os.Stdin, "found %s\n", subsystemPath)
				return subsystemPath, nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.Errorf("unable to find subsystem path %s in file %s", cgroupFilePath, subsystem)
}

func (c *conmonmon) deregisterConmon(info *conmonInfo) {
	fmt.Fprintf(os.Stderr, "deregistering conmon %d\n", info.conmonPID)
	//c.ep.Del(info.eventFD.Fd())
	info.oomControl.Close()
	info.eventFD.Close()
}
