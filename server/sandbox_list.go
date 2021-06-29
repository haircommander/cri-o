package server

import (
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/cri/types"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/fields"
)

// ListPodSandbox returns a list of SandBoxes.
func (s *Server) ListPodSandbox(ctx context.Context, req *types.ListPodSandboxRequest) (*types.ListPodSandboxResponse, error) {
	podList := s.filterSandboxList(ctx, req.Filter, s.ContainerServer.ListSandboxes())
	respList := make([]*types.PodSandbox, 0, len(podList))

	for _, sb := range podList {
		pod := &types.PodSandbox{
			ID:          sb.ID(),
			CreatedAt:   sb.CreatedAt().UnixNano(),
			State:       sb.State(),
			Labels:      sb.Labels(),
			Annotations: sb.Annotations(),
			Metadata: &types.PodSandboxMetadata{
				Name:      sb.Metadata().Name,
				UID:       sb.Metadata().UID,
				Namespace: sb.Metadata().Namespace,
				Attempt:   sb.Metadata().Attempt,
			},
		}
		respList = append(respList, pod)
	}

	return &types.ListPodSandboxResponse{
		Items: respList,
	}, nil
}

// filterSandboxList applies a protobuf-defined filter to retrieve only intended pod sandboxes. Not matching
// the filter is not considered an error but will return an empty response.
func (s *Server) filterSandboxList(ctx context.Context, filter *types.PodSandboxFilter, podList []*sandbox.Sandbox) []*sandbox.Sandbox {
	// Filter by pod id first.
	if filter == nil {
		return podList
	}
	if filter.ID != "" {
		id, err := s.PodIDIndex().Get(filter.ID)
		if err != nil {
			// Not finding an ID in a filtered list should not be considered
			// and error; it might have been deleted when stop was done.
			// Log and return an empty struct.
			log.Warnf(ctx, "Unable to find pod %s with filter", filter.ID)
			return []*sandbox.Sandbox{}
		}
		sb := s.getSandbox(id)
		if sb == nil {
			podList = []*sandbox.Sandbox{}
		} else {
			podList = []*sandbox.Sandbox{sb}
		}
	}
	finalList := make([]*sandbox.Sandbox, 0, len(podList))
	for _, pod := range podList {
		// Skip sandboxes that aren't created yet
		if !pod.Created() {
			continue
		}

		if filter.State != nil {
			if pod.State() != filter.State.State {
				continue
			}
		}
		if filter.LabelSelector != nil {
			sel := fields.SelectorFromSet(filter.LabelSelector)
			if !sel.Matches(pod.Labels()) {
				continue
			}
		}
		finalList = append(finalList, pod)
	}
	return finalList
}
