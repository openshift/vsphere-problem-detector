package check

import (
	"context"
	"fmt"
	"sync"

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
	// Version "" means that a CheckNode call is retrieving it right now.
	esxiVersions     map[string]esxiVersionInfo
	esxiVersionsLock sync.Mutex
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

type esxiVersionInfo struct {
	version    string
	apiVersion string
}

func init() {
	legacyregistry.MustRegister(esxiVersionMetric)
}

func (c *CollectNodeESXiVersion) Name() string {
	return "CollectNodeESXiVersion"
}

func (c *CollectNodeESXiVersion) StartCheck() error {
	c.esxiVersions = make(map[string]esxiVersionInfo)
	return nil
}

func (c *CollectNodeESXiVersion) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	hostRef := vm.Runtime.Host
	if hostRef == nil {
		return fmt.Errorf("error getting ESXi host for node %s: vm.runtime.host is empty", node.Name)
	}
	hostName := hostRef.Value
	if ver, processed := c.checkOrMarkHostProcessing(hostName); processed {
		klog.V(4).Infof("Node %s runs on cached ESXi host %s: %s", node.Name, hostName, ver)
		return nil
	}

	// Load the HostSystem properties
	host := object.NewHostSystem(ctx.VMClient, *hostRef)
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
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
	c.setHostVersion(hostName, version, apiVersion)

	return nil
}

func (c *CollectNodeESXiVersion) FinishCheck(ctx *CheckContext) {
	// Count the versions
	versions := make(map[esxiVersionInfo]int)
	for _, esxiVersion := range c.esxiVersions {
		versions[esxiVersion]++
	}

	// Report the count
	for esxiVersion, count := range versions {
		esxiVersionMetric.WithLabelValues(esxiVersion.version, esxiVersion.apiVersion).Set(float64(count))
	}
	return
}

// checkOrMarkHostProcessing returns true, if the host version is already known
// or another go routine is retrieving it right now.
// When it's the first time the host is processed, mark it as being processed and
// return false. The caller is then responsible for retrieving the host and
// calling setHostVersion().
func (c *CollectNodeESXiVersion) checkOrMarkHostProcessing(hostName string) (string, bool) {
	c.esxiVersionsLock.Lock()
	defer c.esxiVersionsLock.Unlock()
	var esxiVersion string
	ver, found := c.esxiVersions[hostName]
	if ver.version == "" {
		esxiVersion = "<in progress>"
	}
	if found {
		return ver.version, true
	}
	// Mark the hostName as in progress
	c.esxiVersions[hostName] = esxiVersionInfo{"", ""}
	return esxiVersion, false
}

func (c *CollectNodeESXiVersion) setHostVersion(hostName string, version string, apiVersion string) {
	c.esxiVersionsLock.Lock()
	defer c.esxiVersionsLock.Unlock()
	c.esxiVersions[hostName] = esxiVersionInfo{version, apiVersion}
}
