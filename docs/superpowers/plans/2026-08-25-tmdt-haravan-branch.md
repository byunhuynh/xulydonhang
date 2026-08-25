# Nhánh xử lý đơn TMĐT (Haravan) — Kế hoạch triển khai

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Thêm một nhánh xử lý trong app chính: người dùng thả workbook `XUẤT HÀNG HN-LA MỚI.xlsx` vào danh sách rồi bấm "Xử lý"; app hỏi khoảng ngày, gọi Haravan Omni API, quy đổi mã thành phẩm, ghi sheet `Haravan` trong chính workbook đó và ghi dòng đặt hàng vào `dondathang.xlsx`.

**Architecture:** Gộp module `GO/haravan` (đang untracked) vào module chính thành `GO/internal/tmdt/{haravan,lookup,export}`. Nhánh rẽ đặt ở `runReservedBatch` trong `GO/app.go`, KHÔNG đụng `RealProcessor` — vì `Processor.Process(ctx, path, stt)` không có chỗ nhận khoảng ngày lẫn `Emitter`. Tầng quy đổi `tmdt/mapping.go` nhận đầu vào trung tính `[]OrderLine` để golden test nạp từ fixture, không cần mạng.

**Tech Stack:** Go 1.26, Wails v2.13, excelize v2.11.0, React 19 + zustand + GSAP, test Go chuẩn `testing`, test frontend `node --experimental-strip-types --test`.

**Spec:** `docs/superpowers/specs/2026-08-25-tmdt-haravan-branch-design.md`

## Global Constraints

- Module chính là `order-processor` (`GO/go.mod`), Go 1.26, excelize **v2.11.0**. Sau khi gộp, KHÔNG còn `GO/haravan/go.mod`.
- Comment code viết bằng **tiếng Việt**, giải thích *vì sao*, đúng như phần còn lại của `GO/internal/processing`.
- Access token Haravan **không bao giờ** được ghi vào `process:log` hay bất kỳ log nào.
- Hằng số nghiệp vụ, chép nguyên văn từ spec: VAT `8`, Số ngày được nợ `15`, Trạng thái `Chưa thực hiện`, Là dòng ghi chú `Không`, Mã kho `TP_HN_12` (HN) / `LA_KHOTMDT` (LA), Mã đơn vị `TMĐT_MB` (HN) / `TMĐT_MN` (LA), shop không quy đổi `CLEVY VIỆT NAM`, `#N/A` là chuỗi `lookup.NotAvailable`.
- Công thức: `Số lượng = SL đặt × SLTPᵢ`, `Đơn giá = Giá sản phẩm ÷ SLTPᵢ ÷ 1.08`. Chiết khấu cấp đơn KHÔNG tham gia.
- Kho: `location_name == "Kho Hà Nội"` → `HN`; mọi giá trị khác → `LA`.
- Khoảng ngày: tối đa 7 ngày tính cả hai đầu, ngày cuối ≤ hôm qua. Backend kiểm lại, không tin frontend.
- `go test ./...` trong `GO/` phải xanh sau MỖI task. Golden suite vendor hiện tại (151/151) là ràng buộc không hồi quy.
- Repo này có `codex.exe` chạy song song và working tree đang có file sửa dở (`GO/frontend/src/lib/zaloMessage.ts`, `GO/frontend/src/types.ts`, `GO/internal/processing/jit_airway_processor.go`, `GO/internal/processing/types.go`, `dondathang.xlsx`). **Mọi commit phải liệt kê đường dẫn tường minh** (`git commit -- <path>...`), tuyệt đối không `git add -A`, không `git commit -a`, không `git apply`.

---

## Cấu trúc file

| File | Trách nhiệm |
|---|---|
| `GO/internal/tmdt/haravan/*.go` | (chuyển từ `GO/haravan/internal/haravan`) client HTTP, kiểu dữ liệu, nhận diện sàn |
| `GO/internal/tmdt/lookup/lookup.go` | (chuyển) đọc 2 bảng tra cứu + **mới**: `AppendComboRows` |
| `GO/internal/tmdt/export/*.go` | (chuyển) `StandardWriter`/`Writer` — chỉ CLI dùng |
| `GO/internal/tmdt/detect.go` | nhận diện workbook TMĐT theo tên sheet |
| `GO/internal/tmdt/mapping.go` | **trái tim**: `[]OrderLine` → `[]SheetRow` + `[]excelwriter.TMDTRow` + danh sách mã thiếu |
| `GO/internal/tmdt/sheet.go` | ghi sheet `Haravan` vào workbook đang có |
| `GO/internal/processing/excelwriter/tmdt.go` | ghi dòng TMĐT vào `dondathang.xlsx` (Z/AT/AU để trống) |
| `GO/cmd/haravan-export/main.go` | (chuyển) CLI cũ |
| `GO/app_tmdt.go` | `InspectTMDTFiles`, `ResolveTMDTMissing`, `CancelTMDTMissing`, `processTMDTFile` |
| `GO/frontend/src/lib/tmdtDateRange.ts` | ràng buộc khoảng ngày (thuần hàm, test được) |
| `GO/frontend/src/components/TMDTDateRangeModal.tsx` | lịch chọn khoảng ngày |
| `GO/frontend/src/components/TMDTMissingModal.tsx` | modal khai mã thành phẩm còn thiếu |

---

## Task 1: Gộp module `GO/haravan` vào module chính

**Files:**
- Move: `GO/haravan/internal/haravan/` → `GO/internal/tmdt/haravan/`
- Move: `GO/haravan/internal/lookup/` → `GO/internal/tmdt/lookup/`
- Move: `GO/haravan/internal/export/` → `GO/internal/tmdt/export/`
- Move: `GO/haravan/cmd/haravan-export/` → `GO/cmd/haravan-export/`
- Move: `GO/haravan/README.md` → `GO/docs/haravan-export.md`
- Move: `GO/haravan/chuan-22-23.xlsx`, `GO/haravan/XUAT HANG 24-08.xlsx` → `GO/internal/tmdt/testdata/`
- Delete: `GO/haravan/go.mod`, `GO/haravan/go.sum`, `GO/haravan/haravan-export.exe`, `GO/haravan/.env`, `GO/haravan/.env.example`, `GO/haravan/.gitignore`, thư mục `GO/haravan/`
- Modify: `GO/go.mod` (thêm dependency của phần Haravan nếu `go mod tidy` đòi)

**Interfaces:**
- Consumes: —
- Produces: các package `order-processor/internal/tmdt/haravan`, `order-processor/internal/tmdt/lookup`, `order-processor/internal/tmdt/export` với API y hệt bản cũ: `haravan.NewClient(token) *Client`, `Client.ListOrders(ctx, ListOptions, func(page int, orders []Order) error) error`, `Client.CountOrders`, `haravan.ListOptions{CreatedAtMin, CreatedAtMax time.Time, ...}`, `haravan.Order`, `haravan.LineItem`, `haravan.ShopName(*Order) string`, `haravan.DetectChannel(*Order, []ChannelRule) string`, `haravan.DefaultChannelRules`, `haravan.VNLocation *time.Location`, `haravan.NewShopFilter(string) ShopFilter`, `ShopFilter.Excluded(shop string) bool`, `lookup.Load(path string) (*Tables, error)`, `Tables.ByCombo(string) (*ComboRow, bool)`, `Tables.ByProductVariant(product, variant string) (*ComboRow, bool)`, `Tables.MisaCode(shop string) (string, bool)`, `lookup.ComboRow{Product, Variant, Combo string; TP, SL [4]string}`, `lookup.NotAvailable = "#N/A"`, `lookup.SheetDataShop = "data shop"`, `lookup.SheetMisa = "Mã misa"`.

- [ ] **Step 1: Ghi lại trạng thái test hiện tại của module rời**

Chạy trong `GO/haravan/` để biết bộ test này đang xanh trước khi động vào:

```bash
cd "GO/haravan" && go test ./... 2>&1 | tail -20
```

Expected: tất cả PASS. Ghi lại danh sách package đã chạy — cuối task phải chạy lại đúng bấy nhiêu package ở vị trí mới.

- [ ] **Step 2: Di chuyển thư mục**

```bash
cd "GO"
mkdir -p internal/tmdt cmd docs internal/tmdt/testdata
git mv --force haravan/internal/haravan internal/tmdt/haravan 2>/dev/null || mv haravan/internal/haravan internal/tmdt/haravan
mv haravan/internal/lookup internal/tmdt/lookup
mv haravan/internal/export internal/tmdt/export
mv haravan/cmd/haravan-export cmd/haravan-export
mv haravan/README.md docs/haravan-export.md
mv "haravan/chuan-22-23.xlsx" "haravan/XUAT HANG 24-08.xlsx" internal/tmdt/testdata/
rm -rf haravan
```

`GO/haravan/` là thư mục **untracked** nên `git mv` sẽ báo lỗi; nhánh `||` dùng `mv` thường là đường đi thật. Xoá luôn `.env` (chứa token thật) cùng `rm -rf` — token sẽ chuyển sang `settings.bhconfig` ở Task 3.

- [ ] **Step 3: Đổi đường dẫn import**

Mọi file vừa chuyển đang import theo module cũ. Đổi hết:

```bash
cd "GO"
grep -rl "haravan-order-export/internal" internal/tmdt cmd | while read -r f; do
  sed -i 's#haravan-order-export/internal#order-processor/internal/tmdt#g' "$f"
done
grep -rn "haravan-order-export" internal/tmdt cmd || echo "sạch"
```

Expected: in ra `sạch`.

- [ ] **Step 4: Cập nhật dependency và biên dịch**

```bash
cd "GO" && go mod tidy && go build ./... 2>&1 | head -30
```

Expected: không lỗi. `go mod tidy` sẽ tự kéo các indirect dependency mà `export` cần; excelize nhảy từ 2.9.1 (bản module rời) lên 2.11.0 (bản module chính) — đây là thay đổi có chủ đích, ghi trong spec.

- [ ] **Step 5: Chạy lại toàn bộ test, gồm cả bộ test vừa chuyển**

```bash
cd "GO" && go test ./... 2>&1 | tail -30
```

Expected: PASS toàn bộ, trong đó phải thấy `order-processor/internal/tmdt/export`, `.../lookup`, `.../haravan`. Nếu test của `export` fail vì đổi excelize, sửa ngay tại đây — đó chính là rủi ro spec đã nêu, không được để trôi sang task sau.

- [ ] **Step 6: Kiểm CLI vẫn build được**

```bash
cd "GO" && go build -o /dev/null ./cmd/haravan-export && echo "CLI OK"
```

Expected: in ra `CLI OK`.

- [ ] **Step 7: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/internal/tmdt GO/cmd/haravan-export GO/docs/haravan-export.md GO/go.mod GO/go.sum
git commit -m "refactor(tmdt): gộp module haravan-order-export vào module chính

Đường dẫn import đổi sang order-processor/internal/tmdt/*; excelize
thống nhất về v2.11.0; CLI chuyển sang GO/cmd/haravan-export." -- GO/internal/tmdt GO/cmd/haravan-export GO/docs/haravan-export.md GO/go.mod GO/go.sum
```

---

## Task 2: Nhận diện workbook TMĐT

**Files:**
- Create: `GO/internal/tmdt/detect.go`
- Test: `GO/internal/tmdt/detect_test.go`

**Interfaces:**
- Consumes: `lookup.SheetDataShop`, `lookup.SheetMisa` (Task 1)
- Produces: `tmdt.SheetHaravan = "Haravan"`, `tmdt.IsWorkbook(path string) bool`

- [ ] **Step 1: Viết test thất bại**

`GO/internal/tmdt/detect_test.go`:

```go
package tmdt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// makeWorkbook dựng 1 file xlsx tạm với đúng danh sách sheet cho trước.
func makeWorkbook(t *testing.T, name string, sheets ...string) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for _, s := range sheets {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet(%q): %v", s, err)
		}
	}
	f.DeleteSheet("Sheet1")
	path := filepath.Join(t.TempDir(), name)
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	return path
}

func TestIsWorkbook(t *testing.T) {
	full := makeWorkbook(t, "full.xlsx", "Mã misa", "data shop", "Haravan")
	if !IsWorkbook(full) {
		t.Errorf("workbook đủ 3 sheet phải được nhận là workbook TMĐT")
	}

	// Thiếu sheet Haravan: app sẽ tự tạo sheet đó, nên vẫn phải nhận.
	noHaravan := makeWorkbook(t, "no-haravan.xlsx", "Mã misa", "data shop")
	if !IsWorkbook(noHaravan) {
		t.Errorf("thiếu mỗi sheet Haravan vẫn phải nhận là workbook TMĐT")
	}

	// Thiếu bảng tra cứu: không đủ dữ liệu để quy đổi, không phải file TMĐT.
	noLookup := makeWorkbook(t, "no-lookup.xlsx", "Haravan", "Sheet2")
	if IsWorkbook(noLookup) {
		t.Errorf("thiếu bảng tra cứu thì không phải workbook TMĐT")
	}

	// dondathang.xlsx tuyệt đối không được nhận nhầm.
	dondathang := makeWorkbook(t, "dondathang.xlsx", "Don dat hang")
	if IsWorkbook(dondathang) {
		t.Errorf("dondathang.xlsx không phải workbook TMĐT")
	}

	// PDF và file không mở được: false, không panic.
	pdf := filepath.Join(t.TempDir(), "order.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatalf("ghi file pdf giả: %v", err)
	}
	if IsWorkbook(pdf) {
		t.Errorf("file PDF không phải workbook TMĐT")
	}
	if IsWorkbook(filepath.Join(t.TempDir(), "khong-ton-tai.xlsx")) {
		t.Errorf("file không tồn tại phải trả về false")
	}
}
```

- [ ] **Step 2: Chạy test để xác nhận thất bại**

Run: `cd GO && go test ./internal/tmdt/ -run TestIsWorkbook -v`
Expected: FAIL — `undefined: IsWorkbook`.

- [ ] **Step 3: Viết bản cài đặt tối thiểu**

`GO/internal/tmdt/detect.go`:

```go
// Package tmdt là nhánh xử lý đơn thương mại điện tử (Shopee/TikTok Shop
// đồng bộ qua Haravan) — song song với các nhánh vendor siêu thị dạng PDF
// trong internal/processing.
package tmdt

import (
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/tmdt/lookup"
)

// SheetHaravan là sheet app ghi dữ liệu đơn vào, nằm ngay trong workbook
// tra cứu của người dùng. Người dùng đã tạo sẵn sheet trống tên đúng như
// vậy; nếu không có, WriteHaravanSheet tự tạo.
const SheetHaravan = "Haravan"

// IsWorkbook nhận diện workbook TMĐT bằng SỰ CÓ MẶT CỦA HAI BẢNG TRA CỨU,
// không bằng tên file. Tên file thay đổi theo tháng ("XUẤT HÀNG 25-08
// HN-LA MỚI.xlsx", "XUẤT HÀNG HN-LA MỚI.xlsx"...) nên bắt theo tên vừa
// mong manh vừa dễ nhận nhầm; hai sheet "data shop" + "Mã misa" thì chỉ
// workbook này mới có, và cũng chính là thứ nhánh TMĐT thực sự cần đọc.
//
// Sheet "Haravan" KHÔNG nằm trong điều kiện: đó là sheet đầu ra, app tự
// tạo nếu thiếu.
//
// Mọi lỗi (không phải xlsx, file hỏng, không tồn tại) đều trả về false
// chứ không phải error: hàm này chạy trên từng file người dùng thả vào,
// và "không phải file TMĐT" là câu trả lời đúng cho mọi trường hợp đó.
func IsWorkbook(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return false
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		return false
	}
	defer f.Close()

	has := map[string]bool{}
	for _, name := range f.GetSheetList() {
		has[strings.ToLower(strings.TrimSpace(name))] = true
	}
	return has[strings.ToLower(lookup.SheetDataShop)] && has[strings.ToLower(lookup.SheetMisa)]
}
```

- [ ] **Step 4: Chạy test để xác nhận đạt**

Run: `cd GO && go test ./internal/tmdt/ -run TestIsWorkbook -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/tmdt/detect.go GO/internal/tmdt/detect_test.go
git commit -m "feat(tmdt): nhận diện workbook TMĐT theo hai bảng tra cứu" -- GO/internal/tmdt/detect.go GO/internal/tmdt/detect_test.go
```

---

## Task 3: Cấu hình Haravan trong Cài đặt

**Files:**
- Modify: `GO/internal/appsettings/store.go` (struct `Settings`, hàm `ensureMaps`)
- Modify: `GO/internal/appsettings/migrate.go` (thêm `parseTagBlock(..., "haravan")`)
- Test: `GO/internal/appsettings/store_test.go` (thêm case)
- Modify: `GO/frontend/src/components/SettingsModal.tsx`

**Interfaces:**
- Consumes: —
- Produces: `appsettings.Settings.Haravan map[string]string` với hai khoá quy ước `access_token` và `exclude_shops`.

- [ ] **Step 1: Viết test thất bại**

Thêm vào `GO/internal/appsettings/store_test.go`:

```go
func TestSettingsHaravanRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "settings.bhconfig"))

	want := Settings{
		Gid:      map[string]string{"MAKH": "1"},
		Zalo:     map[string]string{},
		Reminder: map[string]string{},
		Haravan:  map[string]string{"access_token": "abc123", "exclude_shops": "CLEVY VIỆT NAM"},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load(filepath.Join(dir, "khong-co-settings.ini"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Haravan["access_token"] != "abc123" {
		t.Errorf("access_token = %q, muốn %q", got.Haravan["access_token"], "abc123")
	}
	if got.Haravan["exclude_shops"] != "CLEVY VIỆT NAM" {
		t.Errorf("exclude_shops = %q, muốn %q", got.Haravan["exclude_shops"], "CLEVY VIỆT NAM")
	}
}

func TestLoadFillsEmptyHaravanMap(t *testing.T) {
	// File .bhconfig cũ (viết trước khi có nhánh TMĐT) không có khoá
	// "haravan" — Load phải trả map rỗng chứ không phải nil, để
	// SettingsModal đọc được ngay mà không nil-check ở mọi chỗ dùng.
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.bhconfig")
	if err := os.WriteFile(path, []byte(`{"gid":{},"zalo":{},"reminder":{}}`), 0o644); err != nil {
		t.Fatalf("ghi file cũ: %v", err)
	}
	got, err := NewStore(path).Load(filepath.Join(dir, "khong-co.ini"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Haravan == nil {
		t.Fatalf("Haravan = nil, muốn map rỗng")
	}
}
```

- [ ] **Step 2: Chạy test để xác nhận thất bại**

Run: `cd GO && go test ./internal/appsettings/ -run 'Haravan' -v`
Expected: FAIL — `unknown field Haravan in struct literal`.

- [ ] **Step 3: Thêm trường `Haravan`**

Trong `GO/internal/appsettings/store.go`, sửa struct và `ensureMaps`:

```go
type Settings struct {
	Gid      map[string]string `json:"gid"`
	Zalo     map[string]string `json:"zalo"`
	Reminder map[string]string `json:"reminder"`
	// Haravan giữ cấu hình nhánh TMĐT. Hai khoá quy ước:
	//   access_token  - private token Haravan, scope com.read_orders
	//   exclude_shops - danh sách shop bỏ qua, ngăn bởi dấu phẩy
	// Vẫn là map[string]string như 3 nhóm còn lại để popup Cài đặt dùng
	// lại nguyên KeyValueEditor, không phải viết form riêng.
	Haravan map[string]string `json:"haravan"`
}
```

Trong `ensureMaps`, thêm:

```go
	if s.Haravan == nil {
		s.Haravan = map[string]string{}
	}
```

Trong `store.go`, ở nhánh trả về Settings rỗng khi không có file nào (dòng ~60), thêm `Haravan: map[string]string{}`. Trong `migrate.go`, sau khối `reminder`, thêm:

```go
	haravan, err := parseTagBlock(string(content), "haravan")
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: migrate <haravan>: %w", err)
	}
	return Settings{Gid: gid, Zalo: zalo, Reminder: reminder, Haravan: haravan}, true, nil
```

(bỏ dòng `return` cũ).

- [ ] **Step 4: Chạy test để xác nhận đạt**

Run: `cd GO && go test ./internal/appsettings/ -v`
Expected: PASS toàn bộ, gồm cả các test cũ.

- [ ] **Step 5: Thêm tab Haravan vào popup Cài đặt**

Trong `GO/frontend/src/components/SettingsModal.tsx`:

- dòng 10: `type SettingsTab = 'gid' | 'zalo' | 'reminder' | 'haravan'`
- dòng 20: `useState({ gid: false, zalo: false, reminder: false, haravan: false })`
- dòng 42: `const hasDuplicates = dupState.gid || dupState.zalo || dupState.reminder || dupState.haravan`
- dòng 54-57, thêm vào mảng `tabs`: `{ key: 'haravan', label: 'Haravan (TMĐT)' }`
- sau khối `{tab === 'reminder' && (...)}`, thêm khối cùng khuôn:

```tsx
          {tab === 'haravan' && (
            <KeyValueEditor
              entries={settings.haravan}
              onChange={(haravan) => setSettings({ ...settings, haravan })}
              onDuplicateChange={(hasDup) => setDupState((d) => ({ ...d, haravan: hasDup }))}
              keyLabel="Khoá"
              valueLabel="Giá trị"
              hint="access_token = private token Haravan (scope com.read_orders) · exclude_shops = danh sách shop bỏ qua, ngăn bởi dấu phẩy"
            />
          )}
```

Đối chiếu tên prop với khối `reminder` ngay bên trên và dùng đúng bộ prop mà `KeyValueEditor` thực sự nhận — nếu `keyLabel`/`valueLabel`/`hint` không tồn tại thì bỏ, giữ đúng ba prop `entries`/`onChange`/`onDuplicateChange`.

- [ ] **Step 6: Kiểm frontend biên dịch**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: không lỗi. (`settings.haravan` có kiểu từ bindings Wails — nếu báo thiếu, chạy `cd GO && wails generate module` rồi chạy lại.)

- [ ] **Step 7: Commit**

```bash
git add GO/internal/appsettings GO/frontend/src/components/SettingsModal.tsx GO/frontend/wailsjs
git commit -m "feat(settings): thêm nhóm cấu hình Haravan cho nhánh TMĐT" -- GO/internal/appsettings GO/frontend/src/components/SettingsModal.tsx GO/frontend/wailsjs
```

---

## Task 4: Fixture vàng cho tầng quy đổi

**Files:**
- Create: `GO/internal/tmdt/testdata/golden/orders.csv`
- Create: `GO/internal/tmdt/testdata/golden/expected_dondathang.csv`
- Create: `GO/internal/tmdt/testdata/golden/lookup.xlsx`
- Create: `GO/internal/tmdt/testdata/golden/ten_hang.csv`
- Create: `GO/internal/tmdt/testdata/golden/README.md`
- Create: `docs/superpowers/plans/tmdt-golden-fixture.py` (script sinh fixture, giữ lại để tái sinh)

**Interfaces:**
- Consumes: —
- Produces: bốn file fixture ở trên. `orders.csv` có header `order_code,shop,kho_ban,kenh_ban_hang,ngay_dat_hang,so_luong,ten_san_pham,gia_tri_thuoc_tinh_1,gia_san_pham,ma_san_pham`. `expected_dondathang.csv` có header `A,B,C,D,E,G,L,Q,T,U,V,X,Y,AE,AJ,AM,AO,AV` (không có `S` — xem Step 3). `ten_hang.csv` có header `ma_tp,ten_hang`.

- [ ] **Step 1: Viết script sinh fixture**

`docs/superpowers/plans/tmdt-golden-fixture.py`:

```python
"""Sinh fixture vàng cho GO/internal/tmdt từ hai file Excel thật.

Chạy:  .venv/Scripts/python.exe docs/superpowers/plans/tmdt-golden-fixture.py

Đầu vào (đường dẫn tuyệt đối, máy người dùng):
  - master : XUẤT HÀNG HN-LA MỚI.xlsx  -> sheet "Đơn hàng haravan" + 2 bảng tra cứu
  - golden : đơn hàng/mẫu chuẩn.xlsx   -> sheet "Don dat hang", dữ liệu từ dòng 9
"""
import csv, os, openpyxl

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
MASTER = r"C:\Users\Admin\Desktop\Xuất hàng TMĐT\Tháng 07-2026\XUẤT HÀNG HN-LA MỚI.xlsx"
GOLDEN = os.path.join(ROOT, "đơn hàng", "mẫu chuẩn.xlsx")
OUT = os.path.join(ROOT, "GO", "internal", "tmdt", "testdata", "golden")
os.makedirs(OUT, exist_ok=True)

def s(v):
    return "" if v is None else str(v)

# --- orders.csv: 1 dòng cho mỗi dòng hàng trong sheet "Đơn hàng haravan" ---
ORDER_COLS = [0, 84, 69, 70, 16, 18, 19, 21, 26, 28]
ORDER_HEAD = ["order_code", "shop", "kho_ban", "kenh_ban_hang", "ngay_dat_hang",
              "so_luong", "ten_san_pham", "gia_tri_thuoc_tinh_1", "gia_san_pham", "ma_san_pham"]
wb = openpyxl.load_workbook(MASTER, read_only=True, data_only=True)
ws = wb["Đơn hàng haravan"]
n = 0
with open(os.path.join(OUT, "orders.csv"), "w", newline="", encoding="utf-8") as fh:
    w = csv.writer(fh)
    w.writerow(ORDER_HEAD)
    for row in ws.iter_rows(min_row=2, values_only=True):
        if not row or not row[0]:
            continue
        w.writerow([s(row[c]) if c < len(row) else "" for c in ORDER_COLS])
        n += 1
print("orders.csv:", n, "dòng")

# --- lookup.xlsx: chỉ 2 bảng tra cứu, lấy từ CHÍNH master đã sinh golden ---
out_wb = openpyxl.Workbook()
out_wb.remove(out_wb.active)
for name in ("Mã misa", "data shop"):
    src, dst = wb[name], out_wb.create_sheet(name)
    for row in src.iter_rows(values_only=True):
        dst.append(list(row))
out_wb.save(os.path.join(OUT, "lookup.xlsx"))
print("lookup.xlsx: đã sao 2 sheet tra cứu")

# --- expected_dondathang.csv: cột S bị loại, xem chú thích trong plan ---
EXP_COLS = [0, 1, 2, 3, 4, 6, 11, 16, 19, 20, 21, 23, 24, 30, 35, 38, 40, 47]
EXP_HEAD = ["A", "B", "C", "D", "E", "G", "L", "Q", "T", "U", "V", "X", "Y", "AE", "AJ", "AM", "AO", "AV"]
gwb = openpyxl.load_workbook(GOLDEN, read_only=True, data_only=True)
gws = gwb["Don dat hang"]
m = 0
with open(os.path.join(OUT, "expected_dondathang.csv"), "w", newline="", encoding="utf-8") as fh:
    w = csv.writer(fh)
    w.writerow(EXP_HEAD)
    for row in gws.iter_rows(min_row=9, values_only=True):
        if not row or not row[0]:
            continue
        w.writerow([s(row[c]) if c < len(row) else "" for c in EXP_COLS])
        m += 1
print("expected_dondathang.csv:", m, "dòng")

# --- ten_hang.csv: bản đồ MÃ TP -> Tên hàng, dùng làm ProductNamer trong test ---
names = {}
for row in gws.iter_rows(min_row=9, values_only=True):
    if row and row[0] and row[16]:
        names.setdefault(s(row[16]), s(row[18]))
with open(os.path.join(OUT, "ten_hang.csv"), "w", newline="", encoding="utf-8") as fh:
    w = csv.writer(fh)
    w.writerow(["ma_tp", "ten_hang"])
    for k in sorted(names):
        w.writerow([k, names[k]])
print("ten_hang.csv:", len(names), "mã")
```

- [ ] **Step 2: Chạy script và kiểm số dòng**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
PYTHONIOENCODING=utf-8 .venv/Scripts/python.exe docs/superpowers/plans/tmdt-golden-fixture.py
```

Expected, chính xác các con số này — lệch là fixture sai, dừng lại điều tra chứ không đi tiếp:
```
orders.csv: 1585 dòng
lookup.xlsx: đã sao 2 sheet tra cứu
expected_dondathang.csv: 1430 dòng
```

- [ ] **Step 3: Ghi lý do cột `S` bị loại khỏi fixture**

Cột `S` (Tên hàng) không lấy từ dữ liệu Haravan mà tra `productdata.Store` — vốn nạp từ Google Sheets qua mạng. Tầng quy đổi vì thế nhận một hàm `ProductName func(tp string) string` do người gọi truyền vào; golden test truyền hàm dựng từ `ten_hang.csv`. So sánh cột `S` trong golden test sẽ là so kết quả với chính đầu vào của nó, tức vô nghĩa. Thay vào đó Task 5 có một test riêng kiểm rằng tầng quy đổi gọi `ProductName` với đúng mã thành phẩm.

Thêm ghi chú này vào đầu `orders.csv`? **Không** — CSV phải sạch để `encoding/csv` đọc. Ghi vào `GO/internal/tmdt/testdata/golden/README.md`:

```markdown
# Fixture vàng nhánh TMĐT

Sinh lại bằng: `.venv/Scripts/python.exe docs/superpowers/plans/tmdt-golden-fixture.py`

| File | Nội dung |
|---|---|
| `orders.csv` | 1.585 dòng hàng, sheet "Đơn hàng haravan" của workbook thật, 22–23/08/2026 |
| `lookup.xlsx` | 2 bảng tra cứu lấy từ CHÍNH workbook đó (không lấy bản mới hơn — bảng tra cứu đổi theo thời gian, lấy lệch là golden sai) |
| `expected_dondathang.csv` | 1.430 dòng đầu ra đúng, sheet "Don dat hang" của `đơn hàng/mẫu chuẩn.xlsx` |
| `ten_hang.csv` | bản đồ MÃ TP → Tên hàng, dùng làm `ProductName` trong test |

Cột `S` (Tên hàng) KHÔNG có trong `expected_dondathang.csv`: nó tra
`productdata.Store` chứ không suy từ dữ liệu Haravan, nên so sánh nó ở
golden test là so kết quả với chính đầu vào. Xem `TestBuildGoiProductName`.
```

- [ ] **Step 4: Commit fixture**

```bash
git add GO/internal/tmdt/testdata/golden docs/superpowers/plans/tmdt-golden-fixture.py
git commit -m "test(tmdt): fixture vàng 1585 dòng vào / 1430 dòng ra cho tầng quy đổi" -- GO/internal/tmdt/testdata/golden docs/superpowers/plans/tmdt-golden-fixture.py
```

---

## Task 5: Ghi dòng TMĐT vào `dondathang.xlsx`

**Files:**
- Create: `GO/internal/processing/excelwriter/tmdt.go`
- Test: `GO/internal/processing/excelwriter/tmdt_test.go`

**Interfaces:**
- Consumes: hằng `sheetName = "Don dat hang"` (đã có trong `dondathang.go`)
- Produces:
  - `excelwriter.TMDTRow{EntryDate, OrderNumber, ShipTo, CustomerCode, Description, SKU, ProductName string; IsPromoItem bool; Warehouse string; Qty, UnitPrice float64; RegionCode, StatCode, Note string}`
  - `excelwriter.WriteTMDTRows(path string, rows []TMDTRow) (startRow int, err error)`

- [ ] **Step 1: Viết test thất bại**

`GO/internal/processing/excelwriter/tmdt_test.go`:

```go
package excelwriter

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// newTemplate dựng 1 dondathang.xlsx rỗng: sheet "Don dat hang" với 8
// dòng tiêu đề của khuôn AMIS, dữ liệu bắt đầu từ dòng 9 — đúng như
// ClearOrderRows giả định.
func newTemplate(t *testing.T) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	if _, err := f.NewSheet(sheetName); err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	f.DeleteSheet("Sheet1")
	for r := 1; r <= 8; r++ {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		if err := f.SetCellValue(sheetName, cell, "tiêu đề"); err != nil {
			t.Fatalf("SetCellValue: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "dondathang.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	return path
}

func TestWriteTMDTRows(t *testing.T) {
	path := newTemplate(t)

	rows := []TMDTRow{{
		EntryDate:    "23/08/2026",
		OrderNumber:  "ĐĐHTMĐT-TikTok-585694438276170905",
		ShipTo:       "HN",
		CustomerCode: "MN_TMDT_00016",
		Description:  "TMĐT-TikTok - Tẩy lồng máy giặt Blue - 585694438276170905 - Ngày đổ 23/08/2026 - HN",
		SKU:          "TP10127",
		ProductName:  "Bột tẩy lồng Blue Túi 150g - MỚI SẢN XUẤT",
		Warehouse:    "TP_HN_12",
		Qty:          1,
		UnitPrice:    26851.85185185185,
		RegionCode:   "TMĐT_MB",
		StatCode:     "HN",
		Note:         "585694438276170905",
	}, {
		EntryDate:    "23/08/2026",
		OrderNumber:  "ĐĐHTMĐT-Shopee-2608235QED370T",
		ShipTo:       "LA",
		CustomerCode: "MN_TMDT_00003",
		Description:  "TMĐT-Shopee - Blue Việt Nam - 2608235QED370T - Ngày đổ 23/08/2026 - LA",
		SKU:          "TP32743",
		ProductName:  "Bột tẩy lồng máy giặt Blue 150g",
		IsPromoItem:  true,
		Warehouse:    "LA_KHOTMDT",
		Qty:          1,
		UnitPrice:    0,
		RegionCode:   "TMĐT_MN",
		StatCode:     "LA",
		Note:         "2608235QED370T",
	}}

	startRow, err := WriteTMDTRows(path, rows)
	if err != nil {
		t.Fatalf("WriteTMDTRows: %v", err)
	}
	if startRow != 9 {
		t.Fatalf("startRow = %d, muốn 9", startRow)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại file: %v", err)
	}
	defer f.Close()

	want := map[string]string{
		"A9": "23/08/2026", "B9": "ĐĐHTMĐT-TikTok-585694438276170905",
		"C9": "Chưa thực hiện", "D9": "23/08/2026", "E9": "HN",
		"G9": "MN_TMDT_00016", "Q9": "TP10127",
		"S9": "Bột tẩy lồng Blue Túi 150g - MỚI SẢN XUẤT",
		"T9": "Không", "U9": "Không", "V9": "TP_HN_12", "X9": "1",
		"Y9": "26851.85185185185",
		"AE9": "8", "AJ9": "TMĐT_MB", "AM9": "HN",
		"AO9": "585694438276170905", "AV9": "15",
		// Dòng thứ hai: hàng tặng, kho Long An.
		"U10": "Có", "Y10": "0", "V10": "LA_KHOTMDT", "AJ10": "TMĐT_MN",
	}
	for cell, expect := range want {
		got, err := f.GetCellValue(sheetName, cell)
		if err != nil {
			t.Fatalf("GetCellValue(%s): %v", cell, err)
		}
		if got != expect {
			t.Errorf("%s = %q, muốn %q", cell, got, expect)
		}
	}

	// Z (Thành tiền), AT (Trọng lượng), AU (số thùng) PHẢI trống — mẫu
	// chuẩn TMĐT để trống cả ba, khác hẳn writeRow của nhánh vendor.
	for _, cell := range []string{"Z9", "AT9", "AU9", "Z10", "AT10", "AU10"} {
		got, err := f.GetCellValue(sheetName, cell)
		if err != nil {
			t.Fatalf("GetCellValue(%s): %v", cell, err)
		}
		if got != "" {
			t.Errorf("%s = %q, muốn trống", cell, got)
		}
	}
}

func TestWriteTMDTRowsAppendsAfterExistingRows(t *testing.T) {
	// Một batch có thể ghi file PDF vendor trước rồi mới tới file TMĐT:
	// dòng TMĐT phải nối tiếp, không đè.
	path := newTemplate(t)
	if _, err := WriteTMDTRows(path, []TMDTRow{{EntryDate: "22/08/2026", SKU: "TP1"}}); err != nil {
		t.Fatalf("lần ghi 1: %v", err)
	}
	startRow, err := WriteTMDTRows(path, []TMDTRow{{EntryDate: "23/08/2026", SKU: "TP2"}})
	if err != nil {
		t.Fatalf("lần ghi 2: %v", err)
	}
	if startRow != 10 {
		t.Fatalf("startRow lần 2 = %d, muốn 10", startRow)
	}
	f, _ := excelize.OpenFile(path)
	defer f.Close()
	if got, _ := f.GetCellValue(sheetName, "Q9"); got != "TP1" {
		t.Errorf("Q9 = %q, muốn TP1 (dòng cũ đã bị đè)", got)
	}
	if got, _ := f.GetCellValue(sheetName, "Q10"); got != "TP2" {
		t.Errorf("Q10 = %q, muốn TP2", got)
	}
}

func TestWriteTMDTRowsEmptyIsNoop(t *testing.T) {
	path := newTemplate(t)
	startRow, err := WriteTMDTRows(path, nil)
	if err != nil {
		t.Fatalf("WriteTMDTRows(nil): %v", err)
	}
	if startRow != 9 {
		t.Errorf("startRow = %d, muốn 9", startRow)
	}
}
```

- [ ] **Step 2: Chạy test để xác nhận thất bại**

Run: `cd GO && go test ./internal/processing/excelwriter/ -run TMDT -v`
Expected: FAIL — `undefined: TMDTRow`, `undefined: WriteTMDTRows`.

- [ ] **Step 3: Viết bản cài đặt tối thiểu**

`GO/internal/processing/excelwriter/tmdt.go`:

```go
package excelwriter

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// Hằng số nghiệp vụ của đơn TMĐT — mọi dòng đều giống nhau, xác nhận trên
// cả 1.430 dòng của "đơn hàng/mẫu chuẩn.xlsx", không có ngoại lệ nào.
const (
	tmdtStatus   = "Chưa thực hiện" // C
	tmdtNoteRow  = "Không"          // T — đơn TMĐT không có dòng ghi chú
	tmdtVAT      = 8                // AE
	tmdtDebtDays = 15               // AV
)

// TMDTRow là một dòng đặt hàng TMĐT. Mỗi dòng hàng trên sàn sinh ra MỘT
// TMDTRow cho MỖI mã thành phẩm nó quy đổi ra (combo 2 thành phẩm → 2
// TMDTRow), không gộp trùng.
//
// Vì sao không dùng lại Row của nhánh vendor: writeRow LUÔN ghi cột Z
// (công thức "=Y*X" hoặc số 0) và ghi AT/AU cho mọi dòng không phải dòng
// ghi chú, còn mẫu chuẩn TMĐT để trống cả ba ô đó. Thêm ba cờ phủ định
// nữa vào Row — vốn đã mang sẵn 6 biệt lệ riêng của từng vendor
// (NoCaseCount, StoreName, SiteCode, UseZFormula, IsNoteRow,
// PriceMismatch) — sẽ làm struct đó khó đọc hơn phần tiết kiệm được.
type TMDTRow struct {
	EntryDate    string  // A và D (ngày đơn hàng = ngày giao hàng)
	OrderNumber  string  // B — "ĐĐHTMĐT-{kênh}-{mã đơn}"
	ShipTo       string  // E — "HN" | "LA"
	CustomerCode string  // G — mã MISA
	Description  string  // L
	SKU          string  // Q — mã thành phẩm
	ProductName  string  // S
	IsPromoItem  bool    // U — true khi đơn giá = 0 (hàng tặng)
	Warehouse    string  // V — "TP_HN_12" | "LA_KHOTMDT"
	Qty          float64 // X
	UnitPrice    float64 // Y
	RegionCode   string  // AJ — "TMĐT_MB" | "TMĐT_MN"
	StatCode     string  // AM — "HN" | "LA"
	Note         string  // AO — mã đơn hàng gốc trên sàn
}

// WriteTMDTRows ghi nối tiếp rows vào sheet "Don dat hang", trả về số
// dòng đầu tiên đã ghi. Không tự gọi ClearOrderRows: batch đã dọn file
// một lần ở đầu (xem runReservedBatch), nên hàm này chỉ append — nhờ đó
// một batch trộn file PDF vendor với file TMĐT vẫn ra đủ cả hai.
func WriteTMDTRows(path string, rows []TMDTRow) (startRow int, err error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, fmt.Errorf("excelwriter: mở %s: %w", path, err)
	}
	defer f.Close()

	existing, err := f.GetRows(sheetName)
	if err != nil {
		return 0, fmt.Errorf("excelwriter: đọc %s: %w", sheetName, err)
	}
	// Dữ liệu bắt đầu từ dòng 9 (dòng 1–8 là khối tiêu đề khuôn AMIS).
	currentRow := len(existing) + 1
	if currentRow < 9 {
		currentRow = 9
	}
	firstRow := currentRow

	if len(rows) == 0 {
		return firstRow, nil
	}

	yesNo := func(b bool) string {
		if b {
			return "Có"
		}
		return "Không"
	}

	for _, row := range rows {
		writes := []struct {
			col   string
			value interface{}
		}{
			{"A", row.EntryDate}, {"B", row.OrderNumber}, {"C", tmdtStatus},
			{"D", row.EntryDate}, {"E", row.ShipTo}, {"G", row.CustomerCode},
			{"L", row.Description}, {"Q", row.SKU}, {"S", row.ProductName},
			{"T", tmdtNoteRow}, {"U", yesNo(row.IsPromoItem)}, {"V", row.Warehouse},
			{"X", row.Qty}, {"Y", row.UnitPrice}, {"AE", tmdtVAT},
			{"AJ", row.RegionCode}, {"AM", row.StatCode}, {"AO", row.Note},
			{"AV", tmdtDebtDays},
		}
		for _, w := range writes {
			cell := fmt.Sprintf("%s%d", w.col, currentRow)
			if err := f.SetCellValue(sheetName, cell, w.value); err != nil {
				return 0, fmt.Errorf("excelwriter: ghi %s: %w", cell, err)
			}
		}
		currentRow++
	}

	if err := f.Save(); err != nil {
		return 0, fmt.Errorf("excelwriter: lưu %s: %w", path, err)
	}
	return firstRow, nil
}
```

- [ ] **Step 4: Chạy test để xác nhận đạt**

Run: `cd GO && go test ./internal/processing/excelwriter/ -v`
Expected: PASS toàn bộ — cả test mới lẫn `dondathang_test.go` cũ phải xanh.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/processing/excelwriter/tmdt.go GO/internal/processing/excelwriter/tmdt_test.go
git commit -m "feat(excelwriter): ghi dòng đặt hàng TMĐT, để trống Z/AT/AU" -- GO/internal/processing/excelwriter/tmdt.go GO/internal/processing/excelwriter/tmdt_test.go
```

---

## Task 6: Tầng quy đổi + golden test 1.430 dòng

Đây là trái tim của nhánh. Task này KHÔNG chạm mạng, KHÔNG chạm UI.

**Files:**
- Create: `GO/internal/tmdt/mapping.go`
- Test: `GO/internal/tmdt/mapping_test.go`
- Test: `GO/internal/tmdt/mapping_golden_test.go`

**Interfaces:**
- Consumes: `lookup.Tables` (Task 1), `excelwriter.TMDTRow` (Task 5), fixture `testdata/golden/*` (Task 4)
- Produces:
  - `tmdt.OrderLine{OrderCode, Shop, KhoBan, KenhBanHang string; CreatedAt time.Time; Quantity float64; Title, VariantTitle string; Price, Subtotal, Total float64; SKU, Attributes string}`
  - `tmdt.SheetRow{OrderCode string; Subtotal, Total float64; OrderDate string; Quantity float64; Title, VariantTitle string; Price float64; SKU, Attributes, KhoBan, KenhBanHang string; CreatedAt time.Time; TP, SL [4]string; Shop, Misa string}`
  - `tmdt.MissingCombo{Key, Product, Variant, Combo string; LineCount int}`
  - `tmdt.Result{SheetRows []SheetRow; OrderRows []excelwriter.TMDTRow; Missing []MissingCombo; MissingShops map[string]int}`
  - `tmdt.Options{ProductName func(tp string) string}`
  - `tmdt.Build(lines []OrderLine, tables *lookup.Tables, opt Options) Result`
  - `tmdt.ChannelLabel(raw string) string`
  - `tmdt.MissingKey(sku, title, variant string) string`
  - `tmdt.ShopKhongQuyDoi = "CLEVY VIỆT NAM"`

- [ ] **Step 1: Viết test đơn vị thất bại (các quy tắc lẻ)**

`GO/internal/tmdt/mapping_test.go`:

```go
package tmdt

import (
	"testing"
	"time"

	"order-processor/internal/tmdt/lookup"
)

func vnTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

// fakeTables dựng bảng tra cứu nhỏ trong bộ nhớ để test quy tắc, không
// phải mở file Excel.
func fakeTables(t *testing.T) *lookup.Tables {
	t.Helper()
	tb, err := lookup.FromRows(
		// data shop: Tên sản phẩm | Phân loại | Mã combo | TP1 | SL1 | ...
		[][]string{
			{"Tên sản phẩm", "Phân loại", "Mã combo", "MÃ TP 1", "SLTP1", "MÃ TP 2", "SLTP2", "MÃ TP 3", "SLTP3", "MÃ TP 4", "SLTP4"},
			{"Bột tẩy lồng", "Combo 5 Túi", "SP000443", "TP10127", "5", "", "", "", "", "", ""},
			{"Combo đôi", "Bộ 2 món", "SP999", "TP111", "1", "TP222", "2", "", "", "", ""},
			{"Không mã combo", "Loại A", "", "TP333", "1", "", "", "", "", "", ""},
		},
		// Mã misa: cột B = tên kênh, cột D = mã MISA, dữ liệu từ dòng 3
		[][]string{
			{"", "Tên Kênh", "KÊNH BÁN", "Mã MISA"},
			{"", "", "", ""},
			{"", "Tẩy lồng máy giặt Blue", "TIKTOK", "MN_TMDT_00016"},
		},
	)
	if err != nil {
		t.Fatalf("lookup.FromRows: %v", err)
	}
	return tb
}

func baseLine() OrderLine {
	return OrderLine{
		OrderCode:   "585694423512745362",
		Shop:        "Tẩy lồng máy giặt Blue",
		KhoBan:      "Kho Hà Nội",
		KenhBanHang: "tiktokshop",
		Quantity:    1,
		Title:       "Bột tẩy lồng",
		VariantTitle: "Combo 5 Túi",
		Price:       88999,
		SKU:         "SP000443",
	}
}

func TestChannelLabel(t *testing.T) {
	cases := map[string]string{
		"tiktokshop": "TikTok",
		"TikTok Shop": "TikTok",
		"shopee":     "Shopee",
		"Shopee":     "Shopee",
		"web":        "web",
		"":           "",
	}
	for in, want := range cases {
		if got := ChannelLabel(in); got != want {
			t.Errorf("ChannelLabel(%q) = %q, muốn %q", in, got, want)
		}
	}
}

func TestBuildQtyAndUnitPrice(t *testing.T) {
	line := baseLine()
	line.CreatedAt = vnTime(t, "2026-08-23T23:54:05+07:00")

	res := Build([]OrderLine{line}, fakeTables(t), Options{ProductName: func(tp string) string { return "tên " + tp }})

	if len(res.OrderRows) != 1 {
		t.Fatalf("có %d dòng dondathang, muốn 1", len(res.OrderRows))
	}
	row := res.OrderRows[0]
	// SLTP = 5 → Số lượng = 1 × 5 = 5; Đơn giá = 88999 ÷ 5 ÷ 1.08.
	if row.Qty != 5 {
		t.Errorf("Qty = %v, muốn 5", row.Qty)
	}
	const wantPrice = 88999.0 / 5 / 1.08
	if row.UnitPrice != wantPrice {
		t.Errorf("UnitPrice = %v, muốn %v", row.UnitPrice, wantPrice)
	}
	if row.SKU != "TP10127" {
		t.Errorf("SKU = %q, muốn TP10127", row.SKU)
	}
	if row.ProductName != "tên TP10127" {
		t.Errorf("ProductName = %q — Build phải gọi Options.ProductName với mã thành phẩm", row.ProductName)
	}
	if row.EntryDate != "23/08/2026" {
		t.Errorf("EntryDate = %q, muốn 23/08/2026", row.EntryDate)
	}
	if row.OrderNumber != "ĐĐHTMĐT-TikTok-585694423512745362" {
		t.Errorf("OrderNumber = %q", row.OrderNumber)
	}
	wantDesc := "TMĐT-TikTok - Tẩy lồng máy giặt Blue - 585694423512745362 - Ngày đổ 23/08/2026 - HN"
	if row.Description != wantDesc {
		t.Errorf("Description = %q,\nmuốn                %q", row.Description, wantDesc)
	}
	if row.ShipTo != "HN" || row.Warehouse != "TP_HN_12" || row.RegionCode != "TMĐT_MB" || row.StatCode != "HN" {
		t.Errorf("cụm kho = %q/%q/%q/%q, muốn HN/TP_HN_12/TMĐT_MB/HN",
			row.ShipTo, row.Warehouse, row.RegionCode, row.StatCode)
	}
	if row.CustomerCode != "MN_TMDT_00016" {
		t.Errorf("CustomerCode = %q, muốn MN_TMDT_00016", row.CustomerCode)
	}
	if row.Note != "585694423512745362" {
		t.Errorf("Note = %q, muốn mã đơn gốc", row.Note)
	}
}

func TestBuildLongAnWarehouse(t *testing.T) {
	line := baseLine()
	line.KhoBan = "Miền Nam - Kho mặc định"
	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	row := res.OrderRows[0]
	if row.ShipTo != "LA" || row.Warehouse != "LA_KHOTMDT" || row.RegionCode != "TMĐT_MN" || row.StatCode != "LA" {
		t.Errorf("cụm kho = %q/%q/%q/%q, muốn LA/LA_KHOTMDT/TMĐT_MN/LA",
			row.ShipTo, row.Warehouse, row.RegionCode, row.StatCode)
	}
}

func TestBuildPromoRowWhenPriceZero(t *testing.T) {
	line := baseLine()
	line.Price = 0
	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	if !res.OrderRows[0].IsPromoItem {
		t.Errorf("giá 0 phải đánh dấu Hàng khuyến mại = Có")
	}
	if res.OrderRows[0].UnitPrice != 0 {
		t.Errorf("UnitPrice = %v, muốn 0", res.OrderRows[0].UnitPrice)
	}
}

func TestBuildComboWithTwoComponents(t *testing.T) {
	line := baseLine()
	line.Title, line.VariantTitle, line.SKU = "Combo đôi", "Bộ 2 món", "SP999"
	line.Quantity, line.Price = 2, 100000

	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	if len(res.OrderRows) != 2 {
		t.Fatalf("có %d dòng, muốn 2 (một dòng cho mỗi thành phẩm)", len(res.OrderRows))
	}
	if res.OrderRows[0].SKU != "TP111" || res.OrderRows[0].Qty != 2 {
		t.Errorf("thành phẩm 1 = %q × %v, muốn TP111 × 2", res.OrderRows[0].SKU, res.OrderRows[0].Qty)
	}
	if res.OrderRows[1].SKU != "TP222" || res.OrderRows[1].Qty != 4 {
		t.Errorf("thành phẩm 2 = %q × %v, muốn TP222 × 4", res.OrderRows[1].SKU, res.OrderRows[1].Qty)
	}
	// Sheet Haravan vẫn CHỈ 1 dòng cho dòng hàng này.
	if len(res.SheetRows) != 1 {
		t.Errorf("có %d dòng sheet, muốn 1", len(res.SheetRows))
	}
}

func TestBuildLooksUpByProductVariantWhenNoSKU(t *testing.T) {
	line := baseLine()
	line.SKU = ""
	line.Title, line.VariantTitle = "Không mã combo", "Loại A"
	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	if len(res.OrderRows) != 1 || res.OrderRows[0].SKU != "TP333" {
		t.Fatalf("không tra được theo Tên sản phẩm + Phân loại: %+v", res.OrderRows)
	}
}

func TestBuildCollectsMissingComboUnique(t *testing.T) {
	a := baseLine()
	a.SKU, a.Title, a.VariantTitle = "SP-CHUA-KHAI", "Sản phẩm lạ", "Loại lạ"
	b := a // đúng mã đó, dòng thứ hai
	res := Build([]OrderLine{a, b}, fakeTables(t), Options{})

	if len(res.Missing) != 1 {
		t.Fatalf("có %d mục thiếu, muốn 1 (gom unique theo khoá)", len(res.Missing))
	}
	m := res.Missing[0]
	if m.Key != MissingKey("SP-CHUA-KHAI", "Sản phẩm lạ", "Loại lạ") {
		t.Errorf("Key = %q", m.Key)
	}
	if m.LineCount != 2 {
		t.Errorf("LineCount = %d, muốn 2", m.LineCount)
	}
	if m.Combo != "SP-CHUA-KHAI" || m.Product != "Sản phẩm lạ" || m.Variant != "Loại lạ" {
		t.Errorf("mục thiếu thiếu thông tin điền sẵn: %+v", m)
	}
	// Vẫn phải sinh dòng dondathang mang #N/A, không được bỏ âm thầm.
	if len(res.OrderRows) != 2 {
		t.Fatalf("có %d dòng dondathang, muốn 2", len(res.OrderRows))
	}
	if res.OrderRows[0].SKU != lookup.NotAvailable {
		t.Errorf("SKU = %q, muốn %q", res.OrderRows[0].SKU, lookup.NotAvailable)
	}
	if res.SheetRows[0].TP[0] != lookup.NotAvailable {
		t.Errorf("sheet TP1 = %q, muốn %q", res.SheetRows[0].TP[0], lookup.NotAvailable)
	}
}

func TestBuildMissingShopKeepsNAAndCounts(t *testing.T) {
	line := baseLine()
	line.Shop = "Shop lạ chưa khai"
	res := Build([]OrderLine{line}, fakeTables(t), Options{})
	if res.OrderRows[0].CustomerCode != lookup.NotAvailable {
		t.Errorf("CustomerCode = %q, muốn %q", res.OrderRows[0].CustomerCode, lookup.NotAvailable)
	}
	if res.MissingShops["Shop lạ chưa khai"] != 1 {
		t.Errorf("MissingShops = %v, muốn đếm 1 dòng", res.MissingShops)
	}
}

func TestBuildClevyStaysInSheetButNotInDondathang(t *testing.T) {
	line := baseLine()
	line.Shop = ShopKhongQuyDoi
	res := Build([]OrderLine{line}, fakeTables(t), Options{})

	if len(res.SheetRows) != 1 {
		t.Fatalf("có %d dòng sheet, muốn 1 — đơn CLEVY vẫn nằm trong sheet Haravan", len(res.SheetRows))
	}
	if res.SheetRows[0].TP[0] != "" {
		t.Errorf("TP1 = %q, muốn TRỐNG (không quy đổi theo thiết kế, khác #N/A)", res.SheetRows[0].TP[0])
	}
	if len(res.OrderRows) != 0 {
		t.Errorf("có %d dòng dondathang, muốn 0", len(res.OrderRows))
	}
	if len(res.Missing) != 0 {
		t.Errorf("CLEVY không phải mã thiếu, không được hỏi người dùng: %+v", res.Missing)
	}
}
```

- [ ] **Step 2: Chạy test để xác nhận thất bại**

Run: `cd GO && go test ./internal/tmdt/ -run 'TestBuild|TestChannelLabel' -v`
Expected: FAIL — `undefined: Build`, `undefined: lookup.FromRows`, …

- [ ] **Step 3: Tách `lookup.FromRows` ra khỏi `lookup.Load`**

`lookup.Load` hiện đọc file rồi dựng bảng trong cùng một hàm. Test đơn vị cần dựng bảng từ dữ liệu trong bộ nhớ. Trong `GO/internal/tmdt/lookup/lookup.go`, tách phần dựng bảng ra:

```go
// FromRows dựng bảng tra cứu từ dữ liệu thô của hai sheet — cùng logic
// Load dùng, tách ra để test dựng bảng mà không cần file Excel thật.
// dataShop và misa là kết quả GetRows của hai sheet tương ứng, KỂ CẢ
// dòng tiêu đề.
func FromRows(dataShop, misa [][]string) (*Tables, error) {
	t := &Tables{
		byProductVariant: map[string]*ComboRow{},
		byCombo:          map[string]*ComboRow{},
		misa:             map[string]string{},
	}
	if len(dataShop) < 2 {
		return nil, fmt.Errorf("sheet %q không có dữ liệu", SheetDataShop)
	}
	for _, r := range dataShop[1:] {
		// ... chuyển nguyên khối vòng lặp hiện có trong Load vào đây ...
	}
	for i := 2; i < len(misa); i++ {
		// ... chuyển nguyên khối vòng lặp misa hiện có trong Load vào đây ...
	}
	if t.Combos == 0 || t.Misa == 0 {
		return nil, fmt.Errorf("thiếu dữ liệu tra cứu (data shop: %d dòng, Mã misa: %d dòng)", t.Combos, t.Misa)
	}
	return t, nil
}
```

`Load` rút gọn còn: mở file, `GetRows` hai sheet, gọi `FromRows`, bọc lỗi kèm tên file. Chạy `go test ./internal/tmdt/lookup/ -v` để chắc `lookup_test.go` cũ vẫn xanh sau khi tách.

- [ ] **Step 4: Viết `mapping.go`**

`GO/internal/tmdt/mapping.go`:

```go
package tmdt

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/tmdt/lookup"
)

// ShopKhongQuyDoi là shop CỐ Ý không quy đổi ra mã thành phẩm — công thức
// Excel cũ trả về rỗng cho toàn bộ cột MÃ TP/SLTP của shop này, và nhánh
// mới giữ nguyên quy tắc đó. Phân biệt rõ với "chưa khai báo": ô trống là
// đúng thiết kế, còn #N/A là thứ cần hỏi người dùng.
const ShopKhongQuyDoi = "CLEVY VIỆT NAM"

// vatDivisor: giá trên sàn đã gồm VAT 8%, còn cột Đơn giá của AMIS là giá
// chưa thuế.
const vatDivisor = 1.08

// OrderLine là MỘT dòng hàng đã tách khỏi kiểu dữ liệu của Haravan. Tầng
// quy đổi cố ý KHÔNG nhận *haravan.Order: nhờ vậy golden test nạp được
// 1.585 dòng thật từ CSV mà không cần mạng, và tầng này không phụ thuộc
// vào hình dạng JSON của một API bên ngoài.
type OrderLine struct {
	OrderCode    string    // Mã đơn hàng trên sàn
	Shop         string    // tên shop (note attribute BranchName)
	KhoBan       string    // location_name thô, ví dụ "Kho Hà Nội"
	KenhBanHang  string    // source_name thô, ví dụ "tiktokshop"
	CreatedAt    time.Time // đã ở giờ VN
	Quantity     float64
	Title        string
	VariantTitle string
	Price        float64 // giá 1 đơn vị dòng hàng, đã gồm VAT
	Subtotal     float64 // Tổng tiền của đơn — chỉ để ghi ra sheet
	Total        float64 // Tổng cộng của đơn — chỉ để ghi ra sheet
	SKU          string
	Attributes   string
}

// SheetRow là một dòng của sheet "Haravan" — một dòng hàng, một dòng sheet
// (KHÔNG tách theo thành phẩm như dondathang).
type SheetRow struct {
	OrderCode    string
	Subtotal     float64
	Total        float64
	OrderDate    string
	Quantity     float64
	Title        string
	VariantTitle string
	Price        float64
	SKU          string
	Attributes   string
	KhoBan       string
	KenhBanHang  string
	CreatedAt    time.Time
	TP           [4]string
	SL           [4]string
	Shop         string
	Misa         string
}

// MissingCombo là một mã CHƯA khai báo trong sheet "data shop", đã gom
// unique — 300 dòng cùng thiếu một mã chỉ thành một mục.
type MissingCombo struct {
	Key       string `json:"key"`
	Product   string `json:"product"`
	Variant   string `json:"variant"`
	Combo     string `json:"combo"`
	LineCount int    `json:"lineCount"`
}

type Options struct {
	// ProductName tra tên hàng theo mã thành phẩm (cột S). Bản thật truyền
	// productdata.Store.GetProductInfo; nil thì để trống tên hàng.
	ProductName func(tp string) string
}

type Result struct {
	SheetRows    []SheetRow
	OrderRows    []excelwriter.TMDTRow
	Missing      []MissingCombo
	MissingShops map[string]int
}

// MissingKey là khoá gom nhóm mã thiếu: có Mã sản phẩm thì dùng nó, không
// có thì ghép Tên sản phẩm + Phân loại — ĐÚNG hai nhánh mà bảng tra cứu
// dùng để tra, nên khoá luôn tương ứng 1-1 với một dòng "data shop" cần bổ sung.
func MissingKey(sku, title, variant string) string {
	if s := strings.TrimSpace(sku); s != "" {
		return "sku:" + s
	}
	return "pv:" + strings.TrimSpace(title) + "|" + strings.TrimSpace(variant)
}

// ChannelLabel chuẩn hoá tên sàn về đúng dạng dùng trong cột "Số đơn hàng"
// và "Diễn giải": "tiktokshop" / "TikTok Shop" đều thành "TikTok".
func ChannelLabel(raw string) string {
	s := strings.TrimSpace(raw)
	low := strings.ToLower(s)
	switch {
	case strings.Contains(low, "tiktok") || strings.Contains(low, "tik tok") || low == "tts":
		return "TikTok"
	case strings.Contains(low, "shopee") || strings.Contains(low, "shoppe") || low == "spx":
		return "Shopee"
	}
	return s
}

// warehouseOf quy đổi tên kho của Haravan ra bộ 4 mã mà AMIS cần.
func warehouseOf(khoBan string) (shipTo, maKho, maDonVi string) {
	if strings.EqualFold(strings.TrimSpace(khoBan), "Kho Hà Nội") {
		return "HN", "TP_HN_12", "TMĐT_MB"
	}
	return "LA", "LA_KHOTMDT", "TMĐT_MN"
}

func Build(lines []OrderLine, tables *lookup.Tables, opt Options) Result {
	res := Result{MissingShops: map[string]int{}}
	missingIdx := map[string]int{} // khoá → vị trí trong res.Missing

	for _, line := range lines {
		shop := strings.TrimSpace(line.Shop)
		misa, ok := tables.MisaCode(shop)
		if !ok {
			misa = lookup.NotAvailable
			res.MissingShops[shop]++
		}

		sheet := SheetRow{
			OrderCode: line.OrderCode, Subtotal: line.Subtotal, Total: line.Total,
			OrderDate: line.CreatedAt.Format(time.RFC3339), Quantity: line.Quantity,
			Title: line.Title, VariantTitle: line.VariantTitle, Price: line.Price,
			SKU: line.SKU, Attributes: line.Attributes, KhoBan: line.KhoBan,
			KenhBanHang: line.KenhBanHang, CreatedAt: line.CreatedAt,
			Shop: shop, Misa: misa,
		}

		noConvert := strings.EqualFold(shop, ShopKhongQuyDoi)
		var combo *lookup.ComboRow
		found := false
		if !noConvert {
			if strings.TrimSpace(line.SKU) == "" {
				combo, found = tables.ByProductVariant(line.Title, line.VariantTitle)
			} else {
				combo, found = tables.ByCombo(line.SKU)
			}
			if !found {
				key := MissingKey(line.SKU, line.Title, line.VariantTitle)
				if i, seen := missingIdx[key]; seen {
					res.Missing[i].LineCount++
				} else {
					missingIdx[key] = len(res.Missing)
					res.Missing = append(res.Missing, MissingCombo{
						Key: key, Product: strings.TrimSpace(line.Title),
						Variant: strings.TrimSpace(line.VariantTitle),
						Combo:   strings.TrimSpace(line.SKU), LineCount: 1,
					})
				}
				for i := 0; i < 4; i++ {
					sheet.TP[i], sheet.SL[i] = lookup.NotAvailable, lookup.NotAvailable
				}
			} else {
				for i := 0; i < 4; i++ {
					sheet.TP[i] = blankIfZero(combo.TP[i])
					sheet.SL[i] = blankIfZero(combo.SL[i])
				}
			}
		}
		res.SheetRows = append(res.SheetRows, sheet)

		if noConvert {
			// Cố ý không sinh dòng đặt hàng: không có mã thành phẩm để ghi.
			continue
		}
		res.OrderRows = append(res.OrderRows, orderRowsFor(line, sheet, opt)...)
	}
	return res
}

// orderRowsFor sinh dòng dondathang cho MỘT dòng hàng: một dòng cho mỗi
// thành phẩm có mã. Mã chưa khai báo (#N/A) vẫn sinh ĐÚNG MỘT dòng mang
// #N/A — bỏ âm thầm vài trăm dòng khỏi file hạch toán nguy hiểm hơn nhiều
// so với một ô #N/A mà AMIS sẽ báo lỗi ngay khi import.
func orderRowsFor(line OrderLine, sheet SheetRow, opt Options) []excelwriter.TMDTRow {
	channel := ChannelLabel(firstNonEmptyStr(line.KenhBanHang, sheet.KenhBanHang))
	shipTo, maKho, maDonVi := warehouseOf(line.KhoBan)
	date := line.CreatedAt.Format("02/01/2006")
	desc := fmt.Sprintf("TMĐT-%s - %s - %s - Ngày đổ %s - %s",
		channel, sheet.Shop, line.OrderCode, date, shipTo)

	base := excelwriter.TMDTRow{
		EntryDate:    date,
		OrderNumber:  fmt.Sprintf("ĐĐHTMĐT-%s-%s", channel, line.OrderCode),
		ShipTo:       shipTo,
		CustomerCode: sheet.Misa,
		Description:  desc,
		IsPromoItem:  line.Price == 0,
		Warehouse:    maKho,
		RegionCode:   maDonVi,
		StatCode:     shipTo,
		Note:         line.OrderCode,
	}

	name := func(tp string) string {
		if opt.ProductName == nil {
			return ""
		}
		return opt.ProductName(tp)
	}

	if sheet.TP[0] == lookup.NotAvailable {
		row := base
		row.SKU = lookup.NotAvailable
		// Chưa biết SLTP nên giữ nguyên số lượng đặt và giá của dòng hàng.
		row.Qty = line.Quantity
		row.UnitPrice = line.Price / vatDivisor
		return []excelwriter.TMDTRow{row}
	}

	var out []excelwriter.TMDTRow
	for i := 0; i < 4; i++ {
		tp := sheet.TP[i]
		if tp == "" {
			continue
		}
		sl := parseSL(sheet.SL[i])
		if sl == 0 {
			continue
		}
		row := base
		row.SKU = tp
		row.ProductName = name(tp)
		row.Qty = line.Quantity * sl
		row.UnitPrice = line.Price / sl / vatDivisor
		out = append(out, row)
	}
	return out
}

// parseSL đọc SLTP. Bảng tra cứu là do người dùng gõ tay nên giá trị có
// thể kèm khoảng trắng hoặc xuống dòng; giá trị không đọc được coi như 0
// (bỏ qua thành phẩm đó) thay vì làm hỏng cả lần chạy.
func parseSL(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// blankIfZero: công thức cũ bọc kết quả trong IF(KQ=0,"",KQ) nên giá trị 0
// hiển thị thành rỗng. Cũng cắt luôn khoảng trắng/xuống dòng thừa mà bảng
// tra cứu gõ tay hay dính (ví dụ "TP10127\n").
func blankIfZero(s string) string {
	s = strings.TrimSpace(s)
	if s == "0" {
		return ""
	}
	return s
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
```

- [ ] **Step 5: Chạy test đơn vị để xác nhận đạt**

Run: `cd GO && go test ./internal/tmdt/... -run 'TestBuild|TestChannelLabel|TestIsWorkbook' -v`
Expected: PASS toàn bộ.

- [ ] **Step 6: Viết golden test 1.430 dòng**

`GO/internal/tmdt/mapping_golden_test.go`:

```go
package tmdt

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const goldenDir = "testdata/golden"

func readCSV(t *testing.T, name string) ([]string, [][]string) {
	t.Helper()
	fh, err := os.Open(filepath.Join(goldenDir, name))
	if err != nil {
		t.Fatalf("mở %s: %v", name, err)
	}
	defer fh.Close()
	r := csv.NewReader(fh)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("đọc %s: %v", name, err)
	}
	if len(rows) < 2 {
		t.Fatalf("%s không có dữ liệu", name)
	}
	return rows[0], rows[1:]
}

// parseVNTime chấp nhận các biến thể ngày mà Excel/Haravan sinh ra.
func parseVNTime(t *testing.T, s string) time.Time {
	t.Helper()
	s = strings.TrimSpace(s)
	layouts := []string{
		time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05-07:00", "2006-01-02T15:04:05", "2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if v, err := time.Parse(l, s); err == nil {
			return v
		}
	}
	t.Fatalf("không parse được thời gian %q", s)
	return time.Time{}
}

func num(t *testing.T, s string) float64 {
	t.Helper()
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("không parse được số %q: %v", s, err)
	}
	return v
}

func TestBuildGoldenAgainstMauChuan(t *testing.T) {
	_, orderRows := readCSV(t, "orders.csv")
	lines := make([]OrderLine, 0, len(orderRows))
	for _, r := range orderRows {
		// order_code,shop,kho_ban,kenh_ban_hang,ngay_dat_hang,so_luong,
		// ten_san_pham,gia_tri_thuoc_tinh_1,gia_san_pham,ma_san_pham
		lines = append(lines, OrderLine{
			OrderCode: r[0], Shop: r[1], KhoBan: r[2], KenhBanHang: r[3],
			CreatedAt: parseVNTime(t, r[4]), Quantity: num(t, r[5]),
			Title: r[6], VariantTitle: r[7], Price: num(t, r[8]), SKU: r[9],
		})
	}
	if len(lines) != 1585 {
		t.Fatalf("fixture orders.csv có %d dòng, muốn 1585 — sinh lại fixture", len(lines))
	}

	tables, err := lookup.Load(filepath.Join(goldenDir, "lookup.xlsx"))
	if err != nil {
		t.Fatalf("nạp bảng tra cứu: %v", err)
	}

	// ProductName dựng từ fixture: cột S tra productdata.Store (qua mạng),
	// không suy được từ dữ liệu Haravan — xem testdata/golden/README.md.
	_, nameRows := readCSV(t, "ten_hang.csv")
	names := map[string]string{}
	for _, r := range nameRows {
		names[r[0]] = r[1]
	}

	got := Build(lines, tables, Options{ProductName: func(tp string) string { return names[tp] }})

	if len(got.SheetRows) != 1585 {
		t.Errorf("SheetRows = %d, muốn 1585 (một dòng sheet cho mỗi dòng hàng)", len(got.SheetRows))
	}
	if len(got.Missing) != 0 {
		t.Errorf("golden không được thiếu mã nào, nhưng thiếu %d: %+v", len(got.Missing), got.Missing)
	}

	_, want := readCSV(t, "expected_dondathang.csv")
	if len(got.OrderRows) != len(want) {
		t.Fatalf("OrderRows = %d, muốn %d", len(got.OrderRows), len(want))
	}

	// Cột trong expected_dondathang.csv, đúng thứ tự header:
	// A,B,C,D,E,G,L,Q,T,U,V,X,Y,AE,AJ,AM,AO,AV
	for i, w := range want {
		g := got.OrderRows[i]
		check := func(col, gotVal, wantVal string) {
			t.Helper()
			if gotVal != wantVal {
				t.Errorf("dòng %d cột %s: được %q, muốn %q", i+1, col, gotVal, wantVal)
			}
		}
		check("A", g.EntryDate, w[0])
		check("B", g.OrderNumber, w[1])
		check("C", "Chưa thực hiện", w[2])
		check("D", g.EntryDate, w[3])
		check("E", g.ShipTo, w[4])
		check("G", g.CustomerCode, w[5])
		check("L", g.Description, w[6])
		check("Q", g.SKU, w[7])
		check("T", "Không", w[8])
		promo := "Không"
		if g.IsPromoItem {
			promo = "Có"
		}
		check("U", promo, w[9])
		check("V", g.Warehouse, w[10])
		if math.Abs(g.Qty-num(t, w[11])) > 1e-9 {
			t.Errorf("dòng %d cột X: được %v, muốn %v", i+1, g.Qty, w[11])
		}
		if math.Abs(g.UnitPrice-num(t, w[12])) > 1e-6 {
			t.Errorf("dòng %d cột Y: được %v, muốn %v", i+1, g.UnitPrice, w[12])
		}
		check("AE", "8", strings.TrimSpace(w[13]))
		check("AJ", g.RegionCode, w[14])
		check("AM", g.StatCode, w[15])
		check("AO", g.Note, w[16])
		check("AV", "15", strings.TrimSpace(w[17]))
		if t.Failed() && i > 5 {
			t.Fatalf("dừng sớm sau %d dòng lệch — sửa quy tắc rồi chạy lại", i+1)
		}
	}
	_ = fmt.Sprint
}
```

Thêm `"order-processor/internal/tmdt/lookup"` vào khối import của file này.

- [ ] **Step 7: Chạy golden test**

Run: `cd GO && go test ./internal/tmdt/ -run TestBuildGolden -v 2>&1 | head -40`
Expected: PASS. Nếu FAIL, đọc dòng lệch đầu tiên — thông báo in đủ số dòng, tên cột, giá trị được và giá trị muốn. **Sửa `mapping.go` cho khớp mẫu chuẩn, tuyệt đối không sửa fixture để test xanh.**

Hai điểm lệch dự kiến hay gặp và cách xử lý:
- **Thứ tự dòng**: mẫu chuẩn xếp theo đúng thứ tự dòng của sheet `Đơn hàng haravan`. `Build` cũng lặp theo thứ tự `lines`, nên hai bên khớp. Nếu lệch, kiểm lại script fixture chứ không sắp xếp lại trong `Build`.
- **Đơn giá lệch ở chữ số cuối**: đó là sai số dấu phẩy động, đã cho dung sai `1e-6`. Lệch lớn hơn nghĩa là công thức sai, không phải sai số.

- [ ] **Step 8: Commit**

```bash
git add GO/internal/tmdt/mapping.go GO/internal/tmdt/mapping_test.go GO/internal/tmdt/mapping_golden_test.go GO/internal/tmdt/lookup/lookup.go
git commit -m "feat(tmdt): tầng quy đổi đơn TMĐT, khớp 1430/1430 dòng mẫu chuẩn" -- GO/internal/tmdt/mapping.go GO/internal/tmdt/mapping_test.go GO/internal/tmdt/mapping_golden_test.go GO/internal/tmdt/lookup/lookup.go
```

---

## Task 7: Ghi sheet `Haravan` vào workbook đang có

**Files:**
- Create: `GO/internal/tmdt/sheet.go`
- Test: `GO/internal/tmdt/sheet_test.go`

**Interfaces:**
- Consumes: `tmdt.SheetRow` (Task 6), `tmdt.SheetHaravan` (Task 2)
- Produces: `tmdt.HaravanHeaders []string` (23 phần tử), `tmdt.WriteHaravanSheet(path string, rows []SheetRow) error`

- [ ] **Step 1: Viết test thất bại**

`GO/internal/tmdt/sheet_test.go`:

```go
package tmdt

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestWriteHaravanSheetReplacesContent(t *testing.T) {
	// Workbook giống file thật: 2 bảng tra cứu + 1 sheet Haravan có rác cũ.
	f := excelize.NewFile()
	for _, s := range []string{"Mã misa", "data shop", SheetHaravan} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet(%q): %v", s, err)
		}
	}
	f.DeleteSheet("Sheet1")
	if err := f.SetCellValue("data shop", "A1", "Tên sản phẩm"); err != nil {
		t.Fatalf("SetCellValue: %v", err)
	}
	// Rác của lần chạy trước, phải bị xoá sạch.
	for r := 1; r <= 40; r++ {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		if err := f.SetCellValue(SheetHaravan, cell, "rác cũ"); err != nil {
			t.Fatalf("SetCellValue: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "XUẤT HÀNG HN-LA MỚI.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	f.Close()

	rows := []SheetRow{{
		OrderCode: "585694438276170905", Subtotal: 29000, Total: 29000,
		OrderDate: "2026-08-23T23:56:56+07:00", Quantity: 1,
		Title: "Bột Tẩy Lồng", VariantTitle: "1 TÚI", Price: 29000,
		SKU: "", Attributes: "Tên : Giá trị", KhoBan: "Kho Hà Nội",
		KenhBanHang: "tiktokshop",
		CreatedAt:   time.Date(2026, 8, 23, 23, 56, 56, 0, time.FixedZone("ICT", 7*3600)),
		TP:          [4]string{"TP10127", "", "", ""},
		SL:          [4]string{"1", "", "", ""},
		Shop:        "Tẩy lồng máy giặt Blue", Misa: "MN_TMDT_00016",
	}}
	if err := WriteHaravanSheet(path, rows); err != nil {
		t.Fatalf("WriteHaravanSheet: %v", err)
	}

	got, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại: %v", err)
	}
	defer got.Close()

	// Hai bảng tra cứu KHÔNG được mất — người dùng khai sản phẩm ở đó.
	if v, _ := got.GetCellValue("data shop", "A1"); v != "Tên sản phẩm" {
		t.Errorf("data shop!A1 = %q — bảng tra cứu bị hỏng", v)
	}
	if _, err := got.GetSheetIndex("Mã misa"); err != nil {
		t.Errorf("sheet Mã misa biến mất: %v", err)
	}

	// Tiêu đề đúng 23 cột.
	if len(HaravanHeaders) != 23 {
		t.Fatalf("HaravanHeaders có %d cột, muốn 23", len(HaravanHeaders))
	}
	for i, h := range HaravanHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if v, _ := got.GetCellValue(SheetHaravan, cell); v != h {
			t.Errorf("%s = %q, muốn %q", cell, v, h)
		}
	}

	want := map[string]string{
		"A2": "585694438276170905", "E2": "1", "F2": "Bột Tẩy Lồng",
		"G2": "1 TÚI", "H2": "29000", "I2": "", "J2": "Tên : Giá trị",
		"K2": "Kho Hà Nội", "L2": "tiktokshop",
		"N2": "TP10127", "O2": "1", "P2": "",
		"V2": "Tẩy lồng máy giặt Blue", "W2": "MN_TMDT_00016",
	}
	for cell, expect := range want {
		v, err := got.GetCellValue(SheetHaravan, cell)
		if err != nil {
			t.Fatalf("GetCellValue(%s): %v", cell, err)
		}
		if v != expect {
			t.Errorf("%s = %q, muốn %q", cell, v, expect)
		}
	}

	// Rác cũ ở dòng 3..40 phải sạch.
	for r := 3; r <= 40; r++ {
		cell, _ := excelize.CoordinatesToCellName(1, r)
		if v, _ := got.GetCellValue(SheetHaravan, cell); v != "" {
			t.Errorf("%s = %q — rác của lần chạy trước chưa bị xoá", cell, v)
		}
	}
}

func TestWriteHaravanSheetCreatesSheetWhenMissing(t *testing.T) {
	f := excelize.NewFile()
	for _, s := range []string{"Mã misa", "data shop"} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet: %v", err)
		}
	}
	f.DeleteSheet("Sheet1")
	path := filepath.Join(t.TempDir(), "khong-co-sheet-haravan.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	f.Close()

	if err := WriteHaravanSheet(path, nil); err != nil {
		t.Fatalf("WriteHaravanSheet: %v", err)
	}
	got, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại: %v", err)
	}
	defer got.Close()
	if v, _ := got.GetCellValue(SheetHaravan, "A1"); v != HaravanHeaders[0] {
		t.Errorf("sheet Haravan chưa được tạo kèm tiêu đề, A1 = %q", v)
	}
}
```

- [ ] **Step 2: Chạy test để xác nhận thất bại**

Run: `cd GO && go test ./internal/tmdt/ -run TestWriteHaravanSheet -v`
Expected: FAIL — `undefined: WriteHaravanSheet`, `undefined: HaravanHeaders`.

- [ ] **Step 3: Viết bản cài đặt tối thiểu**

`GO/internal/tmdt/sheet.go`:

```go
package tmdt

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// HaravanHeaders là 23 cột của bố cục "chuẩn" — giống hệt standardHeaders
// trong internal/tmdt/export/standard.go (bản CLI đã đối chiếu 100% với
// file người dùng làm tay). Giữ hai bản riêng thay vì dùng chung một biến:
// bản của export thuộc về StreamWriter ghi ra FILE MỚI, còn bản này ghi
// vào MỘT SHEET của workbook đang có — hai đường ghi khác nhau, và nếu mai
// này bố cục CLI đổi thì sheet trong workbook của người dùng không được đổi theo.
var HaravanHeaders = []string{
	"Mã đơn hàng", "Tổng tiền", "Tổng cộng", "Ngày đặt hàng",
	"Số lượng sản phẩm", "Tên sản phẩm", "Giá trị thuộc tính 1",
	"Giá sản phẩm", "Mã sản phẩm", "Thuộc tính", "Kho bán", "Kênh bán hàng",
	"Thời gian Đặt",
	"MÃ TP 1", "SLTP1", "MÃ TP 2", "SLTP2", "MÃ TP 3", "SLTP3", "MÃ TP 4", "SLTP4",
	"Shop", "Mã misa",
}

// WriteHaravanSheet ghi ĐÈ sheet "Haravan" của workbook tại path: xoá hẳn
// sheet cũ rồi tạo lại, nên rác của lần chạy trước không sót dòng nào (xoá
// từng ô sẽ để lại kiểu dáng và vùng dimension cũ). Hai sheet tra cứu
// "data shop" / "Mã misa" không bị đụng tới — đó là nơi người dùng khai
// sản phẩm, mất là mất dữ liệu thật.
func WriteHaravanSheet(path string, rows []SheetRow) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("tmdt: mở %s: %w", path, err)
	}
	defer f.Close()

	if idx, err := f.GetSheetIndex(SheetHaravan); err == nil && idx >= 0 {
		if err := f.DeleteSheet(SheetHaravan); err != nil {
			return fmt.Errorf("tmdt: xoá sheet %s: %w", SheetHaravan, err)
		}
	}
	if _, err := f.NewSheet(SheetHaravan); err != nil {
		return fmt.Errorf("tmdt: tạo sheet %s: %w", SheetHaravan, err)
	}

	header := make([]interface{}, len(HaravanHeaders))
	for i, h := range HaravanHeaders {
		header[i] = h
	}
	if err := f.SetSheetRow(SheetHaravan, "A1", &header); err != nil {
		return fmt.Errorf("tmdt: ghi tiêu đề: %w", err)
	}

	for i, r := range rows {
		cells := []interface{}{
			r.OrderCode, r.Subtotal, r.Total, r.OrderDate, r.Quantity,
			r.Title, r.VariantTitle, r.Price, r.SKU, r.Attributes,
			r.KhoBan, r.KenhBanHang, r.CreatedAt.Format("02/01/2006 15:04:05"),
			r.TP[0], r.SL[0], r.TP[1], r.SL[1], r.TP[2], r.SL[2], r.TP[3], r.SL[3],
			r.Shop, r.Misa,
		}
		axis := fmt.Sprintf("A%d", i+2)
		if err := f.SetSheetRow(SheetHaravan, axis, &cells); err != nil {
			return fmt.Errorf("tmdt: ghi dòng %d: %w", i+2, err)
		}
	}

	if err := f.Save(); err != nil {
		return fmt.Errorf("tmdt: lưu %s: %w", path, err)
	}
	return nil
}
```

- [ ] **Step 4: Chạy test để xác nhận đạt**

Run: `cd GO && go test ./internal/tmdt/ -v`
Expected: PASS toàn bộ (gồm golden test của Task 6).

- [ ] **Step 5: Commit**

```bash
git add GO/internal/tmdt/sheet.go GO/internal/tmdt/sheet_test.go
git commit -m "feat(tmdt): ghi đè sheet Haravan trong workbook tra cứu" -- GO/internal/tmdt/sheet.go GO/internal/tmdt/sheet_test.go
```

---

## Task 8: Bổ sung dòng vào sheet `data shop`

**Files:**
- Modify: `GO/internal/tmdt/lookup/lookup.go`
- Test: `GO/internal/tmdt/lookup/append_test.go`

**Interfaces:**
- Consumes: `lookup.ComboRow`, `lookup.SheetDataShop`
- Produces: `lookup.AppendComboRows(path string, rows []ComboRow) (firstRow int, err error)`

- [ ] **Step 1: Viết test thất bại**

`GO/internal/tmdt/lookup/append_test.go`:

```go
package lookup

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// newLookupWorkbook dựng workbook có 2 bảng tra cứu tối thiểu: 1 dòng
// tiêu đề + 2 dòng dữ liệu ở "data shop", 1 shop ở "Mã misa".
func newLookupWorkbook(t *testing.T) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for _, s := range []string{SheetDataShop, SheetMisa} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet(%q): %v", s, err)
		}
	}
	f.DeleteSheet("Sheet1")

	dataShop := [][]interface{}{
		{"Tên sản phẩm", "Phân loại", "Mã combo", "MÃ TP 1", "SLTP1", "MÃ TP 2", "SLTP2", "MÃ TP 3", "SLTP3", "MÃ TP 4", "SLTP4"},
		{"Sản phẩm A", "Loại 1", "SP001", "TP001", "1", "", "", "", "", "", ""},
		{"Sản phẩm B", "Loại 2", "SP002", "TP002", "2", "", "", "", "", "", ""},
	}
	for i, row := range dataShop {
		r := row
		if err := f.SetSheetRow(SheetDataShop, sprintfAxis(i+1), &r); err != nil {
			t.Fatalf("SetSheetRow: %v", err)
		}
	}
	misa := [][]interface{}{
		{"", "Tên Kênh", "KÊNH BÁN", "Mã MISA"},
		{"", "", "", ""},
		{"", "Shop X", "SHOPEE", "MN_TMDT_00001"},
	}
	for i, row := range misa {
		r := row
		if err := f.SetSheetRow(SheetMisa, sprintfAxis(i+1), &r); err != nil {
			t.Fatalf("SetSheetRow: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "lookup.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	return path
}

func sprintfAxis(row int) string {
	cell, _ := excelize.CoordinatesToCellName(1, row)
	return cell
}

func TestAppendComboRows(t *testing.T) {
	path := newLookupWorkbook(t)

	firstRow, err := AppendComboRows(path, []ComboRow{{
		Product: "Sản phẩm mới", Variant: "Combo 3 Túi", Combo: "SP777",
		TP:      [4]string{"TP777", "TP888", "", ""},
		SL:      [4]string{"3", "1", "", ""},
	}})
	if err != nil {
		t.Fatalf("AppendComboRows: %v", err)
	}
	if firstRow != 4 {
		t.Fatalf("firstRow = %d, muốn 4 (ngay dưới 3 dòng đã có)", firstRow)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại: %v", err)
	}
	defer f.Close()

	want := map[string]string{
		"A4": "Sản phẩm mới", "B4": "Combo 3 Túi", "C4": "SP777",
		"D4": "TP777", "E4": "3", "F4": "TP888", "G4": "1",
		"H4": "", "I4": "", "J4": "", "K4": "",
	}
	for cell, expect := range want {
		got, _ := f.GetCellValue(SheetDataShop, cell)
		if got != expect {
			t.Errorf("%s = %q, muốn %q", cell, got, expect)
		}
	}
	// Dòng cũ không được đụng tới.
	if got, _ := f.GetCellValue(SheetDataShop, "A2"); got != "Sản phẩm A" {
		t.Errorf("A2 = %q — dòng có sẵn bị ghi đè", got)
	}
	f.Close()

	// Nạp lại: mã mới phải tra được ngay, cả hai nhánh tra.
	tb, err := Load(path)
	if err != nil {
		t.Fatalf("Load sau khi bổ sung: %v", err)
	}
	row, ok := tb.ByCombo("SP777")
	if !ok {
		t.Fatalf("ByCombo(SP777) không tìm thấy")
	}
	if row.TP[0] != "TP777" || row.SL[1] != "1" {
		t.Errorf("dòng vừa bổ sung sai: %+v", row)
	}
	if _, ok := tb.ByProductVariant("Sản phẩm mới", "Combo 3 Túi"); !ok {
		t.Errorf("ByProductVariant không tìm thấy dòng vừa bổ sung")
	}
}

func TestAppendComboRowsSkipsTrailingBlankRows(t *testing.T) {
	// Bảng gõ tay hay có dòng trống ở cuối; dòng mới phải chèn ngay dưới
	// dòng CÓ DỮ LIỆU cuối cùng, không nhảy xuống sau vùng trống.
	path := newLookupWorkbook(t)
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở: %v", err)
	}
	if err := f.SetCellValue(SheetDataShop, "A9", ""); err != nil {
		t.Fatalf("SetCellValue: %v", err)
	}
	if err := f.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.Close()

	firstRow, err := AppendComboRows(path, []ComboRow{{Product: "X", Combo: "SPX", TP: [4]string{"TPX"}, SL: [4]string{"1"}}})
	if err != nil {
		t.Fatalf("AppendComboRows: %v", err)
	}
	if firstRow != 4 {
		t.Errorf("firstRow = %d, muốn 4 — dòng trống cuối bảng không được tính", firstRow)
	}
}

func TestAppendComboRowsEmptyIsNoop(t *testing.T) {
	path := newLookupWorkbook(t)
	if _, err := AppendComboRows(path, nil); err != nil {
		t.Fatalf("AppendComboRows(nil): %v", err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("workbook bị hỏng sau lần gọi rỗng: %v", err)
	}
}
```

- [ ] **Step 2: Chạy test để xác nhận thất bại**

Run: `cd GO && go test ./internal/tmdt/lookup/ -run TestAppendComboRows -v`
Expected: FAIL — `undefined: AppendComboRows`.

- [ ] **Step 3: Viết bản cài đặt tối thiểu**

Thêm vào cuối `GO/internal/tmdt/lookup/lookup.go`:

```go
// AppendComboRows ghi tiếp các dòng khai báo mới vào sheet "data shop",
// đúng cột A..K, ngay dưới dòng CÓ DỮ LIỆU cuối cùng — trả về số dòng đầu
// tiên đã ghi.
//
// Vì sao phải tự dò dòng cuối thay vì dùng len(GetRows(...)): bảng này do
// người dùng gõ tay, thường có dòng trống lẫn ở cuối, và excelize đếm cả
// dòng chỉ mang kiểu dáng. Ghi xuống sau vùng trống sẽ tạo một khoảng hở
// giữa bảng, khiến chính người dùng khó rà soát về sau.
func AppendComboRows(path string, rows []ComboRow) (firstRow int, err error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, fmt.Errorf("lookup: mở %s: %w", path, err)
	}
	defer f.Close()

	existing, err := f.GetRows(SheetDataShop)
	if err != nil {
		return 0, fmt.Errorf("lookup: đọc sheet %q: %w", SheetDataShop, err)
	}
	lastData := 1 // ít nhất là dòng tiêu đề
	for i, r := range existing {
		for _, cell := range r {
			if strings.TrimSpace(cell) != "" {
				lastData = i + 1
				break
			}
		}
	}
	firstRow = lastData + 1
	if len(rows) == 0 {
		return firstRow, nil
	}

	current := firstRow
	for _, row := range rows {
		cells := []interface{}{
			row.Product, row.Variant, row.Combo,
			row.TP[0], row.SL[0], row.TP[1], row.SL[1],
			row.TP[2], row.SL[2], row.TP[3], row.SL[3],
		}
		axis, cellErr := excelize.CoordinatesToCellName(1, current)
		if cellErr != nil {
			return 0, fmt.Errorf("lookup: tính ô dòng %d: %w", current, cellErr)
		}
		if err := f.SetSheetRow(SheetDataShop, axis, &cells); err != nil {
			return 0, fmt.Errorf("lookup: ghi dòng %d vào %q: %w", current, SheetDataShop, err)
		}
		current++
	}

	if err := f.Save(); err != nil {
		return 0, fmt.Errorf("lookup: lưu %s: %w", path, err)
	}
	return firstRow, nil
}
```

- [ ] **Step 4: Chạy test để xác nhận đạt**

Run: `cd GO && go test ./internal/tmdt/... -v 2>&1 | tail -20`
Expected: PASS toàn bộ.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/tmdt/lookup/lookup.go GO/internal/tmdt/lookup/append_test.go
git commit -m "feat(lookup): bổ sung dòng khai báo mới vào sheet data shop" -- GO/internal/tmdt/lookup/lookup.go GO/internal/tmdt/lookup/append_test.go
```

---

## Task 9: Ràng buộc khoảng ngày (hàm thuần, frontend)

**Files:**
- Create: `GO/frontend/src/lib/tmdtDateRange.ts`
- Test: `GO/frontend/src/lib/tmdtDateRange.test.ts`
- Modify: `GO/frontend/package.json` (thêm file test vào script `test`)

**Interfaces:**
- Consumes: —
- Produces: `TMDTDateRange{from: string; to: string}` (chuỗi `YYYY-MM-DD`), `MAX_RANGE_DAYS = 7`, `toISODate(d: Date): string`, `parseISODate(s: string): Date`, `addDays(d: Date, n: number): Date`, `maxSelectableDate(today: Date): Date`, `isSelectableDay(day: Date, today: Date, anchor: string | null): boolean`, `presetRange(preset: 'yesterday' | '3days' | '7days', today: Date): TMDTDateRange`, `normalizeRange(a: string, b: string): TMDTDateRange`, `validateRange(range: TMDTDateRange, today: Date): string | null`, `formatRangeLabel(range: TMDTDateRange): string`

- [ ] **Step 1: Viết test thất bại**

`GO/frontend/src/lib/tmdtDateRange.test.ts`:

```ts
import { strict as assert } from 'node:assert'
import { test } from 'node:test'
import {
  MAX_RANGE_DAYS,
  addDays,
  formatRangeLabel,
  isSelectableDay,
  maxSelectableDate,
  normalizeRange,
  parseISODate,
  presetRange,
  toISODate,
  validateRange,
} from './tmdtDateRange.ts'

// Mốc "hôm nay" cố định cho mọi test: 25/08/2026.
const today = parseISODate('2026-08-25')

test('maxSelectableDate là hôm qua, không phải hôm nay', () => {
  assert.equal(toISODate(maxSelectableDate(today)), '2026-08-24')
})

test('isSelectableDay chặn hôm nay và tương lai', () => {
  assert.equal(isSelectableDay(parseISODate('2026-08-24'), today, null), true)
  assert.equal(isSelectableDay(parseISODate('2026-08-25'), today, null), false)
  assert.equal(isSelectableDay(parseISODate('2026-08-26'), today, null), false)
})

test('isSelectableDay chặn ngày cách mốc đã chọn quá 6 ngày', () => {
  const anchor = '2026-08-18'
  // 18 → 24 là đúng 7 ngày tính cả hai đầu: hợp lệ.
  assert.equal(isSelectableDay(parseISODate('2026-08-24'), today, anchor), true)
  // 17 → 18 ... đi về phía trước cũng vẫn trong 7 ngày.
  assert.equal(isSelectableDay(parseISODate('2026-08-12'), today, anchor), true)
  // Ngày thứ 8 ở cả hai phía: chặn.
  assert.equal(isSelectableDay(parseISODate('2026-08-11'), today, anchor), false)
  // Phía sau bị chặn bởi ràng buộc "≤ hôm qua" trước cả ràng buộc 7 ngày.
  assert.equal(isSelectableDay(parseISODate('2026-08-25'), today, anchor), false)
})

test('presetRange cho ra đúng khoảng, luôn kết thúc ở hôm qua', () => {
  assert.deepEqual(presetRange('yesterday', today), { from: '2026-08-24', to: '2026-08-24' })
  assert.deepEqual(presetRange('3days', today), { from: '2026-08-22', to: '2026-08-24' })
  assert.deepEqual(presetRange('7days', today), { from: '2026-08-18', to: '2026-08-24' })
})

test('normalizeRange sắp lại thứ tự khi người dùng bấm ngày sau trước', () => {
  assert.deepEqual(normalizeRange('2026-08-24', '2026-08-20'), { from: '2026-08-20', to: '2026-08-24' })
  assert.deepEqual(normalizeRange('2026-08-20', '2026-08-24'), { from: '2026-08-20', to: '2026-08-24' })
})

test('validateRange trả null khi hợp lệ', () => {
  assert.equal(validateRange({ from: '2026-08-18', to: '2026-08-24' }, today), null)
  assert.equal(validateRange({ from: '2026-08-24', to: '2026-08-24' }, today), null)
})

test('validateRange chặn khoảng dài hơn 7 ngày', () => {
  const msg = validateRange({ from: '2026-08-17', to: '2026-08-24' }, today)
  assert.ok(msg && msg.includes('7'), `muốn thông báo nhắc giới hạn 7 ngày, được: ${msg}`)
})

test('validateRange chặn ngày hôm nay và tương lai', () => {
  assert.ok(validateRange({ from: '2026-08-24', to: '2026-08-25' }, today))
  assert.ok(validateRange({ from: '2026-08-25', to: '2026-08-25' }, today))
})

test('validateRange chặn chuỗi ngày rỗng hoặc sai định dạng', () => {
  assert.ok(validateRange({ from: '', to: '2026-08-24' }, today))
  assert.ok(validateRange({ from: '24/08/2026', to: '2026-08-24' }, today))
})

test('addDays không bị lệch vì múi giờ', () => {
  assert.equal(toISODate(addDays(parseISODate('2026-08-01'), -1)), '2026-07-31')
  assert.equal(toISODate(addDays(parseISODate('2026-12-31'), 1)), '2027-01-01')
})

test('formatRangeLabel đọc được cho người Việt', () => {
  assert.equal(formatRangeLabel({ from: '2026-08-24', to: '2026-08-24' }), '24/08/2026')
  assert.equal(formatRangeLabel({ from: '2026-08-18', to: '2026-08-24' }), '18/08/2026 → 24/08/2026')
})

test('MAX_RANGE_DAYS là 7', () => {
  assert.equal(MAX_RANGE_DAYS, 7)
})
```

- [ ] **Step 2: Đăng ký file test và chạy để xác nhận thất bại**

Trong `GO/frontend/package.json`, thêm `src/lib/tmdtDateRange.test.ts` vào cuối danh sách của script `test`.

Run: `cd GO/frontend && npm test 2>&1 | tail -20`
Expected: FAIL — không tìm thấy module `./tmdtDateRange.ts`.

- [ ] **Step 3: Viết bản cài đặt tối thiểu**

`GO/frontend/src/lib/tmdtDateRange.ts`:

```ts
// Ràng buộc khoảng ngày của nhánh TMĐT, tách khỏi component để test được
// bằng node:test mà không cần dựng DOM.
//
// Mọi ngày biểu diễn bằng chuỗi "YYYY-MM-DD" và Date đặt ở GIỜ UTC 00:00.
// Dùng UTC chứ không dùng giờ địa phương là có chủ đích: cộng/trừ ngày ở
// giờ địa phương sẽ lệch một ngày vào các mốc chuyển giờ, và chuỗi gửi
// xuống backend phải là ngày lịch thuần chứ không mang giờ.

export interface TMDTDateRange {
  from: string
  to: string
}

/** Số ngày tối đa một lần lấy dữ liệu, tính cả hai đầu. */
export const MAX_RANGE_DAYS = 7

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/

export function toISODate(d: Date): string {
  const y = d.getUTCFullYear()
  const m = String(d.getUTCMonth() + 1).padStart(2, '0')
  const day = String(d.getUTCDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

export function parseISODate(s: string): Date {
  const [y, m, d] = s.split('-').map(Number)
  return new Date(Date.UTC(y, m - 1, d))
}

export function addDays(d: Date, n: number): Date {
  return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate() + n))
}

/** Ngày muộn nhất được chọn: HÔM QUA. Đơn hôm nay chưa chốt nên không lấy. */
export function maxSelectableDate(today: Date): Date {
  return addDays(today, -1)
}

function daysBetween(a: Date, b: Date): number {
  return Math.round((b.getTime() - a.getTime()) / 86_400_000)
}

/**
 * isSelectableDay: day có được bấm hay không. anchor là ngày đầu người
 * dùng đã chọn (null khi chưa chọn gì). Chặn NGAY TRÊN LỊCH thay vì báo
 * lỗi sau khi bấm — người dùng thấy được giới hạn trước khi va vào nó.
 */
export function isSelectableDay(day: Date, today: Date, anchor: string | null): boolean {
  if (day.getTime() > maxSelectableDate(today).getTime()) return false
  if (!anchor) return true
  return Math.abs(daysBetween(parseISODate(anchor), day)) <= MAX_RANGE_DAYS - 1
}

export function presetRange(preset: 'yesterday' | '3days' | '7days', today: Date): TMDTDateRange {
  const to = maxSelectableDate(today)
  const span = preset === 'yesterday' ? 1 : preset === '3days' ? 3 : 7
  return { from: toISODate(addDays(to, -(span - 1))), to: toISODate(to) }
}

export function normalizeRange(a: string, b: string): TMDTDateRange {
  return a <= b ? { from: a, to: b } : { from: b, to: a }
}

/** validateRange trả null khi hợp lệ, hoặc câu thông báo tiếng Việt. */
export function validateRange(range: TMDTDateRange, today: Date): string | null {
  if (!ISO_DATE.test(range.from) || !ISO_DATE.test(range.to)) {
    return 'Chưa chọn khoảng thời gian.'
  }
  const from = parseISODate(range.from)
  const to = parseISODate(range.to)
  if (from.getTime() > to.getTime()) {
    return 'Ngày bắt đầu phải trước ngày kết thúc.'
  }
  if (to.getTime() > maxSelectableDate(today).getTime()) {
    return 'Chỉ lấy được dữ liệu đến hết ngày hôm qua.'
  }
  if (daysBetween(from, to) + 1 > MAX_RANGE_DAYS) {
    return `Khoảng thời gian tối đa ${MAX_RANGE_DAYS} ngày.`
  }
  return null
}

export function formatRangeLabel(range: TMDTDateRange): string {
  const vn = (s: string) => {
    const [y, m, d] = s.split('-')
    return `${d}/${m}/${y}`
  }
  return range.from === range.to ? vn(range.from) : `${vn(range.from)} → ${vn(range.to)}`
}
```

- [ ] **Step 4: Chạy test để xác nhận đạt**

Run: `cd GO/frontend && npm test 2>&1 | tail -20`
Expected: PASS toàn bộ, gồm cả 7 file test cũ.

- [ ] **Step 5: Commit**

```bash
git add GO/frontend/src/lib/tmdtDateRange.ts GO/frontend/src/lib/tmdtDateRange.test.ts GO/frontend/package.json
git commit -m "feat(frontend): ràng buộc khoảng ngày cho nhánh TMĐT" -- GO/frontend/src/lib/tmdtDateRange.ts GO/frontend/src/lib/tmdtDateRange.test.ts GO/frontend/package.json
```

---

## Task 10: Modal lịch chọn khoảng ngày

**Files:**
- Create: `GO/frontend/src/components/TMDTDateRangeModal.tsx`

**Interfaces:**
- Consumes: `lib/tmdtDateRange` (Task 9), `lib/useModalEntrance`
- Produces: component `TMDTDateRangeModal({ fileNames, onConfirm, onCancel }: { fileNames: string[]; onConfirm: (range: TMDTDateRange) => void; onCancel: () => void })`

- [ ] **Step 1: Viết component**

`GO/frontend/src/components/TMDTDateRangeModal.tsx`:

```tsx
import { useMemo, useRef, useState } from 'react'
import { useModalEntrance } from '../lib/useModalEntrance'
import {
  MAX_RANGE_DAYS,
  addDays,
  formatRangeLabel,
  isSelectableDay,
  maxSelectableDate,
  normalizeRange,
  parseISODate,
  presetRange,
  toISODate,
  validateRange,
  type TMDTDateRange,
} from '../lib/tmdtDateRange'

interface Props {
  fileNames: string[]
  onConfirm: (range: TMDTDateRange) => void
  onCancel: () => void
}

const WEEKDAYS = ['T2', 'T3', 'T4', 'T5', 'T6', 'T7', 'CN']

// Lịch tự vẽ chứ không dùng <input type="date"> của WebView2: input gốc
// không cho vô hiệu hoá từng ngày theo ràng buộc 7 ngày, và giao diện của
// nó lạc hẳn khỏi tông màu app.
export function TMDTDateRangeModal({ fileNames, onConfirm, onCancel }: Props) {
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef)

  // "Hôm nay" chốt một lần lúc mở modal: nếu đọc lại mỗi lần render, một
  // modal mở qua nửa đêm sẽ tự đổi ràng buộc dưới tay người dùng.
  const today = useMemo(() => parseISODate(toISODate(new Date())), [])
  const [month, setMonth] = useState(() => {
    const max = maxSelectableDate(today)
    return new Date(Date.UTC(max.getUTCFullYear(), max.getUTCMonth(), 1))
  })
  const [anchor, setAnchor] = useState<string | null>(null)
  const [range, setRange] = useState<TMDTDateRange | null>(null)

  const error = range ? validateRange(range, today) : 'Chưa chọn khoảng thời gian.'

  // Ô đầu tiên của lưới là thứ Hai của tuần chứa ngày 1.
  const gridStart = useMemo(() => {
    const weekdayMon0 = (month.getUTCDay() + 6) % 7
    return addDays(month, -weekdayMon0)
  }, [month])

  const days = useMemo(
    () => Array.from({ length: 42 }, (_, i) => addDays(gridStart, i)),
    [gridStart],
  )

  function pick(day: Date) {
    const iso = toISODate(day)
    if (!anchor || range) {
      setAnchor(iso)
      setRange({ from: iso, to: iso })
      return
    }
    setRange(normalizeRange(anchor, iso))
  }

  function applyPreset(preset: 'yesterday' | '3days' | '7days') {
    const r = presetRange(preset, today)
    setAnchor(r.from)
    setRange(r)
    setMonth(new Date(Date.UTC(parseISODate(r.to).getUTCFullYear(), parseISODate(r.to).getUTCMonth(), 1)))
  }

  function inRange(day: Date): boolean {
    if (!range) return false
    const iso = toISODate(day)
    return iso >= range.from && iso <= range.to
  }

  return (
    <div
      ref={backdropRef}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-modal="true"
      aria-label="Chọn khoảng thời gian lấy đơn TMĐT"
    >
      <div ref={cardRef} className="w-full max-w-md rounded-lg border border-border bg-bg p-5 shadow-xl">
        <h2 className="font-sans text-base font-semibold text-ink">Lấy đơn TMĐT theo khoảng ngày</h2>
        <p className="mt-1 font-sans text-xs text-muted">
          {fileNames.join(', ')} — chỉ lấy đến hết ngày hôm qua, tối đa {MAX_RANGE_DAYS} ngày.
        </p>

        <div className="mt-3 flex gap-1.5">
          {([
            ['yesterday', 'Hôm qua'],
            ['3days', '3 ngày'],
            ['7days', '7 ngày'],
          ] as const).map(([key, label]) => (
            <button
              key={key}
              type="button"
              onClick={() => applyPreset(key)}
              className="rounded-md border border-border px-2.5 py-1 font-sans text-xs font-medium text-muted hover:bg-white/[0.04] hover:text-ink"
            >
              {label}
            </button>
          ))}
        </div>

        <div className="mt-4 flex items-center justify-between">
          <button
            type="button"
            aria-label="Tháng trước"
            onClick={() => setMonth(new Date(Date.UTC(month.getUTCFullYear(), month.getUTCMonth() - 1, 1)))}
            className="rounded-md border border-border px-2 py-1 font-sans text-xs text-muted hover:text-ink"
          >
            ‹
          </button>
          <span className="font-sans text-sm font-semibold text-ink">
            Tháng {month.getUTCMonth() + 1}/{month.getUTCFullYear()}
          </span>
          <button
            type="button"
            aria-label="Tháng sau"
            onClick={() => setMonth(new Date(Date.UTC(month.getUTCFullYear(), month.getUTCMonth() + 1, 1)))}
            className="rounded-md border border-border px-2 py-1 font-sans text-xs text-muted hover:text-ink"
          >
            ›
          </button>
        </div>

        <div className="mt-2 grid grid-cols-7 gap-1">
          {WEEKDAYS.map((w) => (
            <div key={w} className="py-1 text-center font-sans text-[11px] font-medium text-muted">
              {w}
            </div>
          ))}
          {days.map((day) => {
            const iso = toISODate(day)
            const otherMonth = day.getUTCMonth() !== month.getUTCMonth()
            const selectable = isSelectableDay(day, today, range ? null : anchor)
            const selected = inRange(day)
            return (
              <button
                key={iso}
                type="button"
                disabled={!selectable}
                aria-pressed={selected}
                onClick={() => pick(day)}
                className={`rounded-md py-1.5 font-mono text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-30 ${
                  selected
                    ? 'bg-accent/[0.18] font-semibold text-accent'
                    : otherMonth
                      ? 'text-muted/60 hover:bg-white/[0.04]'
                      : 'text-ink hover:bg-white/[0.06]'
                }`}
              >
                {day.getUTCDate()}
              </button>
            )
          })}
        </div>

        <div className="mt-4 min-h-[1.25rem] font-sans text-xs">
          {error ? (
            <span className="text-red-400">{error}</span>
          ) : (
            <span className="text-muted">Đã chọn: {formatRangeLabel(range as TMDTDateRange)}</span>
          )}
        </div>

        <div className="mt-3 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-md border border-border px-3 py-1.5 font-sans text-xs font-medium text-muted hover:text-ink"
          >
            Huỷ
          </button>
          <button
            type="button"
            disabled={error !== null}
            onClick={() => range && onConfirm(range)}
            className="rounded-md bg-accent px-3 py-1.5 font-sans text-xs font-semibold text-black disabled:cursor-not-allowed disabled:opacity-40"
          >
            Bắt đầu xử lý
          </button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Kiểm biên dịch**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: không lỗi. Nếu tên lớp màu (`bg-bg`, `text-ink`, `text-muted`, `border-border`, `text-accent`) không có trong cấu hình Tailwind của dự án, đối chiếu `GO/frontend/src/components/SettingsModal.tsx` và dùng đúng bộ tên nó đang dùng — KHÔNG tự bịa màu mới.

- [ ] **Step 3: Commit**

```bash
git add GO/frontend/src/components/TMDTDateRangeModal.tsx
git commit -m "feat(frontend): modal lịch chọn khoảng ngày cho nhánh TMĐT" -- GO/frontend/src/components/TMDTDateRangeModal.tsx
```

---

## Task 11: Nối nhánh TMĐT vào batch (backend)

**Files:**
- Create: `GO/app_tmdt.go`
- Create: `GO/app_tmdt_test.go`
- Modify: `GO/app.go` — struct `App` (thêm 2 trường), `NewApp` (khởi tạo channel), `ProcessFiles`, `runBatch`, `runReservedBatch`

**Interfaces:**
- Consumes: `tmdt.IsWorkbook`, `tmdt.Build`, `tmdt.Options`, `tmdt.SheetRow`, `tmdt.MissingCombo`, `tmdt.WriteHaravanSheet`, `tmdt.ChannelLabel` (Task 2/6/7), `lookup.Load`, `lookup.AppendComboRows`, `lookup.ComboRow` (Task 8), `excelwriter.WriteTMDTRows` (Task 5), `haravan.NewClient/ListOptions/ListOrders/DetectChannel/DefaultChannelRules/ShopName/VNLocation` (Task 1), `appsettings.Settings.Haravan` (Task 3)
- Produces:
  - `main.TMDTDateRange{From, To string}`
  - `main.TMDTComboEntry{Key, Product, Variant, Combo string; TP, SL [4]string}`
  - `(*App).InspectTMDTFiles(paths []string) ([]string, error)`
  - `(*App).ProcessFiles(files []string, stt int, ranges map[string]TMDTDateRange)` — **chữ ký mới**
  - `(*App).ResolveTMDTMissing(entries []TMDTComboEntry) error`
  - `(*App).CancelTMDTMissing() error`
  - sự kiện `tmdt:missing` mang `[]tmdt.MissingCombo`

- [ ] **Step 1: Viết test thất bại**

`GO/app_tmdt_test.go`:

```go
package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/tmdt/lookup"
)

func TestInspectTMDTFiles(t *testing.T) {
	dir := t.TempDir()

	wb := excelize.NewFile()
	for _, s := range []string{lookup.SheetMisa, lookup.SheetDataShop, "Haravan"} {
		if _, err := wb.NewSheet(s); err != nil {
			t.Fatalf("NewSheet: %v", err)
		}
	}
	wb.DeleteSheet("Sheet1")
	tmdtPath := filepath.Join(dir, "XUẤT HÀNG HN-LA MỚI.xlsx")
	if err := wb.SaveAs(tmdtPath); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	wb.Close()

	other := excelize.NewFile()
	otherPath := filepath.Join(dir, "don-vendor.xlsx")
	if err := other.SaveAs(otherPath); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	other.Close()

	app := &App{}
	got, err := app.InspectTMDTFiles([]string{otherPath, tmdtPath})
	if err != nil {
		t.Fatalf("InspectTMDTFiles: %v", err)
	}
	if len(got) != 1 || got[0] != tmdtPath {
		t.Errorf("InspectTMDTFiles = %v, muốn chỉ %q", got, tmdtPath)
	}
}

func TestParseTMDTRangeRejectsBadInput(t *testing.T) {
	today := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)

	if _, _, err := parseTMDTRange(TMDTDateRange{From: "2026-08-18", To: "2026-08-24"}, today); err != nil {
		t.Errorf("khoảng 7 ngày hợp lệ bị từ chối: %v", err)
	}
	if _, _, err := parseTMDTRange(TMDTDateRange{From: "2026-08-17", To: "2026-08-24"}, today); err == nil {
		t.Errorf("khoảng 8 ngày phải bị từ chối")
	}
	if _, _, err := parseTMDTRange(TMDTDateRange{From: "2026-08-25", To: "2026-08-25"}, today); err == nil {
		t.Errorf("ngày hôm nay phải bị từ chối")
	}
	if _, _, err := parseTMDTRange(TMDTDateRange{From: "24/08/2026", To: "2026-08-24"}, today); err == nil {
		t.Errorf("định dạng sai phải bị từ chối")
	}
	if _, _, err := parseTMDTRange(TMDTDateRange{From: "2026-08-24", To: "2026-08-20"}, today); err == nil {
		t.Errorf("from sau to phải bị từ chối")
	}

	// Biên giờ: from ở 00:00:00+07, to ở 23:59:59+07.
	from, to, err := parseTMDTRange(TMDTDateRange{From: "2026-08-22", To: "2026-08-23"}, today)
	if err != nil {
		t.Fatalf("parseTMDTRange: %v", err)
	}
	if got := from.Format(time.RFC3339); got != "2026-08-22T00:00:00+07:00" {
		t.Errorf("from = %s", got)
	}
	if got := to.Format(time.RFC3339); got != "2026-08-23T23:59:59+07:00" {
		t.Errorf("to = %s", got)
	}
}

func TestWaitForTMDTResolutionCancel(t *testing.T) {
	app := &App{tmdtResolve: make(chan tmdtResolution, 1)}
	go func() {
		if err := app.CancelTMDTMissing(); err != nil {
			t.Errorf("CancelTMDTMissing: %v", err)
		}
	}()
	res, ok := app.waitForTMDTResolution(200 * time.Millisecond)
	if !ok {
		t.Fatalf("waitForTMDTResolution báo hết giờ dù đã có phản hồi")
	}
	if !res.cancel {
		t.Errorf("muốn cancel = true")
	}
}

func TestWaitForTMDTResolutionTimeout(t *testing.T) {
	app := &App{tmdtResolve: make(chan tmdtResolution, 1)}
	start := time.Now()
	if _, ok := app.waitForTMDTResolution(50 * time.Millisecond); ok {
		t.Errorf("muốn hết giờ khi không ai trả lời")
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Errorf("trả về sớm hơn hạn giờ")
	}
}

func TestSummaryRowsGroupByShopAndDate(t *testing.T) {
	rows := summaryTMDTRows("XUẤT HÀNG HN-LA MỚI.xlsx", []summaryKeyCount{
		{shop: "Blue HN", date: "23/08/2026", channel: "TikTok", misa: "MB_TMDT_00001", shipTo: "HN", orders: 3, lines: 5},
		{shop: "Blue HN", date: "22/08/2026", channel: "TikTok", misa: "MB_TMDT_00001", shipTo: "HN", orders: 1, lines: 1, hasNA: true},
	})
	if len(rows) != 2 {
		t.Fatalf("có %d dòng tóm tắt, muốn 2", len(rows))
	}
	if rows[0].System != "TMĐT-TikTok" {
		t.Errorf("System = %q, muốn TMĐT-TikTok", rows[0].System)
	}
	if rows[0].PO != "Blue HN · 23/08/2026" {
		t.Errorf("PO = %q", rows[0].PO)
	}
	if rows[0].StatusKind != "done" {
		t.Errorf("StatusKind = %q, muốn done", rows[0].StatusKind)
	}
	if rows[1].StatusKind != "warning" {
		t.Errorf("nhóm còn #N/A phải là warning, được %q", rows[1].StatusKind)
	}
}
```

- [ ] **Step 2: Chạy test để xác nhận thất bại**

Run: `cd GO && go test . -run TMDT -v`
Expected: FAIL — `undefined: TMDTDateRange`, `undefined: parseTMDTRange`, …

- [ ] **Step 3: Thêm hai trường vào `App` và khởi tạo**

Trong `GO/app.go`, thêm vào struct `App` (cạnh `updateJITPeriodFn`):

```go
	// tmdtResolve nhận phản hồi của modal sửa mã thiếu. Đệm 1 để
	// ResolveTMDTMissing/CancelTMDTMissing không bị chặn nếu nhánh TMĐT
	// vừa hết giờ chờ đúng lúc người dùng bấm.
	tmdtResolve chan tmdtResolution
	// tmdtWaiting cho biết đang có nhánh TMĐT chờ phản hồi — dùng để từ
	// chối lời gọi Resolve/Cancel lạc (người dùng bấm khi không có modal).
	tmdtWaiting atomic.Bool
```

Trong `NewApp`, khi dựng `app := &App{...}`, thêm:

```go
		tmdtResolve: make(chan tmdtResolution, 1),
```

- [ ] **Step 4: Đổi chữ ký `ProcessFiles` / `runBatch` / `runReservedBatch`**

Trong `GO/app.go`:

```go
// ProcessFiles chạy xử lý các file đã chọn trong nền. ranges gắn khoảng
// ngày cho từng file TMĐT (khoá là đúng đường dẫn file); batch không có
// file TMĐT thì truyền map rỗng hoặc nil.
func (a *App) ProcessFiles(files []string, stt int, ranges map[string]TMDTDateRange) {
	if !a.reserveBatch() {
		a.emitter.Emit("process:log", "⚠️ Đã có một batch đang xử lý, vui lòng đợi hoàn tất.")
		return
	}
	go a.runReservedBatch(a.emitter, files, stt, ranges)
}

func (a *App) runBatch(emitter Emitter, files []string, stt int, ranges map[string]TMDTDateRange) {
	if !a.reserveBatch() {
		emitter.Emit("process:log", "⚠️ Đã có một batch đang xử lý, vui lòng đợi hoàn tất.")
		return
	}
	a.runReservedBatch(emitter, files, stt, ranges)
}

func (a *App) runReservedBatch(emitter Emitter, files []string, stt int, ranges map[string]TMDTDateRange) {
```

Trong vòng lặp file của `runReservedBatch`, thay lời gọi `a.processOne(f, current, emitRow)` bằng:

```go
		var rows []processing.OrderRow
		var err error
		if tmdt.IsWorkbook(f) {
			rows, err = a.processTMDTFile(emitter, f, ranges[f], emitRow)
		} else {
			rows, err = a.processOne(f, current, emitRow)
		}
```

Thêm `"order-processor/internal/tmdt"` vào khối import của `app.go`.

Mọi lời gọi `runBatch` trong `GO/app_test.go` phải thêm tham số thứ 4 là `nil` — chạy `go vet ./...` để tìm hết chỗ gọi.

- [ ] **Step 5: Viết `GO/app_tmdt.go`**

```go
package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"order-processor/internal/fileset"
	"order-processor/internal/processing"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/tmdt"
	"order-processor/internal/tmdt/haravan"
	"order-processor/internal/tmdt/lookup"
)

// tmdtMissingTimeout là hạn chờ người dùng khai mã thiếu. Nhánh TMĐT dừng
// giữa batch trong khi ĐANG GIỮ a.excelMu — đây là chỗ duy nhất trong app
// làm vậy — nên phải có hạn giờ: người dùng bỏ đi mà không bấm gì thì
// batch vẫn kết thúc chứ không khoá Excel vĩnh viễn.
const tmdtMissingTimeout = 10 * time.Minute

// TMDTDateRange là khoảng ngày (giờ VN) người dùng chọn cho một file TMĐT.
type TMDTDateRange struct {
	From string `json:"from"` // "2026-08-22"
	To   string `json:"to"`   // "2026-08-23", tính hết ngày
}

// TMDTComboEntry là một dòng người dùng vừa khai trong modal sửa mã thiếu
// — đúng hình dạng một dòng cột A..K của sheet "data shop".
type TMDTComboEntry struct {
	Key     string    `json:"key"`
	Product string    `json:"product"`
	Variant string    `json:"variant"`
	Combo   string    `json:"combo"`
	TP      [4]string `json:"tp"`
	SL      [4]string `json:"sl"`
}

type tmdtResolution struct {
	entries []TMDTComboEntry
	cancel  bool
}

// InspectTMDTFiles trả về những đường dẫn trong paths là workbook TMĐT.
// Frontend gọi hàm này khi người dùng bấm "Xử lý" để biết có cần bật modal
// chọn ngày hay không.
func (a *App) InspectTMDTFiles(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range fileset.FilterValid(paths) {
		if tmdt.IsWorkbook(p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// ResolveTMDTMissing nhận các dòng người dùng vừa khai.
func (a *App) ResolveTMDTMissing(entries []TMDTComboEntry) error {
	if !a.tmdtWaiting.Load() {
		return fmt.Errorf("không có yêu cầu khai mã nào đang chờ")
	}
	select {
	case a.tmdtResolve <- tmdtResolution{entries: entries}:
		return nil
	default:
		return fmt.Errorf("đã có phản hồi được gửi trước đó")
	}
}

// CancelTMDTMissing bỏ qua việc khai mã: các dòng thiếu vẫn được ghi với
// #N/A để người dùng thấy, không bị bỏ âm thầm.
func (a *App) CancelTMDTMissing() error {
	if !a.tmdtWaiting.Load() {
		return fmt.Errorf("không có yêu cầu khai mã nào đang chờ")
	}
	select {
	case a.tmdtResolve <- tmdtResolution{cancel: true}:
		return nil
	default:
		return fmt.Errorf("đã có phản hồi được gửi trước đó")
	}
}

func (a *App) waitForTMDTResolution(timeout time.Duration) (tmdtResolution, bool) {
	a.tmdtWaiting.Store(true)
	defer a.tmdtWaiting.Store(false)

	var done <-chan struct{}
	if a.ctx != nil {
		done = a.ctx.Done()
	}
	select {
	case res := <-a.tmdtResolve:
		return res, true
	case <-time.After(timeout):
		return tmdtResolution{cancel: true}, false
	case <-done:
		return tmdtResolution{cancel: true}, false
	}
}

// parseTMDTRange đổi khoảng ngày của frontend thành mốc giờ VN và KIỂM LẠI
// cả hai ràng buộc. Kiểm lại ở backend chứ không tin frontend: một bản
// frontend cũ hoặc một lời gọi qua bindings có thể gửi khoảng 3 tháng, và
// đó là 90 lần gọi API vô ích cùng một file Excel sai.
func parseTMDTRange(r TMDTDateRange, today time.Time) (from, to time.Time, err error) {
	const layout = "2006-01-02"
	fromDay, err := time.ParseInLocation(layout, strings.TrimSpace(r.From), haravan.VNLocation)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("ngày bắt đầu %q không đúng dạng YYYY-MM-DD", r.From)
	}
	toDay, err := time.ParseInLocation(layout, strings.TrimSpace(r.To), haravan.VNLocation)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("ngày kết thúc %q không đúng dạng YYYY-MM-DD", r.To)
	}
	if toDay.Before(fromDay) {
		return time.Time{}, time.Time{}, fmt.Errorf("ngày bắt đầu phải trước ngày kết thúc")
	}
	yesterday := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, haravan.VNLocation).AddDate(0, 0, -1)
	if toDay.After(yesterday) {
		return time.Time{}, time.Time{}, fmt.Errorf("chỉ lấy được dữ liệu đến hết ngày hôm qua (%s)", yesterday.Format("02/01/2006"))
	}
	if days := int(toDay.Sub(fromDay).Hours()/24) + 1; days > 7 {
		return time.Time{}, time.Time{}, fmt.Errorf("khoảng thời gian tối đa 7 ngày, đang chọn %d ngày", days)
	}
	from = time.Date(fromDay.Year(), fromDay.Month(), fromDay.Day(), 0, 0, 0, 0, haravan.VNLocation)
	to = time.Date(toDay.Year(), toDay.Month(), toDay.Day(), 23, 59, 59, 0, haravan.VNLocation)
	return from, to, nil
}

// summaryKeyCount là số liệu gộp của một nhóm (shop, ngày).
type summaryKeyCount struct {
	shop    string
	date    string
	channel string
	misa    string
	shipTo  string
	orders  int
	lines   int
	hasNA   bool
}

// summaryTMDTRows đổi số liệu gộp thành dòng cho bảng kết quả. Đơn TMĐT
// KHÔNG đổ từng dòng lên bảng: một tuần có thể ~2.500 đơn / ~5.000 dòng,
// vừa làm ngập bảng vừa phá cơ chế tick chọn PO để gửi Zalo.
func summaryTMDTRows(fileName string, groups []summaryKeyCount) []processing.OrderRow {
	rows := make([]processing.OrderRow, 0, len(groups))
	for _, g := range groups {
		kind := processing.StatusKindDone
		status := fmt.Sprintf("%s - %d đơn / %d dòng", processing.StatusDone, g.orders, g.lines)
		if g.hasNA {
			kind = processing.StatusKindWarning
			status = fmt.Sprintf("%s - %d đơn / %d dòng, còn mã #N/A", processing.StatusWarning, g.orders, g.lines)
		}
		rows = append(rows, processing.OrderRow{
			FileName:    fileName,
			Page:        g.shipTo,
			System:      "TMĐT-" + g.channel,
			MaKhachHang: g.misa,
			PO:          fmt.Sprintf("%s · %s", g.shop, g.date),
			Status:      status,
			StatusKind:  kind,
		})
	}
	return rows
}

// processTMDTFile là toàn bộ nhánh TMĐT của một file. Trả về các dòng tóm
// tắt đã phát, để runReservedBatch đếm stt như với mọi file khác.
func (a *App) processTMDTFile(emitter Emitter, path string, rng TMDTDateRange, emit func(processing.OrderRow)) ([]processing.OrderRow, error) {
	fail := func(format string, args ...any) ([]processing.OrderRow, error) {
		msg := fmt.Sprintf(format, args...)
		emitter.Emit("process:log", "❌ "+msg)
		return nil, fmt.Errorf("%s", msg)
	}

	settings, err := a.appSettingsStore.Load(resolveRepoFile("settings.ini"))
	if err != nil {
		return fail("không đọc được cấu hình: %v", err)
	}
	token := strings.TrimSpace(settings.Haravan["access_token"])
	if token == "" {
		return fail("chưa có access token Haravan — mở Cài đặt ▸ Haravan (TMĐT) và điền khoá access_token")
	}

	from, to, err := parseTMDTRange(rng, time.Now().In(haravan.VNLocation))
	if err != nil {
		return fail("khoảng thời gian không hợp lệ: %v", err)
	}

	tables, err := lookup.Load(path)
	if err != nil {
		return fail("không đọc được bảng tra cứu trong %s: %v", path, err)
	}

	emitter.Emit("process:log", fmt.Sprintf("⏳ Đang lấy đơn TMĐT từ %s đến %s...",
		from.Format("02/01/2006"), to.Format("02/01/2006")))

	client := haravan.NewClient(token)
	// Logger của client in ra stdout kèm URL request — KHÔNG bao giờ chứa
	// token (token nằm ở header Authorization), nhưng vẫn tắt để log của
	// app chỉ có một nguồn duy nhất là process:log.
	client.Logger = nil

	var lines []tmdt.OrderLine
	opt := haravan.ListOptions{CreatedAtMin: from, CreatedAtMax: to}
	err = client.ListOrders(context.Background(), opt, func(page int, orders []haravan.Order) error {
		for i := range orders {
			o := &orders[i]
			// DetectChannel trả "Shopee" | "TikTok Shop" | "" — chuỗi rỗng
			// nghĩa là không phải đơn sàn, bỏ qua.
			channel := haravan.DetectChannel(o, haravan.DefaultChannelRules)
			if channel == "" {
				continue
			}
			shop := haravan.ShopName(o)
			items := o.LineItems
			if len(items) == 0 {
				continue
			}
			for j := range items {
				li := &items[j]
				lines = append(lines, tmdt.OrderLine{
					OrderCode:    firstNonEmptyTMDT(o.Name, o.OrderNumber),
					Shop:         shop,
					KhoBan:       o.LocationName,
					KenhBanHang:  channel,
					CreatedAt:    o.CreatedAt.InVN(),
					Quantity:     float64(li.Quantity),
					Title:        li.Title,
					VariantTitle: li.VariantTitle,
					Price:        li.Price.Float(),
					Subtotal:     o.SubtotalPrice.Float(),
					Total:        o.TotalPrice.Float(),
					SKU:          li.SKU,
					Attributes:   haravan.LineItemAttributes(li),
				})
			}
		}
		emitter.Emit("process:log", fmt.Sprintf("   ...đã tải trang %d (%d dòng hàng)", page, len(lines)))
		return nil
	})
	if err != nil {
		return fail("gọi Haravan API thất bại: %v", err)
	}
	if len(lines) == 0 {
		emitter.Emit("process:log", "⚠️ Không có đơn TMĐT nào trong khoảng thời gian đã chọn.")
		return nil, nil
	}

	namer := a.tmdtProductNamer()
	res := tmdt.Build(lines, tables, tmdt.Options{ProductName: namer})

	if len(res.Missing) > 0 {
		emitter.Emit("process:log", fmt.Sprintf("⚠️ Có %d mã chưa khai báo trong sheet \"data shop\" — đang chờ bổ sung...", len(res.Missing)))
		emitter.Emit("tmdt:missing", res.Missing)

		resolution, answered := a.waitForTMDTResolution(tmdtMissingTimeout)
		switch {
		case !answered:
			emitter.Emit("process:log", "⚠️ Hết thời gian chờ khai mã — các dòng thiếu sẽ mang #N/A.")
		case resolution.cancel:
			emitter.Emit("process:log", "⚠️ Đã bỏ qua khai mã — các dòng thiếu sẽ mang #N/A.")
		default:
			combos := make([]lookup.ComboRow, 0, len(resolution.entries))
			for _, e := range resolution.entries {
				if strings.TrimSpace(e.TP[0]) == "" {
					continue // để trống = giữ #N/A cho mục này
				}
				combos = append(combos, lookup.ComboRow{
					Product: e.Product, Variant: e.Variant, Combo: e.Combo,
					TP: e.TP, SL: e.SL,
				})
			}
			if len(combos) > 0 {
				if _, err := lookup.AppendComboRows(path, combos); err != nil {
					return fail("không ghi được vào sheet \"data shop\": %v", err)
				}
				emitter.Emit("process:log", fmt.Sprintf("✅ Đã bổ sung %d dòng vào sheet \"data shop\".", len(combos)))
				tables, err = lookup.Load(path)
				if err != nil {
					return fail("không nạp lại được bảng tra cứu: %v", err)
				}
				res = tmdt.Build(lines, tables, tmdt.Options{ProductName: namer})
			}
		}
	}

	for shop, n := range res.MissingShops {
		emitter.Emit("process:log", fmt.Sprintf("⚠️ Shop %q chưa có trong sheet %q (%d dòng → Mã khách hàng = %s)",
			shop, lookup.SheetMisa, n, lookup.NotAvailable))
	}

	if err := tmdt.WriteHaravanSheet(path, res.SheetRows); err != nil {
		return fail("không ghi được sheet %q (file có đang mở trong Excel không?): %v", tmdt.SheetHaravan, err)
	}
	emitter.Emit("process:log", fmt.Sprintf("✅ Đã ghi %d dòng vào sheet %q.", len(res.SheetRows), tmdt.SheetHaravan))

	if _, err := excelwriter.WriteTMDTRows(a.excelPath, res.OrderRows); err != nil {
		return fail("không ghi được dondathang.xlsx (file có đang mở trong Excel không?): %v", err)
	}
	emitter.Emit("process:log", fmt.Sprintf("✅ Đã ghi %d dòng vào dondathang.xlsx.", len(res.OrderRows)))

	rows := summaryTMDTRows(baseNameTMDT(path), groupTMDTSummary(res))
	for _, row := range rows {
		emit(row)
	}
	return rows, nil
}

// tmdtProductNamer lấy hàm tra tên hàng theo mã thành phẩm từ Store đang
// dùng. Trả về hàm luôn rỗng khi processor là bản giả (test) hoặc chưa nạp
// xong dữ liệu — thiếu tên hàng không đáng để chặn cả lần chạy.
func (a *App) tmdtProductNamer() func(string) string {
	rp, ok := a.processor.(*processing.RealProcessor)
	if !ok || rp == nil || rp.Store == nil {
		return func(string) string { return "" }
	}
	store := rp.Store
	return func(tp string) string {
		info, found := store.GetProductInfo(tp)
		if !found {
			return ""
		}
		return info.Name
	}
}

// groupTMDTSummary gộp kết quả theo (shop, ngày) — đơn vị soát tự nhiên
// của người dùng: họ đối chiếu số đơn từng shop từng ngày với trang quản
// trị sàn.
func groupTMDTSummary(res tmdt.Result) []summaryKeyCount {
	type key struct{ shop, date string }
	agg := map[key]*summaryKeyCount{}
	seenOrder := map[string]bool{}

	for _, r := range res.OrderRows {
		// EntryDate là "dd/mm/yyyy"; PO của dòng tóm tắt cần đúng dạng đó.
		shop := shopFromDescription(r.Description)
		k := key{shop: shop, date: r.EntryDate}
		g, ok := agg[k]
		if !ok {
			g = &summaryKeyCount{
				shop: shop, date: r.EntryDate,
				channel: channelFromOrderNumber(r.OrderNumber),
				misa:    r.CustomerCode, shipTo: r.ShipTo,
			}
			agg[k] = g
		}
		g.lines++
		if !seenOrder[k.shop+k.date+r.Note] {
			seenOrder[k.shop+k.date+r.Note] = true
			g.orders++
		}
		if r.SKU == lookup.NotAvailable || r.CustomerCode == lookup.NotAvailable {
			g.hasNA = true
		}
	}

	out := make([]summaryKeyCount, 0, len(agg))
	for _, g := range agg {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].shop != out[j].shop {
			return out[i].shop < out[j].shop
		}
		return out[i].date < out[j].date
	})
	return out
}

// shopFromDescription tách tên shop ra khỏi cột Diễn giải, vốn có dạng
// "TMĐT-{kênh} - {shop} - {mã đơn} - Ngày đổ {ngày} - {kho}". Đọc lại từ
// Diễn giải thay vì mang thêm một trường chỉ để tóm tắt: TMDTRow là hợp
// đồng ghi Excel, không phải chỗ nhồi dữ liệu phục vụ UI.
func shopFromDescription(desc string) string {
	parts := strings.Split(desc, " - ")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func channelFromOrderNumber(orderNumber string) string {
	// "ĐĐHTMĐT-TikTok-585694..." → "TikTok"
	parts := strings.SplitN(orderNumber, "-", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func firstNonEmptyTMDT(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func baseNameTMDT(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}
```

`tmdtProductNamer` chỉ đọc `info.Name` từ giá trị `GetProductInfo` trả về nên **không** cần import `productdata` — nếu `go build` báo import thừa thì đó là đúng, bỏ nó đi.

- [ ] **Step 6: Chạy test để xác nhận đạt**

Run: `cd GO && go build ./... && go test . -run TMDT -v`
Expected: PASS. Sau đó `cd GO && go test ./...` phải xanh toàn bộ (các lời gọi `runBatch` trong `app_test.go` đã được thêm tham số `nil`).

- [ ] **Step 7: Sinh lại Wails bindings**

```bash
cd "GO" && wails generate module
grep -n "ProcessFiles\|InspectTMDTFiles\|ResolveTMDTMissing\|CancelTMDTMissing" frontend/wailsjs/go/main/App.d.ts
```

Expected: `ProcessFiles` giờ nhận 3 tham số, và ba method mới đều có mặt. **Bỏ bước này là lỗi âm thầm**: frontend sẽ gọi bản 2 tham số và `ranges` mất hẳn.

- [ ] **Step 8: Commit**

```bash
git add GO/app.go GO/app_tmdt.go GO/app_tmdt_test.go GO/app_test.go GO/frontend/wailsjs
git commit -m "feat(app): nối nhánh TMĐT vào batch, chờ khai mã có hạn giờ" -- GO/app.go GO/app_tmdt.go GO/app_tmdt_test.go GO/app_test.go GO/frontend/wailsjs
```

---

## Task 12: Modal khai mã thiếu + nối luồng frontend

**Files:**
- Create: `GO/frontend/src/lib/tmdtMissing.ts`
- Create: `GO/frontend/src/lib/tmdtMissing.test.ts`
- Create: `GO/frontend/src/components/TMDTMissingModal.tsx`
- Modify: `GO/frontend/src/store/appStore.ts`
- Modify: `GO/frontend/src/hooks/useWailsEvents.ts`
- Modify: `GO/frontend/src/components/ControlPanel.tsx`
- Modify: `GO/frontend/src/App.tsx`
- Modify: `GO/frontend/package.json`

**Interfaces:**
- Consumes: `TMDTDateRangeModal` (Task 10), bindings `InspectTMDTFiles` / `ProcessFiles` / `ResolveTMDTMissing` / `CancelTMDTMissing` (Task 11)
- Produces:
  - `TMDTMissingCombo{key: string; product: string; variant: string; combo: string; lineCount: number}`
  - `TMDTComboDraft{key: string; product: string; variant: string; combo: string; tp: [string,string,string,string]; sl: [string,string,string,string]}`
  - `emptyDraft(m: TMDTMissingCombo): TMDTComboDraft`
  - `isDraftFilled(d: TMDTComboDraft): boolean`
  - `draftError(d: TMDTComboDraft): string | null`
  - store: `tmdtMissing: TMDTMissingCombo[] | null`, `setTMDTMissing(list: TMDTMissingCombo[] | null): void`
  - component `TMDTMissingModal`

**Lưu ý phối hợp:** `GO/frontend/src/types.ts` đang bị sửa dở bởi phiên codex song song. Task này **không** thêm gì vào file đó — kiểu dữ liệu TMĐT đặt trong `lib/tmdtMissing.ts`.

- [ ] **Step 1: Viết test thất bại cho phần logic thuần**

`GO/frontend/src/lib/tmdtMissing.test.ts`:

```ts
import { strict as assert } from 'node:assert'
import { test } from 'node:test'
import { draftError, emptyDraft, isDraftFilled, type TMDTMissingCombo } from './tmdtMissing.ts'

const missing: TMDTMissingCombo = {
  key: 'sku:SP777',
  product: 'Sản phẩm mới',
  variant: 'Combo 3 Túi',
  combo: 'SP777',
  lineCount: 12,
}

test('emptyDraft mang sẵn thông tin điền trước, để trống phần người dùng phải nhập', () => {
  const d = emptyDraft(missing)
  assert.equal(d.key, 'sku:SP777')
  assert.equal(d.product, 'Sản phẩm mới')
  assert.equal(d.variant, 'Combo 3 Túi')
  assert.equal(d.combo, 'SP777')
  assert.deepEqual(d.tp, ['', '', '', ''])
  assert.deepEqual(d.sl, ['', '', '', ''])
})

test('isDraftFilled chỉ tính là đã khai khi có mã thành phẩm đầu tiên', () => {
  const d = emptyDraft(missing)
  assert.equal(isDraftFilled(d), false)
  assert.equal(isDraftFilled({ ...d, tp: ['TP777', '', '', ''], sl: ['1', '', '', ''] }), true)
})

test('draftError đòi số lượng cho mỗi mã đã điền', () => {
  const d = emptyDraft(missing)
  // Chưa điền gì: không phải lỗi — bỏ trống là giữ #N/A cho mục này.
  assert.equal(draftError(d), null)
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['', '', '', ''] }))
  assert.equal(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['1', '', '', ''] }), null)
})

test('draftError chặn số lượng không phải số dương', () => {
  const d = emptyDraft(missing)
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['0', '', '', ''] }))
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['abc', '', '', ''] }))
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['-2', '', '', ''] }))
})

test('draftError chặn số lượng điền mà thiếu mã thành phẩm tương ứng', () => {
  const d = emptyDraft(missing)
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['1', '2', '', ''] }))
})
```

- [ ] **Step 2: Đăng ký test và chạy để xác nhận thất bại**

Thêm `src/lib/tmdtMissing.test.ts` vào script `test` của `GO/frontend/package.json`.

Run: `cd GO/frontend && npm test 2>&1 | tail -15`
Expected: FAIL — không tìm thấy `./tmdtMissing.ts`.

- [ ] **Step 3: Viết `lib/tmdtMissing.ts`**

```ts
// Kiểu dữ liệu và kiểm tra đầu vào cho modal khai mã thành phẩm còn thiếu.
//
// Đặt ở lib chứ không ở types.ts: types.ts đang được một phiên khác sửa
// song song, và phần logic ở đây cần test được bằng node:test.

/** Một mã chưa khai báo trong sheet "data shop", đã gom unique ở backend. */
export interface TMDTMissingCombo {
  key: string
  product: string
  variant: string
  combo: string
  lineCount: number
}

/** Bản nháp người dùng đang điền — đúng hình dạng một dòng cột A..K. */
export interface TMDTComboDraft {
  key: string
  product: string
  variant: string
  combo: string
  tp: [string, string, string, string]
  sl: [string, string, string, string]
}

export function emptyDraft(m: TMDTMissingCombo): TMDTComboDraft {
  return {
    key: m.key,
    product: m.product,
    variant: m.variant,
    combo: m.combo,
    tp: ['', '', '', ''],
    sl: ['', '', '', ''],
  }
}

/** Đã khai hay chưa. Bỏ trống hoàn toàn là hợp lệ: mục đó giữ #N/A. */
export function isDraftFilled(d: TMDTComboDraft): boolean {
  return d.tp[0].trim() !== ''
}

/**
 * draftError trả null khi bản nháp dùng được, hoặc câu thông báo tiếng
 * Việt. Kiểm ngay trên form thay vì để backend từ chối: một dòng sai ghi
 * vào "data shop" sẽ sai vĩnh viễn cho mọi lần chạy sau.
 */
export function draftError(d: TMDTComboDraft): string | null {
  if (!isDraftFilled(d)) {
    // Chưa điền gì cả — người dùng chủ ý bỏ qua mục này.
    for (let i = 0; i < 4; i += 1) {
      if (d.sl[i].trim() !== '') return 'Đã điền số lượng nhưng thiếu mã thành phẩm.'
    }
    return null
  }
  for (let i = 0; i < 4; i += 1) {
    const tp = d.tp[i].trim()
    const sl = d.sl[i].trim()
    if (tp === '' && sl === '') continue
    if (tp === '') return `Thành phẩm ${i + 1}: đã điền số lượng nhưng thiếu mã.`
    if (sl === '') return `Thành phẩm ${i + 1}: thiếu số lượng.`
    const n = Number(sl)
    if (!Number.isFinite(n) || n <= 0) return `Thành phẩm ${i + 1}: số lượng phải là số lớn hơn 0.`
  }
  return null
}
```

- [ ] **Step 4: Chạy test để xác nhận đạt**

Run: `cd GO/frontend && npm test 2>&1 | tail -15`
Expected: PASS toàn bộ.

- [ ] **Step 5: Thêm state vào store và lắng nghe sự kiện**

Trong `GO/frontend/src/store/appStore.ts`:

- thêm import: `import type { TMDTMissingCombo } from '../lib/tmdtMissing'`
- thêm vào `interface AppState`:

```ts
  tmdtMissing: TMDTMissingCombo[] | null
  setTMDTMissing: (list: TMDTMissingCombo[] | null) => void
```

- thêm vào phần khởi tạo store (cạnh `zaloQR: null`):

```ts
  tmdtMissing: null,
  // Danh sách rỗng cũng coi như "không có gì cần khai" để modal không bật
  // với một form trống — backend chỉ phát sự kiện khi thực sự thiếu, đây
  // là lớp phòng vệ thứ hai.
  setTMDTMissing: (list) => set({ tmdtMissing: list && list.length > 0 ? list : null }),
```

Trong `GO/frontend/src/hooks/useWailsEvents.ts`:

- lấy action: `const setTMDTMissing = useAppStore((s) => s.setTMDTMissing)`
- thêm import kiểu: `import type { TMDTMissingCombo } from '../lib/tmdtMissing'`
- trong `useEffect`, thêm:

```ts
    const offTMDTMissing = EventsOn('tmdt:missing', (list: TMDTMissingCombo[]) => setTMDTMissing(list))
```

- thêm `offTMDTMissing()` vào hàm dọn dẹp trả về ở cuối `useEffect`.

- [ ] **Step 6: Viết `components/TMDTMissingModal.tsx`**

```tsx
import { useMemo, useRef, useState } from 'react'
import { useAppStore } from '../store/appStore'
import { CancelTMDTMissing, ResolveTMDTMissing } from '../../wailsjs/go/main/App'
import { useModalEntrance } from '../lib/useModalEntrance'
import { draftError, emptyDraft, isDraftFilled, type TMDTComboDraft } from '../lib/tmdtMissing'

// Modal bật ĐÚNG MỘT LẦN cho cả lần chạy, sau khi đã quy đổi hết dữ liệu
// và trước khi ghi bất kỳ file nào. Backend đang chờ trên channel: mọi
// đường ra khỏi modal đều phải gọi Resolve hoặc Cancel, nếu không batch
// treo tới khi hết hạn 10 phút.
export function TMDTMissingModal() {
  const missing = useAppStore((s) => s.tmdtMissing)
  const setTMDTMissing = useAppStore((s) => s.setTMDTMissing)
  const appendLog = useAppStore((s) => s.appendLog)
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef, [missing !== null])

  const [drafts, setDrafts] = useState<TMDTComboDraft[]>([])
  const [busy, setBusy] = useState(false)

  // Dựng lại nháp mỗi khi danh sách thiếu đổi (một lần cho mỗi lần chạy).
  const signature = useMemo(() => (missing ?? []).map((m) => m.key).join('|'), [missing])
  const [seenSignature, setSeenSignature] = useState('')
  if (missing && signature !== seenSignature) {
    setSeenSignature(signature)
    setDrafts(missing.map(emptyDraft))
  }

  if (!missing) return null

  const errors = drafts.map(draftError)
  const firstError = errors.find((e) => e !== null) ?? null
  const filledCount = drafts.filter(isDraftFilled).length

  function updateCell(rowIdx: number, field: 'tp' | 'sl', col: number, value: string) {
    setDrafts((prev) =>
      prev.map((d, i) => {
        if (i !== rowIdx) return d
        const next = [...d[field]] as [string, string, string, string]
        next[col] = value
        return { ...d, [field]: next }
      }),
    )
  }

  async function submit() {
    setBusy(true)
    try {
      await ResolveTMDTMissing(drafts.filter(isDraftFilled))
      setTMDTMissing(null)
    } catch (err) {
      appendLog(`❌ Lỗi gửi khai báo mã: ${String(err)}`)
    } finally {
      setBusy(false)
    }
  }

  async function skip() {
    setBusy(true)
    try {
      await CancelTMDTMissing()
      setTMDTMissing(null)
    } catch (err) {
      appendLog(`❌ Lỗi bỏ qua khai báo mã: ${String(err)}`)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      ref={backdropRef}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-modal="true"
      aria-label="Khai mã thành phẩm còn thiếu"
    >
      <div ref={cardRef} className="flex max-h-[85vh] w-full max-w-3xl flex-col rounded-lg border border-border bg-bg p-5 shadow-xl">
        <h2 className="font-sans text-base font-semibold text-ink">
          {missing.length} mã chưa khai báo trong sheet &quot;data shop&quot;
        </h2>
        <p className="mt-1 font-sans text-xs text-muted">
          Điền mã thành phẩm và số lượng. Nội dung này được ghi luôn vào sheet &quot;data shop&quot; nên lần sau
          không phải khai lại. Bỏ trống một mục nghĩa là giữ #N/A cho mục đó.
        </p>

        <div className="mt-3 flex-1 overflow-y-auto">
          {drafts.map((d, i) => (
            <div key={d.key} className="mb-3 rounded-md border border-border p-3">
              <div className="font-sans text-xs text-ink">
                {d.product}
                {d.variant ? <span className="text-muted"> · {d.variant}</span> : null}
              </div>
              <div className="mt-0.5 font-mono text-[11px] text-muted">
                {d.combo ? `Mã sản phẩm: ${d.combo}` : 'Không có mã sản phẩm — tra theo tên + phân loại'}
                {' · '}
                {missing[i]?.lineCount ?? 0} dòng hàng
              </div>
              <div className="mt-2 grid grid-cols-4 gap-2">
                {[0, 1, 2, 3].map((col) => (
                  <div key={col}>
                    <label className="block font-sans text-[11px] text-muted">MÃ TP {col + 1}</label>
                    <input
                      value={d.tp[col]}
                      onChange={(e) => updateCell(i, 'tp', col, e.target.value)}
                      className="mt-0.5 w-full rounded border border-border bg-black/20 px-1.5 py-1 font-mono text-xs text-ink"
                    />
                    <label className="mt-1 block font-sans text-[11px] text-muted">SLTP {col + 1}</label>
                    <input
                      value={d.sl[col]}
                      onChange={(e) => updateCell(i, 'sl', col, e.target.value)}
                      className="mt-0.5 w-full rounded border border-border bg-black/20 px-1.5 py-1 font-mono text-xs text-ink"
                    />
                  </div>
                ))}
              </div>
              {errors[i] ? <div className="mt-1.5 font-sans text-[11px] text-red-400">{errors[i]}</div> : null}
            </div>
          ))}
        </div>

        <div className="mt-3 flex items-center justify-between">
          <span className="font-sans text-xs text-muted">Đã khai {filledCount}/{missing.length} mã</span>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={skip}
              className="rounded-md border border-border px-3 py-1.5 font-sans text-xs font-medium text-muted hover:text-ink disabled:opacity-40"
            >
              Bỏ qua, để #N/A
            </button>
            <button
              type="button"
              disabled={busy || firstError !== null || filledCount === 0}
              onClick={submit}
              className="rounded-md bg-accent px-3 py-1.5 font-sans text-xs font-semibold text-black disabled:cursor-not-allowed disabled:opacity-40"
            >
              Lưu và tiếp tục
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 7: Nối vào `ControlPanel.tsx`**

Đổi import dòng 4 thành:

```tsx
import { GetSTT, InspectTMDTFiles, ProcessFiles, SendZaloMessages } from '../../wailsjs/go/main/App'
import { TMDTDateRangeModal } from './TMDTDateRangeModal'
import type { TMDTDateRange } from '../lib/tmdtDateRange'
```

Thêm state trong component:

```tsx
  // Danh sách file TMĐT đang chờ người dùng chọn khoảng ngày. Modal chỉ
  // bật khi người dùng bấm "Xử lý" — thả file vào không hỏi gì.
  const [pendingTMDT, setPendingTMDT] = useState<string[] | null>(null)
```

Thay `handleProcess` bằng:

```tsx
  async function handleProcess() {
    if (files.length === 0) {
      appendLog('Không có file nào để xử lý!')
      return
    }
    let tmdtFiles: string[] = []
    try {
      tmdtFiles = await InspectTMDTFiles(files)
    } catch (err) {
      appendLog(`❌ Lỗi kiểm tra file TMĐT: ${String(err)}`)
      return
    }
    if (tmdtFiles.length > 0) {
      setPendingTMDT(tmdtFiles)
      return
    }
    await startBatch({})
  }

  async function startBatch(ranges: Record<string, TMDTDateRange>) {
    resetRows()
    setProcessing(true)
    appendLog('🚀 Bắt đầu xử lý...')
    try {
      await ProcessFiles(files, stt, ranges)
    } catch (err) {
      appendLog(`❌ Lỗi xử lý: ${String(err)}`)
      setProcessing(false)
    }
  }
```

Ngay trước `</>`/thẻ bao ngoài của phần `return` của `ControlPanel`, thêm:

```tsx
      {pendingTMDT && (
        <TMDTDateRangeModal
          fileNames={pendingTMDT.map((p) => p.split(/[\\/]/).pop() ?? p)}
          onCancel={() => setPendingTMDT(null)}
          onConfirm={(range) => {
            const ranges: Record<string, TMDTDateRange> = {}
            for (const p of pendingTMDT) ranges[p] = range
            setPendingTMDT(null)
            void startBatch(ranges)
          }}
        />
      )}
```

Nếu `return` của `ControlPanel` hiện chỉ trả về một thẻ duy nhất, bọc nó trong `<>...</>` để thêm được modal. Thêm `useState` vào import `react` ở dòng 1.

- [ ] **Step 8: Gắn `TMDTMissingModal` vào `App.tsx`**

Thêm import cạnh `ZaloQRModal` và render cạnh nó (dòng ~95):

```tsx
import { TMDTMissingModal } from './components/TMDTMissingModal'
...
      <TMDTMissingModal />
```

Đặt cạnh `<ZaloQRModal />` chứ không đặt trong `ControlPanel`: modal này do sự kiện backend điều khiển, phải sống độc lập với tab đang mở.

- [ ] **Step 9: Kiểm biên dịch và test**

```bash
cd GO/frontend && npx tsc --noEmit && npm test 2>&1 | tail -10
```

Expected: không lỗi biên dịch, test xanh.

- [ ] **Step 10: Commit**

```bash
git add GO/frontend/src/lib/tmdtMissing.ts GO/frontend/src/lib/tmdtMissing.test.ts GO/frontend/src/components/TMDTMissingModal.tsx GO/frontend/src/store/appStore.ts GO/frontend/src/hooks/useWailsEvents.ts GO/frontend/src/components/ControlPanel.tsx GO/frontend/src/App.tsx GO/frontend/package.json
git commit -m "feat(frontend): modal khai mã thiếu và nối luồng TMĐT vào nút Xử lý" -- GO/frontend/src/lib/tmdtMissing.ts GO/frontend/src/lib/tmdtMissing.test.ts GO/frontend/src/components/TMDTMissingModal.tsx GO/frontend/src/store/appStore.ts GO/frontend/src/hooks/useWailsEvents.ts GO/frontend/src/components/ControlPanel.tsx GO/frontend/src/App.tsx GO/frontend/package.json
```

---

## Task 13: Kiểm chứng đầu-cuối và chống hồi quy

**Files:**
- Modify: `GO/docs/haravan-export.md` (thêm mục nói nhánh này đã vào app)
- Không tạo file mã mới.

**Interfaces:**
- Consumes: toàn bộ Task 1–12
- Produces: —

- [ ] **Step 1: Chạy toàn bộ test Go**

```bash
cd GO && go test ./... 2>&1 | tail -40
```

Expected: PASS **toàn bộ**, gồm golden suite vendor hiện có (Coop/BigC/Lotte/Satra/Emart/Kingfood/Winmart/Fujimart/JMart/JIT — 151/151). Một test vendor đỏ là lỗi hồi quy do Task 1 hoặc Task 11 gây ra, phải sửa trước khi đi tiếp.

- [ ] **Step 2: Chạy toàn bộ test frontend + biên dịch**

```bash
cd GO/frontend && npm test 2>&1 | tail -20 && npx tsc --noEmit && npm run build 2>&1 | tail -10
```

Expected: test xanh, `tsc` không lỗi, `vite build` thành công.

- [ ] **Step 3: Kiểm CLI vẫn dùng được**

```bash
cd GO && go build -o /dev/null ./cmd/haravan-export && echo "CLI OK"
```

Expected: `CLI OK`.

- [ ] **Step 4: Thử thật trên app đang chạy**

```bash
cd GO && wails dev
```

Trong app:

1. Mở **Cài đặt ▸ Haravan (TMĐT)**, điền `access_token` (lấy từ `.env` cũ trước khi Task 1 xoá nó, hoặc tạo Private Token mới ở partners.haravan.com với quyền `com.read_orders`). Lưu.
2. **Sao lưu trước khi thử**: `cp "đơn hàng/XUẤT HÀNG HN-LA MỚI.xlsx" "đơn hàng/XUẤT HÀNG HN-LA MỚI.backup.xlsx"` và `cp dondathang.xlsx dondathang.backup.xlsx`. Nhánh này ghi đè thật.
3. Thả `đơn hàng/XUẤT HÀNG HN-LA MỚI.xlsx` vào app — **không có** popup nào bật (đúng thiết kế).
4. Bấm **Xử lý** → modal lịch bật. Kiểm: hôm nay và ngày mai bị mờ; chọn một ngày rồi thử bấm ngày thứ 8 → bị mờ; preset *Hôm qua* / *3 ngày* / *7 ngày* cho đúng khoảng.
5. Chọn *Hôm qua*, bấm **Bắt đầu xử lý**. Theo dõi khung log: phải thấy tiến độ từng trang, rồi hai dòng "Đã ghi ... sheet Haravan" và "Đã ghi ... dondathang.xlsx".
6. Mở `đơn hàng/XUẤT HÀNG HN-LA MỚI.xlsx`: sheet `Haravan` có 23 cột tiêu đề + dữ liệu; hai sheet `data shop` / `Mã misa` **còn nguyên**.
7. Mở `dondathang.xlsx`: dữ liệu từ dòng 9, cột `Z`/`AT`/`AU` trống, `AE` = 8, `AV` = 15.
8. Bảng kết quả: chỉ vài dòng tóm tắt dạng `{shop} · {ngày}`, hệ thống `TMĐT-TikTok`/`TMĐT-Shopee`.

- [ ] **Step 5: Thử nhánh khai mã thiếu**

Tạo tình huống thiếu mã một cách có kiểm soát: mở `đơn hàng/XUẤT HÀNG HN-LA MỚI.xlsx`, đổi giá trị `Mã combo` của một dòng `data shop` đang được dùng nhiều (ví dụ `SP000443` → `SP000443X`), lưu, đóng Excel. Chạy lại bước 4.

Kiểm:
1. Modal khai mã bật, hiện đúng tên sản phẩm / phân loại / mã sản phẩm và số dòng ảnh hưởng.
2. Bấm **Bỏ qua, để #N/A** → batch chạy tiếp, `dondathang.xlsx` có dòng mang `#N/A` ở cột Q, log có dòng cảnh báo. Batch **kết thúc**, nút Xử lý trở lại bình thường.
3. Chạy lại, lần này điền `MÃ TP 1` = `TP10127`, `SLTP 1` = `5`, bấm **Lưu và tiếp tục**. Kiểm sheet `data shop` có dòng mới ở cuối, và `dondathang.xlsx` không còn `#N/A` cho mã đó.
4. Chạy lại lần nữa: **không** còn modal — đó là mục đích của việc ghi trở lại bảng.
5. Đóng modal bằng cách không bấm gì trong 10 phút (hoặc tạm sửa `tmdtMissingTimeout` thành `20 * time.Second` để thử nhanh, rồi trả lại) → log báo hết giờ, batch kết thúc, app không treo. **Nhớ trả lại giá trị 10 phút.**

- [ ] **Step 6: Hoàn nguyên dữ liệu thử**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
mv "đơn hàng/XUẤT HÀNG HN-LA MỚI.backup.xlsx" "đơn hàng/XUẤT HÀNG HN-LA MỚI.xlsx"
mv dondathang.backup.xlsx dondathang.xlsx
```

- [ ] **Step 7: Cập nhật tài liệu CLI**

Thêm vào đầu `GO/docs/haravan-export.md`:

```markdown
> **Cập nhật 25/08/2026:** logic của tool này đã được đưa vào app chính
> thành nhánh xử lý TMĐT — thả `XUẤT HÀNG HN-LA MỚI.xlsx` vào app rồi bấm
> "Xử lý". Xem `docs/superpowers/specs/2026-08-25-tmdt-haravan-branch-design.md`.
> CLI ở `GO/cmd/haravan-export` vẫn giữ để đối chiếu và cho hai bố cục
> `haravan` / `full` mà app không dùng.
```

- [ ] **Step 8: Commit**

```bash
git add GO/docs/haravan-export.md
git commit -m "docs(haravan): trỏ CLI sang nhánh TMĐT trong app chính" -- GO/docs/haravan-export.md
```

---

## Đối chiếu kế hoạch với spec

| Yêu cầu trong spec | Task |
|---|---|
| Gộp module `GO/haravan` vào module chính | 1 |
| Nhận diện workbook TMĐT | 2 |
| Token + `exclude_shops` trong `settings.bhconfig` + tab Cài đặt | 3 |
| Fixture vàng 1.585 vào / 1.430 ra | 4 |
| Ghi `dondathang.xlsx`, Z/AT/AU trống, hằng số AE/AV | 5 |
| Toàn bộ quy tắc quy đổi + CLEVY + #N/A + gom unique | 6 |
| Ghi đè sheet `Haravan`, giữ 2 bảng tra cứu | 7 |
| Ghi bổ sung vào `data shop` | 8 |
| Ràng buộc ≤ hôm qua, ≤ 7 ngày, preset | 9, 11 (kiểm lại ở backend) |
| Modal lịch tự vẽ | 10 |
| Nhánh rẽ trong `runReservedBatch`, chờ trên channel + hạn giờ + `ctx.Done()` | 11 |
| Sự kiện `tmdt:missing`, `ResolveTMDTMissing`, `CancelTMDTMissing`, `InspectTMDTFiles`, `ProcessFiles` 3 tham số | 11, 12 |
| Modal khai mã, gom unique, bỏ trống = giữ #N/A | 12 |
| Bảng kết quả chỉ dòng tóm tắt theo (shop, ngày) | 11 |
| Không gửi Zalo cho TMĐT | mặc định — dòng tóm tắt không đi vào `zaloGrouping`, không task nào thêm |
| Không hồi quy golden suite vendor 151/151 | 1, 13 |
| Không bổ sung shop thiếu vào `Mã misa`, chỉ cảnh báo | 11 |
