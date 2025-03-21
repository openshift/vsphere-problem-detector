package check

import (
	"testing"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
)

const (
	HostGroupCloudConfig = "host_group_cloud_config.yaml"
	HostGroupUnitTests   = "Host Group Unit Tests: "
)

func TestHostGroups(t *testing.T) {
	tests := []struct {
		name           string
		cloudConfig    string
		infrastructure *ocpv1.Infrastructure
		actions        []SimulatorAction
		expectErr      string
	}{
		{
			name:           HostGroupUnitTests + "validate basic zonal host groups",
			cloudConfig:    HostGroupCloudConfig,
			infrastructure: testlib.InfrastructureWithMultipleFailureDomainHostGroup(),
			actions:        []SimulatorAction{},
		},
		{
			name:           HostGroupUnitTests + "no host group defined",
			cloudConfig:    HostGroupCloudConfig,
			infrastructure: testlib.InfrastructureWithMultipleFailureDomainHostGroup(),
			actions:        []SimulatorAction{SimulatorNoHostGroup},
			expectErr:      "dc0 refers to host group zone-1 which is not found",
		},
		{
			name:           HostGroupUnitTests + "no vm group defined",
			cloudConfig:    HostGroupCloudConfig,
			infrastructure: testlib.InfrastructureWithMultipleFailureDomainHostGroup(),
			actions:        []SimulatorAction{SimulatorNoVmGroup},
			expectErr:      "dc0 refers to vm group vm-zone-1 which is not found",
		},
		{
			name:           HostGroupUnitTests + "no vm rule defined",
			cloudConfig:    HostGroupCloudConfig,
			infrastructure: testlib.InfrastructureWithMultipleFailureDomainHostGroup(),
			actions:        []SimulatorAction{SimulatorNoVmRule},
			expectErr:      "dc0 refers to vm host rule vm-rule-zone-1 which is not found",
		},
		{
			name:           HostGroupUnitTests + "no hosts in host group",
			cloudConfig:    HostGroupCloudConfig,
			infrastructure: testlib.InfrastructureWithMultipleFailureDomainHostGroup(),
			actions:        []SimulatorAction{SimulatorNoHostsInHostGroup},
			expectErr:      "dc0 does not have any hosts in host group zone-1",
		},
		{
			name:           HostGroupUnitTests + "hosts missing tag. zone tag is applied to cluster",
			cloudConfig:    HostGroupCloudConfig,
			infrastructure: testlib.InfrastructureWithMultipleFailureDomainHostGroup(),
			actions:        []SimulatorAction{SimulatorSkipAttachHostTag},
			expectErr:      `host host-1000 in host group zone-1 does not have an openshift-zone tag with value west-1a;host host-1001 in host group zone-1 does not have an openshift-zone tag with value west-1a`,
		},
	}

	for _, test := range tests {
		var err error
		kubeClient := &testlib.FakeKubeClient{
			Nodes:          testlib.DefaultNodes(),
			Infrastructure: test.infrastructure,
		}

		var checkContext *CheckContext
		var cleanup func()

		checkContext, cleanup, err = SetupSimulatorWithConfig(kubeClient, testlib.HostGroupModel, test.cloudConfig)
		if err != nil {
			t.Fatalf("setupSimulator failed: %s", err)
		}

		err = setupInfrastructureTopology(
			*checkContext,
			test.actions...,
		)

		if err != nil {
			t.Fatalf("unable to provision infrastructure topology. %v", err)
		}

		err = setupTagAttachmentTest(
			*checkContext,
			test.actions...,
		)
		if err != nil {
			t.Fatalf("unable configure tags. %v", err)
		}

		err = CheckHostGroups(checkContext)
		if len(test.expectErr) == 0 {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		} else {
			if err.Error() != test.expectErr {
				t.Fatalf("unexpected error: %v. expected: %s", err, test.expectErr)
			}
		}

		defer cleanup()
	}
}
