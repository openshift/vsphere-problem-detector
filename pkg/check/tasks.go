package check

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog/v2"
)

// CheckFolderList tests that OCP has permissions to list volumes in Datastore.
// This is necessary to create volumes.
func CheckTaskPermissions(ctx *CheckContext) error {
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()

	mgr := view.NewManager(ctx.VMClient)
	view, err := mgr.CreateTaskView(tctx, ctx.VMClient.ServiceContent.TaskManager)
	if err != nil {
		return fmt.Errorf("error creating task view: %s", err)
	}

	taskCount := 0
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	err = view.Collect(tctx, func(tasks []types.TaskInfo) {
		taskCount += len(tasks)
	})

	if err != nil {
		return fmt.Errorf("error listing recent tasks: %s", err)
	}
	klog.V(2).Infof("CheckTaskPermissions: %d tasks found", taskCount)
	return nil
}
