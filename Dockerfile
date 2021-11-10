FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.17-openshift-4.10 AS builder
WORKDIR /go/src/github.com/openshift/vsphere-problem-detector
COPY . .
ENV GO_PACKAGE github.com/openshift/vsphere-problem-detector
RUN make

FROM registry.ci.openshift.org/ocp/4.10:base
COPY --from=builder /go/src/github.com/openshift/vsphere-problem-detector/vsphere-problem-detector /usr/bin/
ENTRYPOINT ["/usr/bin/vsphere-problem-detector"]
LABEL io.openshift.release.operator=true
