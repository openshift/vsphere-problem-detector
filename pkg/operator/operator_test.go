package operator

import (
	"testing"

	"github.com/openshift/vsphere-problem-detector/pkg/util"
)

func TestCheckForDeprecation(t *testing.T) {
	tests := []struct {
		name         string
		clusterInfo  *util.ClusterInfo
		isDeprecated bool
	}{
		{
			name: "on Vsphere 6.5 platform with older HW version",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "6.5.0",
				"host_api_version":    "6.5",
				"vcenter_api_version": "6.5",
				"vcenter_version":     "6.5.0",
				"hw_version":          "vmx-13",
			}),
			isDeprecated: true,
		},
		{
			name: "on Vsphere 6.5 host with everything else latest",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "6.5.0",
				"host_api_version":    "6.5",
				"vcenter_api_version": "7.0.1",
				"vcenter_version":     "7.0.1",
				"hw_version":          "vmx-15",
			}),
			isDeprecated: true,
		},
		{
			name: "on Vsphere 7 platform with HW 15",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "7.0.1",
				"host_api_version":    "7.0.1.2",
				"vcenter_api_version": "7.0.1.2",
				"vcenter_version":     "7.0.1",
				"hw_version":          "vmx-15",
			}),
			isDeprecated: false,
		},
		{
			name: "on Vsphere 7 platform with HW 17",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "7.0.1",
				"host_api_version":    "7.0.1.2",
				"vcenter_api_version": "7.0.1.2",
				"vcenter_version":     "7.0.1",
				"hw_version":          "vmx-17",
			}),
			isDeprecated: false,
		},
		{
			name: "on Vsphere 7 platform with HW 13",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "7.0.1",
				"host_api_version":    "7.0.1.2",
				"vcenter_api_version": "7.0.1.2",
				"vcenter_version":     "7.0.1",
				"hw_version":          "vmx-13",
			}),
			isDeprecated: true,
		},
		{
			name: "on Vsphere 7 platform with older vcenter",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "7.0.1",
				"host_api_version":    "7.0.1.2",
				"vcenter_api_version": "6.7.0",
				"vcenter_version":     "7.0.1",
				"hw_version":          "vmx-15",
			}),
			isDeprecated: true,
		},
		{
			name: "on Vsphere 6.7u3 platform with latest everything",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "6.7.0",
				"host_api_version":    "6.7.3",
				"vcenter_api_version": "6.7.3",
				"vcenter_version":     "6.7.0",
				"hw_version":          "vmx-15",
			}),
			isDeprecated: false,
		},
		{
			name: "on Vsphere 6.7u3 platform with older hardware version",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "6.7.0",
				"host_api_version":    "6.7.3",
				"vcenter_api_version": "6.7.3",
				"vcenter_version":     "6.7.0",
				"hw_version":          "vmx-13",
			}),
			isDeprecated: true,
		},
	}

	for _, tc := range tests {
		info := tc.clusterInfo
		t.Run(tc.name, func(t *testing.T) {
			vsphereProblemOperator := &vSphereProblemDetectorController{}
			result, _ := vsphereProblemOperator.checkForDeprecation(info)
			if result != tc.isDeprecated {
				t.Errorf("expected %v got %v", tc.isDeprecated, result)
			}
		})
	}
}

func TestSyncChecks(t *testing.T) {

}
