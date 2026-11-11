package command

import (
	"reflect"
	"testing"
)

func TestCloseRunsInReverseOnce(t *testing.T) {
	r := NewRegistry()
	var got []string
	r.OnClose(func() { got = append(got, "first") })
	r.OnClose(func() { got = append(got, "second") })
	r.Close()
	r.Close()
	if want := []string{"second", "first"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("closed %v, want %v", got, want)
	}
}
