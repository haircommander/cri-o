package sandbox

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/storage"
	"github.com/containers/storage/pkg/stringid"
	"github.com/cri-o/cri-o/pkg/container"
	"github.com/pkg/errors"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"github.com/cri-o/cri-o/utils"
)

// Sandbox is the interface for managing pod sandboxes
type Sandbox interface {
	Create() error

	Start() error

	Stop() error

	Delete() error

	AddContainer(container.Container) error

	RemoveContainer(container.Container) error

	// SetConfig sets the sandbox configuration and validates it
	SetConfig(*pb.PodSandboxConfig) error

	// SetNameAndID sets the sandbox name and ID
	SetNameAndID() error

	// SetContainerName sets the name of the infra container
	SetContainerName(string)

	// Config returns the sandbox configuration
	Config() *pb.PodSandboxConfig

	// ID returns the id of the pod sandbox
	ID() string

	// Name returns the id of the pod sandbox
	Name() string

	// Below are the functions required to satisfy ContainerFactory interface in 
	// internal/storage/runtime.go
	PodName() string
	PodID() string
	ImageName() string
	ImageID() string
	ContainerName() string
	ContainerID() string
	MetadataName() string
	UID() string
	KubeNamespace() string
	Attempt() uint32
	LabelOptions() []string
	Infra() bool
	Privileged() bool
	IDMappings() *storage.IDMappingOptions
	SetIDMappings(*storage.IDMappingOptions)
}

// sandbox is the hidden default type behind the Sandbox interface
type sandbox struct {
	ctx    context.Context
	config *pb.PodSandboxConfig
	id     string
	name   string
	imageName string
	containerName string
}

// New creates a new, empty Sandbox instance
func New(ctx context.Context, pauseImage string) Sandbox {
	return &sandbox{
		ctx:    ctx,
		config: nil,
		imageName: pauseImage,
	}
}

// SetConfig sets the sandbox configuration and validates it
func (s *sandbox) SetConfig(config *pb.PodSandboxConfig) error {
	if s.config != nil {
		return errors.New("config already set")
	}

	if config == nil {
		return errors.New("config is nil")
	}

	if config.GetMetadata() == nil {
		return errors.New("metadata is nil")
	}

	if config.GetMetadata().GetName() == "" {
		return errors.New("PodSandboxConfig.Metadata.Name should not be empty")
	}
	s.config = config
	return nil
}

// SetNameAndID sets the sandbox name and ID
func (s *sandbox) SetNameAndID() error {
	if s.config == nil {
		return errors.New("config is nil")
	}

	if s.config.GetMetadata().GetNamespace() == "" {
		return errors.New("cannot generate pod name without namespace")
	}

	if s.config.GetMetadata().GetName() == "" {
		return errors.New("cannot generate pod name without name in metadata")
	}

	s.id = stringid.GenerateNonCryptoID()
	s.name = strings.Join([]string{
		"k8s",
		s.config.GetMetadata().GetName(),
		s.config.GetMetadata().GetNamespace(),
		s.config.GetMetadata().GetUid(),
		fmt.Sprintf("%d", s.config.GetMetadata().GetAttempt()),
	}, "_")

	return nil
}

func (s *sandbox) SetContainerName(cname string) {
	s.containerName = cname
}

// Config returns the sandbox configuration
func (s *sandbox) Config() *pb.PodSandboxConfig {
	return s.config
}

// ID returns the id of the pod sandbox
func (s *sandbox) ID() string {
	return s.id
}

// Name returns the id of the pod sandbox
func (s *sandbox) Name() string {
	return s.name
}

func (s *sandbox) PodName() string {
	return s.Name()
}
func (s *sandbox) PodID() string {
	return s.ID()
}
func (s *sandbox) ImageName() string {
	return s.imageName
}

func (s *sandbox) ImageID() string {
	return ""
}

func (s *sandbox) ContainerName() string {
	// TODO FIXME set me
	// Come to think of it, I ahve no idea what this is
	return s.containerName
}

func (s *sandbox) ContainerID() string {
	return s.ID()
}
func (s *sandbox) MetadataName() string {
	return s.config.GetMetadata().GetName()
}
func (s *sandbox) UID() string {
	return s.config.GetMetadata().GetUid()
}
func (s *sandbox) KubeNamespace() string {
	return s.config.GetMetadata().GetNamespace()
}
func (s *sandbox) Attempt() uint32 {
	return s.config.GetMetadata().GetAttempt()
}
// TODO FIXME save this information
func (s *sandbox) LabelOptions() []string {
	var labelOptions []string
	selinuxConfig := s.config.GetLinux().GetSecurityContext().GetSelinuxOptions()
	if selinuxConfig != nil {
		labelOptions = utils.GetLabelOptions(selinuxConfig)
	}
	return labelOptions
}
// TODO FIXME I don't like this function name
func (s *sandbox) Infra() bool {
	return true
}
func (s *sandbox) Privileged() bool {
	securityContext := s.Config().GetLinux().GetSecurityContext()
	if securityContext == nil {
		return false
	}

	if securityContext.Privileged {
		return true
	}

	namespaceOptions := securityContext.GetNamespaceOptions()
	if namespaceOptions == nil {
		return false
	}

	if namespaceOptions.GetNetwork() == pb.NamespaceMode_NODE ||
		namespaceOptions.GetPid() == pb.NamespaceMode_NODE ||
		namespaceOptions.GetIpc() == pb.NamespaceMode_NODE {
		return true
	}

	return false
}

// TODO FIXME
func (s *sandbox) IDMappings() *storage.IDMappingOptions {
	return nil
}
// TODO FIXME
func (s *sandbox) SetIDMappings(*storage.IDMappingOptions) {
}
