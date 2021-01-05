package server

import (
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
	stats, errs := s.CRIStatsForContainers(ctx, container)
	if len(errs) > 0 {
		return nil, errs[0]
	}
	// should never happen, but we should avoid the segfault
	if stats == nil || len(stats) != 1 {
		return nil, errors.Errorf("Unknown error happened finding container stats for %s", req.ContainerId)
	}

	return &pb.ContainerStatsResponse{Stats: stats[0]}, nil
}

func (s *Server) CRIStatsForContainers(ctx context.Context, containers ...*oci.Container) ([]*pb.ContainerStats, []error) {
	stats := make([]*pb.ContainerStats, 0)
	errs := make([]error, 0)
	for _, c := range containers {
		sb := s.GetSandbox(c.Sandbox())
		if sb == nil {
			errs = append(errs, errors.Errorf("unable to get stats for container %s: sandbox %s not found", c.ID(), c.Sandbox()))
			continue
		}
		cgroup := sb.CgroupParent()

		ociStat, err := s.Runtime().ContainerStats(c, cgroup)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		stats = append(stats, s.buildContainerStats(ociStat, c))
	}

	if err := s.config.StatsManager().UpdateWithDiskStats(stats); err != nil {
		errs = append(errs, err)
	}

	return stats, errs
}

// buildContainerStats takes stats directly from the container, and attempts to inject the filesystem
// usage of the container.
// This is not taken care of by the container because we access information on the server level (storage driver).
func (s *Server) buildContainerStats(stats *oci.ContainerStats, container *oci.Container) *pb.ContainerStats {
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
		WritableLayer: &pb.FilesystemUsage{
			Timestamp: stats.SystemNano,
			FsId:      &pb.FilesystemIdentifier{Mountpoint: container.MountPoint()},
		},
	}
}
