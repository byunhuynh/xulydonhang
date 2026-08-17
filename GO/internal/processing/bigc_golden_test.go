package processing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

// knownDivergences_BigC lists (fixture, row index, column) cells where
// this Go port intentionally computes a different, verified-more-correct
// (or verified-immaterial) value than the frozen Python fixture. Key
// format: "<source_pdf>:<row index>:<column>". Every entry below falls
// into one of two evidence-backed categories (see task-8-report.md for
// the full investigation):
//
//  1. "per-store weight-suffix quirk" (348 entries, column L only):
//     write_to_dondathang_bigc has an unconditional
//     `sheet[f"L{start_row}"] = ...` (xulydonhang.py:4850) on EVERY
//     store-page call. For the file's FIRST successfully-processed
//     store this correctly lands on the header row (later corrected
//     with the true file-wide aggregate weight via a separate overwrite
//     at xulydonhang.py:4893, which only fires on the last page's
//     call). For every OTHER store in the same file, this same
//     unconditional write instead lands on THAT STORE's own first
//     item row (since start_row is recomputed fresh, sheet.max_row+1,
//     at the top of every call) — overwriting plain "BIGC PO<number>"
//     item text with a weight-suffixed value carrying only that
//     store's own small local weight, not the true aggregate. Go's
//     processBigcDocument (Task 6) deliberately does NOT replicate
//     this per-call artifact: it accumulates every successful store's
//     rows in memory and writes them in ONE combined WriteOrderRows
//     call with a correctly pre-computed aggregate weight in the true
//     header row (row 0) only — a documented, already-reviewed
//     architectural decision, not something to "fix" by reintroducing
//     Python's incidental noise into every other store's first row.
//
//  2. "int-vs-float trailing '.0' formatting" (13 entries, row 0/col L
//     only): format_weight_kg (xulydonhang.py:889-898) does
//     `f"{round(value, 2)} kg"`. Python's `round()` preserves int-ness:
//     if every weight*qty term that fed the final aggregate happened to
//     stay Python-int-typed all the way through (both the per-product
//     weight cell in data.xlsx AND qty_ord_pcs being whole numbers),
//     the aggregate is an int and round(int, 2) is still an int, so the
//     formatted string has no decimal point ("920 kg"). If ANY
//     contributing term was float-typed, the aggregate is a float and
//     Python's float formatting always shows a decimal ("920.0 kg").
//     This is confirmed by the frozen fixture JSON itself: the affected
//     fixtures' AT columns for every contributing row serialize as bare
//     integers (e.g. `40`, not `40.0`), which the Python json module
//     only produces for actual Python ints. Go's weightKg accumulator
//     is a single static float64 throughout (the simpler, already-
//     established architecture also used for Coop/Satra/Lotte, whose
//     trimFloat() deliberately always shows one decimal digit — proven
//     correct for those vendors' fixtures), so it cannot reproduce
//     Python's incidental, data-dependent int/float coercion. The
//     NUMERIC weight total is identical in every case (e.g. 920.0 ==
//     920); only the cosmetic trailing ".0" differs.
//
// There used to be a third category here ("live-CSV-snapshot newline
// skew", 71 entries, column AQ) allowlisting AQ mismatches on a claim
// that they came from two independent live Google Sheets CSV fetches
// captured at different times. A code review (2026-08-17) disproved
// that stated cause: all 71 were reproduced DETERMINISTICALLY from the
// single frozen _frozen_pricing.json snapshot alone, correlating 71/71
// with cells whose frozen value contains a literal '\r' (0/542 on
// non-mismatching cells). The actual mechanism is an openpyxl
// round-trip quirk in Task 7's Python fixture-generation harness:
// openpyxl writes a bare CR into the xlsx XML, which is silently
// normalized on read ('\r' -> '\n', '\r\n' -> '\n\n' — independently,
// not collapsed to a single '\n'). Go's excelize instead escapes '\r'
// as "&#xD;", which survives that normalization untouched, so Go's
// raw AQ output diverged from Python's fixture-captured (and
// incidentally mangled) AQ output. Fixed at the source instead of
// allowlisted: bigc_processor.go now normalizes '\r' -> '\n' in
// khuyenmai right where it's set from promo.Value, reproducing
// Python's effective (if incidentally mangled) output exactly. All 71
// entries were verified to no longer mismatch and were removed.
var knownDivergences_BigC = map[string]bool{
	"2631057733376.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:27:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:28:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:32:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:36:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:38:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:44:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:45:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:53:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:57:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:58:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:60:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:64:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:65:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:71:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:73:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733376.pdf:76:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733450.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2631057733450.pdf:12:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733450.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733450.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733450.pdf:16:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733450.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733450.pdf:2:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733450.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057733450.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:100:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:107:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:22:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:23:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:35:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:40:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:41:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:47:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:53:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:56:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:64:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:65:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:66:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:70:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:72:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:84:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:86:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:95:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:97:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:98:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774579.pdf:9:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774651.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2631057774651.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774651.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774651.pdf:16:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774651.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774651.pdf:5:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774651.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774651.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774651.pdf:8:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:100:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:108:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:109:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:111:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:112:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:120:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:128:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:129:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:132:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:136:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:1:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:21:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:30:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:31:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:33:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:46:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:50:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:58:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:59:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:60:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:64:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:74:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:75:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:89:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774693.pdf:9:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2631057774853.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:12:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:17:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:2:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:4:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:8:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774853.pdf:9:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2631057774888.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:12:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:16:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:17:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:2:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:4:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:8:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057774888.pdf:9:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:12:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:17:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:23:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:26:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:27:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:30:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:43:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:53:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837378.pdf:8:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837455.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2631057837455.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837455.pdf:12:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837455.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837455.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837455.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837455.pdf:4:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837455.pdf:5:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837455.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:22:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:28:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:29:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:2:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:36:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:37:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:46:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:48:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:54:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:68:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:70:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:74:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:76:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:78:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:80:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:84:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837522.pdf:89:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2631057837627.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:16:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:17:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:5:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2631057837627.pdf:9:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860744.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860744.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860744.pdf:20:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860744.pdf:28:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860744.pdf:32:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860744.pdf:36:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860744.pdf:41:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860744.pdf:46:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860774.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2632057860774.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860774.pdf:5:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860774.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860774.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057860774.pdf:8:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:12:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:24:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:27:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:31:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:32:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:38:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:39:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:43:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:45:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:58:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:62:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890077.pdf:70:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2632057890159.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:12:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:19:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:5:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057890159.pdf:8:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:16:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:17:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:19:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:1:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:23:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:25:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:30:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:34:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:40:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:47:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:49:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:4:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:53:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:62:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:66:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:69:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:73:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:79:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:85:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938612.pdf:93:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2632057938678.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:16:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:17:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:19:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:5:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938678.pdf:9:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:100:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:108:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:109:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:113:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:122:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:130:L": true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:19:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:30:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:39:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:45:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:47:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:51:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:52:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:55:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:61:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:62:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:71:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:73:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:78:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:79:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:80:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:89:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938704.pdf:99:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:21:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:22:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:24:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:27:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:28:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:29:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:5:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632057938756.pdf:8:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001889.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001889.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001889.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001889.pdf:20:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001889.pdf:22:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001889.pdf:24:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001889.pdf:26:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001889.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001889.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001972.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2632058001972.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001972.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001972.pdf:15:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001972.pdf:2:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001972.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001972.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001972.pdf:8:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:11:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:23:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:25:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:44:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:56:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:68:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:72:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:73:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:74:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:77:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:79:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:81:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:83:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:87:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:91:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058001987.pdf:93:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2632058002053.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:12:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:13:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:17:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:2:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:5:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2632058002053.pdf:9:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028372.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028372.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028372.pdf:16:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028372.pdf:20:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028372.pdf:25:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028372.pdf:27:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028372.pdf:5:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028372.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028409.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2633058028409.pdf:2:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028409.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058028409.pdf:6:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:12:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:16:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:21:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:23:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:26:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:27:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:28:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:29:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:40:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:42:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:44:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:46:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:53:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:57:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058059999.pdf:7:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058060149.pdf:0:L":   true, // int-vs-float trailing ".0" formatting (xulydonhang.py:889-898)
	"2633058060149.pdf:10:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058060149.pdf:14:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058060149.pdf:17:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058060149.pdf:18:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058060149.pdf:19:L":  true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058060149.pdf:3:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058060149.pdf:4:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
	"2633058060149.pdf:8:L":   true, // per-store weight-suffix quirk (xulydonhang.py:4850)
}

func loadFrozenBigcPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("bigc/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen BigC pricing fixture found (run Task 7's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen BigC pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_BigC(t *testing.T) {
	fixturePaths, err := filepath.Glob("bigc/testdata/fixtures/*.json")
	if err != nil {
		t.Fatalf("failed globbing fixtures: %v", err)
	}
	var realFixtures []string
	for _, p := range fixturePaths {
		if filepath.Base(p) != "_frozen_pricing.json" {
			realFixtures = append(realFixtures, p)
		}
	}
	if len(realFixtures) == 0 {
		t.Skip("no golden fixtures found (run Task 7's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenBigcPricingSource(t)

	var mismatches []string
	for _, fixturePath := range realFixtures {
		raw, err := os.ReadFile(fixturePath)
		if err != nil {
			t.Fatalf("failed reading %s: %v", fixturePath, err)
		}
		var fixture fixtureData
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("failed parsing %s: %v", fixturePath, err)
		}

		pdfPath := filepath.Join("..", "..", "..", "đơn hàng", "08-2026", fixture.SourcePDF)
		excelPath := filepath.Join(t.TempDir(), "dondathang.xlsx")
		copyFile(t, "excelwriter/testdata/dondathang.xlsx", excelPath)

		rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
		rows, err := rp.Process(context.Background(), pdfPath, 1)
		if err != nil {
			mismatches = append(mismatches, fixture.SourcePDF+": Process returned error: "+err.Error())
			continue
		}
		if len(rows) == 0 {
			mismatches = append(mismatches, fixture.SourcePDF+": Process produced no rows")
			continue
		}
		for _, row := range rows {
			if row.StatusKind == StatusKindFailed {
				mismatches = append(mismatches, fixture.SourcePDF+": Process produced a Failed row: "+row.Status)
			}
		}

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_BigC)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
