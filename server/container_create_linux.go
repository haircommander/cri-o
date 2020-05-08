// +build linux

package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/containers/libpod/pkg/annotations"
	selinux "github.com/containers/libpod/pkg/selinux"
	createconfig "github.com/containers/libpod/pkg/spec"
	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/opencontainers/runc/libcontainer/devices"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Copied from k8s.io/kubernetes/pkg/kubelet/kuberuntime/labels.go
const podTerminationGracePeriodLabel = "io.kubernetes.pod.terminationGracePeriod"

type configDevice struct {
	Device   rspec.LinuxDevice
	Resource rspec.LinuxDeviceCgroup
}

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(container *oci.Container, cgroupParent string) error {
	if s.defaultIDMappings != nil && !s.defaultIDMappings.Empty() {
		rootPair := s.defaultIDMappings.RootPair()

		for _, path := range []string{container.BundlePath(), container.MountPoint()} {
			if err := os.Chown(path, rootPair.UID, rootPair.GID); err != nil {
				return errors.Wrapf(err, "cannot chown %s to %d:%d", path, rootPair.UID, rootPair.GID)
			}
			if err := makeAccessible(path, rootPair.UID, rootPair.GID); err != nil {
				return errors.Wrapf(err, "cannot make %s accessible to %d:%d", path, rootPair.UID, rootPair.GID)
			}
		}
	}
	return s.Runtime().CreateContainer(container, cgroupParent)
}

// makeAccessible changes the path permission and each parent directory to have --x--x--x
func makeAccessible(path string, uid, gid int) error {
	for ; path != "/"; path = filepath.Dir(path) {
		st, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if int(st.Sys().(*syscall.Stat_t).Uid) == uid && int(st.Sys().(*syscall.Stat_t).Gid) == gid {
			continue
		}
		if st.Mode()&0111 != 0111 {
			if err := os.Chmod(path, st.Mode()|0111); err != nil {
				return err
			}
		}
	}
	return nil
}

// nolint:gocyclo
func (s *Server) createSandboxContainer(ctx context.Context, containerID, containerName string, sb *sandbox.Sandbox, sandboxConfig *pb.PodSandboxConfig, containerConfig *pb.ContainerConfig) (cntr *oci.Container, errRet error) {
	if sb == nil {
		return nil, errors.New("createSandboxContainer needs a sandbox")
	}

	// TODO: simplify this function (cyclomatic complexity here is high)
	// TODO: factor generating/updating the spec into something other projects can vendor

	// creates a spec Generator with the default spec.
	specgen, err := generate.New("linux")
	if err != nil {
		return nil, err
	}
	specgen.HostSpecific = true
	specgen.ClearProcessRlimits()

	ulimits, err := getUlimitsFromConfig(&s.config)
	if err != nil {
		return nil, err
	}
	for _, u := range ulimits {
		specgen.AddProcessRlimits(u.name, u.hard, u.soft)
	}

	var privileged bool
	if containerConfig.GetLinux().GetSecurityContext() != nil {
		if containerConfig.GetLinux().GetSecurityContext().GetPrivileged() {
			privileged = true
		}
		if privileged {
			if !sandboxConfig.GetLinux().GetSecurityContext().GetPrivileged() {
				return nil, errors.New("no privileged container allowed in sandbox")
			}
		}
	}

	imageSpec := containerConfig.GetImage()
	if imageSpec == nil {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Image is nil")
	}

	image := imageSpec.Image
	if image == "" {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Image.Image is empty")
	}
	images, err := s.StorageImageServer().ResolveNames(s.config.SystemContext, image)
	if err != nil {
		if err == storage.ErrCannotParseImageID {
			images = append(images, image)
		} else {
			return nil, err
		}
	}

	// Get imageName and imageRef that are later requested in container status
	var (
		imgResult    *storage.ImageResult
		imgResultErr error
	)
	for _, img := range images {
		imgResult, imgResultErr = s.StorageImageServer().ImageStatus(s.config.SystemContext, img)
		if imgResultErr == nil {
			break
		}
	}
	if imgResultErr != nil {
		return nil, imgResultErr
	}

	imageName := imgResult.Name
	imageRef := imgResult.ID
	if len(imgResult.RepoDigests) > 0 {
		imageRef = imgResult.RepoDigests[0]
	}

	specgen.AddAnnotation(annotations.Image, image)
	specgen.AddAnnotation(annotations.ImageName, imageName)
	specgen.AddAnnotation(annotations.ImageRef, imageRef)

	selinuxConfig := containerConfig.GetLinux().GetSecurityContext().GetSelinuxOptions()
	var labelOptions []string
	if selinuxConfig == nil {
		labelOptions, err = label.DupSecOpt(sb.ProcessLabel())
		if err != nil {
			return nil, err
		}
	} else {
		labelOptions = getLabelOptions(selinuxConfig)
	}

	containerIDMappings := s.defaultIDMappings
	metadata := containerConfig.GetMetadata()

	containerInfo, err := s.StorageRuntimeServer().CreateContainer(s.config.SystemContext,
		sb.Name(), sb.ID(),
		image, imgResult.ID,
		containerName, containerID,
		metadata.Name,
		metadata.Attempt,
		containerIDMappings,
		labelOptions)
	if err != nil {
		return nil, err
	}

	mountLabel := containerInfo.MountLabel
	var processLabel string
	if !privileged {
		processLabel = containerInfo.ProcessLabel
	}
	hostIPC := containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetIpc() == pb.NamespaceMode_NODE
	hostPID := containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetPid() == pb.NamespaceMode_NODE
	hostNet := containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetNetwork() == pb.NamespaceMode_NODE

	// Don't use SELinux separation with Host Pid or IPC Namespace or privileged.
	if hostPID || hostIPC {
		processLabel, mountLabel = "", ""
	}

	if hostNet {
		processLabel = ""
	}

	defer func() {
		if errRet != nil {
			log.Infof(ctx, "createCtrLinux: deleting container %s from storage", containerInfo.ID)
			err2 := s.StorageRuntimeServer().DeleteContainer(containerInfo.ID)
			if err2 != nil {
				log.Warnf(ctx, "Failed to cleanup container directory: %v", err2)
			}
		}
	}()

	labels := containerConfig.GetLabels()

	if err := s.configureGeneratorForDevices(ctx, specgen, containerConfig, sb); err != nil {
		return nil, err
	}

	if err := validateLabels(labels); err != nil {
		return nil, err
	}

	kubeAnnotations := containerConfig.GetAnnotations()
	for k, v := range kubeAnnotations {
		specgen.AddAnnotation(k, v)
	}
	for k, v := range labels {
		specgen.AddAnnotation(k, v)
	}

	// set this container's apparmor profile if it is set by sandbox
	if s.Config().AppArmor().IsEnabled() && !privileged {
		profile, err := s.Config().AppArmor().Apply(
			containerConfig.GetLinux().GetSecurityContext().GetApparmorProfile(),
		)
		if err != nil {
			return nil, errors.Wrapf(err, "applying apparmor profile to container %s", containerID)
		}

		log.Debugf(ctx, "Applied AppArmor profile %s to container %s", profile, containerID)
		specgen.SetProcessApparmorProfile(profile)
	}

	sboxLogDir := sandboxConfig.GetLogDirectory()
	if sboxLogDir == "" {
		sboxLogDir = sb.LogDir()
	}

	logPath := containerConfig.GetLogPath()
	if logPath == "" {
		logPath = filepath.Join(sboxLogDir, containerID+".log")
	} else {
		logPath = filepath.Join(sboxLogDir, logPath)
	}

	// Handle https://issues.k8s.io/44043
	if err := ensureSaneLogPath(logPath); err != nil {
		return nil, err
	}

	log.Debugf(ctx, "setting container's log_path = %s, sbox.logdir = %s, ctr.logfile = %s",
		sboxLogDir, containerConfig.GetLogPath(), logPath,
	)

	specgen.SetProcessTerminal(containerConfig.Tty)
	if containerConfig.Tty {
		specgen.AddProcessEnv("TERM", "xterm")
	}

	linux := containerConfig.GetLinux()
	if linux != nil {
		resources := linux.GetResources()
		if resources != nil {
			specgen.SetLinuxResourcesCPUPeriod(uint64(resources.GetCpuPeriod()))
			specgen.SetLinuxResourcesCPUQuota(resources.GetCpuQuota())
			specgen.SetLinuxResourcesCPUShares(uint64(resources.GetCpuShares()))

			memoryLimit := resources.GetMemoryLimitInBytes()
			if memoryLimit != 0 {
				if err := cgmgr.VerifyMemoryIsEnough(memoryLimit); err != nil {
					return nil, err
				}
				specgen.SetLinuxResourcesMemoryLimit(memoryLimit)
				if node.CgroupHasMemorySwap() {
					specgen.SetLinuxResourcesMemorySwap(memoryLimit)
				}
			}

			specgen.SetProcessOOMScoreAdj(int(resources.GetOomScoreAdj()))
			specgen.SetLinuxResourcesCPUCpus(resources.GetCpusetCpus())
			specgen.SetLinuxResourcesCPUMems(resources.GetCpusetMems())

			// If the kernel has no support for hugetlb, silently ignore the limits
			if node.CgroupHasHugetlb() {
				hugepageLimits := resources.GetHugepageLimits()
				for _, limit := range hugepageLimits {
					specgen.AddLinuxResourcesHugepageLimit(limit.PageSize, limit.Limit)
				}
			}
		}

		specgen.SetLinuxCgroupsPath(s.config.CgroupManager().ContainerCgroupPath(sb.CgroupParent(), containerID))

		if t, ok := kubeAnnotations[podTerminationGracePeriodLabel]; ok {
			// currently only supported by systemd, see
			// https://github.com/opencontainers/runc/pull/2224
			if s.config.CgroupManager().IsSystemd() {
				specgen.AddAnnotation("org.systemd.property.TimeoutStopUSec",
					"uint64 "+t+"000000") // sec to usec
			}
		}

		if privileged {
			specgen.SetupPrivileged(true)
		} else {
			capabilities := linux.GetSecurityContext().GetCapabilities()
			// Ensure we don't get a nil pointer error if the config
			// doesn't set any capabilities
			if capabilities == nil {
				capabilities = &pb.Capability{}
			}
			// Clear default capabilities from spec
			specgen.ClearProcessCapabilities()
			capabilities.AddCapabilities = append(capabilities.AddCapabilities, s.config.DefaultCapabilities...)
			err = setupCapabilities(&specgen, capabilities)
			if err != nil {
				return nil, err
			}
		}
		specgen.SetProcessNoNewPrivileges(linux.GetSecurityContext().GetNoNewPrivs())

		if containerConfig.GetLinux().GetSecurityContext() != nil &&
			!containerConfig.GetLinux().GetSecurityContext().Privileged {
			// TODO(runcom): have just one of this var at the top of the function
			securityContext := containerConfig.GetLinux().GetSecurityContext()
			for _, mp := range []string{
				"/proc/acpi",
				"/proc/kcore",
				"/proc/keys",
				"/proc/latency_stats",
				"/proc/timer_list",
				"/proc/timer_stats",
				"/proc/sched_debug",
				"/proc/scsi",
				"/sys/firmware",
			} {
				specgen.AddLinuxMaskedPaths(mp)
			}
			if securityContext.GetMaskedPaths() != nil {
				specgen.Config.Linux.MaskedPaths = nil
				for _, path := range securityContext.GetMaskedPaths() {
					specgen.AddLinuxMaskedPaths(path)
				}
			}

			for _, rp := range []string{
				"/proc/asound",
				"/proc/bus",
				"/proc/fs",
				"/proc/irq",
				"/proc/sys",
				"/proc/sysrq-trigger",
			} {
				specgen.AddLinuxReadonlyPaths(rp)
			}
			if securityContext.GetReadonlyPaths() != nil {
				specgen.Config.Linux.ReadonlyPaths = nil
				for _, path := range securityContext.GetReadonlyPaths() {
					specgen.AddLinuxReadonlyPaths(path)
				}
			}
		}
	}

	// Join the namespace paths for the pod sandbox container.
	if err := configureGeneratorGivenNamespacePaths(sb.NamespacePaths(), specgen); err != nil {
		return nil, errors.Wrap(err, "failed to configure namespaces in container create")
	}

	if containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetPid() == pb.NamespaceMode_NODE {
		// kubernetes PodSpec specify to use Host PID namespace
		if err := specgen.RemoveLinuxNamespace(string(rspec.PIDNamespace)); err != nil {
			return nil, err
		}
	} else if containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetPid() == pb.NamespaceMode_POD {
		infra := sb.InfraContainer()
		if infra == nil {
			return nil, errors.New("PID namespace requested, but sandbox has no infra container")
		}

		// share Pod PID namespace
		// SEE NOTE ABOVE
		pidNsPath := fmt.Sprintf("/proc/%d/ns/pid", infra.State().Pid)
		if err := specgen.AddOrReplaceLinuxNamespace(string(rspec.PIDNamespace), pidNsPath); err != nil {
			return nil, err
		}
	}

	containerImageConfig := containerInfo.Config
	if containerImageConfig == nil {
		err = fmt.Errorf("empty image config for %s", image)
		return nil, err
	}

	processArgs, err := buildOCIProcessArgs(ctx, containerConfig, containerImageConfig)
	if err != nil {
		return nil, err
	}
	specgen.SetProcessArgs(processArgs)

	// TODO FIXME duplicate statement
	if strings.Contains(processArgs[0], "/sbin/init") || (filepath.Base(processArgs[0]) == "systemd") {
		processLabel, err = selinux.SELinuxInitLabel(processLabel)
		if err != nil {
			return nil, err
		}
	}

	// When running on cgroupv2, automatically add a cgroup namespace for not privileged containers.
	if !privileged && node.CgroupIsV2() {
		if err := specgen.AddOrReplaceLinuxNamespace(string(rspec.CgroupNamespace), ""); err != nil {
			return nil, err
		}
	}

	for idx, ip := range sb.IPs() {
		specgen.AddAnnotation(fmt.Sprintf("%s.%d", annotations.IP, idx), ip)
	}

	if privileged {
		setOCIBindMountsPrivileged(&specgen)
	}

	// Set hostname and add env for hostname
	specgen.SetHostname(sb.Hostname())
	specgen.AddProcessEnv("HOSTNAME", sb.Hostname())

	specgen.AddAnnotation(annotations.Name, containerName)
	specgen.AddAnnotation(annotations.ContainerID, containerID)
	specgen.AddAnnotation(annotations.SandboxID, sb.ID())
	specgen.AddAnnotation(annotations.SandboxName, sb.Name())
	specgen.AddAnnotation(annotations.ContainerType, annotations.ContainerTypeContainer)
	specgen.AddAnnotation(annotations.LogPath, logPath)
	specgen.AddAnnotation(annotations.TTY, fmt.Sprintf("%v", containerConfig.Tty))
	specgen.AddAnnotation(annotations.Stdin, fmt.Sprintf("%v", containerConfig.Stdin))
	specgen.AddAnnotation(annotations.StdinOnce, fmt.Sprintf("%v", containerConfig.StdinOnce))
	specgen.AddAnnotation(annotations.ResolvPath, sb.ResolvPath())
	specgen.AddAnnotation(annotations.ContainerManager, lib.ContainerManagerCRIO)

	created := time.Now()
	specgen.AddAnnotation(annotations.Created, created.Format(time.RFC3339Nano))

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation(annotations.Metadata, string(metadataJSON))

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation(annotations.Labels, string(labelsJSON))

	kubeAnnotationsJSON, err := json.Marshal(kubeAnnotations)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation(annotations.Annotations, string(kubeAnnotationsJSON))

	spp := containerConfig.GetLinux().GetSecurityContext().GetSeccompProfilePath()
	if !privileged {
		if err := s.setupSeccomp(ctx, &specgen, spp); err != nil {
			return nil, err
		}
	}
	specgen.AddAnnotation(annotations.SeccompProfilePath, spp)

	mountPoint, err := s.StorageRuntimeServer().StartContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to mount container %s(%s): %v", containerName, containerID, err)
	}
	defer func() {
		if errRet != nil {
			log.Infof(ctx, "createCtrLinux: stopping storage container %s", containerID)
			if err := s.StorageRuntimeServer().StopContainer(containerID); err != nil {
				log.Warnf(ctx, "couldn't stop storage container: %v: %v", containerID, err)
			}
		}
	}()
	specgen.AddAnnotation(annotations.MountPoint, mountPoint)

	containerVolumes, err := s.configureGeneratorMounts(ctx, specgen, containerConfig, sandboxConfig, sb, privileged, &containerInfo, mountLabel, mountPoint, processArgs)
	if err != nil {
		return nil, err
	}

	if containerImageConfig.Config.StopSignal != "" {
		// this key is defined in image-spec conversion document at https://github.com/opencontainers/image-spec/pull/492/files#diff-8aafbe2c3690162540381b8cdb157112R57
		specgen.AddAnnotation("org.opencontainers.image.stopSignal", containerImageConfig.Config.StopSignal)
	}

	// First add any configured environment variables from crio config.
	// They will get overridden if specified in the image or container config.
	specgen.AddMultipleProcessEnv(s.Config().DefaultEnv)

	// Add environment variables from image the CRI configuration
	envs := mergeEnvs(containerImageConfig, containerConfig.GetEnvs())
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		specgen.AddProcessEnv(parts[0], parts[1])
	}

	// Setup user and groups
	if linux != nil {
		if err := setupContainerUser(ctx, &specgen, mountPoint, mountLabel, containerInfo.RunDir, linux.GetSecurityContext(), containerImageConfig); err != nil {
			return nil, err
		}
	}

	// Set working directory
	// Pick it up from image config first and override if specified in CRI
	containerCwd := "/"
	imageCwd := containerImageConfig.Config.WorkingDir
	if imageCwd != "" {
		containerCwd = imageCwd
	}
	runtimeCwd := containerConfig.WorkingDir
	if runtimeCwd != "" {
		containerCwd = runtimeCwd
	}
	specgen.SetProcessCwd(containerCwd)
	if err := setupWorkingDirectory(mountPoint, mountLabel, containerCwd); err != nil {
		return nil, err
	}

	newAnnotations := map[string]string{}
	for key, value := range containerConfig.GetAnnotations() {
		newAnnotations[key] = value
	}
	for key, value := range sb.Annotations() {
		newAnnotations[key] = value
	}
	if s.ContainerServer.Hooks != nil {
		if _, err := s.ContainerServer.Hooks.Hooks(specgen.Config, newAnnotations, len(containerConfig.GetMounts()) > 0); err != nil {
			return nil, err
		}
	}

	// Set up pids limit if pids cgroup is mounted
	if node.CgroupHasPid() {
		specgen.SetLinuxResourcesPidsLimit(s.config.PidsLimit)
	}

	// by default, the root path is an empty string. set it now.
	specgen.SetRootPath(mountPoint)

	crioAnnotations := specgen.Config.Annotations

	container, err := oci.NewContainer(containerID, containerName, containerInfo.RunDir, logPath, labels, crioAnnotations, kubeAnnotations, image, imageName, imageRef, metadata, sb.ID(), containerConfig.Tty, containerConfig.Stdin, containerConfig.StdinOnce, sb.RuntimeHandler(), containerInfo.Dir, created, containerImageConfig.Config.StopSignal)
	if err != nil {
		return nil, err
	}

	specgen.SetLinuxMountLabel(mountLabel)
	specgen.SetProcessSelinuxLabel(processLabel)

	container.SetIDMappings(containerIDMappings)
	if s.defaultIDMappings != nil && !s.defaultIDMappings.Empty() {
		for _, uidmap := range s.defaultIDMappings.UIDs() {
			specgen.AddLinuxUIDMapping(uint32(uidmap.HostID), uint32(uidmap.ContainerID), uint32(uidmap.Size))
		}
		for _, gidmap := range s.defaultIDMappings.GIDs() {
			specgen.AddLinuxGIDMapping(uint32(gidmap.HostID), uint32(gidmap.ContainerID), uint32(gidmap.Size))
		}
	} else if err := specgen.RemoveLinuxNamespace(string(rspec.UserNamespace)); err != nil {
		return nil, err
	}

	if os.Getenv("_CRIO_ROOTLESS") != "" {
		makeOCIConfigurationRootless(&specgen)
	}

	saveOptions := generate.ExportOptions{}
	if err := specgen.SaveToFile(filepath.Join(containerInfo.Dir, "config.json"), saveOptions); err != nil {
		return nil, err
	}
	if err := specgen.SaveToFile(filepath.Join(containerInfo.RunDir, "config.json"), saveOptions); err != nil {
		return nil, err
	}

	container.SetSpec(specgen.Config)
	container.SetMountPoint(mountPoint)
	container.SetSeccompProfilePath(spp)

	for _, cv := range containerVolumes {
		container.AddVolume(cv)
	}

	return container, nil
}

func addDevicesPlatform(ctx context.Context, sb *sandbox.Sandbox, containerConfig *pb.ContainerConfig, privilegedWithoutHostDevices bool, specgen *generate.Generator) error {
	sp := specgen.Config
	if containerConfig.GetLinux().GetSecurityContext() != nil && containerConfig.GetLinux().GetSecurityContext().GetPrivileged() && !privilegedWithoutHostDevices {
		hostDevices, err := devices.HostDevices()
		if err != nil {
			return err
		}
		for _, hostDevice := range hostDevices {
			rd := rspec.LinuxDevice{
				Path:  hostDevice.Path,
				Type:  string(hostDevice.Type),
				Major: hostDevice.Major,
				Minor: hostDevice.Minor,
				UID:   &hostDevice.Uid,
				GID:   &hostDevice.Gid,
			}
			if hostDevice.Major == 0 && hostDevice.Minor == 0 {
				// Invalid device, most likely a symbolic link, skip it.
				continue
			}
			specgen.AddDevice(rd)
		}
		sp.Linux.Resources.Devices = []rspec.LinuxDeviceCgroup{
			{
				Allow:  true,
				Access: "rwm",
			},
		}
	}

	for _, device := range containerConfig.GetDevices() {
		// pin the device to avoid using `device` within the range scope as
		// wrong function literal
		device := device

		// If we are privileged, we have access to devices on the host.
		// If the requested container path already exists on the host, the container won't see the expected host path.
		// Therefore, we must error out if the container path already exists
		privileged := containerConfig.GetLinux().GetSecurityContext() != nil && containerConfig.GetLinux().GetSecurityContext().GetPrivileged()
		if privileged && device.ContainerPath != device.HostPath {
			// we expect this to not exist
			_, err := os.Stat(device.ContainerPath)
			if err == nil {
				return errors.Errorf("privileged container was configured with a device container path that already exists on the host.")
			}
			if !os.IsNotExist(err) {
				return errors.Wrap(err, "error checking if container path exists on host")
			}
		}

		path, err := resolveSymbolicLink(device.HostPath, "/")
		if err != nil {
			return err
		}
		dev, err := devices.DeviceFromPath(path, device.Permissions)
		// if there was no error, return the device
		if err == nil {
			rd := rspec.LinuxDevice{
				Path:  device.ContainerPath,
				Type:  string(dev.Type),
				Major: dev.Major,
				Minor: dev.Minor,
				UID:   &dev.Uid,
				GID:   &dev.Gid,
			}
			specgen.AddDevice(rd)
			sp.Linux.Resources.Devices = append(sp.Linux.Resources.Devices, rspec.LinuxDeviceCgroup{
				Allow:  true,
				Type:   string(dev.Type),
				Major:  &dev.Major,
				Minor:  &dev.Minor,
				Access: dev.Permissions,
			})
			continue
		}
		// if the device is not a device node
		// try to see if it's a directory holding many devices
		if err == devices.ErrNotADevice {
			// check if it is a directory
			if e := utils.IsDirectory(path); e == nil {
				// mount the internal devices recursively
				// nolint: errcheck
				filepath.Walk(path, func(dpath string, f os.FileInfo, e error) error {
					if e != nil {
						log.Debugf(ctx, "addDevice walk: %v", e)
					}
					childDevice, e := devices.DeviceFromPath(dpath, device.Permissions)
					if e != nil {
						// ignore the device
						return nil
					}
					cPath := strings.Replace(dpath, path, device.ContainerPath, 1)
					rd := rspec.LinuxDevice{
						Path:  cPath,
						Type:  string(childDevice.Type),
						Major: childDevice.Major,
						Minor: childDevice.Minor,
						UID:   &childDevice.Uid,
						GID:   &childDevice.Gid,
					}
					specgen.AddDevice(rd)
					sp.Linux.Resources.Devices = append(sp.Linux.Resources.Devices, rspec.LinuxDeviceCgroup{
						Allow:  true,
						Type:   string(childDevice.Type),
						Major:  &childDevice.Major,
						Minor:  &childDevice.Minor,
						Access: childDevice.Permissions,
					})

					return nil
				})
			}
		}
	}
	return nil
}

func setupWorkingDirectory(rootfs, mountLabel, containerCwd string) error {
	fp, err := securejoin.SecureJoin(rootfs, containerCwd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(fp, 0755); err != nil {
		return err
	}
	if mountLabel != "" {
		if err1 := securityLabel(fp, mountLabel, false); err1 != nil {
			return err1
		}
	}
	return nil
}

func (s *Server) configureGeneratorForDevices(ctx context.Context, specgen generate.Generator, containerConfig *pb.ContainerConfig, sb *sandbox.Sandbox) error {
	configuredDevices, err := getDevicesFromConfig(ctx, &s.config)
	if err != nil {
		return err
	}

	for i := range configuredDevices {
		d := &configuredDevices[i]

		specgen.AddDevice(d.Device)
		specgen.AddLinuxResourcesDevice(d.Resource.Allow, d.Resource.Type, d.Resource.Major, d.Resource.Minor, d.Resource.Access)
	}

	privilegedWithoutHostDevices, err := s.Runtime().PrivilegedWithoutHostDevices(sb.RuntimeHandler())
	if err != nil {
		return err
	}

	return addDevices(ctx, sb, containerConfig, privilegedWithoutHostDevices, &specgen)
}

func getDevicesFromConfig(ctx context.Context, config *libconfig.Config) ([]configDevice, error) {
	linuxdevs := make([]configDevice, 0, len(config.RuntimeConfig.AdditionalDevices))

	for _, d := range config.RuntimeConfig.AdditionalDevices {
		src, dst, permissions, err := createconfig.ParseDevice(d)
		if err != nil {
			return nil, err
		}

		log.Debugf(ctx, "adding device src=%s dst=%s mode=%s", src, dst, permissions)

		dev, err := devices.DeviceFromPath(src, permissions)
		if err != nil {
			return nil, errors.Wrapf(err, "%s is not a valid device", src)
		}

		dev.Path = dst

		linuxdevs = append(linuxdevs,
			configDevice{
				Device: rspec.LinuxDevice{
					Path:     dev.Path,
					Type:     string(dev.Type),
					Major:    dev.Major,
					Minor:    dev.Minor,
					FileMode: &dev.FileMode,
					UID:      &dev.Uid,
					GID:      &dev.Gid,
				},
				Resource: rspec.LinuxDeviceCgroup{
					Allow:  true,
					Type:   string(dev.Type),
					Major:  &dev.Major,
					Minor:  &dev.Minor,
					Access: permissions,
				},
			})
	}

	return linuxdevs, nil
}
