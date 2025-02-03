package check

import (
	"errors"
	"fmt"
	"testing"

	v1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	vapitags "github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
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
								ComputeCluster: "/DC0/host/DC0_C1",
								Datacenter:     "DC0",
								Datastore:      "LocalDS_0",
								Folder:         "/DC0/vm",
								Networks: []string{
									"DC0_DVPG0",
								},
								ResourcePool: "/DC0/host/DC0_C1/Resources/test-resourcepool",
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
								ComputeCluster: "/DC1/host/DC1_C0",
								Datacenter:     "DC1",
								Datastore:      "LocalDS_0",
								Folder:         "/DC1/vm",
								Networks: []string{
									"DC1_DVPG0",
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

func setupInfrastructureTopology(checkContext CheckContext, actions ...SimulatorAction) error {
	hostIndex := 1000

	for _, vCenter := range checkContext.VCenters {
		finder := find.NewFinder(vCenter.VMClient)
		ctx := checkContext.Context

		for _, fd := range checkContext.PlatformSpec.FailureDomains {
			if fd.Server != vCenter.VCenterName {
				continue
			}

			_, err := finder.Datacenter(ctx, fd.Topology.Datacenter)
			if err != nil {
				return fmt.Errorf("unable to find datacenter %s. will attempt to create it. %v\n", fd.Topology.Datacenter, err)
			}

			if len(fd.Topology.ComputeCluster) == 0 {
				fmt.Println("no compute cluster defined in topology")
				continue
			}
			computeCluster, err := finder.ClusterComputeResource(ctx, fd.Topology.ComputeCluster)
			if err != nil {
				return fmt.Errorf("unable to find compute cluster %s. will attempt to create it. %v\n", fd.Topology.ComputeCluster, err)

			}

			if fd.ZoneAffinity == nil || fd.ZoneAffinity.Type != v1.HostGroupFailureDomainZone {
				fmt.Println("failure domain does not have a defined zone affinity or the zone affinity is cluster")
				continue
			}

			clusterConfig, err := computeCluster.Configuration(ctx)
			if err != nil {
				return fmt.Errorf("unable to get compute cluster config %s. %v\n", fd.Topology.ComputeCluster, err)
			}

			for _, group := range clusterConfig.Group {
				fmt.Printf("group: %s\n", group)
			}

			hostGroup := new(types.ClusterHostGroup)
			hostGroup.GetClusterGroupInfo().Name = fd.ZoneAffinity.HostGroup.HostGroup

			if IsSet(SimulatorNoHostsInHostGroup, actions) != SimulatorSetResultFound {
				for range 2 {
					hostSystem := object.NewHostSystem(vCenter.VMClient, types.ManagedObjectReference{
						Type:  "HostSystem",
						Value: fmt.Sprintf("host-%d", hostIndex),
					})

					hostGroup.Host = append(hostGroup.Host, hostSystem.Reference())
					hostIndex++
				}
			}

			vmGroup := new(types.ClusterVmGroup)
			vmGroup.GetClusterGroupInfo().Name = fd.ZoneAffinity.HostGroup.VMGroup

			vmHostRule := new(types.ClusterVmHostRuleInfo)
			vmHostRule.Name = fd.ZoneAffinity.HostGroup.VMHostRule

			update := types.ArrayUpdateSpec{Operation: types.ArrayUpdateOperationAdd}

			spec := &types.ClusterConfigSpecEx{
				GroupSpec: []types.ClusterGroupSpec{},
				RulesSpec: []types.ClusterRuleSpec{},
			}

			if IsSet(SimulatorNoHostGroup, actions) != SimulatorSetResultFound {
				fmt.Printf("adding host group %s to cluster %s\n", fd.ZoneAffinity.HostGroup.HostGroup, fd.Topology.ComputeCluster)
				spec.GroupSpec = append(spec.GroupSpec, types.ClusterGroupSpec{
					ArrayUpdateSpec: update,
					Info:            hostGroup,
				})
			}

			if IsSet(SimulatorNoVmGroup, actions) != SimulatorSetResultFound {
				fmt.Printf("adding vm group %s to cluster %s\n", fd.ZoneAffinity.HostGroup.VMGroup, fd.Topology.ComputeCluster)
				spec.GroupSpec = append(spec.GroupSpec, types.ClusterGroupSpec{
					ArrayUpdateSpec: update,
					Info:            vmGroup,
				})
			}

			if IsSet(SimulatorNoVmRule, actions) != SimulatorSetResultFound {
				fmt.Printf("adding vm rule %s to cluster %s\n", fd.ZoneAffinity.HostGroup.VMHostRule, fd.Topology.ComputeCluster)
				spec.RulesSpec = append(spec.RulesSpec, types.ClusterRuleSpec{
					ArrayUpdateSpec: update,
					Info:            vmHostRule,
				})
			}

			fmt.Printf("applying configuration changes to cluster %s\n", fd.Topology.ComputeCluster)
			task, err := computeCluster.Reconfigure(ctx, spec, true)
			if err != nil {
				return fmt.Errorf("error adding host group to cluser %s. %v", fd.Topology.ComputeCluster, err)
			}

			err = task.Wait(ctx)
			if err != nil {
				return fmt.Errorf("error waiting for task to complete. %v", err)
			}
		}
	}
	return nil
}

func setupTagAttachmentTest(checkContext CheckContext, actions ...SimulatorAction) error {
	var err error
	var regionCategoryID, zoneTagCategoryID string

	for idx, vCenter := range checkContext.VCenters {
		tagMgr := vCenter.TagManager
		finder := find.NewFinder(vCenter.VMClient)
		ctx := checkContext.Context

		if IsSet(SimulatorSkipCreateRegionTagCategeory, actions) != SimulatorSetResultFound {
			regionCategoryID, err = tagMgr.CreateCategory(ctx, &vapitags.Category{
				Name:        "openshift-region",
				Description: "region tag category",
			})
			if err != nil {
				return fmt.Errorf("openshift-region tag category already created: %v", err)
			}

			checkContext.VCenters[idx].RegionTagCategoryID = regionCategoryID
		}

		if IsSet(SimulatorSkipCreateZoneTagCategeory, actions) != SimulatorSetResultFound {
			zoneTagCategoryID, err = tagMgr.CreateCategory(ctx, &vapitags.Category{
				Name:        "openshift-zone",
				Description: "zone tag category",
			})
			if err != nil {
				return fmt.Errorf("openshift-zone tag category already created: %v", err)
			}

			checkContext.VCenters[idx].ZoneTagCategoryID = zoneTagCategoryID
		}

		for _, fd := range checkContext.PlatformSpec.FailureDomains {
			if len(regionCategoryID) > 0 && IsSet(SimulatorSkipAttachRegionTag, actions) != SimulatorSetResultFound {
				tagID := ""
				regionTag, err := tagMgr.GetTagForCategory(ctx, fd.Region, regionCategoryID)
				if err != nil {
					fmt.Printf("error while finding tag, will attempt to create it. %v", err)
					tagID, err = tagMgr.CreateTag(ctx, &vapitags.Tag{
						Name:       fd.Region,
						CategoryID: regionCategoryID,
					})
					if err != nil {
						return fmt.Errorf("unable to create region tag: %s. %v", fd.Region, err)
					}
				} else {
					tagID = regionTag.ID
					fmt.Printf("found region tag %s\n", tagID)
				}

				var attachMo types.ManagedObjectReference
				if fd.RegionAffinity != nil && fd.RegionAffinity.Type == v1.ComputeClusterFailureDomainRegion {
					cluster, err := finder.ClusterComputeResource(ctx, fd.Topology.ComputeCluster)
					if err != nil {
						return fmt.Errorf("unable to find compute cluster %s. %v", fd.Topology.ComputeCluster, err)
					}
					attachMo = cluster.Reference()
				} else {
					datacenter, err := finder.Datacenter(ctx, fd.Topology.Datacenter)
					if err != nil {
						return fmt.Errorf("unable to find datacenter %s. %v", fd.Topology.Datacenter, err)
					}
					attachMo = datacenter.Reference()
				}

				err = tagMgr.AttachTag(ctx, tagID, attachMo)
				if err != nil {
					return fmt.Errorf("unable to attach tag %s to %v. %v", tagID, attachMo, err)
				}
			}

			if len(fd.Topology.ComputeCluster) == 0 {
				continue
			}
			if len(zoneTagCategoryID) > 0 && IsSet(SimulatorSkipAttachZoneTag, actions) != SimulatorSetResultFound {
				tagID, err := tagMgr.CreateTag(ctx, &vapitags.Tag{
					Name:       fd.Zone,
					CategoryID: zoneTagCategoryID,
				})
				if err != nil {
					return fmt.Errorf("unable to create tag %s. %v", fd.Zone, err)
				}

				cluster, err := finder.ClusterComputeResource(ctx, fd.Topology.ComputeCluster)
				if err != nil {
					return fmt.Errorf("unable to find compute cluster %s. %v", fd.Topology.ComputeCluster, err)
				}

				var attachMos []types.ManagedObjectReference

				if fd.ZoneAffinity != nil && (IsSet(SimulatorSkipAttachHostTag, actions) != SimulatorSetResultFound) {
					clusterConfig, err := cluster.Configuration(ctx)
					if err != nil {
						return fmt.Errorf("unable to get cluster configuration. %v", err)
					}

					var hostGroup *types.ClusterHostGroup

					for _, group := range clusterConfig.Group {
						groupInfo := group.GetClusterGroupInfo()

						if _, is := group.(*types.ClusterHostGroup); is {
							if groupInfo.Name != fd.ZoneAffinity.HostGroup.HostGroup {
								continue
							}
							hostGroup = group.(*types.ClusterHostGroup)
						}
					}
					if hostGroup != nil {
						for _, host := range hostGroup.Host {
							attachMos = append(attachMos, host.Reference())
						}
					}

				} else {
					attachMos = append(attachMos, cluster.Reference())
				}

				for _, attachMo := range attachMos {
					err = tagMgr.AttachTag(ctx, tagID, attachMo)
					if err != nil {
						return fmt.Errorf("unable to attach tag %s to %v. %v", tagID, attachMo, err)
					}
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
		modelPath           string
		infrastructure      *v1.Infrastructure
		expectErr           string
		actions             []SimulatorAction
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
			actions: []SimulatorAction{
				SimulatorSkipCreateRegionTagCategeory,
				SimulatorSkipCreateZoneTagCategeory,
			},
			expectErr: "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
		},
		{
			name:                "multi-zone tag categories present and tags attached",
			cloudConfig:         "config_test-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructure(),
		},
		{
			name:                "multi-zone tag categories, missing zone tag attachment",
			cloudConfig:         "config_test-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructure(),
			actions: []SimulatorAction{
				SimulatorSkipAttachZoneTag,
			},
			expectErr: "Multi-Zone support: ClusterComputeResource DC0_C0 for failure domain test-east-1a: tag associated with tag category openshift-zone not attached to this resource or ancestor",
		},
		{
			name:                "multi-zone tag categories, missing region tag attachment",
			cloudConfig:         "config_test-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructure(),
			actions: []SimulatorAction{
				SimulatorSkipAttachRegionTag,
			},
			expectErr: "Multi-Zone support: ClusterComputeResource DC0_C0 for failure domain test-east-1a: tag associated with tag category openshift-region not attached to this resource or ancestor",
		},
		{
			name:                "multi-zone tag categories, missing zone and region tag categories",
			cloudConfig:         "config_test-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructure(),
			actions: []SimulatorAction{
				SimulatorSkipCreateRegionTagCategeory,
				SimulatorSkipCreateZoneTagCategeory,
			},
			expectErr: "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
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
			actions: []SimulatorAction{
				SimulatorSkipCreateRegionTagCategeory,
				SimulatorSkipCreateZoneTagCategeory,
			},
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
			actions: []SimulatorAction{
				SimulatorSkipCreateRegionTagCategeory,
				SimulatorSkipCreateZoneTagCategeory,
			},
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
			actions: []SimulatorAction{
				SimulatorSkipAttachRegionTag,
				SimulatorSkipAttachZoneTag,
			},
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

			actions: []SimulatorAction{
				SimulatorSkipAttachRegionTag,
				SimulatorSkipAttachZoneTag,
			},
			expectErr: "Multi-Zone support: ClusterComputeResource DC0_C0 for failure domain test-east-1a: tag associated with tag category openshift-region not attached to this resource or ancestor,tag associated with tag category openshift-zone not attached to this resource or ancestor;\nMulti-Zone support: ClusterComputeResource DC1_C0 for failure domain test-east-1b: tag associated with tag category openshift-region not attached to this resource or ancestor,tag associated with tag category openshift-zone not attached to this resource or ancestor",
		},
		{
			// This test will say categories needs to be created even though 1 vCenter is missing.  This is expected.
			name:                "multi-vcenter validation - No tags - ini",
			cloudConfig:         "config_single-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructureMultiVCenter(),
			actions: []SimulatorAction{
				SimulatorSkipCreateRegionTagCategeory,
				SimulatorSkipCreateZoneTagCategeory,
			},
			expectErr: "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
		},
		{
			name:                "multi-vcenter validation - No tags - yaml",
			cloudConfig:         "simple_config.yaml",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructureMultiVCenter(),
			actions: []SimulatorAction{
				SimulatorSkipCreateRegionTagCategeory,
				SimulatorSkipCreateZoneTagCategeory,
			},
			expectErr: "Multi-Zone support: tag categories openshift-zone and openshift-region must be created",
		},
		{
			name:                "multi-vcenter tag categories present and tags attached - ini",
			cloudConfig:         "config_single-vcenter.ini",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructureMultiVCenter(),
			expectErr:           "Multi-Zone support: unable to check zone tags: vCenter vcenter2.test.openshift.com for failure domain test-east-1b not found in cloud provider config",
		},
		{
			name:                "multi-vcenter tag categories present and tags attached - yaml",
			cloudConfig:         "simple_config.yaml",
			checkZoneTagsMethod: CheckZoneTags,
			infrastructure:      validInfrastructureMultiVCenter(),
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

				if len(test.modelPath) == 0 {
					test.modelPath = testlib.HostGroupModel
				}

				// SetupSimulator does not seem to match the infrastructure created here.  Need to provide better config.
				checkContext, cleanup, err = SetupSimulatorWithConfig(kubeClient, test.modelPath, test.cloudConfig)
				if err != nil {
					t.Fatalf("setupSimulator failed: %s", err)
				}
				defer cleanup()

				// Perform test
				fmt.Printf("starting test: %s\n", test.name)
				err = setupInfrastructureTopology(*checkContext)
				if err != nil {
					assert.NoError(t, err)
				}

				fmt.Println("setting up tag attachment test")
				err = setupTagAttachmentTest(*checkContext, test.actions...)
				if err != nil {
					assert.NoError(t, err)
				}

				err = test.checkZoneTagsMethod(checkContext)

				for index := range checkContext.VCenters {
					vCenter := checkContext.VCenters[index]

					fmt.Println("tearing down tag attachment test")
					err := teardownTagAttachmentTest(*checkContext, vCenter)
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

func dumpSimulatorContext(checkContext *CheckContext) {
	ctx := checkContext.Context
	vCenters := checkContext.VCenters

	for _, vCenter := range vCenters {
		fmt.Printf("--> vCenter %s\n", vCenter.VCenterName)
		finder := find.NewFinder(vCenter.VMClient)

		datacenters, err := finder.DatacenterList(ctx, "./...")
		if err != nil {
			fmt.Printf("unable to get datacenter list. %v\n", err)
			return
		}

		for _, datacenter := range datacenters {
			fmt.Printf(".... Datacenter %s\n", datacenter.Name())
			finder := find.NewFinder(vCenter.VMClient).SetDatacenter(datacenter)

			datacenterFolders, err := datacenter.Folders(ctx)
			if err != nil {
				fmt.Printf("unable to get datacenter folders. %v\n", err)
				return
			}
			clusters, err := finder.ClusterComputeResourceList(ctx, fmt.Sprintf("%s/...", datacenterFolders.HostFolder.InventoryPath))
			if err != nil {
				fmt.Printf("unable to get clusters from datacenter host folder. %v\n", err)
				return
			}

			for _, cluster := range clusters {
				fmt.Printf("...... Cluster: %s\n", cluster.Name())
			}
		}
	}
}
