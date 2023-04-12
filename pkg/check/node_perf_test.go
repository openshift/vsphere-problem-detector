package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createNode(name string, labels map[string]string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func createMetricCheckDef(master, worker bool) MetricCheckDef {
	return MetricCheckDef{
		metricName:   "disk.fake.test",
		checkMasters: master,
		checkWorkers: worker,
	}
}

func TestCheckNodePerf(t *testing.T) {
	tests := []struct {
		name         string
		node         *v1.Node
		mcd          MetricCheckDef
		expectResult bool
	}{
		{
			name:         "master node checked",
			node:         createNode("master-node-0", map[string]string{"node-role.kubernetes.io/master": ""}),
			mcd:          createMetricCheckDef(true, false),
			expectResult: true,
		},
		{
			name:         "worker node not checked",
			node:         createNode("worker-node-0", map[string]string{"node-role.kubernetes.io/worker": ""}),
			mcd:          createMetricCheckDef(true, false),
			expectResult: false,
		},
		{
			name:         "worker node checked",
			node:         createNode("worker-node-0", map[string]string{"node-role.kubernetes.io/worker": ""}),
			mcd:          createMetricCheckDef(true, true),
			expectResult: true,
		},
		{
			name:         "infra node",
			node:         createNode("infra-node-0", map[string]string{"node-role.kubernetes.io/infra": ""}),
			mcd:          createMetricCheckDef(true, false),
			expectResult: false,
		},
		{
			name: "master/worker node check master flag",
			node: createNode("master-node-0", map[string]string{
				"node-role.kubernetes.io/master": "",
				"node-role.kubernetes.io/worker": "",
			}),
			mcd:          createMetricCheckDef(true, false),
			expectResult: true,
		},
		{
			name: "master/worker node check master and worker",
			node: createNode("master-node-0", map[string]string{
				"node-role.kubernetes.io/master": "",
				"node-role.kubernetes.io/worker": "",
			}),
			mcd:          createMetricCheckDef(true, true),
			expectResult: true,
		},
		{
			name: "infra/worker node not checked",
			node: createNode("infra-node-0", map[string]string{
				"node-role.kubernetes.io/infra":  "",
				"node-role.kubernetes.io/worker": "",
			}),
			mcd:          createMetricCheckDef(true, false),
			expectResult: false,
		},
		{
			name: "infra/worker node checked",
			node: createNode("infra-node-0", map[string]string{
				"node-role.kubernetes.io/infra":  "",
				"node-role.kubernetes.io/worker": "",
			}),
			mcd:          createMetricCheckDef(true, true),
			expectResult: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := checkNodePerf(test.node, test.mcd)

			assert.Equal(t, test.expectResult, result)
		})
	}
}
