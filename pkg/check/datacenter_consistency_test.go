package check

import (
	"testing"
	"time"

	v1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
)

func TestCheckDatacenterConsistency(t *testing.T) {
	tests := []struct {
		name           string
		configFile     string
		infrastructure *v1.Infrastructure
		expectErr      string
	}{
		{
			name:           "legacy config (no failure domains)",
			configFile:     "simple_config.ini",
			infrastructure: testlib.Infrastructure(),
		},
		{
			name:           "all datacenters present in cloud.conf (ini)",
			configFile:     "simple_config.ini",
			infrastructure: testlib.InfrastructureWithFailureDomain(),
		},
		{
			name:       "all datacenters present in cloud.conf (yaml)",
			configFile: "simple_config.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.Datacenter = "cidatacenter"
				inf.Spec.PlatformSpec.VSphere.FailureDomains[1].Topology.Datacenter = "IBMCloud"
				inf.Spec.PlatformSpec.VSphere.VCenters[0].Datacenters = []string{"cidatacenter"}
				inf.Spec.PlatformSpec.VSphere.VCenters[1].Datacenters = []string{"IBMCloud"}
				return inf
			}(),
		},
		{
			name:       "one datacenter missing from cloud.conf (ini)",
			configFile: "simple_config.ini",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithFailureDomain()
				inf.Spec.PlatformSpec.VSphere.FailureDomains = append(
					inf.Spec.PlatformSpec.VSphere.FailureDomains,
					v1.VSpherePlatformFailureDomainSpec{
						Name:   "us-west-1",
						Server: "dc0",
						Topology: v1.VSpherePlatformTopology{
							Datacenter: "DC-MISSING",
						},
					},
				)
				return inf
			}(),
			expectErr: `Datacenter-Consistency: failure domain "us-west-1".*requires datacenter "DC-MISSING" on vCenter "dc0".*cloud-provider-config ConfigMap in the openshift-config namespace`,
		},
		{
			name:       "one datacenter missing from cloud.conf (yaml)",
			configFile: "simple_config.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[1].Topology.Datacenter = "DC-MISSING"
				return inf
			}(),
			expectErr: `Datacenter-Consistency: failure domain "failure-domain-2".*requires datacenter "DC-MISSING" on vCenter "vcenter2.test.openshift.com"`,
		},
		{
			name:       "multiple datacenters missing from cloud.conf",
			configFile: "simple_config.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.Datacenter = "MISSING-1"
				inf.Spec.PlatformSpec.VSphere.FailureDomains[1].Topology.Datacenter = "MISSING-2"
				return inf
			}(),
			expectErr: `(?s)MISSING-1.*MISSING-2`,
		},
		{
			name:       "vCenter not in cloud.conf is skipped (handled by CheckInfraConfig)",
			configFile: "simple_config.ini",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithFailureDomain()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Server = "unknown-vcenter"
				return inf
			}(),
		},
		{
			name:       "datacenter missing from vCenters section in infra CR",
			configFile: "simple_config.ini",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithFailureDomain()
				inf.Spec.PlatformSpec.VSphere.VCenters[0].Datacenters = []string{"OTHER-DC"}
				return inf
			}(),
			expectErr: `Datacenter-Consistency: failure domain "dc0".*datacenter "DC0".*not listed in the vcenters section`,
		},
		{
			name:       "datacenter missing from vCenters section in multi-vCenter infra CR",
			configFile: "simple_config.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.Datacenter = "cidatacenter"
				inf.Spec.PlatformSpec.VSphere.FailureDomains[1].Topology.Datacenter = "IBMCloud"
				inf.Spec.PlatformSpec.VSphere.VCenters[0].Datacenters = []string{"cidatacenter"}
				inf.Spec.PlatformSpec.VSphere.VCenters[1].Datacenters = []string{"OTHER-DC"}
				return inf
			}(),
			expectErr: `Datacenter-Consistency: failure domain "failure-domain-2".*datacenter "IBMCloud".*not listed in the vcenters section`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := &testlib.FakeKubeClient{
				Infrastructure: test.infrastructure,
			}
			ctx, cleanup, err := SetupSimulatorWithConfig(kubeClient, testlib.DefaultModel, test.configFile)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			oldTimeout := *util.Timeout
			*util.Timeout = time.Second
			t.Cleanup(func() {
				*util.Timeout = oldTimeout
			})
			err = CheckDatacenterConsistency(ctx)

			if test.expectErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Regexp(t, test.expectErr, err)
			}
		})
	}
}

func TestParseDatacenters(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"DC0", []string{"DC0"}},
		{"DC0,DC1", []string{"DC0", "DC1"}},
		{" DC0 , DC1 ", []string{"DC0", "DC1"}},
		{"", nil},
	}
	for _, test := range tests {
		result := parseDatacenters(test.input)
		assert.Equal(t, test.expected, result)
	}
}
