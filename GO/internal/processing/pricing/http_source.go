package pricing

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

const spreadsheetID = "1yvxE_SPYXKhofcZdhv1CSKAyiwdY1Mf4pFlsiMbtOr4"

// HTTPSource fetches Coop's live pricing/promotion sheet over HTTP. It
// is the production PricingSource; Task 12's tests substitute a
// fixture-backed implementation instead of hitting the network.
type HTTPSource struct {
	SettingsPath string
	Client       *http.Client
}

func NewHTTPSource(settingsPath string) *HTTPSource {
	return &HTTPSource{SettingsPath: settingsPath, Client: &http.Client{Timeout: 30 * time.Second}}
}

// FetchCoopIndex mirrors find_price_by_sku/find_all_promotions_by_sku_and_time's
// URL construction (both use gid = get_gid("COOP") at their real Coop
// call site) and fetches it once.
func (s *HTTPSource) FetchCoopIndex() (*Index, error) {
	gidMap, err := LoadGidMap(s.SettingsPath)
	if err != nil {
		return nil, err
	}
	gid, ok := gidMap["COOP"]
	if !ok {
		return nil, fmt.Errorf("pricing: no COOP gid in %s", s.SettingsPath)
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
