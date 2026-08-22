package applock

import (
	"testing"
	"time"
)

// realSheetSample is a real gviz JSON response captured from
// licenseSpreadsheetID's gid=0 tab before writing this package (see
// check.go's doc comment on gvizRow for why v, not the "f" display
// text, is what gets parsed - two real rows here used different date
// display formats despite holding the same underlying date type).
const realSheetSample = `/*O_o*/
google.visualization.Query.setResponse({"version":"0.6","reqId":"0","status":"ok","sig":"1815114732","table":{"cols":[{"id":"A","label":"Tên App","type":"string"},{"id":"B","label":"Thời gian","type":"date","pattern":"yyyy-mm-dd hh:mm:ss"}],"rows":[{"c":[{"v":"Sync chấm công KSNB"},{"v":"Date(2026,7,10)","f":"2026-08-10 00:00:00"}]},{"c":[{"v":"Gửi mail Lương - Hà Thành"},{"v":"Date(2027,0,1)","f":"01/01/2027"}]},{"c":[{"v":"Xử lý đơn hàng"},{"v":"Date(2027,0,1)","f":"01/01/2027"}]}],"parsedNumHeaders":1}});`

func TestParseGvizJSON_ParsesRealSheetSample(t *testing.T) {
	rows, err := parseGvizJSON([]byte(realSheetSample))
	if err != nil {
		t.Fatalf("parseGvizJSON: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
}

func TestFindExpiry_FindsRowByName(t *testing.T) {
	rows, err := parseGvizJSON([]byte(realSheetSample))
	if err != nil {
		t.Fatalf("parseGvizJSON: %v", err)
	}

	expiry, err := findExpiry(rows, "Xử lý đơn hàng")
	if err != nil {
		t.Fatalf("findExpiry: %v", err)
	}
	want := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.Local)
	if !expiry.Equal(want) {
		t.Errorf("got expiry %v, want %v", expiry, want)
	}
}

func TestFindExpiry_DifferentRowDifferentDate(t *testing.T) {
	rows, err := parseGvizJSON([]byte(realSheetSample))
	if err != nil {
		t.Fatalf("parseGvizJSON: %v", err)
	}

	expiry, err := findExpiry(rows, "Sync chấm công KSNB")
	if err != nil {
		t.Fatalf("findExpiry: %v", err)
	}
	want := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.Local)
	if !expiry.Equal(want) {
		t.Errorf("got expiry %v, want %v", expiry, want)
	}
}

func TestFindExpiry_RowNotFound(t *testing.T) {
	rows, err := parseGvizJSON([]byte(realSheetSample))
	if err != nil {
		t.Fatalf("parseGvizJSON: %v", err)
	}

	_, err = findExpiry(rows, "App không tồn tại")
	if err == nil {
		t.Fatal("expected an error for a row name that isn't in the sheet, got nil")
	}
}

func TestParseGvizDate_ParsesDateValue(t *testing.T) {
	got, err := parseGvizDate("Date(2027,0,1)")
	if err != nil {
		t.Fatalf("parseGvizDate: %v", err)
	}
	want := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseGvizDate_RejectsMalformedValue(t *testing.T) {
	if _, err := parseGvizDate("not a date"); err == nil {
		t.Fatal("expected an error for a malformed date value, got nil")
	}
}

func TestEvaluate_LockedWhenNowOnExpiryDate(t *testing.T) {
	rows, _ := parseGvizJSON([]byte(realSheetSample))
	now := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.Local) // exactly the expiry date
	status, err := evaluate(rows, "Xử lý đơn hàng", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !status.Locked {
		t.Error("expected Locked=true when now is exactly on the expiry date, got false")
	}
}

func TestEvaluate_LockedWhenNowAfterExpiryDate(t *testing.T) {
	rows, _ := parseGvizJSON([]byte(realSheetSample))
	now := time.Date(2027, time.June, 15, 0, 0, 0, 0, time.Local)
	status, err := evaluate(rows, "Xử lý đơn hàng", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !status.Locked {
		t.Error("expected Locked=true when now is after the expiry date, got false")
	}
}

func TestEvaluate_NotLockedTheDayBeforeExpiry(t *testing.T) {
	rows, _ := parseGvizJSON([]byte(realSheetSample))
	now := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.Local)
	status, err := evaluate(rows, "Xử lý đơn hàng", now)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if status.Locked {
		t.Error("expected Locked=false the day before the expiry date, got true")
	}
}

func TestEvaluate_RowNotFoundIsAnError(t *testing.T) {
	rows, _ := parseGvizJSON([]byte(realSheetSample))
	_, err := evaluate(rows, "App không tồn tại", time.Now())
	if err == nil {
		t.Fatal("expected an error when the row isn't found, got nil")
	}
}
