package licence

import "testing"

func TestDefaults(t *testing.T) {
	pd, _ := Defaults(PublicDomain, "")
	if !pd.Commercial || !pd.Share || pd.Print != nil || pd.NeedsAttribution() {
		t.Fatalf("public domain defaults wrong: %+v", pd)
	}
	nc, _ := Defaults(CreativeCommons, "CC-BY-NC-4.0")
	if nc.Commercial || !nc.Share || !nc.NeedsAttribution() {
		t.Fatalf("CC BY-NC defaults wrong: %+v", nc)
	}
	cc0, _ := Defaults(CreativeCommons, "CC0-1.0")
	if cc0.NeedsAttribution() || !cc0.Commercial {
		t.Fatalf("CC0 defaults wrong: %+v", cc0)
	}
	bought, _ := Defaults(PersonalPurchase, "")
	if bought.Share || bought.Commercial || bought.Print == nil || bought.Print.Percent != 10 {
		t.Fatalf("purchase defaults wrong: %+v", bought)
	}
	bought.Print.Percent = 50
	if bought.Export.Percent != 10 {
		t.Fatal("print and export limits share storage")
	}
	if _, err := Defaults("mystery", ""); err == nil {
		t.Fatal("unknown type accepted")
	}
	if _, err := Defaults(CreativeCommons, "MIT"); err == nil {
		t.Fatal("non-CC id accepted for creative_commons")
	}
}

func TestCheck(t *testing.T) {
	bought, _ := Defaults(PersonalPurchase, "")
	bought.Confirmed = true
	d := bought.Check(Request{Action: Print, Pages: 30, TotalPages: 300})
	if !d.Allowed || d.MaxPages != 30 || d.Confirm {
		t.Fatalf("30 of 300 pages at 10%%: %+v", d)
	}
	if d := bought.Check(Request{Action: Export, Pages: 31, TotalPages: 300}); d.Allowed {
		t.Fatal("31 of 300 pages allowed at 10%")
	}
	if d := bought.Check(Request{Action: Export, Pages: 1}); d.Allowed {
		t.Fatal("percentage limit with unknown length allowed")
	}
	if d := bought.Check(Request{Action: Share}); d.Allowed {
		t.Fatal("sharing a purchased book allowed")
	}
	bought.Print = &Limit{Pages: 100, Percent: 50}
	if d := bought.Check(Request{Action: Print, Pages: 100, TotalPages: 1000}); !d.Allowed || d.MaxPages != 100 {
		t.Fatalf("smaller of pages and percent not used: %+v", d)
	}

	nc, _ := Defaults(CreativeCommons, "CC-BY-NC-SA-4.0")
	nc.RightsHolder = "Open Bible Society"
	if d := nc.Check(Request{Action: Print, Pages: 999, Commercial: true}); d.Allowed {
		t.Fatal("commercial print of NC work allowed")
	}
	d = nc.Check(Request{Action: Print, Pages: 999})
	if !d.Allowed || d.Attribution != "© Open Bible Society, licensed under CC-BY-NC-SA-4.0" || !d.Confirm {
		t.Fatalf("NC print: %+v", d)
	}
}
