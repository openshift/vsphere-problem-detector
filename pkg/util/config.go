package util

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/gcfg.v1"
	vsphere "k8s.io/cloud-provider-vsphere/pkg/common/config"
	"k8s.io/klog/v2"
	legacy "k8s.io/legacy-cloud-providers/vsphere"
)

// VSphereConfig contains configuration for cloud provider.  It wraps the legacy version and the newer upstream version
// with yaml support
type VSphereConfig struct {
	Config       *vsphere.Config
	LegacyConfig *legacy.VSphereConfig
}

// LoadConfig load the desired config into this config object.
func (c *VSphereConfig) LoadConfig(data string) error {
	var err error
	c.Config, err = vsphere.ReadConfig([]byte(data))
	if err != nil {
		return fmt.Errorf("unable to load config: %v", err)
	}

	// Load legacy if cfgString is not yaml.  May be needed in areas that require legacy logic.
	lCfg := &legacy.VSphereConfig{}
	err = gcfg.ReadStringInto(lCfg, data)
	if err != nil {
		// For now, we can just log an info so that we know.
		klog.V(2).Infof("Unable to load cloud config as legacy ini. %v", err)
		return nil
	}

	c.LegacyConfig = lCfg
	return nil
}

// GetVCenterHostname get the vcenter's hostname.
func (c *VSphereConfig) GetVCenterHostname(vcenter string) string {
	return c.Config.VirtualCenter[vcenter].VCenterIP
}

// IsInsecure returns true if the vcenter is configured to have an insecure connection.
func (c *VSphereConfig) IsInsecure(vcenter string) bool {
	return c.Config.VirtualCenter[vcenter].InsecureFlag
}

// GetDatacenters gets the datacenters.  Falls back to legacy style ini lookup if vcenter not found in primary config.
func (c *VSphereConfig) GetDatacenters(vcenter string) ([]string, error) {
	var datacenters []string
	if c.Config.VirtualCenter[vcenter] != nil {
		datacenters = strings.Split(c.Config.VirtualCenter[vcenter].Datacenters, ",")
	} else {
		// If here, then legacy config may be in use.
		datacenters = []string{c.LegacyConfig.Workspace.Datacenter}
	}
	logrus.Infof("Gathered the following data centers: %v", datacenters)
	return datacenters, nil
}

// GetWorkspaceDatacenter get the legacy datacenter from workspace config.
func (c *VSphereConfig) GetWorkspaceDatacenter() string {
	return c.LegacyConfig.Workspace.Datacenter
}

// GetDefaultDatastore get the default datastore.  This is primarily used with legacy ini config.
func (c *VSphereConfig) GetDefaultDatastore() string {
	return c.LegacyConfig.Workspace.DefaultDatastore
}
