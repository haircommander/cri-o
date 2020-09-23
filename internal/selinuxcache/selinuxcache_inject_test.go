// +build test

package selinuxcache

func (s *SELinuxCache) SetLabelFunc(lf labeller) {
	s.labelFunc = lf
}
