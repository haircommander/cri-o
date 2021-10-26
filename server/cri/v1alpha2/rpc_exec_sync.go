package v1alpha2

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ExecSync(
	ctx context.Context, req *pb.ExecSyncRequest,
) (*pb.ExecSyncResponse, error) {
	r := &types.ExecSyncRequest{
		ContainerId: req.ContainerId,
		Cmd:         req.Cmd,
		Timeout:     req.Timeout,
	}
	res, err := s.server.ExecSync(ctx, r)
	if err != nil {
		return nil, err
	}
	return &pb.ExecSyncResponse{
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		ExitCode: res.ExitCode,
	}, nil
}
