package server

import (
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Server) ContainerStats(ctx context.Context, req *pb.ContainerStatsRequest) (*pb.ContainerStatsResponse, error) {
	container, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}
	sb := s.GetSandbox(container.Sandbox())
	if sb == nil {
		return nil, errors.Errorf("unable to get stats for container %s: sandbox %s not found", container.ID(), container.Sandbox())
	}
	cgroup := sb.CgroupParent()

	stats, err := s.Runtime().ContainerStats(container, cgroup)
	if err != nil {
		return nil, err
	}

	return &pb.ContainerStatsResponse{Stats: s.buildContainerStats(ctx, stats, container)}, nil
}

// buildContainerStats takes stats directly from the container, and attempts to inject the filesystem
// usage of the container.
// This is not taken care of by the container because we access information on the server level (storage driver).
func (s *Server) buildContainerStats(ctx context.Context, stats *oci.ContainerStats, container *oci.Container) *pb.ContainerStats {
	writableLayer, err := s.writableLayerForContainer(stats, container)
	if err != nil {
		log.Warnf(ctx, "%v", err)
	}
	return &pb.ContainerStats{
		Attributes: &pb.ContainerAttributes{
			Id: container.ID(),
			Metadata: &pb.ContainerMetadata{
				Name:    container.Metadata().Name,
				Attempt: container.Metadata().Attempt,
			},
			Labels:      container.Labels(),
			Annotations: container.Annotations(),
		},
		Cpu: &pb.CpuUsage{
			Timestamp:            stats.SystemNano,
			UsageCoreNanoSeconds: &pb.UInt64Value{Value: stats.CPUNano},
		},
		Memory: &pb.MemoryUsage{
			Timestamp:       stats.SystemNano,
			WorkingSetBytes: &pb.UInt64Value{Value: stats.WorkingSetBytes},
		},
		WritableLayer: writableLayer,
	}
}

func (s *Server) writableLayerForContainer(stats *oci.ContainerStats, container *oci.Container) (*pb.FilesystemUsage, error) {
	writableLayer := &pb.FilesystemUsage{
		Timestamp: stats.SystemNano,
		FsId:      &pb.FilesystemIdentifier{Mountpoint: container.MountPoint()},
	}
	driver, err := s.Store().GraphDriver()
	if err != nil {
		return writableLayer, errors.Wrapf(err, "unable to get graph driver for disk usage for container %s", container.ID())
	}

	usage, err := driver.ReadWriteDiskUsage(container.ID())
	if err != nil {
		return writableLayer, errors.Wrapf(err, "unable to get disk usage for container %s", container.ID())
	}
	writableLayer.UsedBytes = &pb.UInt64Value{Value: uint64(usage.Size)}
	writableLayer.InodesUsed = &pb.UInt64Value{Value: uint64(usage.InodeCount)}
	return writableLayer, nil
}
