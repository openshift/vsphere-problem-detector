package check

import (
	"errors"
	"fmt"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/vmware/govmomi/find"
	vapitags "github.com/vmware/govmomi/vapi/tags"

	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
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

func validInfrastructureMultiVCenter() *v1.Infrastructure {
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
							Server: "vcenter.test.openshift.com",
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
							Server: "vcenter2.test.openshift.com",
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
					VCenters: []v1.VSpherePlatformVCenterSpec{
						{
							Server: "vcenter.test.openshift.com",
							Datacenters: []string{
								"DC0",
							},
						},
						{
							Server: "vcenter2.test.openshift.com",
							Datacenters: []string{
								"DC1",
							},
						},
					},
				},
			},
		},
	}
}

func teardownTagAttachmentTest(checkContext CheckContext, vCenter *VCenter) error {
	tagMgr := vCenter.TagManager
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

func setupTagAttachmentTest(checkContext CheckContext, vCenter *VCenter, finder *find.Finder, attachmentMask int64) error {
	tagMgr := vCenter.TagManager
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
		cloudConfig         string
		checkZoneTagsMethod func(*CheckContext) error
		infrastructure      *v1.Infrastructure
		tagTestMask         int64
		expectErr           string
	}{
		{
			name:                "multi-zone validation - No failure domain - No tags",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure: func() *v1.Infrastructure {
				inf := validInfrastructure()
				inf.Spec.PlatformSpec.VSphere.FailureDomains = nil
				return inf
			}(),
		},
		{
			name:                "multi-zone validation - VSphere nil - No tags",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure: func() *v1.Infrastructure {
				inf := validInfrastructure()
				inf.Spec.PlatformSpec.VSphere = nil
				return inf
			}(),
		},
		{
			name:                "multi-zone validation - No tags",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructure(),
			expectErr:           "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
		},
		{
			name:                "multi-zone tag categories present and tags attached",
			cloudConfig:         "config_test-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructure(),
			tagTestMask: tagTestCreateZoneCategory |
				tagTestCreateRegionCategory |
				tagTestAttachRegionTags |
				tagTestAttachZoneTags,
		},
		{
			name:                "multi-zone tag categories, missing zone tag attachment",
			cloudConfig:         "config_test-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructure(),
			tagTestMask: tagTestCreateZoneCategory |
				tagTestCreateRegionCategory |
				tagTestAttachRegionTags,
			//expectErr: "platform.vsphere.failureDomains.topology.computeCluster: Internal error: tag associated with tag category openshift-zone not attached to this resource or ancestor",
			expectErr: "Multi-Zone support: ClusterComputeResource DC0_C0 for failure domain test-east-1a: tag associated with tag category openshift-zone not attached to this resource or ancestor",
		},
		{
			name:                "multi-zone tag categories, missing region tag attachment",
			cloudConfig:         "config_test-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructure(),
			tagTestMask: tagTestCreateZoneCategory |
				tagTestCreateRegionCategory |
				tagTestAttachZoneTags,
			//expectErr: "platform.vsphere.failureDomains.topology.computeCluster: Internal error: tag associated with tag category openshift-zone not attached to this resource or ancestor",
			expectErr: "Multi-Zone support: ClusterComputeResource DC0_C0 for failure domain test-east-1a: tag associated with tag category openshift-region not attached to this resource or ancestor",
		},
		{
			name:                "multi-zone tag categories, missing zone and region tag categories",
			cloudConfig:         "config_test-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructure(),
			tagTestMask:         tagTestNothingCreatedOrAttached,
			expectErr:           "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
		},
		{
			name:                "multi-vcenter validation - No failure domain - No tags - ini",
			cloudConfig:         "config_single-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure: func() *v1.Infrastructure {
				inf := validInfrastructureMultiVCenter()
				inf.Spec.PlatformSpec.VSphere.FailureDomains = nil
				return inf
			}(),
		},
		{
			name:                "multi-vcenter validation - No failure domain - No tags - yaml",
			cloudConfig:         "simple_config.yaml",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure: func() *v1.Infrastructure {
				inf := validInfrastructureMultiVCenter()
				inf.Spec.PlatformSpec.VSphere.FailureDomains = nil
				return inf
			}(),
		},
		{
			name:                "multi-vcenter validation - No vCenters in spec  - No tags - ini",
			cloudConfig:         "config_single-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure: func() *v1.Infrastructure {
				inf := validInfrastructureMultiVCenter()
				inf.Spec.PlatformSpec.VSphere.VCenters = nil
				return inf
			}(),
			expectErr: "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
		},
		{
			name:                "multi-vcenter validation - No vCenters in spec  - No tags - yaml",
			cloudConfig:         "simple_config.yaml",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure: func() *v1.Infrastructure {
				inf := validInfrastructureMultiVCenter()
				inf.Spec.PlatformSpec.VSphere.VCenters = nil
				return inf
			}(),
			expectErr: "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
		},
		{
			name:                "multi-vcenter validation - No vCenters in spec  - tag categories present - ini",
			cloudConfig:         "config_single-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure: func() *v1.Infrastructure {
				inf := validInfrastructureMultiVCenter()
				inf.Spec.PlatformSpec.VSphere.VCenters = nil
				return inf
			}(),
			tagTestMask: tagTestCreateZoneCategory |
				tagTestCreateRegionCategory,
			expectErr: "Multi-Zone support: unable to check zone tags: vCenter vcenter2.test.openshift.com for failure domain test-east-1b not found in cloud provider config",
		},
		{
			name:                "multi-vcenter validation - No vCenters in spec  - tag categories present - yaml",
			cloudConfig:         "simple_config.yaml",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure: func() *v1.Infrastructure {
				inf := validInfrastructureMultiVCenter()
				inf.Spec.PlatformSpec.VSphere.VCenters = nil
				return inf
			}(),
			tagTestMask: tagTestCreateZoneCategory |
				tagTestCreateRegionCategory,
			expectErr: "Multi-Zone support: ClusterComputeResource DC0_C0 for failure domain test-east-1a: tag associated with tag category openshift-region not attached to this resource or ancestor,tag associated with tag category openshift-zone not attached to this resource or ancestor;\nMulti-Zone support: ClusterComputeResource DC1_C0 for failure domain test-east-1b: tag associated with tag category openshift-region not attached to this resource or ancestor,tag associated with tag category openshift-zone not attached to this resource or ancestor",
		},
		{
			// This test will say categories needs to be created even though 1 vCenter is missing.  This is expected.
			name:                "multi-vcenter validation - No tags - ini",
			cloudConfig:         "config_single-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructureMultiVCenter(),
			expectErr:           "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
		},
		{
			name:                "multi-vcenter validation - No tags - yaml",
			cloudConfig:         "simple_config.yaml",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructureMultiVCenter(),
			expectErr:           "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
		},
		{
			name:                "multi-vcenter tag categories present and tags attached - ini",
			cloudConfig:         "config_single-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructureMultiVCenter(),
			tagTestMask: tagTestCreateZoneCategory |
				tagTestCreateRegionCategory |
				tagTestAttachRegionTags |
				tagTestAttachZoneTags,
			expectErr: "Multi-Zone support: unable to check zone tags: vCenter vcenter2.test.openshift.com for failure domain test-east-1b not found in cloud provider config",
		},
		{
			name:                "multi-vcenter tag categories present and tags attached - yaml",
			cloudConfig:         "simple_config.yaml",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructureMultiVCenter(),
			tagTestMask: tagTestCreateZoneCategory |
				tagTestCreateRegionCategory |
				tagTestAttachRegionTags |
				tagTestAttachZoneTags,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if test.checkZoneTagsMethod != nil {
				fmt.Println("Setting up test...")
				// Setup test
				kubeClient := &testlib.FakeKubeClient{
					Nodes:          testlib.DefaultNodes(),
					Infrastructure: test.infrastructure,
				}

				var checkContext *CheckContext
				var cleanup func()

				// SetupSimulator does not seem to match the infrastructure created here.  Need to provide better config.
				checkContext, cleanup, err = SetupSimulatorWithConfig(kubeClient, testlib.DefaultModel, test.cloudConfig)
				if err != nil {
					t.Fatalf("setupSimulator failed: %s", err)
				}
				defer cleanup()

				// Perform test
				fmt.Println("Starting test...")
				for index := range checkContext.VCenters {
					vCenter := checkContext.VCenters[index]
					finder := find.NewFinder(vCenter.VMClient)

					if test.tagTestMask != 0 {
						err = setupTagAttachmentTest(*checkContext, vCenter, finder, test.tagTestMask)
						if err != nil {
							assert.NoError(t, err)
						}
					}
				}

				err = test.checkZoneTagsMethod(checkContext)

				for index := range checkContext.VCenters {
					vCenter := checkContext.VCenters[index]

					if test.tagTestMask != 0 {
						err := teardownTagAttachmentTest(*checkContext, vCenter)
						if err != nil {
							assert.NoError(t, err)
						}
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
