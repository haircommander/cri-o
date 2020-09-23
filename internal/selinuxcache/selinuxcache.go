package selinuxcache

import (
	"fmt"

	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// SELinuxCache is the centralized point that tracks
// container SELinux labels before the container is created.
// After a container name is registered, a cache entry should be added.
// Once the container is marked as successfully created, its cache
// entry should be removed.
// If a container fails to be created because the context times out,
// the SELinuxCache allows the server to not redo potentially
// expensive SELinux labelling operations, potentially saving
// the server from timing out multiple times.
// Additions to the SELinux cache and any updates should
// be protected by a lock by the callers.
// The server's updateLock is a good candidate for this.
type SELinuxCache struct {
	cache     map[string]*entry
	labelFunc labeller
}

type labeller func(string, string, bool) error

// New creates a new SELinuxCache
func New() *SELinuxCache {
	return &SELinuxCache{
		cache:     make(map[string]*entry),
		labelFunc: label.Relabel,
	}
}

// An entry contains data for a single container
// it tracks the paths that have been chowned by this label
// already
type entry struct {
	paths map[string]string
}

// AddSELinuxCacheEntry should be called after a container name is reserved.
// It specifically should be called before SELinuxContainerPath() is called
// for this container name.
func (s *SELinuxCache) AddSELinuxCacheEntry(name string) {
	if _, ok := s.cache[name]; ok {
		return
	}

	s.cache[name] = &entry{
		paths: make(map[string]string),
	}
}

// RemoveSELinuxCacheEntry should be called after a container is successfully created
// It wiptes the cache entry for that container.
// If it is not called, subsequent attempts to add the entry will be skipped and
// old label information will be used
func (s *SELinuxCache) RemoveSELinuxCacheEntry(name string) {
	delete(s.cache, name)
}

func (s *SELinuxCache) LabelContainerPath(name, path, secLabel string, shared bool) error {
	entryForName, ok := s.cache[name]
	// sanity check, a name should always be registered before
	// we are asked to label
	if !ok {
		return errors.Errorf("container %s not registered with SELinuxCache", name)
	}

	// If we have an entry that matches this container name, path and label
	// then we've already done the labelling
	if cachedLabel, ok := entryForName.paths[path]; ok {
		if cachedLabel == secLabel {
			logrus.Debugf("SELinux cache hit for container %s in path %s, no need to relabel", name, path)
			return nil
		}
	}

	if err := s.labelFunc(path, secLabel, shared); err != nil && !errors.Is(err, unix.ENOTSUP) {
		return fmt.Errorf("relabel failed %s: %v", path, err)
	}
	entryForName.paths[path] = secLabel
	return nil
}
