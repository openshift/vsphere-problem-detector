package check

import "k8s.io/apimachinery/pkg/util/errors"

type KnownErrorLabel string

const (
	FailedGettingDataCenter      KnownErrorLabel = "failed_getting_datacenter"
	MissingPermissionsDataCenter KnownErrorLabel = "missing_permissions_datacenter"

	FailedGettingDatastore  KnownErrorLabel = "failed_getting_datastore"
	FailedBrowsingDatastore KnownErrorLabel = "failed_browsing_datastore"
	FailedListingDatastore  KnownErrorLabel = "failed_listing_datastore"

	MissingPermissionsDatastore KnownErrorLabel = "missing_permissions_datastore"

	FailedGettingStoragePolicy KnownErrorLabel = "failed_getting_storage_policy"
	StoragePolicyNotFound      KnownErrorLabel = "storage_policy_not_found"
	FailedListingStoragePolicy KnownErrorLabel = "failed_listing_storage_policy"

	FailedGettingDatastoreCluster KnownErrorLabel = "failed_getting_datastore_cluster"
	DatastoreClusterInUse         KnownErrorLabel = "datastore_cluster_inuse"

	VcenterNotFound KnownErrorLabel = "vcenter_not_found"

	NodeMissingPermissions KnownErrorLabel = "node_missing_permissions"
	FailedGettingNode      KnownErrorLabel = "failed_getting_node"

	FailedGettingHost KnownErrorLabel = "failed_getting_host"

	FailedDiskLatency KnownErrorLabel = "failed_disk_latency"

	EmptyNodeProviderId KnownErrorLabel = "empty_node_provider_id"
	EmptyNodeDiskUUID   KnownErrorLabel = "empty_node_disk_uuid"
	FalseNodeDiskUUID   KnownErrorLabel = "false_node_disk_uuid"

	ResourcePoolMissingPermissions KnownErrorLabel = "resource_pool_missing_permissions"

	MissingTaskPermissions KnownErrorLabel = "missing_task_permissions"

	FailedGettingFolder      KnownErrorLabel = "failed_getting_folder"
	MissingFolderPermissions KnownErrorLabel = "missing_folder_permissions"

	FailedGettingTagsCategories KnownErrorLabel = "failed_getting_categories"
	MissingTagsCategories       KnownErrorLabel = "missing_tags_categories"
	MissingZoneRegions          KnownErrorLabel = "missing_zone_regions"

	InvalidComputerClusterPath  KnownErrorLabel = "invalid_compute_cluster_path"
	FailedGettingComputeCluster KnownErrorLabel = "failed_getting_compute_cluster"

	// Openshift API error labels
	OpenshiftAPIError KnownErrorLabel = "openshift_api_error"

	// we don't actually emit this error in code except for test
	MiscError KnownErrorLabel = "misc_error"
)

type ErrorItem struct {
	ErrorLabel    KnownErrorLabel
	ErrorInstance error
}

// CheckError contains one or more errors encountered
// while performing vSphere specific cluster checks.
// CheckError intentionally does not implement error interface
// because doing so results in unintentional errors such as
// https://go.dev/doc/faq#nil_error
type CheckError struct {
	ErrorItems map[KnownErrorLabel]error
}

func NewEmptyCheckErrorAggregator() *CheckError {
	return &CheckError{
		ErrorItems: map[KnownErrorLabel]error{},
	}
}

func NewCheckError(errorLabel KnownErrorLabel, err error) *CheckError {
	return &CheckError{
		ErrorItems: map[KnownErrorLabel]error{
			errorLabel: err,
		},
	}
}

func (c *CheckError) addError(errorLabel KnownErrorLabel, err error) *CheckError {
	if c.ErrorItems == nil {
		c.ErrorItems = map[KnownErrorLabel]error{}
	}
	c.ErrorItems[errorLabel] = err
	return c
}

// Join can be used to check if we really had errors when aggregating for errors
func (c *CheckError) Join() *CheckError {
	if len(c.ErrorItems) > 0 {
		return c
	}
	return nil
}

func (c *CheckError) GetErrors() error {
	if len(c.ErrorItems) == 0 {
		return nil
	}
	errorList := []error{}
	for _, err := range c.ErrorItems {
		errorList = append(errorList, err)
	}
	return errors.NewAggregate(errorList)
}

// addCheckError merges another CheckError into this CheckError
func (c *CheckError) addCheckError(e *CheckError) *CheckError {
	if c.ErrorItems == nil {
		c.ErrorItems = map[KnownErrorLabel]error{}
	}
	for errorLabel, err := range e.ErrorItems {
		c.ErrorItems[errorLabel] = err
	}
	return c
}
