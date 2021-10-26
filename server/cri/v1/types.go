package v1

import (
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func NewPodSandboxConfig() *pb.PodSandboxConfig {
	return &pb.PodSandboxConfig{
		Metadata:     &pb.PodSandboxMetadata{},
		DnsConfig:    &pb.DNSConfig{},
		PortMappings: []*pb.PortMapping{},
		Linux:        NewLinuxPodSandboxConfig(),
	}
}

func NewLinuxPodSandboxConfig() *pb.LinuxPodSandboxConfig {
	return &pb.LinuxPodSandboxConfig{
		SecurityContext: NewLinuxSandboxSecurityContext(),
	}
}

func NewLinuxSandboxSecurityContext() *pb.LinuxSandboxSecurityContext {
	return &pb.LinuxSandboxSecurityContext{
		NamespaceOptions: &pb.NamespaceOption{},
		SelinuxOptions:   &pb.SELinuxOption{},
		RunAsUser:        &pb.Int64Value{},
		RunAsGroup:       &pb.Int64Value{},
	}
}

func NewContainerConfig() *pb.ContainerConfig {
	return &pb.ContainerConfig{
		Metadata: &pb.ContainerMetadata{},
		Image:    &pb.ImageSpec{},
		Linux:    NewLinuxContainerConfig(),
	}
}

func NewLinuxContainerConfig() *pb.LinuxContainerConfig {
	return &pb.LinuxContainerConfig{
		Resources:       &pb.LinuxContainerResources{},
		SecurityContext: NewLinuxContainerSecurityContext(),
	}
}

func NewLinuxContainerSecurityContext() *pb.LinuxContainerSecurityContext {
	return &pb.LinuxContainerSecurityContext{
		Capabilities:     &pb.Capability{},
		NamespaceOptions: &pb.NamespaceOption{},
		SelinuxOptions:   &pb.SELinuxOption{},
		RunAsUser:        &pb.Int64Value{},
		RunAsGroup:       &pb.Int64Value{},
	}
}
