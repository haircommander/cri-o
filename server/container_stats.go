package server

import (
	"context"
	"path/filepath"

	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	crioStorage "github.com/cri-o/cri-o/utils"
	"github.com/pkg/errors"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) buildContainerStats(ctx context.Context, stats *oci.ContainerStats, container *oci.Container) *types.ContainerStats {
	// TODO: Fix this for other storage drivers. This will only work with overlay.
	var writableLayer *types.FilesystemUsage
	if s.ContainerServer.Config().RootConfig.Storage == "overlay" {
		diffDir := filepath.Join(filepath.Dir(container.MountPoint()), "diff")
		bytesUsed, inodeUsed, err := crioStorage.GetDiskUsageStats(diffDir)
		if err != nil {
			log.Warnf(ctx, "Unable to get disk usage for container %s， %s", container.ID(), err)
		}
		writableLayer = &types.FilesystemUsage{
			Timestamp:  stats.SystemNano,
			FsId:       &types.FilesystemIdentifier{Mountpoint: container.MountPoint()},
			UsedBytes:  &types.UInt64Value{Value: bytesUsed},
			InodesUsed: &types.UInt64Value{Value: inodeUsed},
		}
	}
	return &types.ContainerStats{
		Attributes: &types.ContainerAttributes{
			Id: container.ID(),
			Metadata: &types.ContainerMetadata{
				Name:    container.Metadata().Name,
				Attempt: container.Metadata().Attempt,
			},
			Labels:      container.Labels(),
			Annotations: container.Annotations(),
		},
		Cpu: &types.CpuUsage{
			Timestamp:            stats.SystemNano,
			UsageCoreNanoSeconds: &types.UInt64Value{Value: stats.CPUNano},
		},
		Memory: &types.MemoryUsage{
			Timestamp:       stats.SystemNano,
			WorkingSetBytes: &types.UInt64Value{Value: stats.WorkingSetBytes},
		},
		WritableLayer: writableLayer,
	}
}

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Server) ContainerStats(ctx context.Context, req *types.ContainerStatsRequest) (*types.ContainerStatsResponse, error) {
	container, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}
	sb := s.GetSandbox(container.Sandbox())
	if sb == nil {
		return nil, errors.Errorf("unable to get stats for container %s: sandbox %s not found", container.ID(), container.Sandbox())
	}
	cgroup := sb.CgroupParent()

	stats, err := s.Runtime().ContainerStats(ctx, container, cgroup)
	if err != nil {
		return nil, err
	}

	return &types.ContainerStatsResponse{Stats: s.buildContainerStats(ctx, stats, container)}, nil
}
