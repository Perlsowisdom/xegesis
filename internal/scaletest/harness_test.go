package scaletest

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/all"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// lib is a scratch library driven only through the command registry, as the app and CLI drive it.
type lib struct {
	t    testing.TB
	data string
	st   *store.Store
	reg  *command.Registry
	env  *command.Env
}

func openLib(t testing.TB, data string) *lib {
	t.Helper()
	st, err := store.Open(filepath.Join(data, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &lib{t: t, data: data, st: st, reg: all.Registry(data, secrets.NewMemory()), env: &command.Env{Store: st}}
}

func (l *lib) run(id string, params, out any) error {
	l.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := l.reg.Run(context.Background(), l.env, id, raw)
	if err != nil {
		return err
	}
	if out != nil {
		reflect.ValueOf(out).Elem().SetZero()
		b, _ := json.Marshal(res)
		if err := json.Unmarshal(b, out); err != nil {
			l.t.Fatal(err)
		}
	}
	return nil
}

func (l *lib) must(id string, params, out any) {
	l.t.Helper()
	if err := l.run(id, params, out); err != nil {
		l.t.Fatalf("%s: %v", id, err)
	}
}

// timed runs a command and returns how long it took.
func (l *lib) timed(id string, params, out any) time.Duration {
	l.t.Helper()
	start := time.Now()
	l.must(id, params, out)
	return time.Since(start)
}
