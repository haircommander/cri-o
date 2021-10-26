package v1alpha2

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) UpdateContainerResources(
	ctx context.Context, req *pb.UpdateContainerResourcesRequest,
) (*pb.UpdateContainerResourcesResponse, error) {
	r := &types.UpdateContainerResourcesRequest{
		ContainerId: req.ContainerId,
	}
	if req.Linux != nil {
		r.Linux = &types.LinuxContainerResources{
			CpuPeriod:          req.Linux.CpuPeriod,
			CpuQuota:           req.Linux.CpuQuota,
			CpuShares:          req.Linux.CpuShares,
			MemoryLimitInBytes: req.Linux.MemoryLimitInBytes,
			OomScoreAdj:        req.Linux.OomScoreAdj,
			CpusetCpus:         req.Linux.CpusetCpus,
			CpusetMems:         req.Linux.CpusetMems,
		}
		hugePageLimits := []*types.HugepageLimit{}
		for _, x := range req.Linux.HugepageLimits {
			hugePageLimits = append(hugePageLimits, &types.HugepageLimit{
				PageSize: x.PageSize,
				Limit:    x.Limit,
			})
		}
		r.Linux.HugepageLimits = hugePageLimits
	}

	if err := s.server.UpdateContainerResources(ctx, r); err != nil {
		return nil, err
	}

	return &pb.UpdateContainerResourcesResponse{}, nil
}
