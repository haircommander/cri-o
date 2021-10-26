package v1alpha2

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ContainerStats(
	ctx context.Context, req *pb.ContainerStatsRequest,
) (*pb.ContainerStatsResponse, error) {
	r := &types.ContainerStatsRequest{ContainerId: req.ContainerId}
	res, err := s.server.ContainerStats(ctx, r)
	if err != nil {
		return nil, err
	}
	resp := &pb.ContainerStatsResponse{}
	if res.Stats != nil {
		resp.Stats = &pb.ContainerStats{}
		if res.Stats.Attributes != nil {
			resp.Stats.Attributes = &pb.ContainerAttributes{
				Id:          res.Stats.Attributes.Id,
				Labels:      res.Stats.Attributes.Labels,
				Annotations: res.Stats.Attributes.Annotations,
			}
			if res.Stats.Attributes.Metadata != nil {
				resp.Stats.Attributes.Metadata = &pb.ContainerMetadata{
					Name:    res.Stats.Attributes.Metadata.Name,
					Attempt: res.Stats.Attributes.Metadata.Attempt,
				}
			}
		}
		if res.Stats.Cpu != nil {
			resp.Stats.Cpu = &pb.CpuUsage{
				Timestamp: res.Stats.Cpu.Timestamp,
			}
			if res.Stats.Cpu.UsageCoreNanoSeconds != nil {
				resp.Stats.Cpu.UsageCoreNanoSeconds = &pb.UInt64Value{
					Value: res.Stats.Cpu.UsageCoreNanoSeconds.Value,
				}
			}
		}
		if res.Stats.Memory != nil {
			resp.Stats.Memory = &pb.MemoryUsage{
				Timestamp: res.Stats.Memory.Timestamp,
			}
			if res.Stats.Memory.WorkingSetBytes != nil {
				resp.Stats.Memory.WorkingSetBytes = &pb.UInt64Value{
					Value: res.Stats.Memory.WorkingSetBytes.Value,
				}
			}
		}
		if res.Stats.WritableLayer != nil {
			resp.Stats.WritableLayer = &pb.FilesystemUsage{
				Timestamp: res.Stats.WritableLayer.Timestamp,
			}
			if res.Stats.WritableLayer.FsId != nil {
				resp.Stats.WritableLayer.FsId = &pb.FilesystemIdentifier{
					Mountpoint: res.Stats.WritableLayer.FsId.Mountpoint,
				}
			}
			if res.Stats.WritableLayer.UsedBytes != nil {
				resp.Stats.WritableLayer.UsedBytes = &pb.UInt64Value{
					Value: res.Stats.WritableLayer.UsedBytes.Value,
				}
			}
			if res.Stats.WritableLayer.InodesUsed != nil {
				resp.Stats.WritableLayer.InodesUsed = &pb.UInt64Value{
					Value: res.Stats.WritableLayer.InodesUsed.Value,
				}
			}
		}
	}
	return resp, nil
}
