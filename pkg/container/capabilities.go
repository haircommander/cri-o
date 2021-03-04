package container

import (
	"fmt"
	"strings"

	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/opencontainers/runtime-tools/validate"
	"github.com/syndtr/gocapability/capability"
)

// SetupCapabilities sets process.capabilities in the OCI runtime config.
func (c *container) SetupCapabilities(capabilities *types.Capability) error {
	// Clear default capabilities from spec
	// Note, we never add ambient capabilities, and only touch them to remove them all here.
	// Kubernetes is not yet ambient capabilities aware and pods expect that switching to a
	// non-root user results in the capabilities being dropped. This should be revisited in the future.
	c.spec.ClearProcessCapabilities()

	if capabilities == nil {
		return nil
	}

	toCAPPrefixed := func(cap string) string {
		if !strings.HasPrefix(strings.ToLower(cap), "cap_") {
			return "CAP_" + strings.ToUpper(cap)
		}
		return cap
	}

	allCapabilities := getOCICapabilitiesList()

	// Add/drop all capabilities if "all" is specified, so that
	// following individual add/drop could still work. E.g.
	// AddCapabilities: []string{"ALL"}, DropCapabilities: []string{"CHOWN"}
	// will be all capabilities without `CAP_CHOWN`.
	// see https://github.com/kubernetes/kubernetes/issues/51980
	if inStringSlice(capabilities.AddCapabilities, "ALL") {
		for _, cap := range allCapabilities {
			if err := c.spec.AddProcessCapabilityBounding(cap); err != nil {
				return err
			}
			if err := c.spec.AddProcessCapabilityEffective(cap); err != nil {
				return err
			}
			if err := c.spec.AddProcessCapabilityInheritable(cap); err != nil {
				return err
			}
			if err := c.spec.AddProcessCapabilityPermitted(cap); err != nil {
				return err
			}
		}
	}
	if inStringSlice(capabilities.DropCapabilities, "ALL") {
		for _, cap := range allCapabilities {
			if err := c.spec.DropProcessCapabilityBounding(cap); err != nil {
				return err
			}
			if err := c.spec.DropProcessCapabilityEffective(cap); err != nil {
				return err
			}
			if err := c.spec.DropProcessCapabilityInheritable(cap); err != nil {
				return err
			}
			if err := c.spec.DropProcessCapabilityPermitted(cap); err != nil {
				return err
			}
		}
	}

	for _, cap := range capabilities.AddCapabilities {
		if strings.EqualFold(cap, "ALL") {
			continue
		}
		capPrefixed := toCAPPrefixed(cap)
		// Validate capability
		if !inStringSlice(allCapabilities, capPrefixed) {
			return fmt.Errorf("unknown capability %q to add", capPrefixed)
		}
		if err := c.spec.AddProcessCapabilityBounding(capPrefixed); err != nil {
			return err
		}
		if err := c.spec.AddProcessCapabilityEffective(capPrefixed); err != nil {
			return err
		}
		if err := c.spec.AddProcessCapabilityInheritable(capPrefixed); err != nil {
			return err
		}
		if err := c.spec.AddProcessCapabilityPermitted(capPrefixed); err != nil {
			return err
		}
	}

	for _, cap := range capabilities.DropCapabilities {
		if strings.EqualFold(cap, "ALL") {
			continue
		}
		capPrefixed := toCAPPrefixed(cap)
		if err := c.spec.DropProcessCapabilityBounding(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %v", capPrefixed, err)
		}
		if err := c.spec.DropProcessCapabilityEffective(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %v", capPrefixed, err)
		}
		if err := c.spec.DropProcessCapabilityInheritable(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %v", capPrefixed, err)
		}
		if err := c.spec.DropProcessCapabilityPermitted(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %v", capPrefixed, err)
		}
	}

	return nil
}

// getOCICapabilitiesList returns a list of all available capabilities.
func getOCICapabilitiesList() []string {
	caps := make([]string, 0, len(capability.List()))
	for _, cap := range capability.List() {
		if cap > validate.LastCap() {
			continue
		}
		caps = append(caps, "CAP_"+strings.ToUpper(cap.String()))
	}
	return caps
}

// inStringSlice checks whether a string is inside a string slice.
// Comparison is case insensitive.
func inStringSlice(ss []string, str string) bool {
	for _, s := range ss {
		if strings.EqualFold(s, str) {
			return true
		}
	}
	return false
}
