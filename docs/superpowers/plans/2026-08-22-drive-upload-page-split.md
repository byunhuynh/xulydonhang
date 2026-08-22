# Cắt trang PDF trước khi Upload Drive (per-page, không phải nguyên file) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sửa 1 vấn đề Important tìm thấy ở whole-branch review của plan `2026-08-22-drive-upload.md`: hiện tại, với 1 file PDF gộp nhiều đơn hàng (nhiều trang), mỗi đơn tự upload lại NGUYÊN file gốc lên Drive — vừa lãng phí băng thông/dung lượng, vừa cho ra nhiều link khác nhau cho cùng 1 file. Đổi sang cắt đúng TRANG PDF của đơn đó trước khi upload — khớp đúng cơ chế `cat_trang_hien_tai` của bản Python gốc (đã xác nhận qua đọc code thật: PyMuPDF `dst.insert_pdf(src_doc, from_page=idx, to_page=idx)`, cắt theo TRANG, không cắt theo ranh giới PO trong text).

**Architecture:** Package mới `internal/pdfpage` bọc thư viện `github.com/pdfcpu/pdfcpu` (đã verify thật qua spike: cắt trang, output là PDF hợp lệ, đúng số trang) để cắt 1 trang PDF ra file tạm. `Process()` (dispatcher chung cho 8 vendor không phải BigC) truyền số trang PDF thật (`pageNumbers[pageIdx]`) vào từng hàm xử lý vendor. Mỗi vendor, tại đúng điểm gọi `driveupload.Upload` đã có sẵn (từ plan trước), cắt trang hiện tại ra file tạm rồi upload file tạm đó thay vì `filePath` gốc — dọn file tạm ngay sau khi `Upload()` return (vì `Upload` đã đọc file đồng bộ xong lúc đó). BigC KHÔNG đổi gì — vẫn upload nguyên file như đã có, theo đúng xác nhận của user ("riêng đơn bigc cần toàn bộ").

**Tech Stack:** Go, `github.com/pdfcpu/pdfcpu` (mới thêm — pure Go, Apache 2.0, đang được bảo trì tích cực, đã verify version v0.15.0 hoạt động đúng qua spike thật với fixture Coop/BigC có sẵn trong repo).

**Spec:** Không có spec file riêng — đây là điều chỉnh trực tiếp 1 phần đã được duyệt của `docs/superpowers/specs/2026-08-22-drive-upload-design.md` (tính năng upload Drive đã được duyệt đầy đủ; plan này CHỈ sửa cách lấy nội dung để upload, từ "nguyên file" sang "đúng trang", theo quyết định trực tiếp của user sau khi xem finding của whole-branch review). Toàn bộ quyết định/xác nhận nằm trong hội thoại brainstorming trực tiếp với user, không viết thành spec file riêng vì phạm vi nhỏ, rõ ràng.

## Global Constraints

- BigC (`bigc_processor.go`, hàm `processBigcDocument`) KHÔNG đổi gì trong plan này — giữ nguyên upload nguyên file, đã đúng theo yêu cầu user.
- Cắt theo TRANG PDF (dùng `pageNumbers[pageIdx]`, số trang PDF THẬT, không phải `pageIdx` đã compact) — không cố cắt theo ranh giới PO trong text (`SplitMultiPO`'s text-level splitting là việc khác, không liên quan tới file PDF upload). Nhiều PO chung 1 trang PDF (trường hợp hiếm, Coop) sẽ tự nhiên dùng chung 1 file/link vì cùng cắt ra từ 1 trang — đã được user xác nhận chấp nhận được.
- Nếu cắt trang THẤT BẠI (file PDF lỗi/thư viện lỗi...), KHÔNG được làm order fail — fallback: upload NGUYÊN file gốc như plan trước đã làm (hành vi cũ vẫn là 1 phương án dự phòng hợp lệ, không phải lỗi nghiêm trọng), chỉ log cảnh báo.
- File PDF tạm (kết quả cắt trang) PHẢI được xóa ngay sau khi `driveupload.Upload()` return (không đợi background goroutine — `Upload()` đã đọc xong nội dung file đồng bộ trước khi return, an toàn để xóa ngay).
- Sau mỗi task: `go build ./...`, `go vet ./...`, `go test ./...` phải sạch, khớp baseline đã biết (2 fixture Coop lỗi từ trước, không liên quan).
- Không sửa `internal/driveupload/` package — package đó chỉ nhận `path string` để upload, không cần biết path đó là file gốc hay file đã cắt trang.

---

### Task 1: Package `GO/internal/pdfpage/` (cắt trang PDF)

**Files:**
- Modify: `GO/go.mod`, `GO/go.sum` (thêm dependency `github.com/pdfcpu/pdfcpu`)
- Create: `GO/internal/pdfpage/extract.go`
- Create: `GO/internal/pdfpage/extract_test.go`

**Interfaces:**
- Consumes: không phụ thuộc task nào khác (task đầu tiên).
- Produces: `pdfpage.ExtractPage(sourcePath string, pageNumber int) (tempPath string, cleanup func(), err error)` — Task 2 chỉ cần biết chữ ký này tồn tại (không gọi trực tiếp); Task 3-10 (từng vendor) gọi hàm này trực tiếp.

- [ ] **Step 1: Thêm dependency**

Run: `cd GO && go get github.com/pdfcpu/pdfcpu@v0.15.0 && go mod tidy`

**Lưu ý môi trường**: nếu `go get`/`go mod tidy` báo lỗi timeout/connection failed với proxy mặc định (`proxy.golang.org`), thử lại với `GOPROXY=direct go get github.com/pdfcpu/pdfcpu@v0.15.0` rồi `GOPROXY=direct go mod tidy` — đã xác nhận cách này hoạt động được trong môi trường sandbox lúc viết plan. Trên máy thật của user (có mạng bình thường) nhiều khả năng không cần bước này, `go get` mặc định sẽ chạy được ngay.

Expected: `go.mod` có dòng `github.com/pdfcpu/pdfcpu v0.15.0`, `go.sum` được cập nhật, build vẫn sạch.

- [ ] **Step 2: Viết `extract.go`**

```go
// GO/internal/pdfpage/extract.go

// Package pdfpage extracts a single page from a source PDF into a new,
// standalone single-page PDF file - mirrors xulydonhang.py's
// cat_trang_hien_tai (verified by reading the Python source directly:
// PyMuPDF's `dst.insert_pdf(src_doc, from_page=page_index,
// to_page=page_index)`), real page-level extraction, not a whole-file
// copy and not a sub-page/text-boundary split.
package pdfpage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// ExtractPage extracts pageNumber (1-indexed, matching a PDF's own page
// numbering) from sourcePath into a new temporary single-page PDF file,
// returned as tempPath. The caller MUST call the returned cleanup func
// once done with the file (typically immediately after
// driveupload.Upload's synchronous file read completes, not deferred
// to the end of a long-running function) to remove the temp directory
// created for it.
func ExtractPage(sourcePath string, pageNumber int) (tempPath string, cleanup func(), err error) {
	noop := func() {}

	tempDir, err := os.MkdirTemp("", "driveupload-page-*")
	if err != nil {
		return "", noop, fmt.Errorf("pdfpage: create temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(tempDir) }

	pageArg := fmt.Sprintf("%d", pageNumber)
	if err := api.ExtractPagesFile(sourcePath, tempDir, []string{pageArg}, nil); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("pdfpage: extract page %d from %s: %w", pageNumber, sourcePath, err)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		cleanup()
		return "", noop, fmt.Errorf("pdfpage: read output dir after extracting page %d from %s: %w", pageNumber, sourcePath, err)
	}
	if len(entries) == 0 {
		cleanup()
		return "", noop, fmt.Errorf("pdfpage: extract page %d from %s: no output file produced", pageNumber, sourcePath)
	}

	return filepath.Join(tempDir, entries[0].Name()), cleanup, nil
}
```

- [ ] **Step 3: Viết `extract_test.go`**

Dùng fixture PDF thật đã có sẵn trong repo (không tạo fixture giả) — 1 file Coop (1 trang, để test trường hợp phổ biến) và file BigC 2 trang `2633058028692.pdf` (đã xác nhận có thật 2 trang lúc spike, để test cắt đúng trang khác trang 1):

```go
// GO/internal/pdfpage/extract_test.go
package pdfpage

import (
	"os"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func TestExtractPage_SinglePageSource(t *testing.T) {
	src := "../processing/coop/testdata/realpdfs/103098619-00.pdf"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not found: %v", err)
	}

	tempPath, cleanup, err := ExtractPage(src, 1)
	defer cleanup()
	if err != nil {
		t.Fatalf("ExtractPage returned error: %v", err)
	}
	if _, err := os.Stat(tempPath); err != nil {
		t.Fatalf("extracted file does not exist: %v", err)
	}
	if err := api.ValidateFile(tempPath, nil); err != nil {
		t.Fatalf("extracted file is not a valid PDF: %v", err)
	}
	count, err := api.PageCountFile(tempPath)
	if err != nil {
		t.Fatalf("PageCountFile returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("extracted file page count = %d, want 1", count)
	}
}

func TestExtractPage_SelectsCorrectPageFromMultiPageSource(t *testing.T) {
	src := "../bigc/testdata/realpdfs/2633058028692.pdf"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	sourceCount, err := api.PageCountFile(src)
	if err != nil {
		t.Fatalf("PageCountFile on source returned error: %v", err)
	}
	if sourceCount < 2 {
		t.Skipf("fixture only has %d page(s), need >= 2 to test page selection", sourceCount)
	}

	tempPath, cleanup, err := ExtractPage(src, 2)
	defer cleanup()
	if err != nil {
		t.Fatalf("ExtractPage returned error: %v", err)
	}
	count, err := api.PageCountFile(tempPath)
	if err != nil {
		t.Fatalf("PageCountFile returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("extracted file page count = %d, want 1", count)
	}
}

func TestExtractPage_CleanupRemovesTempFile(t *testing.T) {
	src := "../processing/coop/testdata/realpdfs/103098619-00.pdf"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not found: %v", err)
	}

	tempPath, cleanup, err := ExtractPage(src, 1)
	if err != nil {
		t.Fatalf("ExtractPage returned error: %v", err)
	}
	cleanup()
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed after cleanup(), stat error = %v", err)
	}
}

func TestExtractPage_NonexistentSourceReturnsError(t *testing.T) {
	tempPath, cleanup, err := ExtractPage("/does/not/exist.pdf", 1)
	defer cleanup()
	if err == nil {
		t.Fatal("expected an error for a nonexistent source file, got nil")
	}
	if tempPath != "" {
		t.Errorf("expected an empty tempPath on error, got %q", tempPath)
	}
}

func TestExtractPage_PageOutOfRangeReturnsError(t *testing.T) {
	src := "../processing/coop/testdata/realpdfs/103098619-00.pdf"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not found: %v", err)
	}

	tempPath, cleanup, err := ExtractPage(src, 999)
	defer cleanup()
	if err == nil {
		t.Fatal("expected an error for an out-of-range page number, got nil")
	}
	if tempPath != "" {
		t.Errorf("expected an empty tempPath on error, got %q", tempPath)
	}
}
```

- [ ] **Step 4: Chạy test, xác nhận pass**

Run: `cd GO && go test ./internal/pdfpage/... -v`
Expected: tất cả PASS (hoặc SKIP nếu fixture path sai — nếu SKIP xảy ra, kiểm tra lại đường dẫn tương đối tới fixture, đừng bỏ qua).

- [ ] **Step 5: `go vet`**

Run: `cd GO && go vet ./internal/pdfpage/...`
Expected: sạch.

- [ ] **Step 6: Commit**

```bash
cd GO && git add go.mod go.sum internal/pdfpage/extract.go internal/pdfpage/extract_test.go
git commit -m "feat(go): add internal/pdfpage package (cut a single PDF page for Drive upload)"
```

---

### Task 2: Truyền số trang PDF thật qua `Process()` + đổi chữ ký 8 hàm vendor (gộp)

**Files:**
- Modify: `GO/internal/processing/coop_processor.go` (hàm `Process`)
- Modify: `GO/internal/processing/lotte_processor.go` (chữ ký `processLotteSegment`)
- Modify: `GO/internal/processing/satra_processor.go` (chữ ký `processSatraSegment`)
- Modify: `GO/internal/processing/emart_processor.go` (chữ ký `processEmartSegment`)
- Modify: `GO/internal/processing/kingfood_processor.go` (chữ ký `processKingfoodSegment`)
- Modify: `GO/internal/processing/winmart_processor.go` (chữ ký `processWinmartSegment`)
- Modify: `GO/internal/processing/fujimart_processor.go` (chữ ký `processFujimartSegment`)
- Modify: `GO/internal/processing/jmart_processor.go` (chữ ký `processJMartSegment`)

**Interfaces:**
- Consumes: không phụ thuộc `pdfpage` (Task 1) để COMPILE — task này chỉ thêm tham số `int`, chưa dùng `pdfpage.ExtractPage`. Có thể làm song song với Task 1 nếu muốn, nhưng nên làm SAU để tránh 2 subagent cùng sửa `coop_processor.go`/vendor files trùng lúc (rủi ro conflict thấp nhưng không cần thiết).
- Produces: mỗi hàm vendor (`processSegment`, `processLotteSegment`, ...) có thêm tham số `realPageNum int` (vị trí: ngay sau `filePath string`, trước tham số text tiếp theo) — Task 3-10 (từng vendor) dùng tham số này để gọi `pdfpage.ExtractPage(filePath, realPageNum)`.

Đây là 1 task GỘP vì đổi chữ ký hàm mà không đổi tất cả điểm gọi cùng lúc sẽ làm build đỏ — giống lý do đã áp dụng ở các plan trước trong dự án này.

- [ ] **Step 1: Đổi `Process()` trong `coop_processor.go`**

Tìm khối `switch v {` (hiện tại, dòng ~68-191), đổi TỪNG điểm gọi vendor để truyền thêm `pageNumbers[pageIdx]`. Dưới đây là bản đầy đủ đã sửa (thay thế TOÀN BỘ khối từ `switch v {` tới hết `}` đóng switch, giữ nguyên `default:` case không đổi):

```go
		switch v {
		case "Coop":
			segments, ok := splitPageIntoPOs(text)
			if !ok {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Coop",
					Status: StatusFailed + " - không đếm khớp số đơn trên trang", StatusKind: StatusKindFailed,
				})
				continue
			}
			for segIdx, segment := range segments {
				segLabel := fmt.Sprintf("%d/%d", segIdx+1, len(segments))
				row, err := p.processSegment(filePath, pageNumbers[pageIdx], segment, segLabel)
				if err != nil {
					rows = append(rows, OrderRow{
						FileName: filepath.Base(filePath), Page: segLabel, System: "Coop",
						Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
					})
					continue
				}
				rows = append(rows, row)
			}

		case "Lotte":
			row, err := p.processLotteSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Lotte",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)

		case "Satra":
			row, err := p.processSatraSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Satra",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)

		case "Emart":
			row, err := p.processEmartSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Emart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)

		case "Kingfood":
			row, err := p.processKingfoodSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Kingfood",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)

		case "Winmart":
			// Re-extract this page's text with extractWinmartPageText
			// instead of using the shared pass's `text` directly — see
			// that function's doc comment (winmart_pdftext.go) for why.
			// Use pageNumbers[pageIdx] (the real, uncompacted PDF page
			// number), NOT the loop's pageIdx itself: extractPageTexts
			// skips null pages without appending a placeholder, so
			// pageIdx only equals "real page number minus one" when no
			// earlier page in this document was null. Passing pageIdx
			// directly here would silently re-extract the WRONG page
			// whenever an earlier page is null, with no error returned
			// to trigger the fallback below.
			winmartText := text
			if improved, wErr := extractWinmartPageTextFromFile(filePath, pageNumbers[pageIdx]-1); wErr == nil && improved != "" {
				winmartText = improved
			}
			row, err := p.processWinmartSegment(filePath, pageNumbers[pageIdx], winmartText, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "Winmart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)

		case "FujiMart":
			row, err := p.processFujimartSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "FujiMart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)

		case "JMart":
			row, err := p.processJMartSegment(filePath, pageNumbers[pageIdx], text, pageLabel)
			if err != nil {
				rows = append(rows, OrderRow{
					FileName: filepath.Base(filePath), Page: pageLabel, System: "JMart",
					Status: fmt.Sprintf("%s - %v", StatusFailed, err), StatusKind: StatusKindFailed,
				})
				continue
			}
			rows = append(rows, row)

		default:
			reason := "không nhận diện được nhà cung cấp"
			if v != "" {
				reason = "nhà cung cấp " + v + " chưa được hỗ trợ"
			}
			rows = append(rows, OrderRow{
				FileName: filepath.Base(filePath), Page: pageLabel, System: v,
				Status: StatusFailed + " - " + reason, StatusKind: StatusKindFailed,
			})
		}
```

(Duy nhất thay đổi so với bản gốc: mỗi lệnh gọi `p.process<Vendor>Segment(...)` giờ có thêm `pageNumbers[pageIdx]` làm tham số thứ 2, ngay sau `filePath`. Không đổi logic gì khác trong khối này.)

- [ ] **Step 2: Đổi chữ ký `processSegment` trong `coop_processor.go`**

Tìm dòng hiện tại:
```go
func (p *RealProcessor) processSegment(filePath, text, pageLabel string) (OrderRow, error) {
```

Đổi thành:
```go
func (p *RealProcessor) processSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
```

(Chỉ đổi dòng khai báo hàm — KHÔNG đổi bất kỳ dòng nào khác bên trong thân hàm ở task này, kể cả khi `realPageNum` chưa được dùng tới — Go cho phép tham số hàm không dùng, không lỗi biên dịch.)

- [ ] **Step 3-9: Đổi chữ ký 7 hàm vendor còn lại (mỗi hàm 1 dòng, cùng pattern)**

`lotte_processor.go`:
```go
func (p *RealProcessor) processLotteSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
```

`satra_processor.go`:
```go
func (p *RealProcessor) processSatraSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
```

`emart_processor.go`:
```go
func (p *RealProcessor) processEmartSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
```

`kingfood_processor.go`:
```go
func (p *RealProcessor) processKingfoodSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
```

`winmart_processor.go`:
```go
func (p *RealProcessor) processWinmartSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
```

`fujimart_processor.go`:
```go
func (p *RealProcessor) processFujimartSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
```

`jmart_processor.go`:
```go
func (p *RealProcessor) processJMartSegment(filePath string, realPageNum int, text, pageLabel string) (OrderRow, error) {
```

Với mỗi file: chỉ đổi ĐÚNG dòng khai báo hàm (`func (p *RealProcessor) process<Vendor>Segment(...)`), không đổi gì khác trong file đó ở task này.

- [ ] **Step 10: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: build/vet sạch (tham số `realPageNum` chưa dùng bên trong thân hàm KHÔNG gây lỗi biên dịch — chỉ local variable không dùng mới lỗi, tham số hàm thì không). `go test ./...` khớp baseline đã biết.

- [ ] **Step 11: Commit**

```bash
cd GO && git add internal/processing/coop_processor.go internal/processing/lotte_processor.go internal/processing/satra_processor.go internal/processing/emart_processor.go internal/processing/kingfood_processor.go internal/processing/winmart_processor.go internal/processing/fujimart_processor.go internal/processing/jmart_processor.go
git commit -m "refactor(go): thread real PDF page number into Process() dispatch + 8 vendor segment functions"
```

---

### Task 3: Coop — cắt trang trước khi upload

**Files:**
- Modify: `GO/internal/processing/coop_processor.go`

**Interfaces:**
- Consumes: `pdfpage.ExtractPage(sourcePath string, pageNumber int) (tempPath string, cleanup func(), err error)` (Task 1), `realPageNum int` param đã có trong chữ ký `processSegment` (Task 2).

- [ ] **Step 1: Thêm import**

Thêm `"order-processor/internal/pdfpage"` vào import block của `coop_processor.go`.

- [ ] **Step 2: Đổi điểm gọi `driveupload.Upload`**

Tìm (hiện tại, trong `processSegment`):
```go
	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "COOP",
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   info.PONumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}
```

Thay bằng:
```go
	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "COOP",
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   info.PONumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}
```

**Giải thích quan trọng**: `defer cleanup()` ở đây AN TOÀN dù `driveupload.Upload()` được gọi NGAY SAU đó (không defer) — vì `Upload()` đọc xong nội dung file NGAY LÚC nó chạy (đồng bộ, trước khi return), goroutine nền chỉ dùng dữ liệu ĐÃ ĐỌC (base64 trong bộ nhớ), không đụng lại file nữa. `defer cleanup()` sẽ chạy khi `processSegment` return (sau khi `Upload()` đã đọc xong) — không có race. Nếu cắt trang lỗi (`extractErr != nil`), `uploadPath` vẫn là `filePath` gốc (KHÔNG có `cleanup` nào cần gọi vì không tạo file tạm) — log cảnh báo rồi vẫn tiếp tục upload nguyên file như phương án dự phòng, không làm fail order.

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: sạch, khớp baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/coop_processor.go
git commit -m "feat(go): upload extracted PDF page (not whole file) for Coop Drive uploads"
```

---

### Task 4: Lotte — cắt trang trước khi upload

**Files:**
- Modify: `GO/internal/processing/lotte_processor.go`

**Interfaces:** Consumes như Task 3.

- [ ] **Step 1: Thêm import** `"order-processor/internal/pdfpage"` vào `lotte_processor.go`.

- [ ] **Step 2: Đổi điểm gọi `driveupload.Upload`**

Tìm khối (trong `processLotteSegment`):
```go
	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "LOTTE",
		EntryDate:    info.EntryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   info.PONumber,
	}, func(ok bool, err error) {
```

Thay bằng (thêm khối cắt trang NGAY TRƯỚC dòng `driveURL, uploadErr := ...`, đổi `filePath` thành `uploadPath` trong lệnh gọi `Upload`, giữ nguyên callback bên trong không đổi):
```go
	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "LOTTE",
		EntryDate:    info.EntryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   info.PONumber,
	}, func(ok bool, err error) {
```

(Callback bên trong `func(ok bool, err error) { ... }` và khối `if uploadErr != nil && p.LogFunc != nil { ... }` ngay sau đó giữ NGUYÊN không đổi — chỉ đổi phần trước đó như trên.)

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: sạch, khớp baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/lotte_processor.go
git commit -m "feat(go): upload extracted PDF page (not whole file) for Lotte Drive uploads"
```

---

### Task 5: Satra — cắt trang trước khi upload

**Files:**
- Modify: `GO/internal/processing/satra_processor.go`

**Interfaces:** Consumes như Task 3.

- [ ] **Step 1: Thêm import** `"order-processor/internal/pdfpage"` vào `satra_processor.go`.

- [ ] **Step 2: Đổi điểm gọi `driveupload.Upload`** (trong `processSatraSegment`)

Thêm khối cắt trang NGAY TRƯỚC dòng `driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{Vendor: "SATRA", ...`, đổi tham số thứ 2 từ `filePath` thành `uploadPath`:

```go
	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "SATRA",
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
```

(Phần còn lại — callback, xử lý `uploadErr` — giữ nguyên không đổi, chỉ như mô tả ở Task 3/4.)

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: sạch, khớp baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/satra_processor.go
git commit -m "feat(go): upload extracted PDF page (not whole file) for Satra Drive uploads"
```

---

### Task 6: Emart — cắt trang trước khi upload

**Files:**
- Modify: `GO/internal/processing/emart_processor.go`

**Interfaces:** Consumes như Task 3. Lưu ý: `CustomerCode: emartCustomerCode` là hằng số package-level, không đổi gì ở điểm này — chỉ đổi tham số path của `Upload`.

- [ ] **Step 1: Thêm import** `"order-processor/internal/pdfpage"` vào `emart_processor.go`.

- [ ] **Step 2: Đổi điểm gọi `driveupload.Upload`** (trong `processEmartSegment`)

```go
	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "EMART",
		EntryDate:    entryDate,
		CustomerCode: emartCustomerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
```

(Callback + xử lý `uploadErr` giữ nguyên, chỉ thay phần trước dòng `driveURL, uploadErr := ...` và đổi `filePath` → `uploadPath` trong lệnh gọi.)

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: sạch, khớp baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/emart_processor.go
git commit -m "feat(go): upload extracted PDF page (not whole file) for Emart Drive uploads"
```

---

### Task 7: Kingfood — cắt trang trước khi upload

**Files:**
- Modify: `GO/internal/processing/kingfood_processor.go`

**Interfaces:** Consumes như Task 3. `CustomerCode: kingfoodCustomerCode` là hằng số, không đổi.

- [ ] **Step 1: Thêm import** `"order-processor/internal/pdfpage"` vào `kingfood_processor.go`.

- [ ] **Step 2: Đổi điểm gọi `driveupload.Upload`** (trong `processKingfoodSegment`)

```go
	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "KINGFOOD",
		EntryDate:    entryDate,
		CustomerCode: kingfoodCustomerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
```

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: sạch, khớp baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/kingfood_processor.go
git commit -m "feat(go): upload extracted PDF page (not whole file) for Kingfood Drive uploads"
```

---

### Task 8: Winmart — cắt trang trước khi upload

**Files:**
- Modify: `GO/internal/processing/winmart_processor.go`

**Interfaces:** Consumes như Task 3. `EntryDate`/`CancelDate`/`CustomerCode`/`OutputName` đều là biến cục bộ (giống Coop/Lotte/Satra, không phải hằng số).

- [ ] **Step 1: Thêm import** `"order-processor/internal/pdfpage"` vào `winmart_processor.go`.

- [ ] **Step 2: Đổi điểm gọi `driveupload.Upload`** (trong `processWinmartSegment`)

```go
	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "WINMART",
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
```

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: sạch, khớp baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/winmart_processor.go
git commit -m "feat(go): upload extracted PDF page (not whole file) for Winmart Drive uploads"
```

---

### Task 9: FujiMart — cắt trang trước khi upload

**Files:**
- Modify: `GO/internal/processing/fujimart_processor.go`

**Interfaces:** Consumes như Task 3. `CustomerCode: fujimartCustomerCode` là hằng số, không đổi.

- [ ] **Step 1: Thêm import** `"order-processor/internal/pdfpage"` vào `fujimart_processor.go`.

- [ ] **Step 2: Đổi điểm gọi `driveupload.Upload`** (trong `processFujimartSegment`)

```go
	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "FUJIMART",
		EntryDate:    entryDate,
		CustomerCode: fujimartCustomerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
```

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: sạch, khớp baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/fujimart_processor.go
git commit -m "feat(go): upload extracted PDF page (not whole file) for FujiMart Drive uploads"
```

---

### Task 10: JMart — cắt trang trước khi upload

**Files:**
- Modify: `GO/internal/processing/jmart_processor.go`

**Interfaces:** Consumes như Task 3. `CustomerCode: jmartCustomerCode` là hằng số, không đổi.

- [ ] **Step 1: Thêm import** `"order-processor/internal/pdfpage"` vào `jmart_processor.go`.

- [ ] **Step 2: Đổi điểm gọi `driveupload.Upload`** (trong `processJMartSegment`)

```go
	uploadPath := filePath
	if extractedPath, cleanup, extractErr := pdfpage.ExtractPage(filePath, realPageNum); extractErr == nil {
		uploadPath = extractedPath
		defer cleanup()
	} else if p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không cắt được trang PDF để upload Drive (dùng nguyên file thay thế): %v", extractErr))
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, uploadPath, driveupload.Metadata{
		Vendor:       "JMART",
		EntryDate:    entryDate,
		CustomerCode: jmartCustomerCode,
		CancelDate:   cancelDate,
		OutputName:   poNumber,
	}, func(ok bool, err error) {
```

- [ ] **Step 3: Build, vet, test**

Run: `cd GO && go build ./... && go vet ./... && go test ./...`
Expected: sạch, khớp baseline.

- [ ] **Step 4: Commit**

```bash
cd GO && git add internal/processing/jmart_processor.go
git commit -m "feat(go): upload extracted PDF page (not whole file) for JMart Drive uploads"
```

---

## Final Verification

Sau khi cả 10 task xong:
- [ ] `cd GO && go build ./... && go vet ./... && go test ./...` — sạch, khớp baseline đã biết (2 fixture Coop lỗi từ trước, không regression mới).
- [ ] `cd GO && wails build` — build production exe thành công.
- [ ] Grep xác nhận cả 8 file vendor (trừ BigC) đều gọi `pdfpage.ExtractPage(` đúng 1 lần mỗi file: `grep -c "pdfpage.ExtractPage(" internal/processing/coop_processor.go internal/processing/lotte_processor.go internal/processing/satra_processor.go internal/processing/emart_processor.go internal/processing/kingfood_processor.go internal/processing/winmart_processor.go internal/processing/fujimart_processor.go internal/processing/jmart_processor.go` — mỗi dòng phải là `1`.
- [ ] Grep xác nhận `bigc_processor.go` KHÔNG có `pdfpage.ExtractPage(` (BigC không đổi gì trong plan này): `grep -c "pdfpage.ExtractPage(" internal/processing/bigc_processor.go` — phải là `0`.
