package check

import (
	"testing"
	"time"

	v1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
)

func TestCheckFolderPermissions(t *testing.T) {
	// Very simple test, no error cases
	folderTests := []struct {
		name           string
		infrastructure *v1.Infrastructure
		configFile     string
		expectErr      string
	}{
		{
			name:           "no FD with no issues",
			infrastructure: testlib.Infrastructure(),
		},
		{
			name:           "no FD with DS not found",
			configFile:     "config_invalid-ds.ini",
			infrastructure: testlib.Infrastructure(),
			expectErr:      "failed to access datastore RandoDS: cannot find datastore RandoDS in datacenter DC0",
		},
		{
			name:           "FD with no issues",
			infrastructure: testlib.InfrastructureWithFailureDomain(),
		},
		{
			name: "FD with DC not found",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithFailureDomain()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.Datacenter = "RandoDC"
				return inf
			}(),
			expectErr: "failed to access datacenter RandoDC: datacenter 'RandoDC' not found.  Known datacenters: \\[DC0 DC1\\]",
		},
		{
			name: "FD with DS not found",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithFailureDomain()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.Datastore = "RandoDS"
				return inf
			}(),
			expectErr: "failed to access datastore RandoDS: cannot find datastore RandoDS in datacenter DC0",
		},
		{
			name:           "multi vcenter with all folder founds",
			configFile:     "simple_config.yaml",
			infrastructure: testlib.InfrastructureWithMultiVCenters(),
		},
		{
			name:       "multi vcenter with DC not found for FD",
			configFile: "simple_config.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.Datacenter = "RandoDC"
				return inf
			}(),
			expectErr: "failed to access datacenter RandoDC: datacenter 'RandoDC' not found.  Known datacenters: \\[DC0 DC1\\]",
		},
		{
			name:       "multi vcenter with DS not found",
			configFile: "simple_config.yaml",
			infrastructure: func() *v1.Infrastructure {
				inf := testlib.InfrastructureWithMultiVCenters()
				inf.Spec.PlatformSpec.VSphere.FailureDomains[1].Topology.Datastore = "RandoDS"
				return inf
			}(),
			expectErr: "failed to access datastore RandoDS: cannot find datastore RandoDS in datacenter DC1",
		},
		{
			name:           "multi vcenter with vCenter not found",
			infrastructure: testlib.InfrastructureWithMultiVCenters(),
			expectErr:      "unable to check folder permissions for failure domain failure-domain-1: vCenter vcenter.test.openshift.com no found",
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
			err = CheckFolderPermissions(ctx)

			// Assert
			if test.expectErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Regexp(t, test.expectErr, err)
			}
		})
	}
}
