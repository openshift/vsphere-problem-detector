# vSphere Problem Detector

vSphere Problem Detector is an OpenShift operator that ensures the vSphere where OpenShift runs is configured properly. 

## Usage

This operator is deployed automatically in OpenShift clusters when it runs on vSphere.

### Running from command line

```sh
$ ./vsphere-problem-detector start -v 5 --kubeconfig=$KUBECONFIG --namespace=openshift-cluster-storage-operator
```
