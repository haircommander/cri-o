package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) MetricsEndpoints(
	ctx context.Context, req *pb.MetricsEndpointsRequest,
) (*pb.MetricsEndpointsResponse, error) {
	return &pb.MetricsEndpointsResponse{
		Endpoints: []*pb.MetricsEndpoint{
			{
				Host: "localhost:" + string(s.server.Config().MetricsConfig.MetricsPort),
				// Path the Kubelet will register to pull from the source.
				ForwardPath: "/metrics/cadvisor",
				// Path the runtime will register as the source.
				OriginPath: "/metrics",
			},
		},
	}, nil
}
