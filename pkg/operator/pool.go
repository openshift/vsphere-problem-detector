package operator

import (
	"context"
	"sync"

	"github.com/openshift/vsphere-problem-detector/pkg/check"
	"golang.org/x/sync/semaphore"
	"k8s.io/klog/v2"
)

// CheckThreadPool runs individual functions (presumably checks) as go routines.
// It makes sure that only limited number of functions actually run in parallel.
type CheckThreadPool struct {
	wg  sync.WaitGroup
	sem *semaphore.Weighted
}

// Creates a new CheckThreadPool with given max. number of goroutines.
func NewCheckThreadPool(parallelism int64) *CheckThreadPool {
	return &CheckThreadPool{
		sem: semaphore.NewWeighted(parallelism),
	}
}

// RunGoroutine spawns a new go routine with given check.
// The goroutine waits for a semaphore, so only limited number
// of goroutines actually run in parallel.
// If the check cannot start before ctx finishes, the check
// is not performed at all and it is silently thrown away,
// assuming the operator is shutting down anyway.
func (r *CheckThreadPool) RunGoroutine(ctx context.Context, check func()) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		err := r.sem.Acquire(ctx, 1)
		if err != nil {
			// This means the operator is being stopped
			klog.V(2).Infof("Warning: check stopped, operator is shutting down: %s", err)
			return
		}
		defer r.sem.Release(1)
		check()
	}()
}

// Wait blocks until all previously started goroutines finish
// or ctx expires.
func (r *CheckThreadPool) Wait(ctx context.Context) error {
	done := make(chan interface{})
	go func() {
		r.wg.Wait()
		close(done)
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
// It returns status of each check and overall status
// of all checks.
func (r *ResultCollector) Collect() ([]checkResult, []error) {
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
	return checkResults, allErrs
}
