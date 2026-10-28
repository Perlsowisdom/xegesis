package main

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

func TestEveryRunsAndStopsCleanly(t *testing.T) {
	before := runtime.NumGoroutine()
	var runs, seen atomic.Int32
	reg := command.NewRegistry()
	reg.Add(command.Define("test.tick", "Count a tick.", false, func(context.Context, *command.Env, struct{}) (int32, error) {
		return runs.Add(1), nil
	}))
	a := &App{reg: reg, env: &command.Env{}}
	ctx, stop := context.WithCancel(context.Background())
	a.every(ctx, time.Millisecond, "test.tick", func(res any) {
		if _, ok := res.(int32); ok {
			seen.Add(1)
		}
	})
	for deadline := time.Now().Add(5 * time.Second); runs.Load() < 3; {
		if time.Now().After(deadline) {
			t.Fatal("the command did not run repeatedly")
		}
		time.Sleep(time.Millisecond)
	}
	stop()
	a.wg.Wait()
	if seen.Load() < 3 {
		t.Errorf("done called %d times", seen.Load())
	}
	for deadline := time.Now().Add(time.Second); runtime.NumGoroutine() > before; {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines left running, %d before", runtime.NumGoroutine(), before)
		}
		time.Sleep(time.Millisecond)
	}
}
