// GO/internal/processing/pricing/http_source.go
package pricing

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

const spreadsheetID = "1yvxE_SPYXKhofcZdhv1CSKAyiwdY1Mf4pFlsiMbtOr4"

// HTTPSource fetches a vendor's live pricing/promotion sheet over HTTP.
// Every vendor's sheet lives in the same Google Sheet (spreadsheetID),
// on a different tab selected by gid — confirmed in xulydonhang.py's
// find_price_by_sku/find_all_promotions_by_sku_and_time family, which
// all hardcode the same sheet_id and vary only the sheet_name param used
// to resolve gid. It is the production PricingSource; tests substitute a
// fixture-backed implementation instead of hitting the network.
//
// GidMap is a snapshot read once at app startup (see appsettings.Store)
// rather than a file path re-read on every FetchIndex call — the
// previous version re-read settings.ini from disk on every single call,
// which was always wasted work (the file never changes mid-batch).
// Changing a gid via the in-app settings popup takes effect on the next
// app restart, not live mid-session — a deliberate simplicity choice.
type HTTPSource struct {
	GidMap map[string]string
	Client *http.Client
}

func NewHTTPSource(gidMap map[string]string) *HTTPSource {
	return &HTTPSource{GidMap: gidMap, Client: &http.Client{Timeout: 30 * time.Second}}
}

// FetchIndex mirrors find_price_by_sku/find_all_promotions_by_sku_and_time's
// URL construction: sheetKey is the same value as their sheet_name
// parameter (e.g. "COOP", "LOTTE" — must match a key in GidMap).
//
// Uses /export?format=csv rather than /gviz/tq?tqx=out:csv: the gviz
// endpoint was confirmed (see productdata.fetchSheetRows' own doc
// comment) to silently corrupt another tab of a *different* spreadsheet,
// concatenating hundreds of real rows into the CSV header and losing
// them to code that (correctly) treats row 0 as a header. This
// spreadsheet's own vendor tabs (COOP/SATRA/LOTTE/WINMART checked
// directly) were NOT actually losing any SKU rows under gviz — the two
// endpoints returned identical SKU sets, gviz just omits a few blank
// filler rows that ParseIndex already ignores — but switched anyway as
// a precaution against the same silent-data-loss failure mode, since
// /export?format=csv is the more standard, non-gviz export path and
// costs nothing here.
func (s *HTTPSource) FetchIndex(sheetKey string) (*Index, error) {
	gid, ok := s.GidMap[sheetKey]
	if !ok {
		return nil, fmt.Errorf("pricing: no %s gid configured", sheetKey)
	}

	url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv&gid=%s", spreadsheetID, gid)
	resp, err := s.Client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("pricing: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricing: fetch %s: HTTP %d", url, resp.StatusCode)
	}

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1 // rows can have varying column counts
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("pricing: parse CSV from %s: %w", url, err)
	}

	return ParseIndex(rows), nil
}
