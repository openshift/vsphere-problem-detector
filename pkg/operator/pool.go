package operator

import (
	"context"
	"sync"

	"github.com/openshift/vsphere-problem-detector/pkg/check"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
)

// CheckThreadPool runs individual functions (presumably checks) as go routines.
// It makes sure that only limited number of functions actually run in parallel.
type CheckThreadPool struct {
	wg     sync.WaitGroup
	workCh chan func()
}

// Creates a new CheckThreadPool with given max. number of goroutines.
func NewCheckThreadPool(parallelism int, channelBufferSize int) *CheckThreadPool {
	pool := &CheckThreadPool{
		workCh: make(chan func(), channelBufferSize),
	}

	for i := 0; i < parallelism; i++ {
		i := i
		go func() {
			pool.worker(i)
		}()
	}
	return pool
}

func (r *CheckThreadPool) worker(index int) {
	klog.V(5).Infof("Worker %d started", index)
	for work := range r.workCh {
		func() {
			// Make sure to mark the work as done on panic
			defer r.wg.Done()
			work()
		}()
	}
	klog.V(5).Infof("Worker %d finished", index)
}

// RunGoroutine runs given check in a worker goroutine.
// This call can block until a worker goroutine is available.
func (r *CheckThreadPool) RunGoroutine(ctx context.Context, check func()) {
	r.wg.Add(1)
	r.workCh <- check
}

// Wait blocks until all previously started goroutines finish
// or ctx expires.
func (r *CheckThreadPool) Wait(ctx context.Context) error {
	// wg.Wait does not provide a channel, so make one.
	done := make(chan interface{})
	go func() {
		r.wg.Wait()
		close(done)
		// Finish all workers when all work is done
		close(r.workCh)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// ResultCollector accumulates results of each check,
// as the checks run in parallel.
type ResultCollector struct {
	resultsMutex sync.Mutex
	// map check name -> list of all check results incl. all nil errors.
	// For cluster level checks, only one error is expected, for node
	// level checks, every node will have a single error here.
	results map[string][]error
}

// NewResultCollector creates a new ResultCollector
func NewResultsCollector() *ResultCollector {
	return &ResultCollector{
		results: make(map[string][]error),
	}
}

// AddResult stores result of a single check.
// It is allowed to store result of a single check
// several times, e.g. once for each node.
// The collector will merge the result.
func (r *ResultCollector) AddResult(res checkResult) {
	name := res.Name

	r.resultsMutex.Lock()
	defer r.resultsMutex.Unlock()

	oldRes, found := r.results[name]
	if !found {
		r.results[name] = []error{res.Error}
		return
	}

	r.results[name] = append(oldRes, res.Error)
}

// Collect returns currently accumulated checks.
// It merges results of the same check into a single error
// It returns status of each check and overall
// succeeded / failed status of all checks.
func (r *ResultCollector) Collect() ([]checkResult, error) {
	r.resultsMutex.Lock()
	defer r.resultsMutex.Unlock()

	var allErrs []error
	var checkResults []checkResult
	for name, checkErrors := range r.results {
		res := checkResult{
			Name: name,
		}
		var errs []error
		// Filter out all nil errors
		for _, err := range checkErrors {
			if err == nil {
				continue
			}
			errs = append(errs, err)
			allErrs = append(allErrs, err)
		}
		if len(errs) == 0 {
			res.Error = nil
		} else {
			res.Error = check.JoinErrors(errs)
		}
		checkResults = append(checkResults, res)
	}
	return checkResults, errors.NewAggregate(allErrs)
}
