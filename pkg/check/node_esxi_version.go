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

	"github.com/openshift/vsphere-problem-detector/pkg/util"
)

// CollectNodeESXiVersion emits metric with version of each ESXi host that runs at least a single VM with node.
type CollectNodeESXiVersion struct {
	lastMetricEmission map[[2]string]int
}

var _ NodeCheck = &CollectNodeESXiVersion{}

var (
	esxiVersionMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_esxi_version_total",
			Help:           "Number of ESXi hosts with given version.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{versionLabel, apiVersionLabel},
	)
)

func init() {
	legacyregistry.MustRegister(esxiVersionMetric)
}

func (c *CollectNodeESXiVersion) Name() string {
	return "CollectNodeESXiVersion"
}

func (c *CollectNodeESXiVersion) StartCheck() error {
	return nil
}

func (c *CollectNodeESXiVersion) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	hostRef := vm.Runtime.Host
	if hostRef == nil {
		return fmt.Errorf("error getting ESXi host for node %s: vm.runtime.host is empty", node.Name)
	}
	hostName := hostRef.Value
	if ver, processed := ctx.ClusterInfo.MarkHostForProcessing(hostName); processed {
		klog.V(4).Infof("Node %s runs on cached ESXi host %s: %s", node.Name, hostName, ver)
		return nil
	}

	// Load the HostSystem properties
	host := object.NewHostSystem(ctx.VMClient, *hostRef)
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()
	var o mo.HostSystem

	err := host.Properties(tctx, host.Reference(), []string{"name", "config.product"}, &o)
	if err != nil {
		return fmt.Errorf("failed to load ESXi host %s for node %s: %s", hostName, node.Name, err)
	}

	if o.Config == nil {
		return fmt.Errorf("error getting ESXi host version for node %s: host.config is nil", node.Name)
	}
	version := o.Config.Product.Version
	apiVersion := o.Config.Product.ApiVersion
	realHostName := o.Name // "10.0.0.2" or other user-friendly name of the host.
	klog.V(2).Infof("Node %s runs on host %s (%s) with ESXi version: %s", node.Name, hostName, realHostName, version)
	ctx.ClusterInfo.SetHostVersion(hostName, version, apiVersion)

	return nil
}

func (c *CollectNodeESXiVersion) FinishCheck(ctx *CheckContext) {
	versions := make(map[util.ESXiVersionInfo]int)
	for k := range c.lastMetricEmission {
		c.lastMetricEmission[k] = 0
	}

	esxiVersions := ctx.ClusterInfo.GetHostVersions()
	for _, v := range esxiVersions {
		versions[v]++
	}

	// Report the count
	for v, count := range versions {
		esxiVersionMetric.WithLabelValues(v.Version, v.APIVersion).Set(float64(count))
		c.lastMetricEmission[[2]string{v.Version, v.APIVersion}] = count
	}

	for k, v := range c.lastMetricEmission {
		if v == 0 {
			esxiVersionMetric.WithLabelValues(k[0], k[1]).Set(0)
		}
	}

	return
}
