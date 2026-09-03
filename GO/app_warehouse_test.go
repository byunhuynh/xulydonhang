package main

import (
	"path/filepath"
	"testing"

	"order-processor/internal/appsettings"
	"order-processor/internal/processing/warehouse"
)

func newWarehouseTestApp(t *testing.T, saved map[string]string) *App {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.bhconfig")
	store := appsettings.NewStore(path)
	if err := store.Save(appsettings.Settings{Warehouse: saved}); err != nil {
		t.Fatalf("Save settings failed: %v", err)
	}
	return &App{appSettingsStore: store}
}

// TestWarehouseOptions_ListsEveryBranchInDeclaredOrder pins what the Cài
// đặt popup renders. Order matters: branches of one vendor have to sit
// together or the table is unreadable, so this must NOT be sorted
// alphabetically the way MisaRouteOptions is.
func TestWarehouseOptions_ListsEveryBranchInDeclaredOrder(t *testing.T) {
	app := newWarehouseTestApp(t, nil)

	options, err := app.WarehouseOptions()
	if err != nil {
		t.Fatalf("WarehouseOptions returned error: %v", err)
	}
	if len(options) != len(warehouse.Branches) {
		t.Fatalf("got %d rows, want %d", len(options), len(warehouse.Branches))
	}
	for i, b := range warehouse.Branches {
		if options[i].Key != b.Key {
			t.Errorf("row %d key = %q, want %q (declared order)", i, options[i].Key, b.Key)
		}
		if options[i].Label != b.Label {
			t.Errorf("row %d label = %q, want %q", i, options[i].Label, b.Label)
		}
		if options[i].Code != b.Default {
			t.Errorf("row %d code = %q, want the default %q", i, options[i].Code, b.Default)
		}
	}
}

// TestWarehouseOptions_ShowsTheSavedCode covers the popup opening on a
// value the user already changed: the saved code, not the shipped one.
func TestWarehouseOptions_ShowsTheSavedCode(t *testing.T) {
	app := newWarehouseTestApp(t, map[string]string{"tmdt/HN": "TP_HN_14"})

	options, err := app.WarehouseOptions()
	if err != nil {
		t.Fatalf("WarehouseOptions returned error: %v", err)
	}
	for _, o := range options {
		switch o.Key {
		case "tmdt/HN":
			if o.Code != "TP_HN_14" {
				t.Errorf("tmdt/HN code = %q, want the saved %q", o.Code, "TP_HN_14")
			}
		case "chung/MB":
			if o.Code != "TP_HN_12" {
				t.Errorf("chung/MB code = %q, want the untouched default %q", o.Code, "TP_HN_12")
			}
		}
	}
}
