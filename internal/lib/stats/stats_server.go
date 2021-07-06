package statsserver

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"
	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type StatsServer struct {
	shutdown        chan struct{}
	updateFrequency time.Duration
	runtime         *oci.Runtime
	sandboxes       sandbox.Storer
	store           cstorage.Store
	cgroupMgr       cgmgr.CgroupManager
	sboxStats       map[string]*types.PodSandboxStats
	ctrStats        map[string]*types.ContainerStats
	sync.RWMutex
}

func New(runtime *oci.Runtime, store cstorage.Store, manager cgmgr.CgroupManager) *StatsServer {
	ss := &StatsServer{
		shutdown:        make(chan struct{}, 1),
		updateFrequency: time.Second * 10,
		runtime:         runtime,
		store:           store,
		sandboxes:       sandbox.NewMemoryStore(),
		cgroupMgr:       manager,
		sboxStats:       make(map[string]*types.PodSandboxStats),
		ctrStats:        make(map[string]*types.ContainerStats),
	}
	go ss.updateLoop()
	return ss
}

func (ss *StatsServer) updateLoop() {
	for {
		select {
		case <-ss.shutdown:
			return
		case <-time.After(ss.updateFrequency):
		}
		ss.update()
	}
}

func (ss *StatsServer) AddSandbox(sb *sandbox.Sandbox) {
	ss.Lock()
	defer ss.Unlock()
	ss.sandboxes.Add(sb.ID(), sb)
}

func (ss *StatsServer) RemoveSandbox(sb *sandbox.Sandbox) {
	ss.Lock()
	defer ss.Unlock()
	ss.sandboxes.Delete(sb.ID())
	delete(ss.sboxStats, sb.ID())
}

func (ss *StatsServer) StatsForSandbox(sb *sandbox.Sandbox) *types.PodSandboxStats {
	ss.RLock()
	defer ss.RUnlock()
	return ss.statsForSandbox(sb)
}

func (ss *StatsServer) StatsForSandboxes(sboxes []*sandbox.Sandbox) []*types.PodSandboxStats {
	ss.RLock()
	defer ss.RUnlock()
	stats := make([]*types.PodSandboxStats, 0, len(sboxes))
	for _, sb := range sboxes {
		if stat := ss.statsForSandbox(sb); stat != nil {
			stats = append(stats, stat)
		}
	}
	return stats
}

func (ss *StatsServer) statsForSandbox(sb *sandbox.Sandbox) *types.PodSandboxStats {
	sboxStat, ok := ss.sboxStats[sb.ID()]
	if !ok {
		return nil
	}
	return sboxStat
}

func (ss *StatsServer) StatsForContainer(c *oci.Container, sb *sandbox.Sandbox) *types.ContainerStats {
	ss.RLock()
	defer ss.RUnlock()
	return ss.statsForContainer(c)
}

func (ss *StatsServer) StatsForContainers(ctrs []*oci.Container) []*types.ContainerStats {
	ss.RLock()
	defer ss.RUnlock()
	stats := make([]*types.ContainerStats, 0, len(ctrs))
	for _, c := range ctrs {
		if stat := ss.statsForContainer(c); stat != nil {
			stats = append(stats, stat)
		}
	}
	return stats
}

func (ss *StatsServer) statsForContainer(c *oci.Container) *types.ContainerStats {
	ctrStat, ok := ss.ctrStats[c.ID()]
	if !ok {
		return nil
	}
	return ctrStat
}

func (ss *StatsServer) Shutdown() {
	ss.shutdown <- struct{}{}
}

func (ss *StatsServer) update() {
	ss.Lock()
	defer ss.Unlock()

	for _, sb := range ss.sandboxes.List() {
		ss.updateSandbox(sb)
	}
}

func (ss *StatsServer) updateSandbox(sb *sandbox.Sandbox) {
	if sb == nil {
		return
	}
	sandboxStats := &types.PodSandboxStats{
		Attributes: &types.PodSandboxAttributes{
			ID:          sb.ID(),
			Labels:      sb.Labels(),
			Metadata:    sb.Metadata(),
			Annotations: sb.Annotations(),
		},
	}
	if err := ss.cgroupMgr.PopulateSandboxCgroupStats(sb.CgroupParent(), sandboxStats); err != nil {
		logrus.Errorf("Error getting sandbox stats %s: %v", sb.ID(), err)
	}
	if err := ss.populateNetworkStats(sandboxStats, sb); err != nil {
		logrus.Errorf("Error adding network stats for sandbox %s: %v", sb.ID(), err)
	}
	containerStats := make([]*types.ContainerStats, 0, len(sb.Containers().List()))
	for _, c := range sb.Containers().List() {
		if c.StateNoLock().Status == oci.ContainerStateStopped {
			continue
		}
		cStats, err := ss.runtime.ContainerStats(context.TODO(), c, sb.CgroupParent())
		if err != nil {
			logrus.Errorf("Error getting container stats %s: %v", c.ID(), err)
			continue
		}
		ss.populateWritableLayer(cStats, c)
		if oldcStats, ok := ss.ctrStats[c.ID()]; ok {
			updateUsageNanoCores(oldcStats.CPU, cStats.CPU)
		}
		ss.ctrStats[c.ID()] = cStats
		containerStats = append(containerStats, cStats)
	}
	sandboxStats.Containers = containerStats
	if old, ok := ss.sboxStats[sb.ID()]; ok {
		updateUsageNanoCores(old.CPU, sandboxStats.CPU)
	}
	ss.sboxStats[sb.ID()] = sandboxStats
}

func updateUsageNanoCores(old *types.CPUUsage, current *types.CPUUsage) {
	if old == nil || current == nil || old.UsageCoreNanoSeconds == nil || current.UsageCoreNanoSeconds == nil {
		return
	}

	nanoSeconds := current.Timestamp - old.Timestamp

	usageNanoCores := uint64(float64(current.UsageCoreNanoSeconds.Value-old.UsageCoreNanoSeconds.Value) /
		float64(nanoSeconds) * float64(time.Second/time.Nanosecond))

	current.UsageNanoCores = &types.UInt64Value{
		Value: usageNanoCores,
	}
}

func (ss *StatsServer) populateWritableLayer(stats *types.ContainerStats, container *oci.Container) {
	writableLayer, err := ss.writableLayerForContainer(stats, container)
	if err != nil {
		logrus.Errorf("%v", err)
	}
	stats.WritableLayer = writableLayer
}

func (ss *StatsServer) writableLayerForContainer(stats *types.ContainerStats, container *oci.Container) (*types.FilesystemUsage, error) {
	writableLayer := &types.FilesystemUsage{
		Timestamp: time.Now().UnixNano(),
		FsID:      &types.FilesystemIdentifier{Mountpoint: container.MountPoint()},
	}
	driver, err := ss.store.GraphDriver()
	if err != nil {
		return writableLayer, errors.Wrapf(err, "Unable to get graph driver for disk usage for container %s", container.ID())
	}
	id := filepath.Base(filepath.Dir(container.MountPoint()))
	usage, err := driver.ReadWriteDiskUsage(id)
	if err != nil {
		return writableLayer, errors.Wrapf(err, "Unable to get disk usage for container %s", container.ID())
	}
	writableLayer.UsedBytes = &types.UInt64Value{Value: uint64(usage.Size)}
	writableLayer.InodesUsed = &types.UInt64Value{Value: uint64(usage.InodeCount)}

	return writableLayer, nil
}

func (ss *StatsServer) populateNetworkStats(stats *types.PodSandboxStats, sandbox *sandbox.Sandbox) error {
	return ns.WithNetNSPath(sandbox.NetNsPath(), func(_ ns.NetNS) error { // nolint: errcheck
		links, err := netlink.LinkList()
		if err != nil {
			logrus.Errorf("unable to retrieve network namespace links: %v", err)
			return err
		}
		stats.Network = &types.NetworkStats{
			Interfaces: make([]*types.InterfaceStats, 0, len(links)-1),
		}
		for i := range links {
			iface, err := linkToInterface(links[i])
			if err != nil {
				logrus.Errorf("Failed to %v for pod %s", err, sandbox.ID())
				continue
			}
			// TODO FIXME or DefaultInterfaceName?
			if i == 0 {
				stats.Network.DefaultInterface = iface
			} else {
				stats.Network.Interfaces = append(stats.Network.Interfaces, iface)
			}
		}
		return nil
	})
}

func linkToInterface(link netlink.Link) (*types.InterfaceStats, error) {
	attrs := link.Attrs()
	if attrs == nil {
		return nil, errors.New("get stats for iface")
	}
	if attrs.Statistics == nil {
		return nil, errors.Errorf("get stats for iface %s", attrs.Name)
	}
	return &types.InterfaceStats{
		Name:     attrs.Name,
		RxBytes:  &types.UInt64Value{Value: attrs.Statistics.RxBytes},
		RxErrors: &types.UInt64Value{Value: attrs.Statistics.RxErrors},
		TxBytes:  &types.UInt64Value{Value: attrs.Statistics.TxBytes},
		TxErrors: &types.UInt64Value{Value: attrs.Statistics.TxErrors},
	}, nil
}
