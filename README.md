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

### Running with plain k8s

1. Create tls key and crt for running the problem detector. 

```
# mkdir -p /var/run/secrets/serving-cert
# cp /var/run/kubernetes/client-admin.key tls.key
# cp /var/run/kubernetes/client-admin.crt tls.crt
```

If started without this step, vsphere-problem-detector will be started with a self-signed cert.

2. If started via plain k8s. The command to start problem detector is:

Make sure that username and password is embedded in cloud.conf:

```
# export KUBECONFIG=/var/run/kubernetes/admin.kubeconfig
# ./vsphere-problem-detector  --vanilla-kube --cloud-config ~/cloud.conf start --kubeconfig=$KUBECONFIG --namespace=default --v 5
```

3. Getting metrics:

```
~> curl -k --cert /var/run/secrets/serving-cert/tls.crt --key /var/run/secrets/serving-cert/tls.key https://localhost:8443/metrics
```
