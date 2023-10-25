// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"
	json "encoding/json"
	"fmt"

	v1 "github.com/openshift/api/operator/v1"
	operatorv1 "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeServiceCatalogControllerManagers implements ServiceCatalogControllerManagerInterface
type FakeServiceCatalogControllerManagers struct {
	Fake *FakeOperatorV1
}

var servicecatalogcontrollermanagersResource = v1.SchemeGroupVersion.WithResource("servicecatalogcontrollermanagers")

var servicecatalogcontrollermanagersKind = v1.SchemeGroupVersion.WithKind("ServiceCatalogControllerManager")

// Get takes name of the serviceCatalogControllerManager, and returns the corresponding serviceCatalogControllerManager object, and an error if there is any.
func (c *FakeServiceCatalogControllerManagers) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.ServiceCatalogControllerManager, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(servicecatalogcontrollermanagersResource, name), &v1.ServiceCatalogControllerManager{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.ServiceCatalogControllerManager), err
}

// List takes label and field selectors, and returns the list of ServiceCatalogControllerManagers that match those selectors.
func (c *FakeServiceCatalogControllerManagers) List(ctx context.Context, opts metav1.ListOptions) (result *v1.ServiceCatalogControllerManagerList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(servicecatalogcontrollermanagersResource, servicecatalogcontrollermanagersKind, opts), &v1.ServiceCatalogControllerManagerList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1.ServiceCatalogControllerManagerList{ListMeta: obj.(*v1.ServiceCatalogControllerManagerList).ListMeta}
	for _, item := range obj.(*v1.ServiceCatalogControllerManagerList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested serviceCatalogControllerManagers.
func (c *FakeServiceCatalogControllerManagers) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(servicecatalogcontrollermanagersResource, opts))
}

// Create takes the representation of a serviceCatalogControllerManager and creates it.  Returns the server's representation of the serviceCatalogControllerManager, and an error, if there is any.
func (c *FakeServiceCatalogControllerManagers) Create(ctx context.Context, serviceCatalogControllerManager *v1.ServiceCatalogControllerManager, opts metav1.CreateOptions) (result *v1.ServiceCatalogControllerManager, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(servicecatalogcontrollermanagersResource, serviceCatalogControllerManager), &v1.ServiceCatalogControllerManager{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.ServiceCatalogControllerManager), err
}

// Update takes the representation of a serviceCatalogControllerManager and updates it. Returns the server's representation of the serviceCatalogControllerManager, and an error, if there is any.
func (c *FakeServiceCatalogControllerManagers) Update(ctx context.Context, serviceCatalogControllerManager *v1.ServiceCatalogControllerManager, opts metav1.UpdateOptions) (result *v1.ServiceCatalogControllerManager, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(servicecatalogcontrollermanagersResource, serviceCatalogControllerManager), &v1.ServiceCatalogControllerManager{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.ServiceCatalogControllerManager), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeServiceCatalogControllerManagers) UpdateStatus(ctx context.Context, serviceCatalogControllerManager *v1.ServiceCatalogControllerManager, opts metav1.UpdateOptions) (*v1.ServiceCatalogControllerManager, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(servicecatalogcontrollermanagersResource, "status", serviceCatalogControllerManager), &v1.ServiceCatalogControllerManager{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.ServiceCatalogControllerManager), err
}

// Delete takes name of the serviceCatalogControllerManager and deletes it. Returns an error if one occurs.
func (c *FakeServiceCatalogControllerManagers) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(servicecatalogcontrollermanagersResource, name, opts), &v1.ServiceCatalogControllerManager{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeServiceCatalogControllerManagers) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(servicecatalogcontrollermanagersResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1.ServiceCatalogControllerManagerList{})
	return err
}

// Patch applies the patch and returns the patched serviceCatalogControllerManager.
func (c *FakeServiceCatalogControllerManagers) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.ServiceCatalogControllerManager, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(servicecatalogcontrollermanagersResource, name, pt, data, subresources...), &v1.ServiceCatalogControllerManager{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.ServiceCatalogControllerManager), err
}

// Apply takes the given apply declarative configuration, applies it and returns the applied serviceCatalogControllerManager.
func (c *FakeServiceCatalogControllerManagers) Apply(ctx context.Context, serviceCatalogControllerManager *operatorv1.ServiceCatalogControllerManagerApplyConfiguration, opts metav1.ApplyOptions) (result *v1.ServiceCatalogControllerManager, err error) {
	if serviceCatalogControllerManager == nil {
		return nil, fmt.Errorf("serviceCatalogControllerManager provided to Apply must not be nil")
	}
	data, err := json.Marshal(serviceCatalogControllerManager)
	if err != nil {
		return nil, err
	}
	name := serviceCatalogControllerManager.Name
	if name == nil {
		return nil, fmt.Errorf("serviceCatalogControllerManager.Name must be provided to Apply")
	}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(servicecatalogcontrollermanagersResource, *name, types.ApplyPatchType, data), &v1.ServiceCatalogControllerManager{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.ServiceCatalogControllerManager), err
}

// ApplyStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating ApplyStatus().
func (c *FakeServiceCatalogControllerManagers) ApplyStatus(ctx context.Context, serviceCatalogControllerManager *operatorv1.ServiceCatalogControllerManagerApplyConfiguration, opts metav1.ApplyOptions) (result *v1.ServiceCatalogControllerManager, err error) {
	if serviceCatalogControllerManager == nil {
		return nil, fmt.Errorf("serviceCatalogControllerManager provided to Apply must not be nil")
	}
	data, err := json.Marshal(serviceCatalogControllerManager)
	if err != nil {
		return nil, err
	}
	name := serviceCatalogControllerManager.Name
	if name == nil {
		return nil, fmt.Errorf("serviceCatalogControllerManager.Name must be provided to Apply")
	}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(servicecatalogcontrollermanagersResource, *name, types.ApplyPatchType, data, "status"), &v1.ServiceCatalogControllerManager{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.ServiceCatalogControllerManager), err
}
