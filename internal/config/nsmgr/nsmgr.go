package nsmgr

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	nspkg "github.com/containernetworking/plugins/pkg/ns"
	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// NamespaceManager manages the server's namespaces.
// Specifically, it is an interface for how the server is creating namespaces,
// and can be requested to create namespaces for a pod.
type NamespaceManager struct {
	namespacesDir string
	pinnsPath     string
}

// New creates a new NamespaceManager.
func New(namespacesDir, pinnsPath string) *NamespaceManager {
	return &NamespaceManager{
		namespacesDir: namespacesDir,
		pinnsPath:     pinnsPath,
	}
}

func (mgr *NamespaceManager) Initialize() error {
	if err := os.MkdirAll(mgr.namespacesDir, 0o755); err != nil {
		return errors.Wrap(err, "invalid namespaces_dir")
	}

	for _, ns := range supportedNamespacesForPinning() {
		nsDir := mgr.namespaceDirForType(ns)
		if err := utils.IsDirectory(nsDir); err != nil {
			// The file is not a directory, but exists.
			// We should remove it.
			if errors.Is(err, syscall.ENOTDIR) {
				if err := os.Remove(nsDir); err != nil {
					return errors.Wrapf(err, "remove file to create namespaces sub-dir")
				}
				logrus.Infof("Removed file %s to create directory in that path.", nsDir)
			} else if !os.IsNotExist(err) {
				// if it's neither an error because the file exists
				// nor an error because it does not exist, it is
				// some other disk error.
				return errors.Wrapf(err, "checking whether namespaces sub-dir exists")
			}
			if err := os.MkdirAll(nsDir, 0o755); err != nil {
				return errors.Wrap(err, "invalid namespaces sub-dir")
			}
		}
	}
	return nil
}

// NewPodNamespaces creates new namespaces for a pod.
// It's responsible for running pinns and creating the Namespace objects.
// The caller is responsible for cleaning up the namespaces by calling Namespace.Remove().
func (mgr *NamespaceManager) NewPodNamespaces(cfg *PodNamespacesConfig) ([]Namespace, error) {
	if cfg == nil {
		return nil, errors.New("PodNamespacesConfig cannot be nil")
	}
	if len(cfg.Namespaces) == 0 {
		return []Namespace{}, nil
	}

	typeToArg := map[NSType]string{
		IPCNS:  "--ipc",
		UTSNS:  "--uts",
		USERNS: "--user",
		NETNS:  "--net",
	}

	pinnedNamespace := uuid.New().String()
	pinnsArgs := []string{
		"-d", mgr.namespacesDir,
		"-f", pinnedNamespace,
	}

	if len(cfg.Sysctls) != 0 {
		pinnsArgs = append(pinnsArgs, "-s", getSysctlForPinns(cfg.Sysctls))
	}

	var rootPair idtools.IDPair
	if cfg.IDMappings != nil {
		rootPair = cfg.IDMappings.RootPair()
	}

	for _, ns := range cfg.Namespaces {
		arg, ok := typeToArg[ns.Type]
		if !ok {
			return nil, errors.Errorf("Invalid namespace type: %s", ns.Type)
		}
		if ns.Host {
			arg += "=host"
		}
		pinnsArgs = append(pinnsArgs, arg)
		ns.Path = filepath.Join(mgr.namespacesDir, string(ns.Type)+"ns", pinnedNamespace)
		if cfg.IDMappings != nil {
			if err := chownDirToIDPair(ns.Path, rootPair); err != nil {
				return nil, err
			}
		}
	}

	if cfg.IDMappings != nil {
		pinnsArgs = append(pinnsArgs,
			"--uid-mapping="+getMappingsForPinns(cfg.IDMappings.UIDs()),
			"--gid-mapping="+getMappingsForPinns(cfg.IDMappings.GIDs()))
	}

	logrus.Debugf("calling pinns with %v", pinnsArgs)
	output, err := exec.Command(mgr.pinnsPath, pinnsArgs...).CombinedOutput()
	if err != nil {
		logrus.Warnf("pinns %v failed: %s (%v)", pinnsArgs, string(output), err)
		// cleanup the mounts
		for _, ns := range cfg.Namespaces {
			if mErr := unix.Unmount(ns.Path, unix.MNT_DETACH); mErr != nil && mErr != unix.EINVAL {
				logrus.Warnf("failed to unmount %s: %v", ns.Path, mErr)
			}
		}

		return nil, fmt.Errorf("failed to pin namespaces %v: %s %v", cfg.Namespaces, output, err)
	}

	returnedNamespaces := make([]Namespace, 0, len(cfg.Namespaces))
	for _, ns := range cfg.Namespaces {
		ns, err := GetNamespace(ns.Path, ns.Type)
		if err != nil {
			return nil, err
		}

		returnedNamespaces = append(returnedNamespaces, ns)
	}
	return returnedNamespaces, nil
}

// NewPIDNamespaceForPod creates a managed PID namespace.
// It is separate from the other namespaces because pid namespaces need special handling.
// We cannot tell the runtime: "here is your pid namespace!", because it gets created when the container is created,
// and it would would be a bother having conmon (the parent of the container process) unshare, and then do the bind mount.
// Instead, we mount the sandbox's PID namespace after the infra container is created,
// and from then on refer to it for each container create
// Thus, this should be called after the infra container has been created in the runtime.
// This function is heavily based on containernetworking ns package found at
// https://github.com/containernetworking/plugins/blob/5c3c17164270150467498a32c71436c7cd5501be/pkg/ns/ns.go#L140
// Credit goes to the CNI authors
func (mgr *NamespaceManager) NewPIDNamespaceForPod(procEntry, podID string) (_ Namespace, retErr error) {
	// verify the procEntry we were passed is indeed a namespace
	if err := nspkg.IsNSorErr(procEntry); err != nil {
		return nil, err
	}

	nsPath := filepath.Join(mgr.namespaceDirForType(PIDNS), podID)

	// now create an empty file
	f, err := os.Create(nsPath)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating pid namespace path")
	}
	f.Close()

	defer func() {
		if retErr != nil {
			if err2 := os.RemoveAll(nsPath); err != nil {
				logrus.Errorf("failed to remove namespace path %s after failure to pin PID namespace: %v", nsPath, err2)
			}
		}
	}()

	// bind mount the new netns from the pidns entry onto the mount point
	if err := unix.Mount(procEntry, nsPath, "none", unix.MS_BIND, ""); err != nil {
		return nil, errors.Wrapf(err, "error mounting pid namespace path")
	}
	defer func() {
		if retErr != nil {
			if err := unix.Unmount(nsPath, unix.MNT_DETACH); err != nil && err != unix.EINVAL {
				logrus.Errorf("failed umount after failed to pin pid namespace: %v", err)
			}
		}
	}()

	return GetNamespace(nsPath, PIDNS)
}

func chownDirToIDPair(pinPath string, rootPair idtools.IDPair) error {
	if err := os.MkdirAll(filepath.Dir(pinPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(pinPath)
	if err != nil {
		return err
	}
	f.Close()

	return os.Chown(pinPath, rootPair.UID, rootPair.GID)
}

func getMappingsForPinns(mappings []idtools.IDMap) string {
	g := new(bytes.Buffer)
	for _, m := range mappings {
		fmt.Fprintf(g, "%d-%d-%d@", m.ContainerID, m.HostID, m.Size)
	}
	return g.String()
}

func getSysctlForPinns(sysctls map[string]string) string {
	// this assumes there's no sysctl with a `+` in it
	const pinnsSysctlDelim = "+"
	g := new(bytes.Buffer)
	for key, value := range sysctls {
		fmt.Fprintf(g, "'%s=%s'%s", key, value, pinnsSysctlDelim)
	}
	return strings.TrimSuffix(g.String(), pinnsSysctlDelim)
}

// namespacedirForType returns the sub-directory for that particular NSType
// which is of the form `$namespaceDir/$nsType+"ns"`
func (mgr *NamespaceManager) namespaceDirForType(ns NSType) string {
	return filepath.Join(mgr.namespacesDir, string(ns)+"ns")
}
