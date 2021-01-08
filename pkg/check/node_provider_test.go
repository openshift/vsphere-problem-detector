package check

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestCheckNodeProviderID(t *testing.T) {
	tests := []struct {
		name        string
		node        v1.Node
		expectError bool
	}{
		{
			name:        "node with provider",
			node:        node("vm1", withProviderID("3fd46873-7ff8-4a3f-a144-b7678def1010")),
			expectError: false,
		},
		{
			name:        "node without provider",
			node:        node("vm2"),
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			check := CheckNodeProviderID{}
			err := check.StartCheck()
			if err != nil {
				t.Errorf("StartCheck failed: %s", err)
			}

			kubeClient := &fakeKubeClient{
				nodes: []v1.Node{test.node},
			}
			ctx, cleanup, err := setupSimulator(kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			// Act
			err = check.CheckNode(ctx, &test.node, nil)

			// Assert
			if err != nil && !test.expectError {
				t.Errorf("Unexpected error: %s", err)
			}
			if err == nil && test.expectError {
				t.Errorf("Expected error, got none")
			}
		})
	}
}
