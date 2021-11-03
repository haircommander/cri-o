package server_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("PodSandboxRemove", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("PodSandboxRemove", func() {
		It("should fail when sandbox is not created", func() {
			// Given
			Expect(sut.AddSandbox(testSandbox)).To(BeNil())
			Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(BeNil())

			// When
			err := sut.RemovePodSandbox(context.Background(),
				&types.RemovePodSandboxRequest{PodSandboxID: testSandbox.ID()})

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with empty sandbox ID", func() {
			// When
			err := sut.StopPodSandbox(context.Background(),
				&types.StopPodSandboxRequest{})

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
