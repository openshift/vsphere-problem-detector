package check

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

// CollectNodeESXiVersion emits metric with version of each ESXi host that runs at least a single VM with node.
type CollectNodeESXiVersion struct {
	// map ESXI host name ("host-12345", not hostname nor IP address!) -> version
	esxiVersions map[string]string
}

var _ NodeCheck = &CollectNodeESXiVersion{}

var (
	esxiVersionMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_esxi_version_total",
			Help:           "Number of ESXi hosts with given version.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{versionLabel},
	)
)

func init() {
	legacyregistry.MustRegister(esxiVersionMetric)
}

func (c *CollectNodeESXiVersion) Name() string {
	return "CollectNodeESXiVersion"
}

func (c *CollectNodeESXiVersion) StartCheck() error {
	c.esxiVersions = make(map[string]string)
	return nil
}

func (c *CollectNodeESXiVersion) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	hostRef := vm.Runtime.Host
	if hostRef == nil {
		return fmt.Errorf("error getting ESXi host for node %s: vm.runtime.host is empty", node.Name)
	}
	hostName := hostRef.Value
	if ver, found := c.esxiVersions[hostName]; found {
		klog.V(4).Infof("Node %s runs on cached ESXi host %s: %s", node.Name, hostName, ver)
		return nil
	}

	// Load the HostSystem properties
	host := object.NewHostSystem(ctx.VMClient, *hostRef)
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	var o mo.HostSystem

	err := host.Properties(tctx, host.Reference(), []string{"name", "config.product.version"}, &o)
	if err != nil {
		return fmt.Errorf("failed to load ESXi host %s for node %s: %s", hostName, node.Name, err)
	}

	if o.Config == nil {
		return fmt.Errorf("error getting ESXi host version for node %s: host.config is nil", node.Name)
	}
	version := o.Config.Product.Version
	realHostName := o.Name // "10.0.0.2" or other user-friendly name of the host.
	klog.V(2).Infof("Node %s runs on host %s (%s) with ESXi version: %s", node.Name, hostName, realHostName, version)
	c.esxiVersions[hostName] = version

	return nil
}

func (c *CollectNodeESXiVersion) FinishCheck() {
	// Count the versions
	versions := make(map[string]int)
	for _, version := range c.esxiVersions {
		versions[version]++
	}

	// Report the count
	for version, count := range versions {
		esxiVersionMetric.WithLabelValues(version).Set(float64(count))
	}
	return
}
