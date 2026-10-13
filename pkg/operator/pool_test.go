package operator

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestThreadPool(t *testing.T) {
	pool := NewCheckThreadPool(5, 5)
	ctx := context.TODO()
	startTime := time.Now()
	for i := 0; i < 5; i++ {
		i := i
		pool.RunGoroutine(ctx, func() {
			fmt.Printf("running parent task %d\n", i)
			for j := 0; j < 5; j++ {
				j := j
				runSomeTask(fmt.Sprintf("task-%d-%d", i, j))
			}
		})
	}
	waitChannel := make(chan interface{})
	go func() {
		pool.Wait(ctx)
		close(waitChannel)
	}()
	select {
	case <-waitChannel:
		fmt.Printf("test finished successfully")
	case <-time.After(40 * time.Second):
		t.Errorf("test failed to finish")
	}
	fmt.Printf("time taken is: %v", time.Now().Sub(startTime))
}

func runSomeTask(taskID string) {
	fmt.Printf("Running task: %s\n", taskID)
	time.Sleep(5 * time.Second)
}
