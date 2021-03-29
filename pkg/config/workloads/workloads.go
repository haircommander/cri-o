package workloads

import (
	"strconv"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	cgcfgs "github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	CPUShareResource = "cpu"
	CPUSetResource   = "cpuset"
)

type Workloads map[string]*WorkloadConfig

type WorkloadConfig struct {
	// Label is the pod label that activates these workload settings
	Label string `toml:"label"`
	// AnnotationPrefix is the way a pod can override a specific resource for a container.
	// The full annotation must be of the form $annotation_prefix.$resource/$ctrname = $value
	AnnotationPrefix string `toml:"annotation_prefix"`
	// Resources are the names of the resources that can be overridden by label.
	// The key of the map is the resource name. The following resources are supported:
	// `cpu`: configure cpu shares for a given container
	// `cpuset`: configure cpuset for a given container
	// The value of the map is the default value for that resource.
	// If a container is configured to use this workload, and does not specify
	// the annotation with the resource and value, the default value will apply.
	// Default values do not need to be specified.
	Resources map[string]string `toml:"resources"`
}

func (w Workloads) Validate() error {
	for workload, config := range w {
		if err := config.Validate(workload); err != nil {
			return err
		}
	}
	return nil
}

func (w *WorkloadConfig) Validate(workloadName string) error {
	if w.Label == "" {
		return errors.Errorf("label shouldn't be empty for workload %q", workloadName)
	}
	for resource, defaultValue := range w.Resources {
		m, ok := mutators[resource]
		if !ok {
			return errors.Errorf("process resource %s for workload %s: resource not supported", resource, workloadName)
		}
		if err := m.ValidateDefault(defaultValue); err != nil {
			return errors.Wrapf(err, "process resource %s for workload: default value %s invalid", resource, workloadName, defaultValue)
		}
	}
	return nil
}

func (w Workloads) MutateSpecGivenAnnotations(ctrName string, specgen *generate.Generator, sboxLabels, sboxAnnotations map[string]string) error {
	workload := w.fromLabels(sboxLabels)
	if workload == nil {
		return nil
	}
	for resource, defaultValue := range workload.Resources {
		value := valueFromAnnotation(resource, defaultValue, workload.AnnotationPrefix, ctrName, sboxAnnotations)
		if value == "" {
			logrus.Infof("skipping %s ", resource)
			continue
		}
		logrus.Infof("mutating %s %s", resource, value)

		m, ok := mutators[resource]
		if !ok {
			// CRI-O bug
			panic(errors.Errorf("resource %s is not defined", resource))
		}

		if err := m.MutateSpec(specgen, value); err != nil {
			return errors.Wrapf(err, "mutating spec given workload %s", workload.Label)
		}
	}
	return nil
}

func (w Workloads) MutateCgroupGivenAnnotations(mgr cgmgr.CgroupManager, sbParent string, sboxLabels, sboxAnnotations map[string]string) error {
	workload := w.fromLabels(sboxLabels)
	if workload == nil {
		return nil
	}
	for resource, defaultValue := range workload.Resources {
		value := valueFromAnnotation(resource, defaultValue, workload.AnnotationPrefix, "POD", sboxAnnotations)
		if value == "" {
			logrus.Infof("skipping %s ", resource)
			continue
		}
		logrus.Infof("mutating %s %s", resource, value)

		m, ok := mutators[resource]
		if !ok {
			// CRI-O bug
			panic(errors.Errorf("resource %s is not defined", resource))
		}

		if err := m.MutateCgroup(mgr, sbParent, value); err != nil {
			return errors.Wrapf(err, "mutating spec given workload %s", workload.Label)
		}
	}
	return nil
}

func (w Workloads) fromLabels(sboxLabels map[string]string) *WorkloadConfig {
	for _, wc := range w {
		for label, _ := range sboxLabels {
			logrus.Infof("checking for workload %s against %s", wc.Label, label)
			if wc.Label == label {
				logrus.Infof("found workload %s", wc.Label)
				return wc
			}
		}
	}
	return nil
}

func valueFromAnnotation(resource, defaultValue, prefix, ctrName string, annotations map[string]string) string {
	annotationKey := prefix + "." + resource + "/" + ctrName
	value, ok := annotations[annotationKey]
	if !ok {
		logrus.Infof("using default value %s for %s", defaultValue, resource)
		return defaultValue
	}
	return value
}

var mutators = map[string]Mutator{
	CPUShareResource: new(cpuShareMutator),
	CPUSetResource:   new(cpusetMutator),
}

type Mutator interface {
	ValidateDefault(string) error
	MutateSpec(*generate.Generator, string) error
	MutateCgroup(mgr cgmgr.CgroupManager, sbParent, configuredValue string) error
}

type cpusetMutator struct{}

func (m *cpusetMutator) ValidateDefault(set string) error {
	if set == "" {
		return nil
	}
	_, err := cpuset.Parse(set)
	return err
}

func (*cpusetMutator) MutateSpec(specgen *generate.Generator, configuredValue string) error {
	specgen.SetLinuxResourcesCPUCpus(configuredValue)
	logrus.Infof("called set %s", configuredValue)
	return nil
}

func (*cpusetMutator) MutateCgroup(mgr cgmgr.CgroupManager, sbParent, configuredValue string) error {
	cgroup := &cgcfgs.Cgroup{
		Path: sbParent,
		Resources: &cgcfgs.Resources{
			CpusetCpus: configuredValue,
		},
	}
	if err := mgr.Apply(sbParent, cgroup); err != nil {
		return err
	}
	logrus.Infof("called mutate %s", configuredValue)
	return nil
}

type cpuShareMutator struct{}

func (*cpuShareMutator) ValidateDefault(cpuShare string) error {
	if cpuShare == "" {
		return nil
	}
	if _, err := resource.ParseQuantity(cpuShare); err != nil {
		return err
	}
	return nil
}

func (*cpuShareMutator) MutateSpec(specgen *generate.Generator, configuredValue string) error {
	u, err := strconv.ParseUint(configuredValue, 0, 64)
	if err != nil {
		return err
	}
	specgen.SetLinuxResourcesCPUShares(u)
	logrus.Infof("called share %s", configuredValue)
	return nil
}

func (*cpuShareMutator) MutateCgroup(mgr cgmgr.CgroupManager, sbParent, configuredValue string) error {
	u, err := strconv.ParseUint(configuredValue, 0, 64)
	if err != nil {
		return err
	}

	cgroup := &cgcfgs.Cgroup{
		Path: sbParent,
		Resources: &cgcfgs.Resources{
			CpuShares: u,
		},
	}
	if err := mgr.Apply(sbParent, cgroup); err != nil {
		return err
	}
	logrus.Infof("called mutate %s", configuredValue)
	return nil
}
