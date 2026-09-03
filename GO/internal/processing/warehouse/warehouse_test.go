package warehouse

import "testing"

func TestBranches_CoverEveryVendorSlotExactlyOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range Branches {
		if b.Key == "" || b.Label == "" || b.Default == "" {
			t.Errorf("branch %+v has an empty field; every slot needs a key, a label and a built-in default", b)
		}
		if seen[b.Key] {
			t.Errorf("duplicate branch key %q — keys address one slot each", b.Key)
		}
		seen[b.Key] = true
	}
	// The 19 slots the vendor processors actually branch on, enumerated
	// when this was designed. A vendor gaining a branch must add its key
	// here too, or its warehouse silently stays hardcoded.
	want := []string{
		"chung/MB", "chung/khac",
		"bigc/MB", "bigc/MN_MT", "bigc/MN_GC",
		"winmart/MN_MT_WIN1326", "winmart/MB", "winmart/khac",
		"emart/MB", "emart/khac",
		"fujimart/MB", "fujimart/khac",
		"kingfood/MB", "kingfood/khac",
		"jmart",
		"jit/MB", "jit/khac",
		"tmdt/HN", "tmdt/khac",
	}
	if len(Branches) != len(want) {
		t.Errorf("Branches has %d entries, want %d", len(Branches), len(want))
	}
	for _, key := range want {
		if !seen[key] {
			t.Errorf("missing branch key %q", key)
		}
	}
}

func TestResolver_FallsBackToTheBuiltInDefault(t *testing.T) {
	r := NewResolver(nil)
	if got := r.Get("bigc/MN_MT"); got != "LA_KHO2026" {
		t.Errorf("Get(bigc/MN_MT) = %q, want the built-in default %q", got, "LA_KHO2026")
	}
	if got := r.Get("tmdt/HN"); got != "TP_HN_13" {
		t.Errorf("Get(tmdt/HN) = %q, want the built-in default %q", got, "TP_HN_13")
	}
}

func TestResolver_SavedValueWins(t *testing.T) {
	r := NewResolver(map[string]string{"tmdt/HN": "TP_HN_99"})
	if got := r.Get("tmdt/HN"); got != "TP_HN_99" {
		t.Errorf("Get(tmdt/HN) = %q, want the saved %q", got, "TP_HN_99")
	}
	// Overriding one slot must not disturb any other, which is the whole
	// point of splitting per vendor+branch: the 27/08/2026 change moved
	// only TMĐT's Hà Nội warehouse and had to leave retail alone.
	if got := r.Get("chung/MB"); got != "TP_HN_12" {
		t.Errorf("Get(chung/MB) = %q, want the untouched default %q", got, "TP_HN_12")
	}
}

func TestResolver_BlankOrUnknownNeverYieldsAnEmptyWarehouse(t *testing.T) {
	// A blank cell in the settings table means "I cleared it", not "write
	// an empty warehouse into column V" — that would silently produce
	// unusable rows in dondathang.xlsx.
	r := NewResolver(map[string]string{"chung/MB": "   "})
	if got := r.Get("chung/MB"); got != "TP_HN_12" {
		t.Errorf("Get(chung/MB) with a blank override = %q, want the default %q", got, "TP_HN_12")
	}
	// An unknown key is a programming error, not user data; returning ""
	// is right there — it is not a slot anyone can configure.
	if got := r.Get("khong-co-that"); got != "" {
		t.Errorf("Get(unknown) = %q, want empty", got)
	}
}

func TestResolver_TrimsSurroundingSpace(t *testing.T) {
	r := NewResolver(map[string]string{"emart/khac": "  LA_KHO2027 "})
	if got := r.Get("emart/khac"); got != "LA_KHO2027" {
		t.Errorf("Get(emart/khac) = %q, want %q", got, "LA_KHO2027")
	}
}

func TestResolver_NilReceiverStillResolvesDefaults(t *testing.T) {
	// Processors hold this as a plain field; one built without settings
	// (every existing test does) must still get the shipped codes rather
	// than panic.
	var r *Resolver
	if got := r.Get("winmart/khac"); got != "LA_KHO2026" {
		t.Errorf("nil Resolver Get = %q, want the built-in default %q", got, "LA_KHO2026")
	}
}

func TestApplySeed_FillsOnlyMissingKeys(t *testing.T) {
	saved := map[string]string{"tmdt/HN": "TP_HN_99"}

	if !ApplySeed(saved) {
		t.Error("ApplySeed returned false, want true — 18 keys were missing")
	}
	// The whole reason the seed is materialised to disk rather than left
	// in code as a fallback: changing a constant in a later build must
	// not silently move a warehouse the user never touched.
	if saved["tmdt/HN"] != "TP_HN_99" {
		t.Errorf("ApplySeed overwrote a saved value: tmdt/HN = %q, want %q", saved["tmdt/HN"], "TP_HN_99")
	}
	if saved["bigc/MN_MT"] != "LA_KHO2026" {
		t.Errorf("bigc/MN_MT = %q, want the seeded default %q", saved["bigc/MN_MT"], "LA_KHO2026")
	}
	if len(saved) != len(Branches) {
		t.Errorf("seeded map has %d keys, want %d (one per branch)", len(saved), len(Branches))
	}

	if ApplySeed(saved) {
		t.Error("ApplySeed returned true on an already-complete map, want false — that would rewrite the settings file on every launch")
	}
}

func TestApplySeed_LeavesADeliberatelyBlankedValueAlone(t *testing.T) {
	// A key present with an empty value is a user who cleared the box.
	// Re-seeding it would undo that edit on the next launch; Get already
	// falls back to the default for it, so nothing breaks by leaving it.
	saved := map[string]string{"chung/MB": ""}
	ApplySeed(saved)
	if saved["chung/MB"] != "" {
		t.Errorf("chung/MB = %q, want the user's blank left alone", saved["chung/MB"])
	}
}
