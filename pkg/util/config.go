package util

import (
	"fmt"

	"gopkg.in/gcfg.v1"
	vsphere "k8s.io/cloud-provider-vsphere/pkg/common/config"
	"k8s.io/klog/v2"
	legacy "k8s.io/legacy-cloud-providers/vsphere"
)

const (
	CloudConfigNamespace = "openshift-config"
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
