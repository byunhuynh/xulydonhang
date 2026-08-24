package processing

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"order-processor/internal/processing/productdata"
)

func TestJITAirWaybillAllSamplesHaveCompleteOrdersAndMappedProducts(t *testing.T) {
	totalQtyPattern := regexp.MustCompile(`TổngSLsảnphẩm:(\d+)`)
	store, err := productdata.Load(filepath.Join("..", "..", "..", "data.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "đơn hàng", "air_waybill_*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("found %d air-waybill samples, want 4", len(files))
	}

	pages := 0
	productLines := 0
	var failures []string
	for _, path := range files {
		if _, _, ok := parseJITAirWaybillFilename(path); !ok {
			failures = append(failures, filepath.Base(path)+": filename not recognized")
			continue
		}
		texts, extractErr := extractJITAirWaybillPageTexts(path)
		if extractErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", filepath.Base(path), extractErr))
			continue
		}
		pages += len(texts)
		for pageIdx, text := range texts {
			tracking, po, products, parseErr := parseJITAirWaybillPage(text)
			if parseErr != nil {
				compact := jitWhitespacePattern.ReplaceAllString(text, "")
				if len(compact) > 220 {
					compact = compact[:220]
				}
				failures = append(failures, fmt.Sprintf("%s page %d: %v; head=%q", filepath.Base(path), pageIdx+1, parseErr, compact))
				continue
			}
			if tracking == "" || po == "" {
				failures = append(failures, fmt.Sprintf("%s page %d: empty tracking/PO", filepath.Base(path), pageIdx+1))
			}
			productLines += len(products)
			compact := jitWhitespacePattern.ReplaceAllString(text, "")
			qtyMatch := totalQtyPattern.FindStringSubmatch(compact)
			if qtyMatch == nil {
				failures = append(failures, fmt.Sprintf("%s page %d: missing printed total quantity", filepath.Base(path), pageIdx+1))
			} else {
				printedQty, _ := strconv.Atoi(qtyMatch[1])
				parsedQty := 0
				for _, product := range products {
					parsedQty += int(product.Qty)
				}
				if parsedQty != printedQty {
					failures = append(failures, fmt.Sprintf("%s page %d: parsed qty %d, printed total %d", filepath.Base(path), pageIdx+1, parsedQty, printedQty))
				}
			}
			for _, product := range products {
				if _, ok := resolveJITProductSku(store, product.Barcode); !ok {
					failures = append(failures, fmt.Sprintf("%s page %d: unmapped product %q", filepath.Base(path), pageIdx+1, product.Barcode))
				}
			}
		}
	}
	if len(failures) > 0 {
		limit := len(failures)
		if limit > 20 {
			limit = 20
		}
		t.Fatalf("audit failed with %d issues (first %d):\n%v", len(failures), limit, failures[:limit])
	}
	if pages != 525 {
		t.Fatalf("audited %d pages, want 525", pages)
	}
	if productLines != 536 {
		t.Fatalf("audited %d product lines, want 536", productLines)
	}
	t.Logf("audited %d pages and %d product lines", pages, productLines)
}
