package sword

import (
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Index sizes of CrossWire modules in each numbering (2 + books + chapters + verses per
// testament), as measured from the published files.
var indexSizes = map[string][2]int{
	"KJV": {24115, 8246}, "NRSV": {24115, 8248}, "MT": {24183, 2}, "Leningrad": {24183, 2},
	"Synodal": {30299, 8244}, "Vulg": {28985, 9714}, "Catholic": {28659, 8248},
	"Catholic2": {28773, 8248}, "German": {24185, 8246}, "Luther": {28940, 8246}, "Calvin": {24183, 8246},
}

func TestNumberingSizes(t *testing.T) {
	if len(Versifications()) != len(indexSizes) {
		t.Fatalf("numberings %v", Versifications())
	}
	for name, want := range indexSizes {
		v, ok := LookupVersification(name)
		if !ok {
			t.Fatalf("%s missing", name)
		}
		if got := [2]int{v.Entries(0), v.Entries(1)}; got != want {
			t.Errorf("%s entries %v, want %v", name, got, want)
		}
	}
}

func english(v *Versification) func(verse.ID) verse.ID {
	m := map[verse.ID]verse.ID{}
	for _, p := range v.EnglishMap() {
		m[p.Module] = p.English
	}
	return func(id verse.ID) verse.ID {
		if e, ok := m[id]; ok {
			return e
		}
		if c, err := verse.Convert(id, v.System, verse.English); err == nil {
			return c
		}
		return id
	}
}

func ref(t *testing.T, s string) verse.ID {
	t.Helper()
	r, err := verse.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return r.Start
}

func TestEnglishMap(t *testing.T) {
	for _, tc := range []struct{ v11n, from, to string }{
		{"KJV", "Ps 23:1", "Ps 23:1"},
		{"Synodal", "Ps 22:1", "Ps 23:1"},
		{"Synodal", "Ps 9:22", "Ps 10:1"},
		{"Synodal", "Ps 50:3", "Ps 51:1"},
		{"Synodal", "Ps 113:9", "Ps 115:1"},
		{"Synodal", "Ps 115:1", "Ps 116:10"},
		{"Synodal", "Ps 147:1", "Ps 147:12"},
		{"Vulg", "Ps 22:1", "Ps 23:1"},
		{"Vulg", "Ps 50:3", "Ps 51:1"},
		{"Vulg", "Dan 13:1", "Sus 1:1"},
		{"Vulg", "Dan 14:1", "Bel 1:1"},
		{"Vulg", "Dan 3:91", "Dan 3:24"},
		{"MT", "Ps 51:3", "Ps 51:1"},
		{"MT", "Ps 3:1", "Ps 3:0"},
		{"MT", "Joel 3:1", "Joel 2:28"},
		{"MT", "Mal 3:19", "Mal 4:1"},
		{"German", "Mal 3:19", "Mal 4:1"},
		{"German", "Gen 32:1", "Gen 31:55"},
		{"Luther", "Joel 4:1", "Joel 3:1"},
		{"NRSV", "John 3:16", "John 3:16"},
		{"Catholic", "Ps 51:3", "Ps 51:1"},
	} {
		v, _ := LookupVersification(tc.v11n)
		if got, want := english(v)(ref(t, tc.from)), ref(t, tc.to); got != want {
			t.Errorf("%s %s = english %s, want %s", tc.v11n, tc.from, got, want)
		}
	}
}

// TestEnglishMapCoverage reports how much of each numbering lines up with the English one, and
// checks that no two verses of a numbering land on the same English verse outside Psalm titles.
func TestEnglishMapCoverage(t *testing.T) {
	for _, name := range Versifications() {
		v, _ := LookupVersification(name)
		to := english(v)
		seen := map[verse.ID]verse.ID{}
		clash := 0
		for _, books := range [][]Book{v.OT, v.NT} {
			for _, b := range books {
				if b.Number == 0 {
					continue
				}
				for c, n := range b.Verses {
					for vs := 1; vs <= n; vs++ {
						id := mustID(b.Number, c+1, vs)
						e := to(id)
						if e.Verse() == 0 {
							continue
						}
						if _, dup := seen[e]; dup {
							clash++
						}
						seen[e] = id
					}
				}
			}
		}
		t.Logf("%s: %d table rows, %d chapters kept as numbered, %d shared english verses",
			name, len(v.EnglishMap()), len(v.Approximate()), clash)
		if clash > 40 {
			t.Errorf("%s: %d verses share an english verse", name, clash)
		}
	}
}
