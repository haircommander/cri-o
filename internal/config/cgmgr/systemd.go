// +build linux

package cgmgr

import (
	"encoding/binary"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"
	libctr "github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	cgcfgs "github.com/opencontainers/runc/libcontainer/configs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/willf/bitset"
	"golang.org/x/sys/unix"
)

const defaultSystemdParent = "system.slice"

// SystemdManager is the parent type of SystemdV{1,2}Manager.
// it defines all of the common functionality between V1 and V2
type SystemdManager struct {
	memoryPath, memoryMaxFile string
	libctrManager             libctr.Manager
}

// Name returns the name of the cgroup manager (systemd)
func (*SystemdManager) Name() string {
	return systemdCgroupManager
}

// IsSystemd returns that it is a systemd cgroup manager
func (*SystemdManager) IsSystemd() bool {
	return true
}

// ContainerCgroupPath takes arguments sandbox parent cgroup and container ID and returns
// the cgroup path for that containerID. If parentCgroup is empty, it
// uses the default parent system.slice
func (*SystemdManager) ContainerCgroupPath(sbParent, containerID string) string {
	parent := defaultSystemdParent
	if sbParent != "" {
		parent = sbParent
	}
	return parent + ":" + crioPrefix + ":" + containerID
}

// ContainerCgroupAbsolutePath takes arguments sandbox parent cgroup and container ID and
// returns the cgroup path on disk for that containerID. If parentCgroup is empty, it
// uses the default parent system.slice
func (*SystemdManager) ContainerCgroupAbsolutePath(sbParent, containerID string) (string, error) {
	parent := defaultSystemdParent
	if sbParent != "" {
		parent = sbParent
	}
	logrus.Debugf("Expanding systemd cgroup slice %v", parent)
	cgroup, err := systemd.ExpandSlice(parent)
	if err != nil {
		return "", errors.Wrapf(err, "error expanding systemd slice to get container %s stats", containerID)
	}

	return filepath.Join(cgroup, crioPrefix+"-"+containerID+".scope"), nil
}

// MoveConmonToCgroup takes the container ID, cgroup parent, conmon's cgroup (from the config) and conmon's PID
// It attempts to move conmon to the correct cgroup.
// cgroupPathToClean should always be returned empty. It is part of the interface to return the cgroup path
// that cri-o is responsible for cleaning up upon the container's death.
// Systemd takes care of this cleaning for us, so return an empty string
func (*SystemdManager) MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int) (cgroupPathToClean string, _ error) {
	if strings.HasSuffix(conmonCgroup, ".slice") {
		cgroupParent = conmonCgroup
	}
	conmonUnitName := fmt.Sprintf("crio-conmon-%s.scope", cid)

	// Set the systemd KillSignal to SIGPIPE that conmon ignores.
	// This helps during node shutdown so that conmon waits for the container
	// to exit and doesn't forward the SIGTERM that it gets.
	killSignalProp := systemdDbus.Property{
		Name:  "KillSignal",
		Value: dbus.MakeVariant(int(unix.SIGPIPE)),
	}
	logrus.Debugf("Running conmon under slice %s and unitName %s", cgroupParent, conmonUnitName)
	if err := RunUnderSystemdScope(pid, cgroupParent, conmonUnitName, killSignalProp); err != nil {
		return "", errors.Wrapf(err, "failed to add conmon to systemd sandbox cgroup")
	}
	// return empty string as path because cgroup cleanup is done by systemd
	return "", nil
}

// SandboxCgroupPath takes the sandbox parent, and sandbox ID. It
// returns the cgroup parent, cgroup path, and error.
// It also checks there is enough memory in the given cgroup
func (m *SystemdManager) SandboxCgroupPath(sbParent, sbID string) (cgParent, cgPath string, _ error) {
	if sbParent == "" {
		return "", "", nil
	}

	if !strings.HasSuffix(filepath.Base(sbParent), ".slice") {
		return "", "", fmt.Errorf("cri-o configured with systemd cgroup manager, but did not receive slice as parent: %s", sbParent)
	}

	cgParent = convertCgroupFsNameToSystemd(sbParent)
	slicePath, err := systemd.ExpandSlice(cgParent)
	if err != nil {
		return "", "", errors.Wrapf(err, "expanding systemd slice path for %q", cgParent)
	}

	if err := verifyCgroupHasEnoughMemory(slicePath, m.memoryPath, m.memoryMaxFile); err != nil {
		return "", "", err
	}

	cgPath = cgParent + ":" + crioPrefix + ":" + sbID

	return cgParent, cgPath, nil
}

// convertCgroupFsNameToSystemd converts an expanded cgroupfs name to its systemd name.
// For example, it will convert test.slice/test-a.slice/test-a-b.slice to become test-a-b.slice
func convertCgroupFsNameToSystemd(cgroupfsName string) string {
	// TODO: see if libcontainer systemd implementation could use something similar, and if so, move
	// this function up to that library.  At that time, it would most likely do validation specific to systemd
	// above and beyond the simple assumption here that the base of the path encodes the hierarchy
	// per systemd convention.
	return path.Base(cgroupfsName)
}

// CreateSandboxCgroup calls the helper function createSandboxCgroup for this manager.
func (m *SystemdManager) CreateSandboxCgroup(sbParent, containerID string) error {
	return createSandboxCgroup(sbParent, containerID, m)
}

func (m *SystemdManager) Apply(sbParent string, cg *cgcfgs.Cgroup) error {
	conn, err := systemdDbus.New()
	if err != nil {
		return err
	}
	properties, err := genV1ResourcesProperties(cg, conn)
	if err != nil {
		return err
	}
	return conn.SetUnitProperties(sbParent, true, properties...)
}

func genV1ResourcesProperties(c *cgcfgs.Cgroup, conn *systemdDbus.Conn) ([]systemdDbus.Property, error) {
	var properties []systemdDbus.Property
	r := c.Resources

	if r.CpuShares != 0 {
		properties = append(properties,
			newProp("CPUShares", r.CpuShares))
	}

	if err := addCpuset(conn, &properties, r.CpusetCpus, r.CpusetMems); err != nil {
		return nil, err
	}

	return properties, nil
}

func addCpuset(conn *systemdDbus.Conn, props *[]systemdDbus.Property, cpus, mems string) error {
	if cpus == "" && mems == "" {
		return nil
	}

	// TODO FIXME reimplement this
	// systemd only supports AllowedCPUs/AllowedMemoryNodes since v244
	//sdVer := systemdVersion(conn)
	//if sdVer < 244 {
	//	logrus.Debugf("systemd v%d is too old to support AllowedCPUs/AllowedMemoryNodes"+
	//		" (settings will still be applied to cgroupfs)", sdVer)
	//	return nil
	//}

	if cpus != "" {
		bits, err := rangeToBits(cpus)
		if err != nil {
			return fmt.Errorf("resources.CPU.Cpus=%q conversion error: %w",
				cpus, err)
		}
		*props = append(*props,
			newProp("AllowedCPUs", bits))
	}
	if mems != "" {
		bits, err := rangeToBits(mems)
		if err != nil {
			return fmt.Errorf("resources.CPU.Mems=%q conversion error: %w",
				mems, err)
		}
		*props = append(*props,
			newProp("AllowedMemoryNodes", bits))
	}
	return nil
}

func rangeToBits(str string) ([]byte, error) {
	bits := &bitset.BitSet{}

	for _, r := range strings.Split(str, ",") {
		// allow extra spaces around
		r = strings.TrimSpace(r)
		// allow empty elements (extra commas)
		if r == "" {
			continue
		}
		ranges := strings.SplitN(r, "-", 2)
		if len(ranges) > 1 {
			start, err := strconv.ParseUint(ranges[0], 10, 32)
			if err != nil {
				return nil, err
			}
			end, err := strconv.ParseUint(ranges[1], 10, 32)
			if err != nil {
				return nil, err
			}
			if start > end {
				return nil, errors.New("invalid range: " + r)
			}
			for i := uint(start); i <= uint(end); i++ {
				bits.Set(i)
			}
		} else {
			val, err := strconv.ParseUint(ranges[0], 10, 32)
			if err != nil {
				return nil, err
			}
			bits.Set(uint(val))
		}
	}

	val := bits.Bytes()
	if len(val) == 0 {
		// do not allow empty values
		return nil, errors.New("empty value")
	}
	ret := make([]byte, len(val)*8)
	for i := range val {
		// bitset uses BigEndian internally
		binary.BigEndian.PutUint64(ret[i*8:], val[len(val)-1-i])
	}
	// remove upper all-zero bytes
	for ret[0] == 0 {
		ret = ret[1:]
	}

	return ret, nil
}

// RunUnderSystemdScope adds the specified pid to a systemd scope
func RunUnderSystemdScope(pid int, slice, unitName string, properties ...systemdDbus.Property) error {
	conn, err := systemdDbus.New()
	if err != nil {
		return err
	}
	defaultProperties := []systemdDbus.Property{
		newProp("PIDs", []uint32{uint32(pid)}),
		newProp("Delegate", true),
		newProp("DefaultDependencies", false),
	}
	properties = append(defaultProperties, properties...)
	if slice != "" {
		properties = append(properties, systemdDbus.PropSlice(slice))
	}
	ch := make(chan string)
	_, err = conn.StartTransientUnit(unitName, "replace", properties, ch)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Block until job is started
	<-ch

	return nil
}

func newProp(name string, units interface{}) systemdDbus.Property {
	return systemdDbus.Property{
		Name:  name,
		Value: dbus.MakeVariant(units),
	}
}
