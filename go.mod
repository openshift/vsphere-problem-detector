module github.com/openshift/vsphere-problem-detector

go 1.15

require (
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/openshift/api v0.0.0-20210730095913-85e1d547cdee
	github.com/openshift/build-machinery-go v0.0.0-20210712174854-1bb7fd1518d3
	github.com/openshift/client-go v0.0.0-20210730113412-1811c1b3fc0e
	github.com/openshift/library-go v0.0.0-20210830145332-4a9873bf5e74
	github.com/prometheus/client_golang v1.11.0
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/vmware/govmomi v0.26.1
	google.golang.org/appengine v1.6.6 // indirect
	gopkg.in/gcfg.v1 v1.2.3
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/component-base v0.22.1
	k8s.io/klog/v2 v2.10.0
	k8s.io/legacy-cloud-providers v0.22.1
)
