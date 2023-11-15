package check

import (
	"github.com/openshift/vsphere-problem-detector/pkg/cache"
	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
)

func SetupSimulator(kubeClient *testlib.FakeKubeClient, modelDir string) (ctx *CheckContext, cleanup func(), err error) {
	setup, cleanup, err := testlib.SetupSimulator(kubeClient, modelDir)

	ctx = &CheckContext{
		Context:     setup.Context,
		VMConfig:    setup.VMConfig,
		TagManager:  setup.TagManager,
		ClusterInfo: setup.ClusterInfo,
		VMClient:    setup.VMClient,
		Cache:       cache.NewCheckCache(setup.VMClient),
		KubeClient:  kubeClient,
	}

	ctx.Username = setup.Username
	if kubeClient != nil && kubeClient.Infrastructure != nil {
		ConvertToPlatformSpec(kubeClient.Infrastructure, ctx)
	}
	return ctx, cleanup, err
}
