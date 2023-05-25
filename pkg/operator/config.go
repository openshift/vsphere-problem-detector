package operator

import (
	"fmt"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	listerv1 "k8s.io/client-go/listers/core/v1"
)

type DetectorConfig struct {
	Disabled bool `json:"disabled,omitempty"`
}

var (
	defaultConfig = DetectorConfig{
		// detector is enabled by default
		Disabled: false,
	}
)

const (
	detectorConfigMapName = "vsphere-problem-detector"
	configKey             = "config.yaml"
)

func ParseConfigMap(lister listerv1.ConfigMapLister) (*DetectorConfig, error) {
	cm, err := lister.ConfigMaps(operatorNamespace).Get(detectorConfigMapName)
	if err != nil {
		if errors.IsNotFound(err) {
			// Missing ConfigMap indicates default config
			return &defaultConfig, nil
		}
		return nil, err
	}

	data, found := cm.Data[configKey]
	if !found {
		return nil, fmt.Errorf("invalid format of ConfigMap %s: expected key %s", detectorConfigMapName, configKey)
	}

	config := defaultConfig
	err = yaml.Unmarshal([]byte(data), &config)
	if err != nil {
		return nil, fmt.Errorf("invalid format of ConfigMap %s: %s", detectorConfigMapName, err)
	}
	return &config, nil
}
