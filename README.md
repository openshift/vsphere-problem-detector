# vSphere Problem Detector

vSphere Problem Detector is an OpenShift operator that ensures the vSphere where OpenShift runs is configured properly. 

## Usage

This operator is deployed automatically in OpenShift clusters when it runs on vSphere.

### Running from command line

```sh
$ ./vsphere-problem-detector start -v 5 --kubeconfig=$KUBECONFIG --namespace=openshift-cluster-storage-operator
```


## WIP

This is work-in-progress operator.

Until proper `VSphereProblemDetector` CRD is introduced in openshift/api, `ClusterCSIDriver` CR with name `csi.ovirt.org` is used!
Results of checks are rendered as `clustercsidriver.Status.Conditions` instead of proper fields.
