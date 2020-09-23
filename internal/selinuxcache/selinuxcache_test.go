package selinuxcache_test

import (
	"github.com/cri-o/cri-o/internal/selinuxcache"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var (
	testName   = "name"
	testPath   = "/dev/null"
	testLabel  = "s0,s1,s2"
	testShared = true
)

func successfulLabel(string, string, bool) error {
	return nil
}

func failingLabel(string, string, bool) error {
	return errors.New("fail")
}

func setupSUT() *selinuxcache.SELinuxCache {
	sut := selinuxcache.New()
	sut.AddSELinuxCacheEntry(testName)
	sut.SetLabelFunc(successfulLabel)
	return sut
}

// The actual test suite
var _ = t.Describe("SELinuxCache", func() {
	// Setup the SUT
	t.Describe("LabelContainerPath", func() {
		It("should fail to add a path to a non-existent container", func() {
			// Given
			sut := selinuxcache.New()
			// When
			err := sut.LabelContainerPath(testName, testPath, testLabel, testShared)
			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should succeed relabelling a new path", func() {
			// Given
			sut := setupSUT()
			// When
			err := sut.LabelContainerPath(testName, testPath, testLabel, testShared)
			// Then
			Expect(err).To(BeNil())
		})
		It("should fail if label fails", func() {
			// Given
			sut := setupSUT()
			sut.SetLabelFunc(failingLabel)
			// When
			err := sut.LabelContainerPath(testName, testPath, testLabel, testShared)
			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should not relabel if already succeeded", func() {
			// Given
			sut := setupSUT()
			err := sut.LabelContainerPath(testName, testPath, testLabel, testShared)
			Expect(err).To(BeNil())
			// we set this to fail so we know whether LabelContainerPath
			// attempts to label the path
			sut.SetLabelFunc(failingLabel)

			// When
			err = sut.LabelContainerPath(testName, testPath, testLabel, testShared)

			// Then
			Expect(err).To(BeNil())
		})
		It("should not relabel if already succeeded, even with newly created entry", func() {
			// Given
			sut := setupSUT()
			err := sut.LabelContainerPath(testName, testPath, testLabel, testShared)
			Expect(err).To(BeNil())
			// we set this to fail so we know whether LabelContainerPath
			// attempts to label the path
			sut.SetLabelFunc(failingLabel)
			sut.AddSELinuxCacheEntry(testName)

			// When
			err = sut.LabelContainerPath(testName, testPath, testLabel, testShared)

			// Then
			Expect(err).To(BeNil())
		})
		It("should relabel given new path", func() {
			// Given
			sut := setupSUT()
			err := sut.LabelContainerPath(testName, testPath, testLabel, testShared)
			Expect(err).To(BeNil())
			// we set this to fail so we know whether LabelContainerPath
			// attempts to label the path again
			sut.SetLabelFunc(failingLabel)
			sut.AddSELinuxCacheEntry(testName)

			// When
			err = sut.LabelContainerPath(testName, "path2", testLabel, testShared)

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should relabel given new label", func() {
			// Given
			sut := setupSUT()
			err := sut.LabelContainerPath(testName, testPath, testLabel, testShared)
			Expect(err).To(BeNil())
			// we set this to fail so we know whether LabelContainerPath
			// attempts to label the path again
			sut.SetLabelFunc(failingLabel)
			sut.AddSELinuxCacheEntry(testName)

			// When
			err = sut.LabelContainerPath(testName, testPath, "s1,s1,s2", testShared)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
