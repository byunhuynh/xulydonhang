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
type HTTPSource struct {
	SettingsPath string
	Client       *http.Client
}

func NewHTTPSource(settingsPath string) *HTTPSource {
	return &HTTPSource{SettingsPath: settingsPath, Client: &http.Client{Timeout: 30 * time.Second}}
}

// FetchIndex mirrors find_price_by_sku/find_all_promotions_by_sku_and_time's
// URL construction: sheetKey is the same value as their sheet_name
// parameter (e.g. "COOP", "LOTTE" — must match a key in settings.ini's
// <gid> block), resolved to a gid and fetched once.
func (s *HTTPSource) FetchIndex(sheetKey string) (*Index, error) {
	gidMap, err := LoadGidMap(s.SettingsPath)
	if err != nil {
		return nil, err
	}
	gid, ok := gidMap[sheetKey]
	if !ok {
		return nil, fmt.Errorf("pricing: no %s gid in %s", sheetKey, s.SettingsPath)
	}

	url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", spreadsheetID, gid)
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
