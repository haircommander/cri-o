package v1

import (
	"context"
	"strconv"

	"github.com/cri-o/cri-o/internal/log"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) MetricsEndpoints(
	ctx context.Context, req *pb.MetricsEndpointsRequest,
) (*pb.MetricsEndpointsResponse, error) {
	log.Infof(ctx, "%d", s.server.Config().MetricsConfig.MetricsPort)
	return &pb.MetricsEndpointsResponse{
		Endpoints: []*pb.MetricsEndpoint{
			{
				Host: "localhost:" + strconv.Itoa(s.server.Config().MetricsConfig.MetricsPort),
				// Path the Kubelet will register to pull from the source.
				ForwardPath: "/metrics/cadvisor2",
				// Path the runtime will register as the source.
				OriginPath: "/metrics",
			},
		},
	}, nil
}
