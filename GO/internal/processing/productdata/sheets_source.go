package productdata

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

// productDataSpreadsheetID identifies the live Google Sheet holding this
// project's MaKH (customer) and SanPham (product) data — a DIFFERENT
// spreadsheet from pricing.HTTPSource's own hardcoded spreadsheet ID
// (that one holds per-vendor price/promotion sheets; this one holds the
// data data.xlsx used to be the sole source of). Mirrors how the pricing
// package hardcodes its own spreadsheet ID as a package-level constant —
// the gid of each individual TAB within it is still configurable via
// settings.ini (see LoadFromSheets), only the spreadsheet itself is
// fixed in code.
const productDataSpreadsheetID = "14nGamUI8fhPiVPNm-Nf_VyHK-8Cx8rvG3KQFFT0spe0"

// LoadFromSheets builds a Store from the live Google Sheet, replacing
// the local data.xlsx file as the production source — so that updating
// customer/product data takes effect on every machine running this app
// without needing to distribute an updated file, the same reasoning
// that already applies to pricing.HTTPSource's own live fetch. gidMap
// is a snapshot read once at app startup by the caller (see
// appsettings.Store) — must contain "MAKH" and "SANPHAM" keys naming
// each tab's gid within productDataSpreadsheetID.
//
// There is deliberately no offline fallback to a local file: if the
// network is unreachable, this returns an error and the caller (NewApp)
// fails to start, exactly like the local data.xlsx path already did on
// a read failure — a conscious choice discussed with the project owner,
// matching this app's existing full dependency on the same network for
// live pricing/promotion lookups mid-processing (see pricing.HTTPSource),
// rather than adding asymmetric resilience only for this one data
// source.
func LoadFromSheets(gidMap map[string]string, client *http.Client) (*Store, error) {
	customerRows, err := fetchSheetRows(client, gidMap, "MAKH")
	if err != nil {
		return nil, err
	}
	productRows, err := fetchSheetRows(client, gidMap, "SANPHAM")
	if err != nil {
		return nil, err
	}

	return newStore(customerRows, productRows), nil
}

// fetchSheetRows fetches one tab (identified by sheetKey's gid in
// gidMap) of productDataSpreadsheetID as CSV — mirroring
// pricing.HTTPSource.FetchIndex's own URL construction and CSV parsing
// exactly, just against a different spreadsheet ID and consumed as raw
// [][]string rows (loadCustomerRows/loadProducts) instead of a
// *pricing.Index.
func fetchSheetRows(client *http.Client, gidMap map[string]string, sheetKey string) ([][]string, error) {
	gid, ok := gidMap[sheetKey]
	if !ok {
		return nil, fmt.Errorf("productdata: no %s gid configured", sheetKey)
	}

	url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", productDataSpreadsheetID, gid)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("productdata: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("productdata: fetch %s: HTTP %d", url, resp.StatusCode)
	}

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1 // rows can have varying column counts
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("productdata: parse CSV from %s: %w", url, err)
	}
	return rows, nil
}

// NewHTTPClient returns the *http.Client LoadFromSheets should be called
// with in production — a bare 30s-timeout client, matching
// pricing.NewHTTPSource's own default exactly (no cookies/retries/etc.
// needed for a plain CSV GET).
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
