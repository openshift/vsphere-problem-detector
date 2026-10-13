package check

import (
	"testing"
	"time"

	v1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
)

func TestCheckInfraConfig(t *testing.T) {
	// Very simple test, no error cases
	folderTests := []struct {
		name           string
		configFile     string
		infrastructure *v1.Infrastructure
		expectErr      string
	}{
		{
			name:           "good config no FD (ini)",
			configFile:     "simple_config.ini",
			infrastructure: testlib.Infrastructure(),
		},
		{
			name:           "good config with FD (ini)",
			configFile:     "simple_config.ini",
			infrastructure: testlib.InfrastructureWithFailureDomain(),
		},
		{
			name:       "good config single vcenter (yaml)",
			configFile: "config_single-vcenter.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.VCenters = inf.Spec.PlatformSpec.VSphere.VCenters[0:1]
				inf.Spec.PlatformSpec.VSphere.FailureDomains = inf.Spec.PlatformSpec.VSphere.FailureDomains[0:1]
				return inf
			}(),
		},
		{
			name:           "good config multi vcenter (yaml)",
			configFile:     "simple_config.yaml",
			infrastructure: testlib.InfrastructureWithMultiVCenters(),
		},
		{
			name:       "vcenter mismatch infra vs provider (ini)",
			configFile: "simple_config.ini",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithFailureDomain()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Server = "RandoVC"
				inf.Spec.PlatformSpec.VSphere.VCenters[0].Server = "RandoVC"
				return inf
			}(),
			expectErr: "Infra-Config: Infrastructure instance and cloud provider ConfigMap openshift-config/cloud-provider-config do not match.  Infrastructure has \\[RandoVC] and the ConfigMap has \\[dc0]",
		},
		{
			name:       "vcenter mismatch infra vs provider (yaml)",
			configFile: "config_single-vcenter.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.VCenters = inf.Spec.PlatformSpec.VSphere.VCenters[0:1]
				inf.Spec.PlatformSpec.VSphere.FailureDomains = inf.Spec.PlatformSpec.VSphere.FailureDomains[0:1]
				inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Server = "RandoVC"
				inf.Spec.PlatformSpec.VSphere.VCenters[0].Server = "RandoVC"
				return inf
			}(),
			expectErr: "Infra-Config: Infrastructure instance and cloud provider ConfigMap openshift-config/cloud-provider-config do not match.  Infrastructure has \\[RandoVC] and the ConfigMap has \\[vcenter.test.openshift.com]",
		},
		{
			name:       "vcenter mismatch infra vs provider multi vcenter (yaml)",
			configFile: "config_single-vcenter.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[1].Server = "RandoVC"
				inf.Spec.PlatformSpec.VSphere.VCenters[1].Server = "RandoVC"
				return inf
			}(),
			expectErr: "Infra-Config: Infrastructure instance and cloud provider ConfigMap openshift-config/cloud-provider-config do not match.  Infrastructure has 2 vCenters \\[vcenter.test.openshift.com RandoVC] and the ConfigMap has 1 vCenters \\[vcenter.test.openshift.com]",
		},
		{
			name:       "failure domain reference missing vcenter (ini)",
			configFile: "simple_config.ini",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithFailureDomain()

				fd := v1.VSpherePlatformFailureDomainSpec{
					Name:   "RandoFD",
					Server: "RandoVC",
					Topology: v1.VSpherePlatformTopology{
						Datacenter: "DC0",
						Datastore:  "LocalDS_0",
					},
				}

				fds := inf.Spec.PlatformSpec.VSphere.FailureDomains
				inf.Spec.PlatformSpec.VSphere.FailureDomains = append(fds, fd)

				return inf
			}(),
			expectErr: "Infra-Config: Infrastructure failure domain RandoFD references the server RandoVC but is not found in the vCenter section \\[dc0]",
		},
		{
			name:       "failure domain reference missing vcenter (yaml)",
			configFile: "simple_config.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()

				fd := v1.VSpherePlatformFailureDomainSpec{
					Name:   "RandoFD",
					Server: "RandoVC",
					Topology: v1.VSpherePlatformTopology{
						Datacenter: "DC0",
						Datastore:  "LocalDS_0",
					},
				}

				fds := inf.Spec.PlatformSpec.VSphere.FailureDomains
				inf.Spec.PlatformSpec.VSphere.FailureDomains = append(fds, fd)

				return inf
			}(),
			expectErr: "Infra-Config: Infrastructure failure domain RandoFD references the server RandoVC but is not found in the vCenter section \\[vcenter.test.openshift.com vcenter2.test.openshift.com]",
		},
		{
			name:       "vCenter not used (ini)",
			configFile: "config_multi-vcenter.ini",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.FailureDomains = inf.Spec.PlatformSpec.VSphere.FailureDomains[0:1]
				return inf
			}(),
			expectErr: "Infra-Config: vCenter vcenter2.test.openshift.com is configured in the infrastructure instance but not used by any failure domains",
		},
		{
			name:       "vCenter not used (yaml)",
			configFile: "simple_config.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.FailureDomains = inf.Spec.PlatformSpec.VSphere.FailureDomains[0:1]
				return inf
			}(),
			expectErr: "Infra-Config: vCenter vcenter2.test.openshift.com is configured in the infrastructure instance but not used by any failure domains",
		},
	}

	for _, test := range folderTests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			kubeClient := &testlib.FakeKubeClient{
				Infrastructure: test.infrastructure,
			}
			ctx, cleanup, err := SetupSimulatorWithConfig(kubeClient, testlib.DefaultModel, test.configFile)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			// Act
			*util.Timeout = time.Second
			err = CheckInfraConfig(ctx)

			// Assert
			if test.expectErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Regexp(t, test.expectErr, err)
			}
		})
	}
}
