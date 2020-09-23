package selinuxcache_test

import (
	"testing"

	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// TestSELinuxCache runs the created specs
func TestSELinuxCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "SELinuxCache")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	logrus.SetLevel(logrus.PanicLevel)
})

var _ = AfterSuite(func() {
	t.Teardown()
})
