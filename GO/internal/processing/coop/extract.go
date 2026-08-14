package coop

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Product is one extracted line item, mirroring extract_products'
// {"Barcode", "Qty Ord/Pcs", "Extended Cost"} dict.
type Product struct {
	Barcode string
	Qty     float64
	Cost    float64
}

var (
	subTotalSplitPattern = regexp.MustCompile(`(?i)` + spacedPattern("SubTotal"))
	vndSplitPattern      = regexp.MustCompile(`(?i)` + spacedPattern("VND Viet Nam Dong"))
	skuLinePattern       = regexp.MustCompile(`\d{7}-\s*\d`)
	decimalNumberPattern = regexp.MustCompile(`\d[\d,]*\.\d+`)
)

const (
	minSanePrice = 1000.0
	maxSanePrice = 2000000.0
)

// ExtractProducts mirrors xulydonhang.py's extract_products: an
// empirically-tuned heuristic that finds Coop SKU anchors (7 digits,
// dash, 1 digit) and guesses which numbers in the text block between
// two anchors are the quantity and the extended cost, based on block
// position (last block vs. normal) and how many comma-formatted
// ("large") numbers are present. This is not a clean grammar — it
// reproduces the original's exact branch structure on purpose. See
// this task's design note for the one behavioral difference (errors
// instead of silently producing a product with a missing qty/cost).
func ExtractProducts(text string) ([]Product, error) {
	if loc := subTotalSplitPattern.FindStringIndex(text); loc != nil {
		text = text[:loc[0]]
	}
	if locs := vndSplitPattern.FindAllStringIndex(text, -1); len(locs) > 0 {
		last := locs[len(locs)-1]
		text = text[last[1]:]
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	var skuIndices []int
	for i, line := range lines {
		if skuLinePattern.MatchString(line) {
			skuIndices = append(skuIndices, i)
		}
	}

	var products []Product
	for i, start := range skuIndices {
		end := len(lines)
		if i+1 < len(skuIndices) {
			end = skuIndices[i+1]
		}
		block := make([]string, end-start)
		for j, line := range lines[start:end] {
			line = strings.ReplaceAll(line, ", ", ",")
			line = strings.ReplaceAll(line, ". ", ".")
			block[j] = line
		}

		barcode := ""
		if m := skuLinePattern.FindString(block[0]); m != "" {
			barcode = strings.ReplaceAll(m, " ", "")
		}
		if barcode == "" {
			continue
		}

		joined := strings.Join(block, " ")
		nums := findDecimalNumbers(joined)
		var large []string
		for _, n := range nums {
			if strings.Contains(n, ",") {
				large = append(large, n)
			}
		}

		var qtyStr, costStr string
		var ok bool
		if i == len(skuIndices)-1 {
			qtyStr, costStr, ok = selectLastBlockQtyCost(nums, large)
		} else {
			qtyStr, costStr, ok = selectNormalBlockQtyCost(nums, large)
		}
		if !ok {
			return nil, fmt.Errorf("không xác định được số lượng/đơn giá cho mã hàng %s", barcode)
		}

		qty, err := strconv.ParseFloat(strings.ReplaceAll(qtyStr, ",", ""), 64)
		if err != nil {
			return nil, fmt.Errorf("số lượng không hợp lệ cho mã hàng %s: %q", barcode, qtyStr)
		}
		cost, err := strconv.ParseFloat(strings.ReplaceAll(costStr, ",", ""), 64)
		if err != nil {
			return nil, fmt.Errorf("đơn giá không hợp lệ cho mã hàng %s: %q", barcode, costStr)
		}

		products = append(products, Product{Barcode: barcode, Qty: qty, Cost: cost})
	}

	return products, nil
}

// findDecimalNumbers mirrors the regex
// `(?<![a-zA-Z])\d[\d,]*\.\d+(?![a-zA-Z])` — RE2 has no lookaround, so
// the boundary check is manual: keep a match only if the byte
// immediately before/after it (if any) is not an ASCII letter.
func findDecimalNumbers(text string) []string {
	indices := decimalNumberPattern.FindAllStringIndex(text, -1)
	var out []string
	for _, idx := range indices {
		start, end := idx[0], idx[1]
		if start > 0 && isASCIILetterByte(text[start-1]) {
			continue
		}
		if end < len(text) && isASCIILetterByte(text[end]) {
			continue
		}
		out = append(out, text[start:end])
	}
	return out
}

func isASCIILetterByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func selectLastBlockQtyCost(nums, large []string) (qty, cost string, ok bool) {
	if len(large) > 0 {
		type candidate struct {
			str string
			val float64
		}
		candidates := make([]candidate, len(large))
		for i, s := range large {
			v, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
			candidates[i] = candidate{s, v}
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].val < candidates[j].val })

		for _, c := range candidates {
			costIdx := indexOf(nums, c.str)
			if costIdx < 0 {
				continue
			}
			if costIdx > 0 {
				if up, ok2 := impliedUnitPrice(c.str, nums[costIdx-1]); ok2 && isSanePrice(up) {
					return nums[costIdx-1], c.str, true
				}
			}
			if costIdx > 1 {
				if up, ok2 := impliedUnitPrice(c.str, nums[costIdx-2]); ok2 && isSanePrice(up) {
					return nums[costIdx-2], c.str, true
				}
			}
		}
	}
	if len(nums) >= 2 {
		return nums[len(nums)-2], nums[len(nums)-1], true
	}
	return "", "", false
}

func selectNormalBlockQtyCost(nums, large []string) (qty, cost string, ok bool) {
	if len(large) >= 2 {
		costStr := nums[len(nums)-1]
		idx0 := indexOf(nums, large[0])
		if idx0 > 0 {
			return nums[idx0-1], costStr, true
		}
		return "", "", false
	}
	if len(nums) >= 2 {
		return nums[len(nums)-2], nums[len(nums)-1], true
	}
	return "", "", false
}

func indexOf(items []string, target string) int {
	for i, v := range items {
		if v == target {
			return i
		}
	}
	return -1
}

func impliedUnitPrice(costStr, qtyStr string) (float64, bool) {
	cost, err1 := strconv.ParseFloat(strings.ReplaceAll(costStr, ",", ""), 64)
	qty, err2 := strconv.ParseFloat(strings.ReplaceAll(qtyStr, ",", ""), 64)
	if err1 != nil || err2 != nil || qty == 0 {
		return 0, false
	}
	return cost / qty, true
}

func isSanePrice(unitPrice float64) bool {
	return unitPrice > minSanePrice && unitPrice < maxSanePrice
}
