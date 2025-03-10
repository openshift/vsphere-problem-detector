package operator

import (
	"context"
	"fmt"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	opv1 "github.com/openshift/api/operator/v1"
	configinformers "github.com/openshift/client-go/config/listers/config/v1"
	fakeop "github.com/openshift/client-go/operator/clientset/versioned/fake"
	operatorinformers "github.com/openshift/client-go/operator/informers/externalversions"
	opinformers "github.com/openshift/client-go/operator/informers/externalversions"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clocktesting "k8s.io/utils/clock/testing"
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
	vCenterVersion string
	apiVersion     string
	err            error // error from runChecks()
	checkErr       error // error in a mock check result
}

func (d *testvSphereChecker) runChecks(ctx context.Context, info *util.ClusterInfo) (*ResultCollector, error) {
	if d.vCenterVersion != "" && info != nil {
		info.SetVCenterVersion("dc0", d.vCenterVersion, d.apiVersion)
	}
	resultCollector := NewResultsCollector()
	resultCollector.AddResult(checkResult{
		Name:  "FakeCheck",
		Error: d.checkErr,
	})
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
		{
			name: "when cluster is deprecated and check had errors",
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
			checkError:          fmt.Errorf("error running checks"),
			hasError:            true,
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
				backoff:       defaultBackoff,
				eventRecorder: events.NewInMemoryRecorder("vsphere-problem-detector", clocktesting.NewFakePassiveClock(time.Now())),
			}

			// if we sho	uld not perform checks, add randomly 10s to next check duration
			if !tc.performChecks {
				vsphereProblemOperator.nextCheck = time.Now().Add(10 * time.Second)
			}

			delay, lastCheckResult, checksPerformed := vsphereProblemOperator.runSyncChecks(context.TODO(), info)
			if tc.expectedCheckPerfom != checksPerformed {
				t.Fatalf("for checks performed expected %v got %v", tc.expectedCheckPerfom, checksPerformed)
			}
			t.Logf("delay is: %v and expectedDelay is: %v\n", delay, tc.expectedDuration)
			if !compareTimeDiffWithinTimeFactor(tc.expectedDuration, delay) {
				t.Fatalf("expected next check duration to be %v got %v", tc.expectedDuration, delay)
			}
			if tc.isDeprecated != lastCheckResult.blockUpgrade {
				t.Fatalf("expected deprecated to be %v got %v", tc.isDeprecated, lastCheckResult.blockUpgrade)
			}
			checkError := lastCheckResult.checkError
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
	allowedTimeFactor := defaultBackoff.Duration - 30*time.Second
	if t1 <= t2 {
		maxTime := time.Duration(float64(t1) + float64(allowedTimeFactor))
		return (t2 < maxTime)
	} else {
		maxTime := time.Duration(float64(t2) + float64(allowedTimeFactor))
		return (t1 < maxTime)
	}
}

func TestSync(t *testing.T) {
	tests := []struct {
		name           string
		mockCheckError error // error returned by a fake check
		runCheckError  error // error returned by runChecks()

		expectedAvailableConditionStatus  opv1.ConditionStatus
		expectedAvailableConditionMessage string

		clusterInfo   *util.ClusterInfo
		isDeprecated  bool
		hasError      bool
		performChecks bool
	}{
		{
			name:                              "sync with no errors",
			mockCheckError:                    nil,
			expectedAvailableConditionStatus:  opv1.ConditionTrue,
			expectedAvailableConditionMessage: "",
		},
		{
			name:                             "error in a single check",
			mockCheckError:                   fmt.Errorf("Mock check failure"),
			expectedAvailableConditionStatus: opv1.ConditionTrue,
			// The error gets propagated to the Available condition message
			expectedAvailableConditionMessage: "Mock check failure",
		},
		{
			name:                             "error in runCheck",
			runCheckError:                    fmt.Errorf("Mock check failure"),
			expectedAvailableConditionStatus: opv1.ConditionTrue,
			// The error gets propagated to the Available condition message
			expectedAvailableConditionMessage: "Mock check failure",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			storageInstance := &opv1.Storage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: opv1.StorageSpec{
					OperatorSpec: opv1.OperatorSpec{
						ManagementState: "Managed",
					},
				},
				Status: opv1.StorageStatus{
					OperatorStatus: opv1.OperatorStatus{},
				},
			}

			opClient := fakeop.NewSimpleClientset(storageInstance)
			opInformerFactory := opinformers.NewSharedInformerFactory(opClient, 0)
			opInformerFactory.Operator().V1().Storages().Informer().GetIndexer().Add(storageInstance)

			operatorClient := &OperatorClient{
				Informers: opInformerFactory,
				Client:    opClient.OperatorV1(),
			}

			c := opv1.ClusterCSIDriver{
				ObjectMeta: metav1.ObjectMeta{
					Name: "csi.vsphere.vmware.com",
				},
			}
			fakeOperatorClient := fakeop.NewSimpleClientset(&c)
			operatorInformer := operatorinformers.NewSharedInformerFactory(fakeOperatorClient, 0)
			operatorInformer.Operator().V1().ClusterCSIDrivers().Informer().GetIndexer().Add(&c)
			clusterCSIDriverInformer := operatorInformer.Operator().V1().ClusterCSIDrivers()

			vsphereProblemOperator := &vSphereProblemDetectorController{
				checkerFunc: func(c *vSphereProblemDetectorController) vSphereCheckerInterface {
					return &testvSphereChecker{
						vCenterVersion: "7.0.3",
						apiVersion:     "7.0.3",
						err:            tc.runCheckError,
						checkErr:       tc.mockCheckError,
					}
				},
				operatorClient:         operatorClient,
				clusterCSIDriverLister: clusterCSIDriverInformer.Lister(),
				infraLister:            &testInfraLister{},
				backoff:                defaultBackoff,
				eventRecorder:          events.NewInMemoryRecorder("vsphere-problem-detector", clocktesting.NewFakePassiveClock(time.Now())),
			}

			err := vsphereProblemOperator.sync(context.TODO(), factory.NewSyncContext(controllerName, events.NewInMemoryRecorder("test-csi-driver", clocktesting.NewFakePassiveClock(time.Now()))))
			if err != nil {
				// sync() should always succeed, regardless if the checks succeeded or not
				t.Errorf("sync returned unexpected error: %s", err)
			}

			storage, err := opClient.OperatorV1().Storages().Get(context.TODO(), "cluster", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Error getting operator status: %s", err)
			}
			if len(storage.Status.Conditions) != 1 {
				t.Fatalf("Expected 1 condition in status, got none: %+v", storage.Status)
			}
			cnd := storage.Status.Conditions[0]
			if cnd.Status != tc.expectedAvailableConditionStatus {
				t.Errorf("Expected Available condition %q, got %q", tc.expectedAvailableConditionStatus, cnd.Status)
			}
			if cnd.Message != tc.expectedAvailableConditionMessage {
				t.Errorf("Expected Available condition message %q, got %q", tc.expectedAvailableConditionMessage, cnd.Message)
			}
		})
	}
}

// Fake InfrastructureLister that provides Infrastructure with vSphere.
type testInfraLister struct{}

var _ configinformers.InfrastructureLister = &testInfraLister{}

var testInfra = configv1.Infrastructure{
	ObjectMeta: metav1.ObjectMeta{
		Name: "cluster",
	},
	Status: configv1.InfrastructureStatus{
		PlatformStatus: &configv1.PlatformStatus{
			Type: configv1.VSpherePlatformType,
		},
	},
}

func (t testInfraLister) List(selector labels.Selector) (ret []*configv1.Infrastructure, err error) {
	return []*configv1.Infrastructure{
		testInfra.DeepCopy(),
	}, nil
}

func (t testInfraLister) Get(name string) (*configv1.Infrastructure, error) {
	return testInfra.DeepCopy(), nil
}
