package check

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// CheckNodeDiskPerf Checks node performance of master nodes.
type CheckNodeDiskPerf struct{}

func (c *CheckNodeDiskPerf) Name() string {
	return "CheckNodePerf"
}

type MetricCheckDef struct {
	errorLevel        int64
	metricName        string
	errorEventMessage string
	checkMasters      bool
	checkWorkers      bool
}

const (
	metricMaxSamples = 300
	/**
	metrics collection interval in seconds(approximate)
	*/
	metricIntervalID = 20

	/**
	minimum required samples before a latency average is calculated
	*/
	minimumRequiredSamples = 30
)

var (
	perfMetricIDMap       = map[string]*types.PerfMetricId{}
	buildCounterIDMapLock sync.Mutex

	metricCheckDefs = []MetricCheckDef{
		{
			errorLevel:        100,
			metricName:        "disk.maxTotalLatency.latest",
			errorEventMessage: "master node has disk latency of greater than 100ms",
			checkMasters:      true,
		},
	}
)

func (c *CheckNodeDiskPerf) BuildCounterIdMap(ctx *CheckContext, vm *mo.VirtualMachine) error {
	buildCounterIDMapLock.Lock()
	defer buildCounterIDMapLock.Unlock()

	if len(perfMetricIDMap) > 0 {
		return nil
	}
	var counterIds []int32

	perf := types.QueryAvailablePerfMetric{
		This:   *ctx.VMClient.ServiceContent.PerfManager,
		Entity: vm.ManagedEntity.Reference(),
	}
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()

	perfList, err := methods.QueryAvailablePerfMetric(tctx, ctx.VMClient.RoundTripper, &perf)
	if err != nil {
		klog.V(2).Infof("error getting available performance metrics: %s", err.Error())
		return err
	}

	var counterIDPerfMetricIDMap = map[int32]*types.PerfMetricId{}
	klog.V(4).Infof("found [%d] performance metrics available", len(perfList.Returnval))
	for _, perf := range perfList.Returnval {
		counterIds = append(counterIds, perf.CounterId)
		copyPerf := perf
		counterIDPerfMetricIDMap[perf.CounterId] = &copyPerf
	}

	perfCounter := types.QueryPerfCounter{
		This:      *ctx.VMClient.ServiceContent.PerfManager,
		CounterId: counterIds,
	}

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()

	perfCounters, err := methods.QueryPerfCounter(tctx, ctx.VMClient.RoundTripper, &perfCounter)
	if err != nil {
		return err
	}

	for _, perfCounter := range perfCounters.Returnval {
		if _, ok := counterIDPerfMetricIDMap[perfCounter.Key]; !ok {
			continue
		}
		perfMetricIDMap[perfCounter.Name()] = counterIDPerfMetricIDMap[perfCounter.Key]
	}
	return nil
}

func (c *CheckNodeDiskPerf) GetPerfMetric(ctx *CheckContext, vm *mo.VirtualMachine, metricName string) (*types.PerfMetricId, error) {
	err := c.BuildCounterIdMap(ctx, vm)
	if err != nil {
		return nil, err
	}
	if _, ok := perfMetricIDMap[metricName]; !ok {
		return nil, fmt.Errorf("unable to find metric with name[%s]", metricName)
	}
	return perfMetricIDMap[metricName], nil
}

func (c *CheckNodeDiskPerf) StartCheck() error {
	return nil
}

func (c *CheckNodeDiskPerf) PerformMetricCheck(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine, mcd MetricCheckDef) *CheckError {
	var (
		hasError = false
	)

	if !checkNodePerf(node, mcd) {
		return nil
	}

	klog.V(4).Infof("performing metric check of: %s", mcd.metricName)

	perfMetricID, err := c.GetPerfMetric(ctx, vm, mcd.metricName)
	if err != nil {
		klog.V(2).Infof("error getting perf metric id: %s", err.Error())
		// returning nil as metric may legitimately not be present for the machine
		return nil
	}

	metricsSpec := []types.PerfQuerySpec{{
		Entity:     vm.ManagedEntity.Reference(),
		MetricId:   []types.PerfMetricId{*perfMetricID},
		MaxSample:  metricMaxSamples,
		IntervalId: metricIntervalID,
	}}

	perfQueryReq := types.QueryPerf{
		This:      *ctx.VMClient.ServiceContent.PerfManager,
		QuerySpec: metricsSpec,
	}

	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()

	perfQueryRes, err := methods.QueryPerf(tctx, ctx.VMClient.RoundTripper, &perfQueryReq)
	if err != nil {
		klog.V(2).Infof("error querying perf metrics: %s", err.Error())
		// returning nil to prevent flagging this test as failed due to a failure to pull metric
		return nil
	}

	for _, queryResult := range perfQueryRes.Returnval {
		basePerfMetricSeries := queryResult.(*types.PerfEntityMetric).Value
		for _, perfMetricSeries := range basePerfMetricSeries {
			metricSeries := perfMetricSeries.(*types.PerfMetricIntSeries)
			klog.V(4).Infof("metric samples[%s]: %+v", mcd.metricName, metricSeries)
			latencyAccum := int64(0)
			samples := int64(len(metricSeries.Value))
			if samples < minimumRequiredSamples {
				klog.V(4).Infof("skipping metric[%s] until minimum required[%d] samples available", mcd.metricName, minimumRequiredSamples)
				continue
			}
			for _, value := range metricSeries.Value {
				latencyAccum = latencyAccum + value
			}
			averageLatency := latencyAccum / samples
			if averageLatency > mcd.errorLevel {
				hasError = true
				klog.V(2).Infof("high average disk latency %d ms measured for node%s", averageLatency, node.Name)
				break
			}
			klog.V(2).Infof("average disk latency of %d ms measured for node %s", averageLatency, node.Name)
		}
	}
	if hasError {
		return NewCheckError(FailedDiskLatency, errors.New(mcd.errorEventMessage))
	}

	return nil
}

func (c *CheckNodeDiskPerf) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) *CheckError {
	for _, mcd := range metricCheckDefs {
		err := c.PerformMetricCheck(ctx, node, vm, mcd)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *CheckNodeDiskPerf) FinishCheck(ctx *CheckContext) {
	return
}

// checkNodePerf Check to see if the specified node should have its performance check.
func checkNodePerf(node *v1.Node, mcd MetricCheckDef) bool {
	checkPerf := true
	_, isMaster := node.Labels["node-role.kubernetes.io/master"]
	_, isWorker := node.Labels["node-role.kubernetes.io/worker"]

	if isMaster && mcd.checkMasters == false {
		klog.V(2).Infof("skipping metric check of [%s] as it is not intended for master nodes", mcd.metricName)
		checkPerf = false
	} else if isWorker && mcd.checkWorkers == false {
		if isMaster && mcd.checkMasters {
			klog.V(2).Infof("master nodes are schedulable, allowing check of [%s] even though check is disabled for workers", mcd.metricName)
		} else {
			klog.V(2).Infof("skipping metric check of [%s] as it is not intended for worker nodes", mcd.metricName)
			checkPerf = false
		}
	} else if !isMaster && !isWorker {
		// An unknown role, just exit and skip
		klog.V(2).Infof("node is not labelled as master or worker, skipping metric check of [%s] for node %s", mcd.metricName, node.Name)
		checkPerf = false
	}
	return checkPerf
}
