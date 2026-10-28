package reader

import (
	"context"
	"testing"
)

func TestRefsFindsReferencesWithUTF16Offsets(t *testing.T) {
	s := &service{}
	got, err := s.refsCmd(context.Background(), nil, RefsParams{Texts: []string{
		"On Romans 8:28. All things",
		"“Quoted” — see Ps 37:23 and John 3:16",
		"No references here",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0]) != 1 || got[0][0].Ref != "Rom.8.28" || got[0][0].Start != 3 || got[0][0].End != 14 {
		t.Fatalf("line 1: %+v", got[0])
	}
	line := []rune("“Quoted” — see Ps 37:23 and John 3:16")
	if len(got[1]) != 2 || string(line[got[1][0].Start:got[1][0].End]) != "Ps 37:23" || got[1][1].Ref != "John.3.16" {
		t.Fatalf("line 2: %+v", got[1])
	}
	if len(got[2]) != 0 {
		t.Fatalf("line 3: %+v", got[2])
	}
}
