# Debug this operator with openshift

1. Using user supplied prompt, request path to current `KUBECONFIG`. Do not attempt to detect one automatically.
2. Switch to `openshift-cluster-storage-operator` project and then extract arguments and parameters running for `vsphere-problem-detector` pod.
3. Create a config in `.vscode/launch.json` from root of this project with name "debug vsphere-problem-detector" if it doesn't exist already.
4. Using arguments extracted from step-2 create a configuration that will start vsphere-problem-detector.
5. Since we are running the debug configuration out of k8s cluster, we also need to supply following arguments:
    - `--namespace=openshift-cluster-storage-operator`
    - `--kubeconfig=$KUBECONFIG`
6. Set log level to 5.
7. Scale down cluster-version-operator running `openshift-cluster-version` namespace.
8. Scale down cluster-storage-operator running in namespace `openshift-cluster-storage-operator`.
9. Scale down any existing deployment of `vsphere-problem-detector` to 0 in same `openshift-cluster-storage-operator` namespace.
10. Ask user to switch to `cmd/vsphere-problem-detector/main.go` and tell user that the project is ready to debug.
11. exit.





