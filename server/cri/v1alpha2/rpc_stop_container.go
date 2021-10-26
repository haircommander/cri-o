package v1alpha2

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) StopContainer(
	ctx context.Context, req *pb.StopContainerRequest,
) (*pb.StopContainerResponse, error) {
	r := &types.StopContainerRequest{
		ContainerId: req.ContainerId,
		Timeout:     req.Timeout,
	}
	if err := s.server.StopContainer(ctx, r); err != nil {
		return nil, err
	}
	return &pb.StopContainerResponse{}, nil
}
