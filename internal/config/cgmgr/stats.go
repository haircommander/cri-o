package cgmgr

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containers/podman/v3/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func populateSandboxCgroupStatsFromPath(cgroupPath string, stats *types.PodSandboxStats) error {
	// checks cgroup just for the container, not the entire pod
	cg, err := cgroups.Load(cgroupPath)
	if err != nil {
		return errors.Wrapf(err, "unable to load cgroup at %s", cgroupPath)
	}

	cgroupStats, err := cg.Stat()
	if err != nil {
		return errors.Wrap(err, "unable to obtain cgroup stats")
	}
	systemNano := time.Now().UnixNano()
	stats.CPU = createCPUStats(systemNano, cgroupStats)
	stats.Memory, err = createMemoryStats(systemNano, cgroupStats, cgroupPath)
	stats.Process = createProcessStats(systemNano, cgroupStats)
	if err != nil {
		return err
	}
	return nil
}

func populateContainerCgroupStatsFromPath(cgroupPath string, stats *types.ContainerStats) error {
	// checks cgroup just for the container, not the entire pod
	cg, err := cgroups.Load(cgroupPath)
	if err != nil {
		return errors.Wrapf(err, "unable to load cgroup at %s", cgroupPath)
	}

	cgroupStats, err := cg.Stat()
	if err != nil {
		return errors.Wrap(err, "unable to obtain cgroup stats")
	}
	systemNano := time.Now().UnixNano()
	stats.CPU = createCPUStats(systemNano, cgroupStats)
	stats.Memory, err = createMemoryStats(systemNano, cgroupStats, cgroupPath)
	if err != nil {
		return err
	}
	return nil
}

func createCPUStats(systemNano int64, cgroupStats *cgroups.Metrics) *types.CPUUsage {
	return &types.CPUUsage{
		Timestamp:            systemNano,
		UsageCoreNanoSeconds: &types.UInt64Value{Value: cgroupStats.CPU.Usage.Total},
	}
}

func createMemoryStats(systemNano int64, cgroupStats *cgroups.Metrics, cgroupPath string) (*types.MemoryUsage, error) {
	memUsage := cgroupStats.Memory.Usage.Usage
	memLimit := MemLimitGivenSystem(cgroupStats.Memory.Usage.Limit)

	memory := &types.MemoryUsage{
		Timestamp:       systemNano,
		WorkingSetBytes: &types.UInt64Value{},
		RssBytes:        &types.UInt64Value{},
		PageFaults:      &types.UInt64Value{},
		MajorPageFaults: &types.UInt64Value{},
		UsageBytes:      &types.UInt64Value{Value: memUsage},
		AvailableBytes:  &types.UInt64Value{Value: memUsage - memLimit},
	}

	if err := updateWithMemoryStats(cgroupPath, memory, memUsage); err != nil {
		return memory, errors.Wrap(err, "unable to update with memory.stat info")
	}
	return memory, nil
}

// MemLimitGivenSystem limit returns the memory limit for a given cgroup
// If the configured memory limit is larger than the total memory on the sys, the
// physical system memory size is returned
func MemLimitGivenSystem(cgroupLimit uint64) uint64 {
	si := &syscall.Sysinfo_t{}
	err := syscall.Sysinfo(si)
	if err != nil {
		return cgroupLimit
	}

	// conversion to uint64 needed to build on 32-bit
	// but lint complains about unnecessary conversion
	// see: pr#2409
	physicalLimit := uint64(si.Totalram) //nolint:unconvert
	if cgroupLimit > physicalLimit {
		return physicalLimit
	}
	return cgroupLimit
}

// updateWithMemoryStats updates the ContainerStats object with info
// from cgroup's memory.stat. Returns an error if the file does not exists,
// or not parsable.
func updateWithMemoryStats(path string, memory *types.MemoryUsage, usage uint64) error {
	var filename, inactive string
	var totalInactive uint64
	// TODO FIXME
	if node.CgroupIsV2() {
		filename = filepath.Join("/sys/fs/cgroup", path, "memory.stat")
		inactive = "inactive_file "
	} else {
		filename = filepath.Join("/sys/fs/cgroup/memory", path, "memory.stat")
		inactive = "total_inactive_file "
	}

	toUpdate := []struct {
		prefix string
		field  *uint64
	}{
		{inactive, &totalInactive},
		{"rss ", &memory.RssBytes.Value},
		{"pgfault ", &memory.PageFaults.Value},
		{"pgmajfault ", &memory.MajorPageFaults.Value},
	}

	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		for _, field := range toUpdate {
			if !strings.HasPrefix(scanner.Text(), field.prefix) {
				continue
			}
			val, err := strconv.Atoi(
				strings.TrimPrefix(scanner.Text(), field.prefix),
			)
			if err != nil {
				return errors.Wrapf(err, "unable to parse %s", field)
			}
			valUint := uint64(val)
			field.field = &valUint
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if usage > totalInactive {
		memory.WorkingSetBytes.Value = usage - totalInactive
	} else {
		logrus.Warnf(
			"unable to account working set stats: total_inactive_file (%d) > memory usage (%d)",
			totalInactive, usage,
		)
	}

	return nil
}

func createProcessStats(systemNano int64, cgroupStats *cgroups.Metrics) *types.ProcessStats {
	return &types.ProcessStats{
		Timestamp:    systemNano,
		ProcessCount: &types.UInt64Value{Value: cgroupStats.Pids.Current},
	}
}
