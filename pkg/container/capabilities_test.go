package container_test

import (
	"github.com/cri-o/cri-o/pkg/container"
	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("Container", func() {
	var config *types.ContainerConfig
	var sboxConfig *types.PodSandboxConfig
	const defaultMounts = 6
	BeforeEach(func() {
		config = &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{Name: "name"},
		}
		sboxConfig = &types.PodSandboxConfig{}
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())
	})
	Context("SetupCapabilities", func() {
		It("should allow nil capabilities", func() {
			// Given
			var caps *types.Capability
			// When
			err := sut.SetupCapabilities(caps)
			// Then
			Expect(err).To(BeNil())
			verifyCapsEmpty(sut)
		})
		It("should add capability", func() {
			// Given
			caps := &types.Capability{
				AddCapabilities: []string{"CAP_CHOWN"},
			}
			// When
			err := sut.SetupCapabilities(caps)
			// Then
			Expect(err).To(BeNil())
			verifyCapAdded(sut, "CAP_CHOWN")
		})
		It("should add cap prefix", func() {
			// Given
			caps := &types.Capability{
				AddCapabilities: []string{"chown"},
			}
			// When
			err := sut.SetupCapabilities(caps)
			// Then
			Expect(err).To(BeNil())
			verifyCapAdded(sut, "CAP_CHOWN")
		})
		It("should drop capability", func() {
			// Given
			caps := &types.Capability{
				DropCapabilities: []string{"CAP_CHOWN"},
			}
			// When
			err := sut.SetupCapabilities(caps)
			// Then
			Expect(err).To(BeNil())
			verifyCapDropped(sut, "CAP_CHOWN")
		})
		It("should drop last", func() {
			// Given
			caps := &types.Capability{
				AddCapabilities:  []string{"chown"},
				DropCapabilities: []string{"chown"},
			}
			// When
			err := sut.SetupCapabilities(caps)
			// Then
			Expect(err).To(BeNil())
			verifyCapDropped(sut, "CAP_CHOWN")
		})
		It("should add given drop all", func() {
			// Given
			caps := &types.Capability{
				AddCapabilities:  []string{"chown"},
				DropCapabilities: []string{"all"},
			}
			// When
			err := sut.SetupCapabilities(caps)
			// Then
			Expect(err).To(BeNil())
			verifyCapAdded(sut, "CAP_CHOWN")
		})
		It("should drop given add all", func() {
			// Given
			caps := &types.Capability{
				AddCapabilities:  []string{"all"},
				DropCapabilities: []string{"chown"},
			}
			// When
			err := sut.SetupCapabilities(caps)
			// Then
			Expect(err).To(BeNil())
			verifyCapDropped(sut, "CAP_CHOWN")
		})
		It("should fail to add unknown", func() {
			// Given
			caps := &types.Capability{
				AddCapabilities: []string{"fake"},
			}
			// When
			err := sut.SetupCapabilities(caps)
			// Then
			Expect(err).NotTo(BeNil())
			verifyCapsEmpty(sut)
		})
		It("should fail to drop unknown", func() {
			// Given
			caps := &types.Capability{
				AddCapabilities: []string{"fake"},
			}
			// When
			err := sut.SetupCapabilities(caps)
			// Then
			Expect(err).NotTo(BeNil())
			verifyCapsEmpty(sut)
		})
	})
})

func verifyCapsEmpty(sut container.Container) {
	Expect(sut.Spec().Config.Process.Capabilities.Ambient).To(BeEmpty())
	Expect(sut.Spec().Config.Process.Capabilities.Bounding).To(BeEmpty())
	Expect(sut.Spec().Config.Process.Capabilities.Effective).To(BeEmpty())
	Expect(sut.Spec().Config.Process.Capabilities.Inheritable).To(BeEmpty())
	Expect(sut.Spec().Config.Process.Capabilities.Permitted).To(BeEmpty())
}

func verifyCapAdded(sut container.Container, cap string) {
	Expect(sut.Spec().Config.Process.Capabilities.Ambient).To(BeEmpty())
	Expect(sut.Spec().Config.Process.Capabilities.Bounding).To(ContainElement("CAP_CHOWN"))
	Expect(sut.Spec().Config.Process.Capabilities.Effective).To(ContainElement("CAP_CHOWN"))
	Expect(sut.Spec().Config.Process.Capabilities.Inheritable).To(ContainElement("CAP_CHOWN"))
	Expect(sut.Spec().Config.Process.Capabilities.Permitted).To(ContainElement("CAP_CHOWN"))
}

func verifyCapDropped(sut container.Container, cap string) {
	Expect(sut.Spec().Config.Process.Capabilities.Ambient).To(BeEmpty())
	Expect(sut.Spec().Config.Process.Capabilities.Bounding).NotTo(ContainElement("CAP_CHOWN"))
	Expect(sut.Spec().Config.Process.Capabilities.Effective).NotTo(ContainElement("CAP_CHOWN"))
	Expect(sut.Spec().Config.Process.Capabilities.Inheritable).NotTo(ContainElement("CAP_CHOWN"))
	Expect(sut.Spec().Config.Process.Capabilities.Permitted).NotTo(ContainElement("CAP_CHOWN"))
}
