package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/containers/buildah/pkg/secrets"
	"github.com/containers/libpod/pkg/annotations"
	"github.com/containers/libpod/pkg/rootless"
	"github.com/containers/storage/pkg/mount"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *Server) configureGeneratorMounts(
	ctx context.Context,
	specgen generate.Generator,
	containerConfig *pb.ContainerConfig,
	sandboxConfig *pb.PodSandboxConfig,
	sb *sandbox.Sandbox,
	privileged bool,
	containerInfo *storage.ContainerInfo,
	mountLabel, mountPoint string,
	processArgs []string) ([]oci.ContainerVolume, error) {

	// # 1
	if s.config.ReadOnly {
		// tmpcopyup is a runc extension and is not part of the OCI spec.
		// WORK ON: Use "overlay" mounts as an alternative to tmpfs with tmpcopyup
		// Look at https://github.com/cri-o/cri-o/pull/1434#discussion_r177200245 for more info on this
		options := []string{"rw", "noexec", "nosuid", "nodev", "tmpcopyup"}
		mounts := map[string]string{
			"/run":     "mode=0755",
			"/tmp":     "mode=1777",
			"/var/tmp": "mode=1777",
		}
		for target, mode := range mounts {
			if !isInCRIMounts(target, containerConfig.GetMounts()) {
				mnt := rspec.Mount{
					Destination: target,
					Type:        "tmpfs",
					Source:      "tmpfs",
					Options:     append(options, mode),
				}
				specgen.AddMount(mnt)
			}
		}
	}


	// # 2
	containerVolumes, ociMountsFromConfig, err := addOCIBindMounts(ctx, mountLabel, containerConfig, &specgen, s.config.RuntimeConfig.BindMountPrefix)
	if err != nil {
		return nil, err
	}

	containerIsInit := false
	// TODO FIXME duplicate statement
	if strings.Contains(processArgs[0], "/sbin/init") || (filepath.Base(processArgs[0]) == "systemd") {
		setupSystemd(specgen.Mounts(), specgen)
		containerIsInit = true
	}

	configureGeneratorMountsForCgroups(specgen, sb.HostNetwork(), privileged, containerIsInit)

	// # 6
	// Remove the default /dev/shm mount to ensure we overwrite it
	specgen.RemoveMount("/dev/shm")

	mnt := rspec.Mount{
		Type:        "bind",
		Source:      sb.ShmPath(),
		Destination: "/dev/shm",
		Options:     []string{"rw", "bind"},
	}
	// bind mount the pod shm
	specgen.AddMount(mnt)

	readOnlyRootfs := s.configureGeneratorForReadOnly(specgen, containerConfig)
	options := []string{"rw"}
	if readOnlyRootfs {
		options = []string{"ro"}
	}
	// # 7
	if sb.ResolvPath() != "" {
		if err := securityLabel(sb.ResolvPath(), mountLabel, false); err != nil {
			return nil, err
		}

		mnt = rspec.Mount{
			Type:        "bind",
			Source:      sb.ResolvPath(),
			Destination: "/etc/resolv.conf",
			Options:     []string{"bind", "nodev", "nosuid", "noexec"},
		}
		// bind mount the pod resolver file
		specgen.AddMount(mnt)
	}

	// # 8
	if sb.HostnamePath() != "" {
		if err := securityLabel(sb.HostnamePath(), mountLabel, false); err != nil {
			return nil, err
		}

		mnt = rspec.Mount{
			Type:        "bind",
			Source:      sb.HostnamePath(),
			Destination: "/etc/hostname",
			Options:     append(options, "bind"),
		}
		specgen.AddMount(mnt)
	}

	// # 8
	if !isInCRIMounts("/etc/hosts", containerConfig.GetMounts()) && hostNetwork(containerConfig) {
		// Only bind mount for host netns and when CRI does not give us any hosts file
		mnt = rspec.Mount{
			Type:        "bind",
			Source:      "/etc/hosts",
			Destination: "/etc/hosts",
			Options:     append(options, "bind"),
		}
		specgen.AddMount(mnt)
	}

	// # 9
	if privileged {
		setOCIBindMountsPrivileged(&specgen)
	}

	// # 10
	// Add image volumes
	volumeMounts, err := addImageVolumes(ctx, mountPoint, s, containerInfo, mountLabel, &specgen)
	if err != nil {
		return nil, err
	}

	// # 11
	var secretMounts []rspec.Mount
	if len(s.config.DefaultMounts) > 0 {
		// This option has been deprecated, once it is removed in the later versions, delete the server/secrets.go file as well
		log.Warnf(ctx, "--default-mounts has been deprecated and will be removed in future versions. Add mounts to either %q or %q", secrets.DefaultMountsFile, secrets.OverrideMountsFile)
		var err error
		secretMounts, err = addSecretsBindMounts(ctx, mountLabel, containerInfo.RunDir, s.config.DefaultMounts, specgen)
		if err != nil {
			return nil, fmt.Errorf("failed to mount secrets: %v", err)
		}
	}
	// Check for FIPS_DISABLE label in the pod config
	disableFips := false
	if value, ok := sandboxConfig.GetLabels()["FIPS_DISABLE"]; ok && value == "true" {
		disableFips = true
	}
	// Add secrets from the default and override mounts.conf files
	secretMounts = append(secretMounts, secrets.SecretMounts(mountLabel, containerInfo.RunDir, s.config.DefaultMountsFile, rootless.IsRootless(), disableFips)...)

	volumesJSON, err := json.Marshal(containerVolumes)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation(annotations.Volumes, string(volumesJSON))

	mounts := []rspec.Mount{}
	mounts = append(mounts, ociMountsFromConfig...)
	mounts = append(mounts, volumeMounts...)
	mounts = append(mounts, secretMounts...)

	sort.Sort(orderedMounts(mounts))

	for _, m := range mounts {
		mnt = rspec.Mount{
			Type:        "bind",
			Source:      m.Source,
			Destination: m.Destination,
			Options:     append(m.Options, "bind"),
		}
		specgen.AddMount(mnt)
	}

	return containerVolumes, nil
}

func configureGeneratorMountsForCgroups(specgen generate.Generator, hostNetwork, privileged, containerIsInit bool) {
	// unconditionally add cgroup mount, even if sys isn't mounted
	cgroupOpts := []string{"nosuid", "noexec", "nodev", "relatime", "ro"}
	sysOpts := make([]string, 0)

	// # 3
	// If the sandbox is configured to run in the host network, do not create a new network namespace
	if hostNetwork {
		sysOpts = []string{"nosuid", "noexec", "nodev", "ro"}
	}

	// # 4
	if privileged {
		cgroupOpts = []string{"nosuid", "noexec", "nodev", "rw", "relatime"}
		sysOpts = []string{"nosuid", "noexec", "nodev", "rw"}
	}


	// # 5
	// It also expects to be able to write to /sys/fs/cgroup/systemd
	if containerIsInit && node.CgroupIsV2() {
		cgroupOpts = []string{"private", "rw"}
	}

	if len(sysOpts) != 0 {
		specgen.RemoveMount("/sys")
		m := rspec.Mount{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     sysOpts,
		}
		specgen.AddMount(m)
	}

	specgen.RemoveMount("/sys/fs/cgroup")
	m := rspec.Mount{
		Destination: "/sys/fs/cgroup",
		Type:        "cgroup",
		Source:      "cgroup",
		Options:     cgroupOpts,
	}
	specgen.AddMount(m)
}

func setOCIBindMountsPrivileged(g *generate.Generator) {
	spec := g.Config
	// clear readonly for /sys and cgroup
	for i := range spec.Mounts {
		clearReadOnly(&spec.Mounts[i])
	}
	spec.Linux.ReadonlyPaths = nil
	spec.Linux.MaskedPaths = nil
}

func clearReadOnly(m *rspec.Mount) {
	var opt []string
	for _, o := range m.Options {
		if o == "rw" {
			return
		} else if o != "ro" {
			opt = append(opt, o)
		}
	}
	m.Options = opt
	m.Options = append(m.Options, "rw")
}

func addOCIBindMounts(ctx context.Context, mountLabel string, containerConfig *pb.ContainerConfig, specgen *generate.Generator, bindMountPrefix string) ([]oci.ContainerVolume, []rspec.Mount, error) {
	volumes := []oci.ContainerVolume{}
	ociMountsFromConfig := []rspec.Mount{}
	mounts := containerConfig.GetMounts()

	// Sort mounts in number of parts. This ensures that high level mounts don't
	// shadow other mounts.
	sort.Sort(criOrderedMounts(mounts))

	// Copy all mounts from default mounts, except for
	// - mounts overridden by supplied mount;
	// - all mounts under /dev if a supplied /dev is present.
	// - all mounts under /sys if a supplied /sys is present.
	mountSet := make(map[string]struct{})
	for _, m := range mounts {
		mountSet[filepath.Clean(m.ContainerPath)] = struct{}{}
	}
	defaultMounts := specgen.Mounts()
	specgen.ClearMounts()

	_, mountDev := mountSet["/dev"]
	_, mountSys := mountSet["/sys"]
	for _, m := range defaultMounts {
		dst := filepath.Clean(m.Destination)
		// filter out mount overridden by a supplied mount
		if _, ok := mountSet[dst]; ok {
			continue
		}
		// filter out everything under /dev if /dev is a supplied mount
		if mountDev && strings.HasPrefix(dst, "/dev/") {
			continue
		}
		// filter out everything under /sys if /sys is a supplied mount
		if mountSys && strings.HasPrefix(dst, "/sys/") {
			continue
		}
		specgen.AddMount(m)
	}

	for _, m := range mounts {
		dest := m.GetContainerPath()
		if dest == "" {
			return nil, nil, fmt.Errorf("mount.ContainerPath is empty")
		}

		if m.HostPath == "" {
			return nil, nil, fmt.Errorf("mount.HostPath is empty")
		}
		src := filepath.Join(bindMountPrefix, m.GetHostPath())

		resolvedSrc, err := resolveSymbolicLink(src, bindMountPrefix)
		if err == nil {
			src = resolvedSrc
		} else {
			if !os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("failed to resolve symlink %q: %v", src, err)
			} else if err = os.MkdirAll(src, 0755); err != nil {
				return nil, nil, fmt.Errorf("failed to mkdir %s: %s", src, err)
			}
		}

		options := []string{"rw"}
		if m.Readonly {
			options = []string{"ro"}
		}
		options = append(options, "rbind")

		// mount propagation
		mountInfos, err := mount.GetMounts()
		if err != nil {
			return nil, nil, err
		}
		switch m.GetPropagation() {
		case pb.MountPropagation_PROPAGATION_PRIVATE:
			options = append(options, "rprivate")
			// Since default root propagation in runc is rprivate ignore
			// setting the root propagation
		case pb.MountPropagation_PROPAGATION_BIDIRECTIONAL:
			if err := ensureShared(src, mountInfos); err != nil {
				return nil, nil, err
			}
			options = append(options, "rshared")
			if err := specgen.SetLinuxRootPropagation("rshared"); err != nil {
				return nil, nil, err
			}
		case pb.MountPropagation_PROPAGATION_HOST_TO_CONTAINER:
			if err := ensureSharedOrSlave(src, mountInfos); err != nil {
				return nil, nil, err
			}
			options = append(options, "rslave")
			if specgen.Config.Linux.RootfsPropagation != "rshared" &&
				specgen.Config.Linux.RootfsPropagation != "rslave" {
				if err := specgen.SetLinuxRootPropagation("rslave"); err != nil {
					return nil, nil, err
				}
			}
		default:
			log.Warnf(ctx, "unknown propagation mode for hostPath %q", m.HostPath)
			options = append(options, "rprivate")
		}

		if m.SelinuxRelabel {
			if err := securityLabel(src, mountLabel, false); err != nil {
				return nil, nil, err
			}
		}

		volumes = append(volumes, oci.ContainerVolume{
			ContainerPath: dest,
			HostPath:      src,
			Readonly:      m.Readonly,
		})

		ociMountsFromConfig = append(ociMountsFromConfig, rspec.Mount{
			Source:      src,
			Destination: dest,
			Options:     options,
		})
	}

	return volumes, ociMountsFromConfig, nil
}

// mountExists returns true if dest exists in the list of mounts
func mountExists(specMounts []rspec.Mount, dest string) bool {
	for _, m := range specMounts {
		if m.Destination == dest {
			return true
		}
	}
	return false
}

// systemd expects to have /run, /run/lock and /tmp on tmpfs
func setupSystemd(mounts []rspec.Mount, g generate.Generator) {
	options := []string{"rw", "rprivate", "noexec", "nosuid", "nodev"}
	for _, dest := range []string{"/run", "/run/lock"} {
		if mountExists(mounts, dest) {
			continue
		}
		tmpfsMnt := rspec.Mount{
			Destination: dest,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     append(options, "tmpcopyup", "size=65536k"),
		}
		g.AddMount(tmpfsMnt)
	}
	for _, dest := range []string{"/tmp", "/var/log/journal"} {
		if mountExists(mounts, dest) {
			continue
		}
		tmpfsMnt := rspec.Mount{
			Destination: dest,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     append(options, "tmpcopyup"),
		}
		g.AddMount(tmpfsMnt)
	}
	if !node.CgroupIsV2() {
		systemdMnt := rspec.Mount{
			Destination: "/sys/fs/cgroup/systemd",
			Type:        "bind",
			Source:      "/sys/fs/cgroup/systemd",
			Options:     []string{"bind", "nodev", "noexec", "nosuid"},
		}
		g.AddMount(systemdMnt)
		g.AddLinuxMaskedPaths("/sys/fs/cgroup/systemd/release_agent")
	}
}

func (s *Server) configureGeneratorForReadOnly(specgen generate.Generator, containerConfig *pb.ContainerConfig) bool {
	readOnlyRootfs := s.config.ReadOnly
	if containerConfig.GetLinux().GetSecurityContext().GetReadonlyRootfs() {
		readOnlyRootfs = true
	}
	specgen.SetRootReadonly(readOnlyRootfs)
	return readOnlyRootfs
}
