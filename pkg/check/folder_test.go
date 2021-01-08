package check

import (
	"testing"
	"time"
)

func TestCheckFolderPermissions(t *testing.T) {
	// Very simple test, no error cases

	// Stage
	kubeClient := &fakeKubeClient{}
	ctx, cleanup, err := setupSimulator(kubeClient, defaultModel)
	if err != nil {
		t.Fatalf("setupSimulator failed: %s", err)
	}
	defer cleanup()

	// Act
	*Timeout = time.Second
	err = CheckFolderPermissions(ctx)

	// Assert
	if err != nil {
		t.Errorf("Unexpected error: %s", err)
	}
}
