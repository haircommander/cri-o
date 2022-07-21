package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) CheckpointContainer(
	ctx context.Context, req *pb.CheckpointContainerRequest,
) (*pb.CheckpointContainerResponse, error) {
	return nil, nil
}
