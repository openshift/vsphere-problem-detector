package check

import (
	"errors"
	"fmt"
	"github.com/vmware/govmomi/find"
	"testing"

	"github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	vapitags "github.com/vmware/govmomi/vapi/tags"
)

const (
	tagTestCreateRegionCategory     = 0x01
	tagTestCreateZoneCategory       = 0x02
	tagTestAttachRegionTags         = 0x04
	tagTestAttachZoneTags           = 0x08
	tagTestNothingCreatedOrAttached = 0x10
)

func validInfrastructure() *v1.Infrastructure {
	return &v1.Infrastructure{
		Spec: v1.InfrastructureSpec{
			CloudConfig: v1.ConfigMapFileReference{
				Key:  "config",
				Name: "cloud-provider-config",
			},
			PlatformSpec: v1.PlatformSpec{
				Type: "VSphere",
				VSphere: &v1.VSpherePlatformSpec{
					FailureDomains: []v1.VSpherePlatformFailureDomainSpec{
						{
							Name:   "test-east-1a",
							Region: "test-region-east",
							Server: "test-vcenter",
							Topology: v1.VSpherePlatformTopology{
								ComputeCluster: "/DC0/host/DC0_C0",
								Datacenter:     "DC0",
								Datastore:      "LocalDS_0",
								Folder:         "/DC0/vm",
								Networks: []string{
									"DC0_DVPG0",
								},
								ResourcePool: "/DC0/host/DC0_C0/Resources/test-resourcepool",
							},
							Zone: "test-zone-1a",
						},
						{
							Name:   "test-east-1b",
							Region: "test-region-east",
							Server: "test-vcenter",
							Topology: v1.VSpherePlatformTopology{
								ComputeCluster: "/DC0/host/DC1_C0",
								Datacenter:     "DC1",
								Datastore:      "LocalDS_0",
								Folder:         "/DC1/vm",
								Networks: []string{
									"DC0_DVPG0",
								},
								ResourcePool: "/DC1/host/DC1_C0/Resources/test-resourcepool",
							},
							Zone: "test-zone-1b",
						},
					},
				},
			},
		},
	}
}

func teardownTagAttachmentTest(checkContext CheckContext) error {
	tagMgr := checkContext.TagManager
	ctx := checkContext.Context
	tags, err := tagMgr.ListTags(ctx)
	if err != nil {
		return err
	}
	attachedMos, err := tagMgr.GetAttachedObjectsOnTags(ctx, tags)
	if err != nil {
		return err
	}

	for _, attachedMo := range attachedMos {
		for _, mo := range attachedMo.ObjectIDs {
			err := tagMgr.DetachTag(ctx, attachedMo.TagID, mo)
			if err != nil {
				return err
			}
		}
		err := tagMgr.DeleteTag(ctx, attachedMo.Tag)
		if err != nil {
			return err
		}
	}

	categories, err := tagMgr.GetCategories(ctx)
	if err != nil {
		return err
	}
	for _, category := range categories {
		cat := category
		err := tagMgr.DeleteCategory(ctx, &cat)
		if err != nil {
			return err
		}
	}
	return nil
}

func setupTagAttachmentTest(checkContext CheckContext, finder *find.Finder, attachmentMask int64) error {
	tagMgr := checkContext.TagManager
	ctx := checkContext.Context
	if attachmentMask&tagTestCreateRegionCategory != 0 {
		categoryID, err := tagMgr.CreateCategory(ctx, &vapitags.Category{
			Name:        "openshift-region",
			Description: "region tag category",
		})
		if err != nil {
			return err
		}

		if attachmentMask&tagTestAttachRegionTags != 0 {
			tagID, err := tagMgr.CreateTag(ctx, &vapitags.Tag{
				Name:       "us-east",
				CategoryID: categoryID,
			})
			if err != nil {
				return err
			}
			datacenters, err := finder.DatacenterList(ctx, "/...")
			if err != nil {
				return err
			}
			for _, datacenter := range datacenters {
				err = tagMgr.AttachTag(ctx, tagID, datacenter)
				if err != nil {
					return err
				}
			}
		}
	}
	if attachmentMask&tagTestCreateZoneCategory != 0 {
		categoryID, err := tagMgr.CreateCategory(ctx, &vapitags.Category{
			Name:        "openshift-zone",
			Description: "zone tag category",
		})
		if err != nil {
			return err
		}
		if attachmentMask&tagTestAttachZoneTags != 0 {
			tagID, err := tagMgr.CreateTag(ctx, &vapitags.Tag{
				Name:       "us-east-1a",
				CategoryID: categoryID,
			})
			if err != nil {
				return err
			}
			clusters, err := finder.ClusterComputeResourceList(ctx, "/...")
			if err != nil {
				return err
			}
			for _, cluster := range clusters {
				err = tagMgr.AttachTag(ctx, tagID, cluster)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name                string
		checkZoneTagsMethod func(*CheckContext) error
		infrastructure      *v1.Infrastructure
		tagTestMask         int64
		expectErr           string
	}{{
		name:                "multi-zone validation - No failure domain - No tags",
		checkZoneTagsMethod: CheckZoneTags,
		infrastructure: func() *v1.Infrastructure {
			inf := validInfrastructure()
			inf.Spec.PlatformSpec.VSphere.FailureDomains = nil
			return inf
		}(),
	}, {
		name:                "multi-zone validation - No tags",
		checkZoneTagsMethod: CheckZoneTags,
		infrastructure:      validInfrastructure(),
		expectErr:           "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
	}, {
		name:                "multi-zone tag categories present and tags attached",
		checkZoneTagsMethod: CheckZoneTags,
		infrastructure:      validInfrastructure(),
		tagTestMask: tagTestCreateZoneCategory |
			tagTestCreateRegionCategory |
			tagTestAttachRegionTags |
			tagTestAttachZoneTags,
	}, {
		name:                "multi-zone tag categories, missing zone tag attachment",
		checkZoneTagsMethod: CheckZoneTags,
		infrastructure:      validInfrastructure(),
		tagTestMask: tagTestCreateZoneCategory |
			tagTestCreateRegionCategory |
			tagTestAttachRegionTags,
		//expectErr: "platform.vsphere.failureDomains.topology.computeCluster: Internal error: tag associated with tag category openshift-zone not attached to this resource or ancestor",
		expectErr: "Multi-Zone support: ClusterComputeResource DC0_C0 for failure domain test-east-1a: tag associated with tag category openshift-zone not attached to this resource or ancestor",
	}, {
		name:                "multi-zone tag categories, missing region tag attachment",
		checkZoneTagsMethod: CheckZoneTags,
		infrastructure:      validInfrastructure(),
		tagTestMask: tagTestCreateZoneCategory |
			tagTestCreateRegionCategory |
			tagTestAttachZoneTags,
		//expectErr: "platform.vsphere.failureDomains.topology.computeCluster: Internal error: tag associated with tag category openshift-zone not attached to this resource or ancestor",
		expectErr: "Multi-Zone support: ClusterComputeResource DC0_C0 for failure domain test-east-1a: tag associated with tag category openshift-region not attached to this resource or ancestor",
	}, {
		name:                "multi-zone tag categories, missing zone and region tag categories",
		checkZoneTagsMethod: CheckZoneTags,
		infrastructure:      validInfrastructure(),
		tagTestMask:         tagTestNothingCreatedOrAttached,
		expectErr:           "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
	},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if test.checkZoneTagsMethod != nil {
				fmt.Println("Setting up test...")
				// Setup test
				kubeClient := &fakeKubeClient{
					nodes:          defaultNodes(),
					infrastructure: test.infrastructure,
				}

				var checkContext *CheckContext
				var cleanup func()
				checkContext, cleanup, err = setupSimulator(kubeClient, defaultModel)
				if err != nil {
					t.Fatalf("setupSimulator failed: %s", err)
				}
				defer cleanup()

				finder := find.NewFinder(checkContext.VMClient)

				// Perform test
				fmt.Println("Starting test...")
				if test.tagTestMask != 0 {
					err = setupTagAttachmentTest(*checkContext, finder, test.tagTestMask)
					if err != nil {
						assert.NoError(t, err)
					}
				}
				err = test.checkZoneTagsMethod(checkContext)
				if test.tagTestMask != 0 {
					err := teardownTagAttachmentTest(*checkContext)
					if err != nil {
						assert.NoError(t, err)
					}
				}
			} else {
				err = errors.New("no test method defined")
			}
			if test.expectErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Regexp(t, test.expectErr, err)
			}
		})
	}
}
