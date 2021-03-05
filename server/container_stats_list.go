package server

import (
	"github.com/containers/libpod/v2/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ListContainerStats returns stats of all running containers.
func (s *Server) ListContainerStats(ctx context.Context, req *pb.ListContainerStatsRequest) (*pb.ListContainerStatsResponse, error) {
	ctrList, err := s.ContainerServer.ListContainers(
		func(container *oci.Container) bool {
			return container.StateNoLock().Status != oci.ContainerStateStopped
		},
	)
	if err != nil {
		return nil, err
	}
	filter := req.GetFilter()
	if filter != nil {
		cFilter := &pb.ContainerFilter{
			Id:            req.Filter.Id,
			PodSandboxId:  req.Filter.PodSandboxId,
			LabelSelector: req.Filter.LabelSelector,
		}
		ctrList = s.filterContainerList(ctx, cFilter, ctrList)
	}

	allStats := make([]*pb.ContainerStats, 0, len(ctrList))
	for _, container := range ctrList {
		sb := s.GetSandbox(container.Sandbox())
		if sb == nil {
			// Because we don't lock, we will get situations where the container was listed, and then
			// its sandbox was deleted before we got to checking its stats.
			// We should not log in this expected situation.
			continue
		}
		cgroup := sb.CgroupParent()
		stats, err := s.Runtime().ContainerStats(container, cgroup)
		if err != nil {
			// ErrCgroupDeleted is another situation that will happen if the container
			// is deleted from underneath the call to this function.
			if !errors.Is(err, cgroups.ErrCgroupDeleted) {
				// The other errors are much less likely, and possibly useful to hear about.
				log.Warnf(ctx, "unable to get stats for container %s: %v", container.ID(), err)
			}
			continue
		}
		response := s.buildContainerStats(ctx, stats, container)
		allStats = append(allStats, response)
	}

	return &pb.ListContainerStatsResponse{
		Stats: allStats,
	}, nil
}
