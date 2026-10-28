package main

import (
	"context"
	"math"
	"runtime"
	"runtime/debug"
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

func TestMemoryLimit(t *testing.T) {
	t.Setenv("GOMEMLIMIT", "")
	if got := setMemoryLimit(); got != memoryLimit || debug.SetMemoryLimit(-1) != memoryLimit {
		t.Errorf("limit %d", got)
	}
	t.Setenv("GOMEMLIMIT", "1GiB")
	setMemoryLimit()
	if got := debug.SetMemoryLimit(-1); got != memoryLimit {
		t.Errorf("GOMEMLIMIT set: the app changed the limit to %d", got)
	}
	debug.SetMemoryLimit(math.MaxInt64)
}

func TestAddsBooksStartsIndexing(t *testing.T) {
	var indexed atomic.Int32
	reg := command.NewRegistry()
	reg.Add(command.Define("library.add", "Add.", false, func(context.Context, *command.Env, struct{}) (int, error) {
		return 1, nil
	}), command.Define("library.items", "List.", false, func(context.Context, *command.Env, struct{}) (int, error) {
		return 1, nil
	}), command.Define("search.index", "Index.", false, func(context.Context, *command.Env, struct{}) (int, error) {
		indexed.Add(1)
		return 1, nil
	}))
	a := &App{ctx: context.Background(), bg: context.Background(), reg: reg, env: &command.Env{}}
	for _, id := range []string{"library.items", "library.add"} {
		if _, err := a.Run(id, "{}"); err != nil {
			t.Fatal(err)
		}
	}
	if indexed.Load() != 1 {
		t.Errorf("search.index ran %d times", indexed.Load())
	}
}
