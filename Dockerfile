FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.21-openshift-4.16 AS builder
WORKDIR /go/src/github.com/openshift/vsphere-problem-detector
COPY . .
ENV GO_PACKAGE github.com/openshift/vsphere-problem-detector
RUN make

FROM registry.ci.openshift.org/ocp/4.16:base-rhel9
COPY --from=builder /go/src/github.com/openshift/vsphere-problem-detector/vsphere-problem-detector /usr/bin/
ENTRYPOINT ["/usr/bin/vsphere-problem-detector"]
LABEL io.openshift.release.operator=true
