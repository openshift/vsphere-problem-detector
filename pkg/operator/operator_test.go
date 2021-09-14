package operator

import (
	"context"
	"fmt"
	"testing"
	"time"

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

type testvSphereChecker struct {
	err error
}

func (d *testvSphereChecker) runChecks(ctx context.Context, info *util.ClusterInfo) (*ResultCollector, error) {
	resultCollector := NewResultsCollector()
	return resultCollector, d.err
}

var _ vSphereCheckerInterface = &testvSphereChecker{}

func TestSyncChecks(t *testing.T) {
	tests := []struct {
		name                string
		checkError          error
		clusterInfo         *util.ClusterInfo
		isDeprecated        bool
		hasError            bool
		performChecks       bool
		expectedCheckPerfom bool
		expectedDuration    time.Duration
	}{
		{
			name: "when no error running checks and latest version",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "7.0.1",
				"host_api_version":    "7.0.1.2",
				"vcenter_api_version": "7.0.1.2",
				"vcenter_version":     "7.0.1",
				"hw_version":          "vmx-15",
			}),
			performChecks:       true,
			expectedCheckPerfom: true,
			expectedDuration:    defaultBackoff.Cap,
		},
		{
			name:       "when there was an error running checks",
			checkError: fmt.Errorf("error running checks"),
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "7.0.1",
				"host_api_version":    "7.0.1.2",
				"vcenter_api_version": "7.0.1.2",
				"vcenter_version":     "7.0.1",
				"hw_version":          "vmx-15",
			}),
			performChecks:       true,
			expectedCheckPerfom: true,
			hasError:            true,
			expectedDuration:    defaultBackoff.Step(),
		},
		{
			name:                "when no checks were ran",
			clusterInfo:         util.MakeClusterInfo(map[string]string{}),
			performChecks:       false,
			expectedCheckPerfom: false,
			expectedDuration:    time.Duration(0),
			isDeprecated:        false,
		},
		{
			name: "when cluster was deprecated",
			clusterInfo: util.MakeClusterInfo(map[string]string{
				"host_name":           "foo.bar",
				"host_version":        "6.7.0",
				"host_api_version":    "6.7.3",
				"vcenter_api_version": "6.7.3",
				"vcenter_version":     "6.7.0",
				"hw_version":          "vmx-13",
			}),
			performChecks:       true,
			expectedCheckPerfom: true,
			isDeprecated:        true,
			expectedDuration:    defaultBackoff.Step(),
		},
	}

	for _, tc := range tests {
		info := tc.clusterInfo
		t.Run(tc.name, func(t *testing.T) {
			vsphereProblemOperator := &vSphereProblemDetectorController{
				checkerFunc: func(c *vSphereProblemDetectorController) vSphereCheckerInterface {
					return &testvSphereChecker{err: tc.checkError}
				},
				backoff:                defaultBackoff,
				lastClusterCheckResult: &clusterCheckResult{},
			}

			// if we should not perform checks, add randomly 10s to next check duration
			if !tc.performChecks {
				vsphereProblemOperator.nextCheck = time.Now().Add(10 * time.Second)
			}

			delay, checksPerformed := vsphereProblemOperator.runSyncChecks(context.TODO(), info)
			if tc.expectedCheckPerfom != checksPerformed {
				t.Fatalf("for checks performed expected %v got %v", tc.expectedCheckPerfom, checksPerformed)
			}
			if !compareTimeDiffWithinTimeFactor(tc.expectedDuration, delay) {
				t.Fatalf("expected next check duration to be %v got %v", tc.expectedDuration, delay)
			}
			if tc.isDeprecated != vsphereProblemOperator.lastClusterCheckResult.blockUpgrade {
				t.Fatalf("expected deprecated to be %v got %v", tc.isDeprecated, vsphereProblemOperator.lastClusterCheckResult.blockUpgrade)
			}
			checkError := vsphereProblemOperator.lastClusterCheckResult.checkError
			if checkError != nil && !tc.hasError {
				t.Fatalf("expected no error got %v", checkError)
			}
			if tc.hasError && (checkError == nil) {
				t.Fatalf("expected error but got nothing")
			}
		})
	}
}

// compareTimeDiff checks if two time durations are within Factor duration
func compareTimeDiffWithinTimeFactor(t1, t2 time.Duration) bool {
	if t1 <= t2 {
		maxTime := time.Duration(float64(t1) + float64(defaultBackoff.Duration))
		return (t2 < maxTime)
	} else {
		maxTime := time.Duration(float64(t2) + float64(defaultBackoff.Duration))
		return (t1 < maxTime)
	}
}
