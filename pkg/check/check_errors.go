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
)

type ErrorItem struct {
	errorLabel    KnownErrorLabel
	errorInstance error
}

type CheckError struct {
	errorItems map[KnownErrorLabel]error
}

var _ error = &CheckError{}

func NewEmptyCheckErrorAggregator() *CheckError {
	return &CheckError{
		errorItems: map[KnownErrorLabel]error{},
	}
}

func NewCheckError(errorLabel KnownErrorLabel, err error) *CheckError {
	return &CheckError{
		errorItems: map[KnownErrorLabel]error{
			errorLabel: err,
		},
	}
}

func (c *CheckError) addError(errorLabel KnownErrorLabel, err error) *CheckError {
	if c.errorItems == nil {
		c.errorItems = map[KnownErrorLabel]error{}
	}
	c.errorItems[errorLabel] = err
	return c
}

// Join can be used to check if we really had errors when aggregating for errors
func (c *CheckError) Join() *CheckError {
	if len(c.errorItems) > 0 {
		return c
	}
	return nil
}

// addCheckError merges another CheckError into this CheckError
func (c *CheckError) addCheckError(e *CheckError) *CheckError {
	if c.errorItems == nil {
		c.errorItems = map[KnownErrorLabel]error{}
	}
	for errorLabel, err := range e.errorItems {
		c.errorItems[errorLabel] = err
	}
	return c
}

func (c *CheckError) Error() string {
	errorList := []error{}
	for _, e := range c.errorItems {
		errorList = append(errorList, e)
	}
	if len(errorList) == 1 {
		return errorList[0].Error()
	}
	aggregatedError := errors.NewAggregate(errorList)
	return aggregatedError.Error()
}
