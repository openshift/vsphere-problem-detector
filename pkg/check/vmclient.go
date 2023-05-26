package check

import (
	"github.com/vmware/govmomi/object"
)

type vmClient struct {
	checkContext *CheckContext

	// create a cached state of various objects we fetched
	dataCenterObjectMap map[string]*object.Datacenter

func (v *vmClient) getDatacenter(dcName string) (*object.Datacenter, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	finder := find.NewFinder(ctx.VMClient, false)
	dc, err := finder.Datacenter(tctx, dcName)
	if err != nil {
		return nil, fmt.Errorf("failed to access datacenter %s: %s", dcName, err)
	}
	return dc, nil
}
