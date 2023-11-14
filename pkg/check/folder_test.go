package check

import (
	"testing"

	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
)

func TestCheckFolderPermissions(t *testing.T) {
	// Very simple test, no error cases

	// Stage
	kubeClient := &testlib.FakeKubeClient{
		Infrastructure: testlib.Infrastructure(),
	}
	ctx, cleanup, err := SetupSimulator(kubeClient, testlib.DefaultModel)
	if err != nil {
		t.Fatalf("setupSimulator failed: %s", err)
	}
	defer cleanup()

	// Act
	//	*util.Timeout = time.Second
	err = CheckFolderPermissions(ctx)

	// Assert
	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	}
}
