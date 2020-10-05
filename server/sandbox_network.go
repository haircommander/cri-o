package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	cnicurrent "github.com/containernetworking/cni/pkg/types/current"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/metrics"
	"github.com/pkg/errors"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

type sandboxNetworkInfo struct {
	ips      []string
	result      cnitypes.Result
	err			error
}

func (ni *sandboxNetworkInfo) Error() error {
	if ni == nil {
		return nil
	}
	return ni.err
}

func (ni *sandboxNetworkInfo) IPs() []string {
	if ni == nil {
		return nil
	}
	return ni.ips
}

func (s *Server) setupNetworkForSandbox(ctx context.Context, sb *sandbox.Sandbox) (chan *sandboxNetworkInfo) {
	nic := new(chan *sandboxNetworkInfo, 1)
	go func() {
		nic <- s.networkStart(ctx, sb)
		close(nic)
	}()
	return nic
}

// networkStart sets up the sandbox's network and returns the pod IP on success
// or an error
func (s *Server) networkStart(ctx context.Context, sb *sandbox.Sandbox) (ni *sandboxNetworkInfo) {
	ni = &sandboxNetworkInfo{}
	overallStart := time.Now()
	// give a network Start call 2 minutes, half of a RunPodSandbox request timeout limit
	startCtx, startCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer startCancel()

	if sb.HostNetwork() {
		return
	}

	podNetwork, err := s.newPodNetwork(sb)
	if err != nil {
		ni.err = err
		return
	}

	// Ensure network resources are cleaned up if the plugin succeeded
	// but an error happened between plugin success and the end of networkStart()
	defer func() {
		if ni.err != nil {
			log.Infof(ctx, "networkStart: stopping network for sandbox %s", sb.ID())
			if err2 := s.networkStop(startCtx, sb); err2 != nil {
				log.Errorf(ctx, "error stopping network on cleanup: %v", err2)
			}
		}
	}()

	podSetUpStart := time.Now()
	_, err = s.config.CNIPlugin().SetUpPodWithContext(startCtx, podNetwork)
	if err != nil {
		ni.err = fmt.Errorf("failed to create pod network sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
		return
	}
	// metric about the CNI network setup operation
	metrics.CRIOOperationsLatency.WithLabelValues("network_setup_pod").
		Observe(metrics.SinceInMicroseconds(podSetUpStart))

	podNetworkStatus, err := s.config.CNIPlugin().GetPodNetworkStatusWithContext(startCtx, podNetwork)
	if err != nil {
		ni.err = fmt.Errorf("failed to get network status for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
		return
	}

	// only one cnitypes.Result is returned since newPodNetwork sets Networks list empty
	ni.result = podNetworkStatus[0].Result
	log.Debugf(ctx, "CNI setup result: %v", ni.result)

	network, err := cnicurrent.GetResult(ni.result)
	if err != nil {
		ni.err = fmt.Errorf("failed to get network JSON for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
		return
	}

	for idx, podIPConfig := range network.IPs {
		podIP := strings.Split(podIPConfig.Address.String(), "/")[0]

		// Apply the hostport mappings only for the first IP to avoid allocating
		// the same host port twice
		if idx == 0 && len(sb.PortMappings()) > 0 {
			ip := net.ParseIP(podIP)
			if ip == nil {
				ni.err = fmt.Errorf("failed to get valid ip address for sandbox %s(%s)", sb.Name(), sb.ID())
				return
			}

			err = s.hostportManager.Add(sb.ID(), &hostport.PodPortMapping{
				Name:         sb.Name(),
				PortMappings: sb.PortMappings(),
				IP:           ip,
				HostNetwork:  false,
			}, "lo")
			if err != nil {
				ni.err = fmt.Errorf("failed to add hostport mapping for sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
				return
			}
		}

		ni.IPs = append(ni.IPs, podIP)
	}

	log.Debugf(ctx, "found POD IPs: %v", ni.ips)

	// metric about the whole network setup operation
	metrics.CRIOOperationsLatency.WithLabelValues("network_setup_overall").
		Observe(metrics.SinceInMicroseconds(overallStart))

	return
}

// getSandboxIP retrieves the IP address for the sandbox
func (s *Server) getSandboxIPs(sb *sandbox.Sandbox) ([]string, error) {
	if sb.HostNetwork() {
		return nil, nil
	}

	podNetwork, err := s.newPodNetwork(sb)
	if err != nil {
		return nil, err
	}
	podNetworkStatus, err := s.config.CNIPlugin().GetPodNetworkStatus(podNetwork)
	if err != nil {
		return nil, fmt.Errorf("failed to get network status for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
	}

	res, err := cnicurrent.GetResult(podNetworkStatus[0].Result)
	if err != nil {
		return nil, fmt.Errorf("failed to get network JSON for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
	}

	podIPs := make([]string, 0, len(res.IPs))
	for _, podIPConfig := range res.IPs {
		podIPs = append(podIPs, strings.Split(podIPConfig.Address.String(), "/")[0])
	}

	return podIPs, nil
}

// networkStop cleans up and removes a pod's network.  It is best-effort and
// must call the network plugin even if the network namespace is already gone
func (s *Server) networkStop(ctx context.Context, sb *sandbox.Sandbox) error {
	if sb.HostNetwork() || sb.NetworkStopped() {
		return nil
	}
	// give a network stop call 1 minutes, half of a StopPod request timeout limit
	stopCtx, stopCancel := context.WithTimeout(ctx, 1*time.Minute)
	defer stopCancel()

	if err := s.hostportManager.Remove(sb.ID(), &hostport.PodPortMapping{
		Name:         sb.Name(),
		PortMappings: sb.PortMappings(),
		HostNetwork:  false,
	}); err != nil {
		log.Warnf(ctx, "failed to remove hostport for pod sandbox %s(%s): %v",
			sb.Name(), sb.ID(), err)
	}

	podNetwork, err := s.newPodNetwork(sb)
	if err != nil {
		return err
	}
	if err := s.config.CNIPlugin().TearDownPodWithContext(stopCtx, podNetwork); err != nil {
		return errors.Wrapf(err, "failed to destroy network for pod sandbox %s(%s)", sb.Name(), sb.ID())
	}

	return sb.SetNetworkStopped(true)
}
