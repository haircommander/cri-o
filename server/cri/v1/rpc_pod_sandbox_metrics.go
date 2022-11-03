package v1

import (
	"context"
	"time"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ListPodSandboxMetrics(
	ctx context.Context, req *pb.ListPodSandboxMetricsRequest,
) (*pb.ListPodSandboxMetricsResponse, error) {
	return &pb.ListPodSandboxMetricsResponse{
		PodMetrics: []*pb.PodSandboxMetrics{
			{
				PodSandboxId: "pod",
				ContainerMetrics: []*pb.ContainerMetrics{
					{
						Metrics: []*pb.Metric{
							{
								Name:      "container_memory",
								Help:      "help container memory",
								Timestamp: time.Now().UnixNano(),
								Labels: []*pb.LabelPair{
									{Name: "id", Value: "ctr id"},
									{Name: "name", Value: "ctr name"},
								},
								Value:      &pb.UInt64Value{Value: 100},
								MetricType: pb.MetricType_COUNTER,
							},
						},
						ContainerId: "ctr",
					},
				},
			},
		},
	}, nil
}

//
//		set := map[string]string{
//			metrics.LabelID:    c.Name,
//			metrics.LabelName:  name,
//			metrics.LabelImage: image,
//			"pod":              podName,
//			"namespace":        namespace,
//			"container":        containerName,
//		}

//type Metric struct {
//	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
//	Help string `protobuf:"bytes,2,opt,name=help,proto3" json:"help,omitempty"`
//	// return 0 if metric is cached
//	Timestamp            int64        `protobuf:"varint,3,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
//	Labels               []*LabelPair `protobuf:"bytes,4,rep,name=labels,proto3" json:"labels,omitempty"`
//	MetricType           MetricType   `protobuf:"varint,5,opt,name=metric_type,json=metricType,proto3,enum=runtime.v1.MetricType" json:"metric_type,omitempty"`
//	Value                *UInt64Value `protobuf:"bytes,6,opt,name=value,proto3" json:"value,omitempty"`
//	XXX_NoUnkeyedLiteral struct{}     `json:"-"`
//	XXX_sizecache        int32        `json:"-"`
//}
