package processing

import (
	"testing"

	"order-processor/internal/processing/warehouse"
)

// TestRegionInfo_UsesTheConfiguredWarehouseForEveryBranch pins the wiring
// between warehouse.Branches and the vendor branching functions: every
// configurable slot must actually reach the branch it names.
//
// Each slot is given a value unique to its own key, so a function reading
// the wrong slot (easy to do — most vendors' branches look alike) fails
// with a message naming the slot it actually read.
func TestRegionInfo_UsesTheConfiguredWarehouseForEveryBranch(t *testing.T) {
	saved := map[string]string{}
	for _, b := range warehouse.Branches {
		saved[b.Key] = "KHO-" + b.Key
	}
	r := warehouse.NewResolver(saved)
	only := func(_, _, w string) string { return w }

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"chung MB", only(regionInfo("MB_COOP001", r)), "KHO-chung/MB"},
		{"chung khac", only(regionInfo("MN_MT_COOP001", r)), "KHO-chung/khac"},

		{"bigc MB", only(bigcRegionInfo("MB_BIGCAC", r)), "KHO-bigc/MB"},
		{"bigc MN_MT", only(bigcRegionInfo("MN_MT_BIGCAC", r)), "KHO-bigc/MN_MT"},
		{"bigc MN_GC", only(bigcRegionInfo("MN_GC_BIGCAC", r)), "KHO-bigc/MN_GC"},
		// The unreachable default branch deliberately mirrors MN_GC.
		{"bigc default", only(bigcRegionInfo("KHONG_KHOP", r)), "KHO-bigc/MN_GC"},

		{"winmart Da Nang", only(winmartRegionInfo("MN_MT_WIN1326", r)), "KHO-winmart/MN_MT_WIN1326"},
		{"winmart MB", only(winmartRegionInfo("MB_WIN0001", r)), "KHO-winmart/MB"},
		{"winmart khac", only(winmartRegionInfo("MN_MT_WIN0001", r)), "KHO-winmart/khac"},

		{"emart MB", only(emartRegionInfo("MB_EMART1", r)), "KHO-emart/MB"},
		{"emart khac", only(emartRegionInfo("MN_EMART1", r)), "KHO-emart/khac"},

		{"fujimart MB", only(fujimartRegionInfo("MB_FUJI1", r)), "KHO-fujimart/MB"},
		{"fujimart khac", only(fujimartRegionInfo("MN_FUJI1", r)), "KHO-fujimart/khac"},

		{"kingfood MB", only(kingfoodRegionInfo("MB_KF0001", r)), "KHO-kingfood/MB"},
		{"kingfood khac", only(kingfoodRegionInfo(kingfoodCustomerCode, r)), "KHO-kingfood/khac"},

		// JMart gets its own slot: a different vendor, and its own
		// hardcoded customer code is the only input it ever sees.
		{"jmart", only(jmartRegionInfo(r)), "KHO-jmart"},

		{"jit WH6_HN", only(jitRegionInfo("WH6_HN", r)), "KHO-jit/MB"},
		{"jit WH6_HTLA", only(jitRegionInfo("WH6_HTLA", r)), "KHO-jit/MB"},
		{"jit khac", only(jitRegionInfo("WH6_LA", r)), "KHO-jit/khac"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: warehouse = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestRegionInfo_WithoutSettingsKeepsTheShippedCodes is the other half:
// an app with nothing configured — and every existing test, which builds
// processors with no resolver at all — must still write exactly the codes
// this app has always written.
func TestRegionInfo_WithoutSettingsKeepsTheShippedCodes(t *testing.T) {
	var r *warehouse.Resolver
	only := func(_, _, w string) string { return w }

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"chung MB", only(regionInfo("MB_COOP001", r)), "TP_HN_12"},
		{"chung khac", only(regionInfo("MN_MT_COOP001", r)), "LA_TP"},
		{"bigc MN_MT", only(bigcRegionInfo("MN_MT_BIGCAC", r)), "LA_KHO2026"},
		{"bigc MN_GC", only(bigcRegionInfo("MN_GC_BIGCAC", r)), "LA_TP"},
		{"winmart Da Nang", only(winmartRegionInfo("MN_MT_WIN1326", r)), "TP_DN_1"},
		{"winmart khac", only(winmartRegionInfo("MN_MT_WIN0001", r)), "LA_KHO2026"},
		{"emart khac", only(emartRegionInfo("MN_EMART1", r)), "LA_KHO2026"},
		{"fujimart khac", only(fujimartRegionInfo("MN_FUJI1", r)), "LA_KHO2026"},
		{"kingfood khac", only(kingfoodRegionInfo(kingfoodCustomerCode, r)), "LA_KHO2026"},
		{"jmart", only(jmartRegionInfo(r)), "LA_TP"},
		{"jit MB", only(jitRegionInfo("WH6_HN", r)), "TP_HN_12"},
		{"jit khac", only(jitRegionInfo("WH6_LA", r)), "LA_KHOTMDT"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: warehouse = %q, want the shipped default %q", c.name, c.got, c.want)
		}
	}
}
