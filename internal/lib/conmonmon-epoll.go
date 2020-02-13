package lib

import (
	"bufio"
	"fmt"
	"strings"
	"os"
	"path/filepath"

	"github.com/cri-o/cri-o/internal/oci"
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
	fmt.Fprintf(os.Stderr, "got %s\n", oomControl)

	eventControl := filepath.Join(cgroupMemoryPath, "cgroup.event_control")
	fmt.Fprintf(os.Stderr, "got %s\n", oomControl)

	info.oomControlPath = oomControl

	c.lock.Lock()
	fmt.Fprintf(os.Stderr, "looking for %s\n", oomControl)
	if _, found := c.oomControlToConmons[oomControl]; found {
		c.lock.Unlock()
		return errors.Errorf("container ID: %s already has a registered conmon", info.ctr.ID())
	}

	if err := c.watcher.Add(oomControl); err != nil {
		return errors.Wrapf(err, "failed to watch %s", oomControl)
	}

	fmt.Fprintf(os.Stderr, "watching %s\n", oomControl)
	c.oomControlToConmons[oomControl] = info
	c.ctrToConmons[info.ctr] = info

	fmt.Fprintf(os.Stderr, "%s is saved\n", oomControl)
	c.lock.Unlock()


	return nil
}

// oomKillContainer does everything required to pretend as though the container OOM'd
// this includes killing, setting its state, and writing that state to disk
func (c *conmonmon) oomKillContainer(ctr *oci.Container) {
	fmt.Fprintf(os.Stderr, "oom killing container\n")
	if err := c.runtime.SignalContainer(ctr, unix.SIGKILL); err != nil {
		// in all likelihood, we'd get here because the container was killed or stopped after we made the last state check,
		// but before we called $runtime kill. We should probably log it just to be sure.
		logrus.Errorf("Failed to spoof OOM of container %s: %v. This could be expected.", ctr.ID(), err)
		return
	}
	c.runtime.SpoofOOM(ctr)
	if err := c.server.ContainerStateToDisk(ctr); err != nil {
		logrus.Errorf("Failed to save spoofed OOM state of container %s: %v", err)
	}
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
	c.watcher.Remove(info.oomControlPath)
}
