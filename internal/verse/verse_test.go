package verse

import "testing"

func TestMakeAndParts(t *testing.T) {
	id, err := Make(45, 8, 28)
	if err != nil || id != 45008028 {
		t.Fatalf("Make = %d, %v", id, err)
	}
	if id.Book() != 45 || id.Chapter() != 8 || id.Verse() != 28 {
		t.Fatal("parts wrong")
	}
	if id.String() != "Romans 8:28" || id.OSIS() != "Rom.8.28" {
		t.Fatalf("got %s / %s", id, id.OSIS())
	}
	for _, bad := range [][3]int{{0, 1, 1}, {85, 1, 1}, {45, 17, 1}, {45, 0, 1}, {45, 1, 1000}} {
		if _, err := Make(bad[0], bad[1], bad[2]); err == nil {
			t.Errorf("Make%v should fail", bad)
		}
	}
}

func TestAliasesAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, b := range books {
		for _, n := range append([]string{b.OSIS, b.Name}, b.Aliases...) {
			f := fold(n)
			if other, ok := seen[f]; ok && other != b.OSIS {
				t.Errorf("%q is used by %s and %s", n, other, b.OSIS)
			}
			seen[f] = b.OSIS
		}
	}
}

func TestParse(t *testing.T) {
	cases := map[string]string{
		"Rom 8:28":          "Romans 8:28",
		"romans 8:28-30":    "Romans 8:28-30",
		"Rom. 8:28–9:2":     "Romans 8:28-9:2",
		"Rom 8":             "Romans 8",
		"Ps 23-24":          "Psalms 23-24",
		"Jude 5":            "Jude 1:5",
		"Jude 5-7":          "Jude 1:5-7",
		"1 John 3:16":       "1 John 3:16",
		"1Jn 3:16":          "1 John 3:16",
		"I John 3:16":       "1 John 3:16",
		"II Kings 5:1":      "2 Kings 5:1",
		"First Peter 1:3":   "1 Peter 1:3",
		"Isa 53:5":          "Isaiah 53:5",
		"Song 2:1":          "Song of Songs 2:1",
		"Rom.8.28":          "Romans 8:28",
		"Rom.8.28-Rom.8.30": "Romans 8:28-30",
		"Sir 1:1":           "Sirach 1:1",
	}
	for in, want := range cases {
		r, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q): %v", in, err)
			continue
		}
		if r.String() != want {
			t.Errorf("Parse(%q) = %s, want %s", in, r, want)
		}
	}
	for _, bad := range []string{"", "Rom", "Xyz 1:1", "Rom 17:1", "Rom 8:30-28", "8:28"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

func TestRanges(t *testing.T) {
	ch, _ := Parse("Rom 8")
	v, _ := Parse("Rom 8:28")
	other, _ := Parse("Rom 9:1-5")
	if !ch.Contains(v.Start) || !ch.Overlaps(v) || ch.Overlaps(other) {
		t.Fatal("range checks wrong")
	}
}

func TestConvert(t *testing.T) {
	cases := []struct{ eng, heb string }{
		{"Mal 4:1", "Mal 3:19"},
		{"Mal 4:6", "Mal 3:24"},
		{"Joel 2:28", "Joel 3:1"},
		{"Joel 3:21", "Joel 4:21"},
		{"Jonah 1:17", "Jonah 2:1"},
		{"Jonah 2:10", "Jonah 2:11"},
		{"Gen 31:55", "Gen 32:1"},
		{"Rom 8:28", "Rom 8:28"},
	}
	for _, c := range cases {
		e, _ := Parse(c.eng)
		h, _ := Parse(c.heb)
		got, err := Convert(e.Start, English, Hebrew)
		if err != nil || got != h.Start {
			t.Errorf("%s to Hebrew = %s, want %s", c.eng, got, c.heb)
		}
		back, _ := Convert(h.Start, Hebrew, English)
		if back != e.Start {
			t.Errorf("%s to English = %s, want %s", c.heb, back, c.eng)
		}
	}
	joel2, _ := Parse("Joel 2")
	r, _ := ConvertRange(joel2, English, Hebrew)
	if r.String() != "Joel 2:1-3:5" {
		t.Errorf("English Joel 2 in Hebrew = %s, want Joel 2:1-3:5", r)
	}
	if _, err := Convert(45008028, English, "latin"); err == nil {
		t.Error("unknown system should fail")
	}
}
