// Package applock checks a live Google Sheet for this app's licensed
// expiry date, so access can be revoked (or extended) by editing one
// cell in a shared admin spreadsheet, without shipping a new build.
// Replaces App.py's old remote lock (a Google Apps Script endpoint
// returning {"active": 0|1}) — that mechanism was never ported to this
// Go rewrite; this is a new, independent design, not a port.
package applock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// licenseSpreadsheetID is a shared admin sheet listing several internal
// apps and their expiry dates, one row each (columns: app name, expiry
// date) — a DIFFERENT spreadsheet from pricing's and productdata's own,
// unrelated to gid/settings configuration.
const licenseSpreadsheetID = "1_oE0sXcarp1z7u-mDfAzx_gm4HMWj55pYucDiCzaCTk"

// licenseGid is the tab within licenseSpreadsheetID holding the app list
// — confirmed empirically (gid=0, the sheet's first/only tab) before
// writing this package, not assumed.
const licenseGid = "0"

// appRowName is this app's exact row label in the sheet's name column —
// confirmed empirically to match a real row ("Xử lý đơn hàng") rather
// than assuming a fixed cell address, so the row can move if other rows
// are added/removed/reordered in the shared sheet.
const appRowName = "Xử lý đơn hàng"

// Status is the result of one license check.
type Status struct {
	// Locked is true once today's date is on or after the sheet's expiry
	// date for this app's row (locked FROM the expiry date onward, not
	// strictly after it).
	Locked bool
	// Expiry is the parsed expiry date read from the sheet.
	Expiry time.Time
}

// Check fetches licenseSpreadsheetID, finds appRowName's expiry date,
// and compares it against now. Returns an error for any fetch/parse
// failure (network unreachable, row not found, malformed date) — the
// caller (app.go) treats a returned error as "status undetermined", a
// deliberately different state from Status.Locked == true (a
// definitively expired license), so a network hiccup doesn't get
// reported to the user as "your license expired".
func Check(client *http.Client, now time.Time) (Status, error) {
	rows, err := fetchRows(client)
	if err != nil {
		return Status{}, err
	}
	return evaluate(rows, appRowName, now)
}

// evaluate is Check's fetch-free core: find appRowName's expiry date in
// rows and compare it against now. Split out from Check so the
// locked/not-locked boundary logic is unit-testable without a live
// network fetch.
func evaluate(rows []gvizRow, rowName string, now time.Time) (Status, error) {
	expiry, err := findExpiry(rows, rowName)
	if err != nil {
		return Status{}, err
	}
	return Status{
		Locked: !now.Before(expiry),
		Expiry: expiry,
	}, nil
}

// gvizRow mirrors the relevant shape of one row in a Google Sheets gviz
// JSON response: each cell's raw value in Cells[i].V. A date-typed
// cell's V is always the literal string "Date(y,m,d[,h,m,s,ms])"
// (JS Date-constructor style, month 0-indexed) REGARDLESS of that
// cell's own display format — deliberately parsed from V, not the
// human-readable "f" field, because two rows in the same real column
// were observed with different display formats ("01/01/2027" vs
// "2026-08-10 00:00:00") despite holding the same underlying type; V is
// the one representation that's actually stable to parse.
type gvizRow struct {
	Cells []struct {
		V json.RawMessage `json:"v"`
	} `json:"c"`
}

type gvizResponse struct {
	Table struct {
		Rows []gvizRow `json:"rows"`
	} `json:"table"`
}

// fetchRows fetches licenseSpreadsheetID's licenseGid tab as gviz JSON
// and unwraps the JSONP-style envelope Google's endpoint wraps it in.
func fetchRows(client *http.Client) ([]gvizRow, error) {
	url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:json&gid=%s", licenseSpreadsheetID, licenseGid)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("applock: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("applock: fetch %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("applock: read response from %s: %w", url, err)
	}

	rows, err := parseGvizJSON(body)
	if err != nil {
		return nil, fmt.Errorf("applock: parse response from %s: %w", url, err)
	}
	return rows, nil
}

// parseGvizJSON strips gviz's JSONP-style wrapper
// (`/*O_o*/\ngoogle.visualization.Query.setResponse({...});`) and
// unmarshals the inner JSON object.
func parseGvizJSON(body []byte) ([]gvizRow, error) {
	start := strings.IndexByte(string(body), '{')
	end := strings.LastIndexByte(string(body), '}')
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object found in gviz response")
	}

	var resp gvizResponse
	if err := json.Unmarshal(body[start:end+1], &resp); err != nil {
		return nil, err
	}
	return resp.Table.Rows, nil
}

var dateValuePattern = regexp.MustCompile(`^Date\((\d+),(\d+),(\d+)`)

// findExpiry scans rows for the first one whose first cell is the
// literal string rowName, and parses its second cell as a gviz date
// value. Scans ALL rows (not skipping a header) since a header row's
// first cell ("Tên App") never equals a real app's name — no special
// header-detection needed.
func findExpiry(rows []gvizRow, rowName string) (time.Time, error) {
	for _, row := range rows {
		if len(row.Cells) < 2 {
			continue
		}
		var name string
		if err := json.Unmarshal(row.Cells[0].V, &name); err != nil {
			continue // not a string cell (e.g. null) - not the row we want
		}
		if name != rowName {
			continue
		}

		var dateStr string
		if err := json.Unmarshal(row.Cells[1].V, &dateStr); err != nil {
			return time.Time{}, fmt.Errorf("applock: row %q has no valid date cell: %w", rowName, err)
		}
		return parseGvizDate(dateStr)
	}
	return time.Time{}, fmt.Errorf("applock: no row named %q found", rowName)
}

// parseGvizDate parses a gviz date cell value, e.g. "Date(2027,0,1)" ->
// 2027-01-01 (gviz months are 0-indexed; time.Month is 1-indexed, hence
// the +1).
func parseGvizDate(s string) (time.Time, error) {
	m := dateValuePattern.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, fmt.Errorf("applock: unrecognized date value %q", s)
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	return time.Date(year, time.Month(month+1), day, 0, 0, 0, 0, time.Local), nil
}

// NewHTTPClient returns the *http.Client Check should be called with in
// production - matches pricing.NewHTTPSource/productdata.NewHTTPClient's
// own default exactly.
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
