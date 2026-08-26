# MISA Push Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Đưa luồng nhập khẩu chứng từ Excel của AMIS Kế toán (hiện là công cụ dòng lệnh `misapush`) vào app Wails, tách đơn theo nhánh kế toán rồi đẩy tự động — Hà Thành và HTLA, mỗi nhánh một file, một lần đẩy.

**Architecture:** Copy nguyên `misa/internal/misa` (chỉ dùng stdlib) vào `order-processor/internal/misa`. Thêm `internal/misapush` giữ 3 việc: quy tắc định tuyến (`route.go`), tách workbook theo dòng (`split.go`), và một lần đẩy cho một nhánh (`push.go`). `App.PushMisa` gom `ExcelRows` theo nhánh, tách file tạm, gọi `Pusher` tuần tự cho từng nhánh, phát sự kiện về frontend. Giao diện thêm 2 tab Cài đặt và một modal push dùng chung `SegmentedControl` với bộ chọn buổi JIT.

**Tech Stack:** Go 1.26, Wails v2.13, excelize v2.11, React 19 + zustand + Tailwind, test frontend bằng `node --experimental-strip-types --test`.

**Spec:** `docs/superpowers/specs/2026-08-26-misa-push-integration-design.md`

## Global Constraints

- **Không thêm dependency Go nào.** `misa/internal/misa` chỉ dùng thư viện chuẩn; `go.mod` và `go.sum` của `GO/` phải giữ nguyên byte-for-byte sau khi xong.
- **Ngôn ngữ:** mọi chuỗi hiện ra cho người dùng (log, nhãn nút, thông điệp lỗi) viết bằng **tiếng Việt có dấu**. Comment trong code cũng tiếng Việt, khớp phong cách các file xung quanh.
- **Tên nhánh trong dữ liệu:** đúng hai chuỗi `"ha_thanh"` và `"htla"`. Không dùng chuỗi hiển thị ("Hà Thành", "HTLA") làm khoá lưu trữ.
- **Sheet Excel:** tên sheet `"Don dat hang"` (không dấu, đúng như file mẫu MISA), dòng dữ liệu đầu tiên là **9**.
- **Ghi sổ:** luôn `Commit: true, Force: false`. Không thêm đường nào cho `Force: true`.
- **Bí mật:** `misa-session.json` ghi quyền `0o600`, phải nằm trong `.gitignore`. Không log giá trị `sid_url` hay bất kỳ token nào.
- **Không đụng vào:** thư mục `misa/` ở gốc repo, `internal/processing/**`, `internal/excelwriter/**`, luồng `ProcessFiles`/`SendZaloMessages` hiện có.
- **Repo dùng chung với một agent khác (codex).** Mỗi lần commit phải nêu **đường dẫn tường minh** cho `git add`. Tuyệt đối không `git add -A`, không `git add .`, không `git commit -a`.
- **Lệnh test Go:** chạy từ thư mục `GO/`. Lệnh test frontend: chạy từ `GO/frontend/`.

---

## File Structure

| File | Trách nhiệm |
|---|---|
| `GO/internal/misa/*.go` | Bản sao nguyên vẹn của thư viện gọi API AMIS Kế toán. Không sửa. |
| `GO/internal/misapush/route.go` | Khoá định tuyến, nhãn hiển thị, bảng gieo mặc định, phép tra. |
| `GO/internal/misapush/split.go` | Tách một workbook thành file chỉ chứa các dòng của một nhánh. |
| `GO/internal/misapush/push.go` | Một lần đẩy cho một nhánh: đăng nhập → đổi bộ dữ liệu → nhập khẩu. |
| `GO/internal/appsettings/store.go` | Thêm 2 map cấu hình `Misa` và `MisaRouting`. |
| `GO/app_misa.go` | Binding Wails: `MisaResolveRoutes`, `MisaRouteOptions`, `PushMisa` + điều phối. |
| `GO/app.go` | Thêm 3 field vào `App`, gieo bảng định tuyến lúc khởi động. |
| `GO/frontend/src/components/SegmentedControl.tsx` | Điều khiển phân đoạn dùng chung. |
| `GO/frontend/src/components/JITPeriodMenu.tsx` | Thu lại thành lớp bọc mỏng của `SegmentedControl`. |
| `GO/frontend/src/lib/misaBranch.ts` | Logic thuần của modal: gom nhóm, đếm, khoá nút, dựng map ghi nhớ. |
| `GO/frontend/src/components/MisaRoutingEditor.tsx` | Bảng định tuyến trong Cài đặt. |
| `GO/frontend/src/components/MisaPushModal.tsx` | Modal chọn đơn + nhánh, và màn hình kết quả. |
| `GO/frontend/src/components/SettingsModal.tsx` | Thêm 2 tab MISA. |
| `GO/frontend/src/components/ControlPanel.tsx` | Bật nút "Push MISA". |
| `GO/frontend/src/store/appStore.ts` | `isPushing`, `misaResults`. |
| `GO/frontend/src/hooks/useWailsEvents.ts` | Bắt `misa:log` / `misa:pushed` / `misa:done`. |

---

### Task 1: Nhập thư viện `internal/misa`

**Files:**
- Create: `GO/internal/misa/auth.go`, `client.go`, `database.go`, `importexcel.go`, `renew.go`, `session.go`
- Create: `GO/internal/misa/database_test.go`, `importexcel_test.go`, `renew_test.go`, `session_test.go`

**Interfaces:**
- Consumes: không có.
- Produces: package `misa` với `NewClient(baseURL string) *Client`, `Client.Log func(format string, args ...any)`, `Client.Login(ctx) error`, `Client.UseSession(*Session)`, `Client.SetRenewFromURL(endpoint string, onNew func(*Session) error)`, `Client.SwitchDatabaseByName(ctx, needle string) (Database, error)`, `Client.ImportExcel(ctx, ImportOptions) (*ImportResult, error)`, `LoadSession(path string) (*Session, error)`, `Session.Save(path string) error`, hằng `RefTypeSAOrder = 3520`, `TableSAOrder = "sa_order"`, kiểu `ImportOptions`, `ImportResult`, `RowError`, `Database`, và `ErrUnauthorized`.

- [ ] **Step 1: Copy 10 file sang module app**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
mkdir -p GO/internal/misa
cp misa/internal/misa/auth.go \
   misa/internal/misa/client.go \
   misa/internal/misa/database.go \
   misa/internal/misa/importexcel.go \
   misa/internal/misa/renew.go \
   misa/internal/misa/session.go \
   misa/internal/misa/database_test.go \
   misa/internal/misa/importexcel_test.go \
   misa/internal/misa/renew_test.go \
   misa/internal/misa/session_test.go \
   GO/internal/misa/
```

- [ ] **Step 2: Kiểm tra không có import nào trỏ ra ngoài package**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
grep -rn "misasniff/" internal/misa/ ; echo "exit=$?"
```
Expected: không in ra dòng nào, `exit=1` (grep không tìm thấy gì). Nếu có dòng nào in ra thì **dừng lại** và báo — giả định "chỉ dùng stdlib" đã sai.

- [ ] **Step 3: Chạy test của package vừa copy**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./internal/misa/
```
Expected: `ok  	order-processor/internal/misa`

- [ ] **Step 4: Khẳng định `go.mod` / `go.sum` không đổi**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git diff --stat GO/go.mod GO/go.sum
```
Expected: **không in ra gì**. Nếu có thay đổi thì đã lỡ thêm dependency — hoàn tác và báo.

- [ ] **Step 5: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/internal/misa
git commit -m "feat(misa): nhập thư viện gọi API AMIS Kế toán vào module app

Copy nguyên 10 file từ misa/internal/misa. Package chỉ dùng thư viện chuẩn
nên go.mod không đổi một dòng nào. Bộ test đi kèm giữ nguyên giá trị: nó
khoá lại tham số từng bước của luồng nhập khẩu 5 bước, việc bắn lại nguyên
vẹn bản đồ cột sang step3/step4, và chặn ghi sổ khi còn dòng không hợp lệ.

misa/ ở gốc repo giữ nguyên làm công cụ khảo sát API."
```

---

### Task 2: `misapush.RouteKey` — quy tắc định tuyến

**Files:**
- Create: `GO/internal/misapush/route.go`
- Test: `GO/internal/misapush/route_test.go`

**Interfaces:**
- Consumes: không có.
- Produces:
  - `const BranchHaThanh = "ha_thanh"`, `const BranchHTLA = "htla"`
  - `const TMDTPrefix = "TMĐT-"`, `const TMDTRouteKey = "TMĐT-*"`
  - `func RouteKey(system, customerCode, shipTo string) string`
  - `func Label(key string) string`
  - `func SeedRouting() map[string]string`
  - `func Lookup(routing map[string]string, key string) string`
  - `func ApplySeed(routing map[string]string) bool`

- [ ] **Step 1: Viết test đỏ**

Create `GO/internal/misapush/route_test.go`:

```go
package misapush

import (
	"reflect"
	"sort"
	"testing"
)

func TestRouteKey_JITTáchTheoKho(t *testing.T) {
	if got := RouteKey("JIT-CHOICE", "MN_JIT_01512", "WH6_HN"); got != "JIT-CHOICE/WH6_HN" {
		t.Errorf("RouteKey JIT WH6_HN = %q, want %q", got, "JIT-CHOICE/WH6_HN")
	}
	if got := RouteKey("JIT-CHOICE", "MN_JIT_01512", "WH6_HTLA"); got != "JIT-CHOICE/WH6_HTLA" {
		t.Errorf("RouteKey JIT WH6_HTLA = %q, want %q", got, "JIT-CHOICE/WH6_HTLA")
	}
}

func TestRouteKey_JITThiếuKhoKhôngPanic(t *testing.T) {
	if got := RouteKey("JIT-CHOICE", "MN_JIT_01512", "   "); got != "JIT-CHOICE" {
		t.Errorf("RouteKey JIT không kho = %q, want %q", got, "JIT-CHOICE")
	}
}

func TestRouteKey_BigCTáchTheoPhânKhúc(t *testing.T) {
	// Đúng 4 mã mà bigc.ResolveCustomerCode sinh ra, không hơn.
	cases := map[string]string{
		"MB_GC_BIGC":   "BigC/GC",
		"MN_GC_BIGCAC": "BigC/GC",
		"MB_MT_BIGC":   "BigC/MT",
		"MN_MT_BIGCAC": "BigC/MT",
	}
	for code, want := range cases {
		if got := RouteKey("BigC", code, ""); got != want {
			t.Errorf("RouteKey BigC %s = %q, want %q", code, got, want)
		}
	}
}

func TestRouteKey_BigCMãThiếuPhầnKhôngPanic(t *testing.T) {
	if got := RouteKey("BigC", "BIGCGARDEN", ""); got != "BigC" {
		t.Errorf("RouteKey BigC mã cũ = %q, want %q", got, "BigC")
	}
}

func TestRouteKey_HệThốngKhácGiữNguyên(t *testing.T) {
	if got := RouteKey("Lotte", "MN_MT_LOT1001", ""); got != "Lotte" {
		t.Errorf("RouteKey Lotte = %q, want %q", got, "Lotte")
	}
	if got := RouteKey("TMĐT-Shopee", "MN_TMDT_00015", ""); got != "TMĐT-Shopee" {
		t.Errorf("RouteKey TMĐT = %q, want %q", got, "TMĐT-Shopee")
	}
}

func TestLabel_DễĐọcChoTừngDạngKhoá(t *testing.T) {
	cases := map[string]string{
		"BigC/GC":            "BigC · gia công",
		"BigC/MT":            "BigC · modern trade",
		"BigC":               "BigC",
		"JIT-CHOICE/WH6_HN":  "JIT · kho WH6_HN",
		"JIT-CHOICE":         "JIT",
		"TMĐT-*":             "TMĐT (mọi sàn)",
		"Lotte":              "Lotte",
	}
	for key, want := range cases {
		if got := Label(key); got != want {
			t.Errorf("Label(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestLookup_KhớpĐúngKhôngPhânBiệtHoaThường(t *testing.T) {
	routing := map[string]string{"lotte": BranchHTLA}
	if got := Lookup(routing, "Lotte"); got != BranchHTLA {
		t.Errorf("Lookup Lotte = %q, want %q", got, BranchHTLA)
	}
	if got := Lookup(routing, "Satra"); got != "" {
		t.Errorf("Lookup khoá chưa map = %q, want chuỗi rỗng", got)
	}
}

func TestLookup_TMĐTRơiVềKhoáTiềnTố(t *testing.T) {
	routing := map[string]string{TMDTRouteKey: BranchHTLA}
	if got := Lookup(routing, "TMĐT-Sàn Chưa Từng Thấy"); got != BranchHTLA {
		t.Errorf("Lookup sàn mới = %q, want %q", got, BranchHTLA)
	}
}

func TestLookup_KhớpĐúngThắngTiềnTố(t *testing.T) {
	routing := map[string]string{
		TMDTRouteKey:  BranchHTLA,
		"TMĐT-Shopee": BranchHaThanh,
	}
	if got := Lookup(routing, "TMĐT-Shopee"); got != BranchHaThanh {
		t.Errorf("Lookup TMĐT-Shopee = %q, want %q (khớp đúng phải thắng tiền tố)", got, BranchHaThanh)
	}
}

func TestSeedRouting_PhủMọiHệThốngProcessorSinhRa(t *testing.T) {
	// Danh sách này là MỌI giá trị OrderRow.System mà các processor hiện có
	// sinh ra, cộng hai khoá tách nhỏ. Thêm processor mới mà quên gieo nhánh
	// thì test này đỏ ngay, thay vì lặng lẽ chặn push giữa lúc cần đẩy đơn.
	want := []string{
		"BigC/GC", "BigC/MT",
		"COOPFOOD", "COOPMART", "Coop",
		"Emart", "FujiMart",
		"JIT-CHOICE/WH6_HN", "JIT-CHOICE/WH6_HTLA",
		"JMart", "Kingfood", "Lotte", "MR.DIY", "Satra",
		"TMĐT-*", "Winmart",
	}
	seed := SeedRouting()
	got := make([]string, 0, len(seed))
	for k := range seed {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SeedRouting keys =\n  %v\nwant\n  %v", got, want)
	}
}

func TestSeedRouting_ĐúngNhánhChoTừngKhoá(t *testing.T) {
	seed := SeedRouting()
	htla := []string{"TMĐT-*", "COOPMART", "COOPFOOD", "Coop", "Lotte", "Satra", "MR.DIY", "FujiMart", "BigC/GC", "JIT-CHOICE/WH6_HTLA"}
	haThanh := []string{"BigC/MT", "Emart", "Winmart", "Kingfood", "JMart", "JIT-CHOICE/WH6_HN"}
	for _, k := range htla {
		if seed[k] != BranchHTLA {
			t.Errorf("seed[%q] = %q, want %q", k, seed[k], BranchHTLA)
		}
	}
	for _, k := range haThanh {
		if seed[k] != BranchHaThanh {
			t.Errorf("seed[%q] = %q, want %q", k, seed[k], BranchHaThanh)
		}
	}
}

func TestApplySeed_ChỉThêmKhoáCònThiếu(t *testing.T) {
	// Lotte đã được người dùng đổi sang Hà Thành — bảng gieo KHÔNG được kéo
	// nó về HTLA, nếu không thì mỗi lần sửa hằng số trong code là đẩy đơn
	// sang sổ của pháp nhân khác mà không ai bấm gì.
	routing := map[string]string{"Lotte": BranchHaThanh}

	changed := ApplySeed(routing)

	if !changed {
		t.Error("ApplySeed = false, want true (còn nhiều khoá chưa có)")
	}
	if routing["Lotte"] != BranchHaThanh {
		t.Errorf("ApplySeed ghi đè Lotte thành %q — không được phép", routing["Lotte"])
	}
	if routing["Satra"] != BranchHTLA {
		t.Errorf("ApplySeed không thêm Satra: %q", routing["Satra"])
	}
}

func TestApplySeed_KhôngĐổiGìThìBáoFalse(t *testing.T) {
	routing := SeedRouting()
	if ApplySeed(routing) {
		t.Error("ApplySeed = true trên map đã đủ khoá, want false")
	}
}
```

- [ ] **Step 2: Chạy test để chắc chắn nó đỏ**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./internal/misapush/
```
Expected: FAIL — `undefined: RouteKey`, `undefined: Label`, `undefined: SeedRouting`, `undefined: Lookup`, `undefined: ApplySeed`.

- [ ] **Step 3: Viết bản cài đặt tối thiểu**

Create `GO/internal/misapush/route.go`:

```go
// Package misapush đẩy đơn hàng đã xử lý lên AMIS Kế toán: quyết định đơn
// nào vào sổ của pháp nhân nào (route.go), tách workbook theo nhánh
// (split.go), và thực hiện một lần nhập khẩu (push.go).
package misapush

import "strings"

// Hai nhánh kế toán. Đây là khoá LƯU TRỮ, không phải chuỗi hiển thị —
// đổi tên bộ dữ liệu bên MISA không được làm hỏng cấu hình đã lưu.
const (
	BranchHaThanh = "ha_thanh"
	BranchHTLA    = "htla"
)

const (
	systemJIT  = "JIT-CHOICE"
	systemBigC = "BigC"

	// TMDTPrefix là tiền tố mà app_tmdt.go gắn trước tên sàn.
	TMDTPrefix = "TMĐT-"
	// TMDTRouteKey đại diện cho MỌI sàn TMĐT. Tên sàn do
	// haravan.DetectChannel dò ra nên không liệt kê hết được; một khoá
	// tiền tố phủ luôn cả sàn mai sau mới có.
	TMDTRouteKey = "TMĐT-*"
)

// RouteKey trả về khoá tra bảng định tuyến cho một đơn.
//
// Tên hệ thống một mình không đủ ở hai chỗ, cả hai đều là yêu cầu nghiệp
// vụ thật:
//   - JIT tách theo kho giao (ShipTo, bóc từ tên file air waybill) — cùng
//     mang System "JIT-CHOICE" và cùng mã khách hàng gán cứng.
//   - BigC tách theo phân khúc mã khách hàng: gia công (GC) và modern
//     trade (MT) vào hai sổ khác nhau, cùng mang System "BigC".
func RouteKey(system, customerCode, shipTo string) string {
	switch system {
	case systemJIT:
		if w := strings.TrimSpace(shipTo); w != "" {
			return systemJIT + "/" + w
		}
		return systemJIT
	case systemBigC:
		if seg := customerSegment(customerCode); seg != "" {
			return systemBigC + "/" + seg
		}
		return systemBigC
	default:
		return system
	}
}

// customerSegment lấy phần giữa của mã khách hàng dạng
// <miền>_<phân khúc>_<mã NCC>, viết hoa. Trả rỗng nếu mã không đủ 3 phần
// (mã đời cũ như "BIGCGARDEN") — bên gọi tự quyết định làm gì với nó.
func customerSegment(code string) string {
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(code)), "_")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

// Label dựng nhãn hiển thị từ khoá định tuyến. Khoá là chuỗi máy đọc,
// nhãn là thứ người dùng nhìn thấy trong Cài đặt và modal push — nhìn
// nhãn phải hiểu ngay vì sao hai đơn BigC lại vào hai sổ khác nhau.
func Label(key string) string {
	switch {
	case key == TMDTRouteKey:
		return "TMĐT (mọi sàn)"
	case key == systemJIT:
		return "JIT"
	case strings.HasPrefix(key, systemJIT+"/"):
		return "JIT · kho " + strings.TrimPrefix(key, systemJIT+"/")
	case key == systemBigC+"/GC":
		return "BigC · gia công"
	case key == systemBigC+"/MT":
		return "BigC · modern trade"
	case strings.HasPrefix(key, systemBigC+"/"):
		return "BigC · " + strings.TrimPrefix(key, systemBigC+"/")
	default:
		return key
	}
}

// SeedRouting là bảng định tuyến mặc định, phủ mọi hệ thống mà các
// processor hiện có sinh ra. Trả về map MỚI mỗi lần gọi để bên gọi sửa
// thoải mái mà không đụng vào bản gốc.
func SeedRouting() map[string]string {
	return map[string]string{
		TMDTRouteKey:                BranchHTLA,
		"COOPMART":                  BranchHTLA,
		"COOPFOOD":                  BranchHTLA,
		"Coop":                      BranchHTLA,
		"Lotte":                     BranchHTLA,
		"Satra":                     BranchHTLA,
		"MR.DIY":                    BranchHTLA,
		"FujiMart":                  BranchHTLA,
		systemBigC + "/GC":          BranchHTLA,
		systemJIT + "/WH6_HTLA":     BranchHTLA,
		systemBigC + "/MT":          BranchHaThanh,
		"Emart":                     BranchHaThanh,
		"Winmart":                   BranchHaThanh,
		"Kingfood":                  BranchHaThanh,
		"JMart":                     BranchHaThanh,
		systemJIT + "/WH6_HN":       BranchHaThanh,
	}
}

// Lookup tra nhánh của một khoá. Khớp đúng trước (không phân biệt hoa
// thường), riêng khoá TMĐT thì thử thêm khoá tiền tố. Trả chuỗi rỗng khi
// chưa map — bên gọi PHẢI coi đó là "chưa biết", không được đoán bừa một
// nhánh.
func Lookup(routing map[string]string, key string) string {
	if b := lookupFold(routing, key); b != "" {
		return b
	}
	if strings.HasPrefix(key, TMDTPrefix) {
		return lookupFold(routing, TMDTRouteKey)
	}
	return ""
}

func lookupFold(routing map[string]string, key string) string {
	if b, ok := routing[key]; ok && b != "" {
		return b
	}
	for k, v := range routing {
		if v != "" && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// ApplySeed điền các khoá còn thiếu của bảng gieo vào routing và cho biết
// có thêm gì không.
//
// KHÔNG BAO GIỜ ghi đè khoá đã có, kể cả khi giá trị hiện tại khác bảng
// gieo. Bảng gieo được vật chất hoá xuống settings.bhconfig ngay lần chạy
// đầu chính là để có tính chất này: sửa hằng số SeedRouting ở phiên bản
// sau không làm xê dịch một cấu hình nào đang chạy. Nếu bảng gieo chỉ
// sống trong code như một giá trị dự phòng, một lần sửa hằng số sẽ lặng
// lẽ đổi nhánh của mọi mục người dùng chưa từng chạm vào — tức là đẩy đơn
// vào sổ của pháp nhân khác mà không ai bấm gì.
func ApplySeed(routing map[string]string) bool {
	changed := false
	for k, v := range SeedRouting() {
		if _, ok := routing[k]; !ok {
			routing[k] = v
			changed = true
		}
	}
	return changed
}
```

- [ ] **Step 4: Chạy test để chắc chắn nó xanh**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./internal/misapush/ -v
```
Expected: PASS toàn bộ 12 test.

- [ ] **Step 5: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/internal/misapush/route.go GO/internal/misapush/route_test.go
git commit -m "feat(misapush): khoá định tuyến nhánh kế toán

RouteKey tách nhỏ đúng hai chỗ mà tên hệ thống không diễn đạt được: JIT
theo kho giao (ShipTo), BigC theo phân khúc mã khách hàng (GC/MT). Còn lại
dùng thẳng tên hệ thống.

ApplySeed chỉ điền khoá còn thiếu, không bao giờ ghi đè — bảng gieo được
vật chất hoá xuống cấu hình để sửa hằng số ở bản sau không xê dịch một cài
đặt nào đang chạy.

Test khoá lại danh sách 16 khoá của bảng gieo, nên thêm processor mới mà
quên gieo nhánh thì đỏ ngay thay vì lặng lẽ chặn push."
```

---

### Task 3: `misapush.SplitWorkbook` — tách file theo nhánh

**Files:**
- Create: `GO/internal/misapush/split.go`
- Test: `GO/internal/misapush/split_test.go`

**Interfaces:**
- Consumes: không có (độc lập với Task 2).
- Produces: `const SheetName = "Don dat hang"`, `const FirstDataRow = 9`, `func SplitWorkbook(src, dst string, keep []int) error`.

- [ ] **Step 1: Viết test đỏ**

Create `GO/internal/misapush/split_test.go`:

```go
package misapush

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildWorkbook dựng một workbook giống mẫu nhập khẩu của MISA: khối tiêu
// đề 8 dòng có ô gộp, rồi n dòng dữ liệu mang công thức tự trỏ vào chính
// dòng mình (Z{r} = Y{r}*X{r}) — đúng thứ excelwriter.WriteOrderRows ghi.
func buildWorkbook(t *testing.T, path string, n int) {
	t.Helper()
	f := excelize.NewFile()
	idx, err := f.NewSheet(SheetName)
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	if err := f.SetCellValue(SheetName, "A1", "FILE MẪU ĐƠN ĐẶT HÀNG"); err != nil {
		t.Fatalf("SetCellValue A1: %v", err)
	}
	if err := f.SetCellValue(SheetName, "Q7", "Chi tiết hàng tiền"); err != nil {
		t.Fatalf("SetCellValue Q7: %v", err)
	}
	if err := f.MergeCell(SheetName, "Q7", "AP7"); err != nil {
		t.Fatalf("MergeCell: %v", err)
	}
	if err := f.SetCellValue(SheetName, "A8", "Ngày đơn hàng (*)"); err != nil {
		t.Fatalf("SetCellValue A8: %v", err)
	}
	if err := f.SetCellValue(SheetName, "B8", "Số đơn hàng (*)"); err != nil {
		t.Fatalf("SetCellValue B8: %v", err)
	}

	for i := 0; i < n; i++ {
		r := FirstDataRow + i
		if err := f.SetCellValue(SheetName, fmt.Sprintf("B%d", r), fmt.Sprintf("PO-%d", r)); err != nil {
			t.Fatalf("SetCellValue B%d: %v", r, err)
		}
		if err := f.SetCellValue(SheetName, fmt.Sprintf("X%d", r), r); err != nil {
			t.Fatalf("SetCellValue X%d: %v", r, err)
		}
		if err := f.SetCellValue(SheetName, fmt.Sprintf("Y%d", r), 100+r); err != nil {
			t.Fatalf("SetCellValue Y%d: %v", r, err)
		}
		if err := f.SetCellFormula(SheetName, fmt.Sprintf("Z%d", r), fmt.Sprintf("Y%d*X%d", r, r)); err != nil {
			t.Fatalf("SetCellFormula Z%d: %v", r, err)
		}
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSplitWorkbook_GiữĐúngCácDòngĐãChọn(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "htla.xlsx")
	buildWorkbook(t, src, 5) // r9..r13

	if err := SplitWorkbook(src, dst, []int{9, 11, 13}); err != nil {
		t.Fatalf("SplitWorkbook: %v", err)
	}

	f, err := excelize.OpenFile(dst)
	if err != nil {
		t.Fatalf("OpenFile dst: %v", err)
	}
	defer f.Close()

	wantPO := []string{"PO-9", "PO-11", "PO-13"}
	for i, want := range wantPO {
		cell := fmt.Sprintf("B%d", FirstDataRow+i)
		got, err := f.GetCellValue(SheetName, cell)
		if err != nil {
			t.Fatalf("GetCellValue %s: %v", cell, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", cell, got, want)
		}
	}

	rows, err := f.GetRows(SheetName)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) != FirstDataRow+len(wantPO)-1 {
		t.Errorf("số dòng = %d, want %d", len(rows), FirstDataRow+len(wantPO)-1)
	}
}

func TestSplitWorkbook_HạĐúngChỉSốCôngThức(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "ha_thanh.xlsx")
	buildWorkbook(t, src, 5)

	if err := SplitWorkbook(src, dst, []int{9, 11, 13}); err != nil {
		t.Fatalf("SplitWorkbook: %v", err)
	}

	f, err := excelize.OpenFile(dst)
	if err != nil {
		t.Fatalf("OpenFile dst: %v", err)
	}
	defer f.Close()

	// Công thức tự trỏ vào chính dòng mình PHẢI theo dòng xuống chỗ mới,
	// nếu không "Thành tiền" sẽ nhân sai hàng và MISA đọc vào số bậy.
	for i := 0; i < 3; i++ {
		r := FirstDataRow + i
		got, err := f.GetCellFormula(SheetName, fmt.Sprintf("Z%d", r))
		if err != nil {
			t.Fatalf("GetCellFormula Z%d: %v", r, err)
		}
		want := fmt.Sprintf("Y%d*X%d", r, r)
		if got != want {
			t.Errorf("Z%d = %q, want %q", r, got, want)
		}
	}
}

func TestSplitWorkbook_GiữNguyênKhốiTiêuĐềVàÔGộp(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "out.xlsx")
	buildWorkbook(t, src, 5)

	if err := SplitWorkbook(src, dst, []int{10}); err != nil {
		t.Fatalf("SplitWorkbook: %v", err)
	}

	f, err := excelize.OpenFile(dst)
	if err != nil {
		t.Fatalf("OpenFile dst: %v", err)
	}
	defer f.Close()

	for cell, want := range map[string]string{
		"A1": "FILE MẪU ĐƠN ĐẶT HÀNG",
		"Q7": "Chi tiết hàng tiền",
		"A8": "Ngày đơn hàng (*)",
		"B8": "Số đơn hàng (*)",
	} {
		got, err := f.GetCellValue(SheetName, cell)
		if err != nil {
			t.Fatalf("GetCellValue %s: %v", cell, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q — khối tiêu đề mẫu MISA phải nguyên vẹn", cell, got, want)
		}
	}

	merged, err := f.GetMergeCells(SheetName)
	if err != nil {
		t.Fatalf("GetMergeCells: %v", err)
	}
	if len(merged) != 1 || merged[0].GetStartAxis() != "Q7" || merged[0].GetEndAxis() != "AP7" {
		t.Errorf("ô gộp = %#v, want đúng một ô Q7:AP7", merged)
	}
}

func TestSplitWorkbook_KhôngSửaFileNguồn(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "out.xlsx")
	buildWorkbook(t, src, 5)

	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile trước: %v", err)
	}
	if err := SplitWorkbook(src, dst, []int{9}); err != nil {
		t.Fatalf("SplitWorkbook: %v", err)
	}
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile sau: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("file nguồn bị sửa — SplitWorkbook chỉ được đọc nguồn, mọi thay đổi phải rơi vào bản sao")
	}
}

func TestSplitWorkbook_KeepRỗngLàLỗi(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	dst := filepath.Join(dir, "out.xlsx")
	buildWorkbook(t, src, 3)

	if err := SplitWorkbook(src, dst, nil); err == nil {
		t.Error("SplitWorkbook với keep rỗng = nil, want lỗi")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("SplitWorkbook để lại file rác khi keep rỗng")
	}
}

func TestSplitWorkbook_ChỉSốNgoàiPhạmViLàLỗi(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dondathang.xlsx")
	buildWorkbook(t, src, 3) // r9..r11

	for _, bad := range [][]int{{8}, {12}, {9, 99}} {
		dst := filepath.Join(dir, fmt.Sprintf("out-%d.xlsx", bad[len(bad)-1]))
		if err := SplitWorkbook(src, dst, bad); err == nil {
			t.Errorf("SplitWorkbook keep=%v = nil, want lỗi", bad)
		}
	}
}
```

- [ ] **Step 2: Chạy test để chắc chắn nó đỏ**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./internal/misapush/ -run TestSplitWorkbook
```
Expected: FAIL — `undefined: SplitWorkbook`, `undefined: SheetName`, `undefined: FirstDataRow`.

- [ ] **Step 3: Viết bản cài đặt tối thiểu**

Create `GO/internal/misapush/split.go`:

```go
package misapush

import (
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

const (
	// SheetName là tên sheet trong mẫu nhập khẩu của MISA — không dấu,
	// đúng như file mẫu tải về từ AMIS Kế toán.
	SheetName = "Don dat hang"
	// FirstDataRow là dòng dữ liệu đầu tiên. Dòng 1..8 là khối hướng dẫn
	// và hàng tiêu đề của mẫu; cùng quy ước mà
	// excelwriter.ClearOrderRows đang dùng.
	FirstDataRow = 9
)

// SplitWorkbook copy src sang dst rồi xoá mọi dòng dữ liệu không nằm
// trong keep, để lại một workbook chỉ chứa đơn của một nhánh kế toán.
//
// Cách làm là copy-rồi-xoá chứ không dựng lại workbook từ đầu: khối tiêu
// đề của mẫu MISA mang ô gộp, style và độ rộng cột: chép tay từng thứ đó
// là thêm một nguồn sai lệch không cần thiết. excelize.RemoveRow tự hạ
// chỉ số các công thức tương đối, nên "Thành tiền" (=Y{r}*X{r}) vẫn trỏ
// đúng hàng của nó sau khi dồn lên.
func SplitWorkbook(src, dst string, keep []int) error {
	if len(keep) == 0 {
		return fmt.Errorf("misapush: không có dòng nào để tách")
	}

	wanted := make(map[int]bool, len(keep))
	for _, r := range keep {
		wanted[r] = true
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("misapush: đọc %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("misapush: ghi %s: %w", dst, err)
	}

	if err := trimRows(dst, wanted); err != nil {
		// Không để lại file dở dang: bước sau sẽ upload thẳng file này
		// lên MISA, một bản cắt nửa chừng là đẩy thiếu đơn mà không ai
		// nhìn thấy.
		os.Remove(dst)
		return err
	}
	return nil
}

func trimRows(path string, wanted map[int]bool) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return fmt.Errorf("misapush: mở %s: %w", path, err)
	}
	defer f.Close()

	rows, err := f.GetRows(SheetName)
	if err != nil {
		return fmt.Errorf("misapush: đọc sheet %q: %w", SheetName, err)
	}
	last := len(rows)

	for r := range wanted {
		if r < FirstDataRow || r > last {
			return fmt.Errorf("misapush: dòng %d nằm ngoài vùng dữ liệu %d..%d của %s",
				r, FirstDataRow, last, path)
		}
	}

	// Xoá từ dưới lên: xoá từ trên xuống thì mọi dòng phía sau tụt chỉ số
	// và những lần xoá tiếp theo sẽ nhắm nhầm hàng.
	for r := last; r >= FirstDataRow; r-- {
		if wanted[r] {
			continue
		}
		if err := f.RemoveRow(SheetName, r); err != nil {
			return fmt.Errorf("misapush: xoá dòng %d: %w", r, err)
		}
	}

	if err := f.Save(); err != nil {
		return fmt.Errorf("misapush: lưu %s: %w", path, err)
	}
	return nil
}
```

- [ ] **Step 4: Chạy test để chắc chắn nó xanh**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./internal/misapush/ -v
```
Expected: PASS toàn bộ, gồm 6 test mới của `SplitWorkbook`.

- [ ] **Step 5: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/internal/misapush/split.go GO/internal/misapush/split_test.go
git commit -m "feat(misapush): tách workbook theo nhánh bằng copy rồi xoá dòng thừa

Giữ nguyên khối tiêu đề mẫu MISA (ô gộp, style, độ rộng cột) thay vì dựng
lại workbook — chép tay những thứ đó là thêm một nguồn sai lệch không cần
thiết. Xoá từ dưới lên để chỉ số dòng phía trên không xê dịch giữa chừng.

Test khoá lại điều dễ hỏng nhất: công thức Thành tiền (=Y{r}*X{r}) tự trỏ
vào chính dòng mình phải theo dòng xuống chỗ mới, nếu không MISA đọc vào
số nhân sai hàng."
```

---

### Task 4: `misapush.Pusher` — một lần đẩy cho một nhánh

**Files:**
- Create: `GO/internal/misapush/push.go`
- Test: `GO/internal/misapush/push_test.go`

**Interfaces:**
- Consumes: package `misa` từ Task 1 (`NewClient`, `LoadSession`, `Session.Save`, `Client.UseSession`, `Client.SetRenewFromURL`, `Client.Login`, `Client.SwitchDatabaseByName`, `Client.ImportExcel`, `ImportOptions`, `ImportResult`, `RefTypeSAOrder`, `TableSAOrder`).
- Produces:
  - `type Request struct { SessionPath, SidURL, Database, FilePath string; BaseURL string; Log func(string) }`
  - `type Pusher interface { Push(ctx context.Context, req Request) (*misa.ImportResult, error) }`
  - `type HTTPPusher struct{}` cài đặt `Pusher`.

- [ ] **Step 1: Viết test đỏ**

Create `GO/internal/misapush/push_test.go`:

```go
package misapush

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeMISA dựng lại đúng chuỗi phản hồi mà AMIS Kế toán trả về, ghi lại
// thứ tự các endpoint đã bị gọi.
type fakeMISA struct {
	mu      sync.Mutex
	calls   []string
	invalid bool // step3 trả về một dòng không hợp lệ
}

const misaContextJSON = `{"TenantId":"t-1","TenantCode":"CODE","DatabaseId":"db-cu","UserId":"u-1"}`

func (f *fakeMISA) record(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, path)
}

func (f *fakeMISA) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeMISA) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case p == "/g2/api/auth/v1/account/login/misa_id":
			f.record("login")
			w.Write([]byte(`{"Success":true,"Data":{"AccessToken":{"Token":"tok","TokenExpired":86400},"Context":` + misaContextJSON + `}}`))

		case strings.HasPrefix(p, "/g2/api/system/v1/database/databases_user_can_see"):
			f.record("databases")
			w.Write([]byte(`{"Success":true,"Data":[{"database_id":"db-htla","database_name":"2025-Thuế-Long An"},{"database_id":"db-ht","database_name":"2025-NỘI BỘ- HÀ THÀNH"}]}`))

		case strings.HasPrefix(p, "/g2/api/auth/v1/account/database-context/"):
			f.record("database-context:" + strings.Split(p, "/")[7])
			w.Write([]byte(`{"Success":true,"Data":{"IsSwitchTenant":false,"Context":` + misaContextJSON + `}}`))

		case p == "/g2/api/file/v1/file/multi":
			f.record("upload")
			w.Write([]byte(`{"Success":true,"Data":[{"name":"tok.xlsx","ext":".xlsx","size":1,"index":0,"source":"htla.xlsx"}]}`))

		case p == "/g2/api/import/v1/import/sheetname":
			f.record("sheetname")
			w.Write([]byte(`{"Success":true,"Data":{"sheets":[{"Index":0,"Name":"Don dat hang","CountRow":10,"MaxDataRow":2,"RowHeaderUltil":7}]}}`))

		case p == "/g2/api/import/v1/import/step2":
			f.record("step2")
			w.Write([]byte(`{"Success":true,"Data":{"excel_columns":["Số đơn hàng"],"columns":[{"column_id":"refno","column_name":"Số đơn hàng","column_excel":0,"not_null":true}],"token":"tok.xlsx"}}`))

		case p == "/g2/api/import/v1/import/step3":
			f.record("step3")
			w.Write([]byte(`{"Success":true,"Data":"sess-3"}`))

		case p == "/g2/api/import/v1/worker/check_step3":
			f.record("check_step3")
			if f.invalid {
				w.Write([]byte(`{"Success":true,"Data":{"end":true,"step_data":[{"refno":"PO-1","is_valid":false,"validate_description":"Mã khách hàng <X> không có trong danh mục"}]}}`))
				return
			}
			w.Write([]byte(`{"Success":true,"Data":{"end":true,"step_data":[{"refno":"PO-1","is_valid":true}]}}`))

		case p == "/g2/api/import/v1/import/step4":
			f.record("step4")
			w.Write([]byte(`{"Success":true,"Data":"sess-4"}`))

		case p == "/g2/api/import/v1/worker/check_step4":
			f.record("check_step4")
			w.Write([]byte(`{"Success":true,"Data":{"end":true,"valid":1,"invalid":0,"skip":0,"result_file":""}}`))

		default:
			f.record("KHÔNG RÕ:" + p)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func writeSession(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "misa-session.json")
	body, err := json.Marshal(map[string]string{
		"sid": "s-1", "tid": "t-1", "mid": "u-1", "dbid": "db-cu", "x_device": "dev-1",
	})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("ghi session: %v", err)
	}
	return path
}

func writeXLSX(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "htla.xlsx")
	if err := os.WriteFile(path, []byte("PK\x03\x04 giả lập"), 0o600); err != nil {
		t.Fatalf("ghi xlsx: %v", err)
	}
	return path
}

func TestHTTPPusher_ĐúngThứTựBướcVàGhiSổ(t *testing.T) {
	fake := &fakeMISA{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	dir := t.TempDir()
	res, err := (&HTTPPusher{}).Push(context.Background(), Request{
		BaseURL:     srv.URL,
		SessionPath: writeSession(t, dir),
		Database:    "Long An",
		FilePath:    writeXLSX(t, dir),
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !res.Committed || res.Valid != 1 {
		t.Errorf("res.Committed=%v res.Valid=%d, want true/1", res.Committed, res.Valid)
	}

	want := []string{"login", "databases", "database-context:db-htla", "upload", "sheetname", "step2", "step3", "check_step3", "step4", "check_step4"}
	got := fake.Calls()
	if len(got) != len(want) {
		t.Fatalf("thứ tự gọi = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("thứ tự gọi = %v, want %v", got, want)
		}
	}
}

func TestHTTPPusher_CònDòngLỗiThìKhôngGọiStep4(t *testing.T) {
	fake := &fakeMISA{invalid: true}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	dir := t.TempDir()
	res, err := (&HTTPPusher{}).Push(context.Background(), Request{
		BaseURL:     srv.URL,
		SessionPath: writeSession(t, dir),
		Database:    "Long An",
		FilePath:    writeXLSX(t, dir),
	})
	if err == nil {
		t.Fatal("Push = nil, want lỗi vì còn dòng không hợp lệ")
	}
	// Kết quả vẫn phải về được để bên gọi liệt kê ĐỦ các dòng lỗi, chứ
	// không chỉ dòng đầu nằm trong thông điệp lỗi.
	if res == nil || len(res.RowErrors) != 1 {
		t.Fatalf("res.RowErrors = %#v, want đúng 1 dòng lỗi", res)
	}
	for _, c := range fake.Calls() {
		if c == "step4" || c == "check_step4" {
			t.Error("đã gọi step4 dù còn dòng không hợp lệ — cả nhánh phải không ghi gì")
		}
	}
}

func TestHTTPPusher_ThiếuPhiênVàThiếuSidURLThìDừngTrướcKhiUpload(t *testing.T) {
	fake := &fakeMISA{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	dir := t.TempDir()
	_, err := (&HTTPPusher{}).Push(context.Background(), Request{
		BaseURL:     srv.URL,
		SessionPath: filepath.Join(dir, "không-có.json"),
		Database:    "Long An",
		FilePath:    writeXLSX(t, dir),
	})
	if err == nil {
		t.Fatal("Push = nil, want lỗi vì không có phiên lẫn nguồn cấp phiên")
	}
	for _, c := range fake.Calls() {
		if c == "upload" {
			t.Error("đã upload file dù chưa có phiên đăng nhập")
		}
	}
}

func TestHTTPPusher_GhiNhậtKýQuaRequestLog(t *testing.T) {
	fake := &fakeMISA{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	dir := t.TempDir()
	var lines []string
	_, err := (&HTTPPusher{}).Push(context.Background(), Request{
		BaseURL:     srv.URL,
		SessionPath: writeSession(t, dir),
		Database:    "Long An",
		FilePath:    writeXLSX(t, dir),
		Log:         func(s string) { lines = append(lines, s) },
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(lines) == 0 {
		t.Error("Request.Log không nhận được dòng nào — người dùng sẽ không thấy tiến độ")
	}
}
```

- [ ] **Step 2: Chạy test để chắc chắn nó đỏ**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./internal/misapush/ -run TestHTTPPusher
```
Expected: FAIL — `undefined: HTTPPusher`, `undefined: Request`.

- [ ] **Step 3: Viết bản cài đặt tối thiểu**

Create `GO/internal/misapush/push.go`:

```go
package misapush

import (
	"context"
	"fmt"

	"order-processor/internal/misa"
)

// Request là một lần đẩy cho MỘT nhánh kế toán.
type Request struct {
	// SessionPath là file misa-session.json. Đọc không được mà có SidURL
	// thì vẫn chạy — nhánh xin phiên mới sẽ lo.
	SessionPath string
	// SidURL là endpoint cấp phiên mới (Apps Script). Rỗng = không tự
	// gia hạn được, phiên chết là dừng.
	SidURL string
	// Database là tên (khớp một phần, không phân biệt hoa thường) hoặc
	// database_id đầy đủ của bộ dữ liệu kế toán.
	Database string
	// FilePath là file .xlsx đã tách riêng cho nhánh này.
	FilePath string
	// BaseURL để trống thì dùng host thật của AMIS Kế toán; test trỏ vào
	// server giả qua đây.
	BaseURL string
	// Log nhận từng dòng tiến độ; để nil thì im lặng.
	Log func(string)
}

// Pusher thực hiện một lần đẩy. Tách thành interface để App test được mà
// không cần mạng.
type Pusher interface {
	Push(ctx context.Context, req Request) (*misa.ImportResult, error)
}

// HTTPPusher là bản thật, gọi API AMIS Kế toán.
type HTTPPusher struct{}

// Push đăng nhập, chuyển sang đúng bộ dữ liệu, rồi nhập khẩu file Excel.
//
// Mỗi lần gọi dựng một Client MỚI. Client giữ Headers biến đổi
// (Authorization, X-MISA-Context) và SwitchDatabase thay X-MISA-Context
// tại chỗ; đổi bộ dữ liệu hai lần trong cùng một client là đường chưa ai
// đi, còn misapush dòng lệnh thì chỉ đổi đúng một lần mỗi lần chạy. Một
// client mỗi nhánh tái hiện chính xác lần chạy đã được kiểm chứng. Giá
// phải trả là một lời gọi cấp token thêm — cấp token mới không giết
// token đang có, nên vô hại.
//
// Luôn Commit=true, Force=false: MISA kiểm tra cả file trước, không dòng
// nào lỗi thì ghi sổ luôn; còn dòng lỗi thì CẢ NHÁNH không ghi gì. Kết
// quả vẫn được trả về kèm lỗi để bên gọi liệt kê đủ các dòng hỏng.
func (p *HTTPPusher) Push(ctx context.Context, req Request) (*misa.ImportResult, error) {
	c := misa.NewClient(req.BaseURL)
	if req.Log != nil {
		c.Log = func(format string, args ...any) { req.Log(fmt.Sprintf(format, args...)) }
	}

	// Gắn nguồn cấp phiên TRƯỚC khi đăng nhập: phiên trong file chết thì
	// client tự xin phiên mới rồi ghi đè file, thay vì bắt người dùng mở
	// trình duyệt chạy misasniff.
	if req.SidURL != "" && req.SessionPath != "" {
		dest := req.SessionPath
		c.SetRenewFromURL(req.SidURL, func(s *misa.Session) error { return s.Save(dest) })
	}

	if s, err := misa.LoadSession(req.SessionPath); err == nil {
		c.UseSession(s)
	} else if req.SidURL == "" {
		return nil, fmt.Errorf("không đọc được phiên %s (%w) và chưa khai URL cấp phiên trong Cài đặt > MISA",
			req.SessionPath, err)
	}

	// Cấp token ngay để phát hiện phiên hỏng TRƯỚC khi upload, thay vì để
	// lỗi nổ ra giữa lúc đang đẩy dữ liệu.
	if err := c.Login(ctx); err != nil {
		return nil, fmt.Errorf("đăng nhập MISA: %w", err)
	}

	db, err := c.SwitchDatabaseByName(ctx, req.Database)
	if err != nil {
		return nil, fmt.Errorf("chọn bộ dữ liệu %q: %w", req.Database, err)
	}
	if req.Log != nil {
		req.Log(fmt.Sprintf("bộ dữ liệu: %s", db.DatabaseName))
	}

	return c.ImportExcel(ctx, misa.ImportOptions{
		FilePath:   req.FilePath,
		RefType:    misa.RefTypeSAOrder,
		TableName:  misa.TableSAOrder,
		SheetIndex: -1,
		Commit:     true,
		Force:      false,
	})
}
```

- [ ] **Step 4: Chạy test để chắc chắn nó xanh**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./internal/misapush/ -v
```
Expected: PASS toàn bộ.

- [ ] **Step 5: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/internal/misapush/push.go GO/internal/misapush/push_test.go
git commit -m "feat(misapush): một lần đẩy cho một nhánh kế toán

Dựng Client mới cho mỗi nhánh thay vì đổi bộ dữ liệu hai lần trên cùng
một client — đường sau chưa ai đi, đường trước tái hiện chính xác một lần
chạy misapush dòng lệnh đã kiểm chứng.

Luôn Commit=true/Force=false. Test khoá lại điều quan trọng nhất: còn một
dòng không hợp lệ thì KHÔNG gọi step4, cả nhánh không ghi gì — nhưng kết
quả vẫn trả về để bên gọi liệt kê đủ dòng hỏng, chứ không chỉ dòng đầu."
```

---

### Task 5: Cấu hình — 2 map mới trong `appsettings`

**Files:**
- Modify: `GO/internal/appsettings/store.go`
- Test: `GO/internal/appsettings/store_test.go`

**Interfaces:**
- Consumes: không có.
- Produces: `appsettings.Settings` thêm 2 field `Misa map[string]string` (json `misa`) và `MisaRouting map[string]string` (json `misa_routing`), cả hai được `ensureMaps` đảm bảo không nil.

- [ ] **Step 1: Viết test đỏ**

Append vào `GO/internal/appsettings/store_test.go`:

```go
func TestStore_Load_FileCũKhôngCóKhốiMisaVẫnRaMapRỗng(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.bhconfig")
	// Đúng hình dạng file .bhconfig của bản trước khi có MISA.
	if err := os.WriteFile(path, []byte(`{"gid":{"COOP":"1"},"zalo":{},"reminder":{},"haravan":{}}`), 0o644); err != nil {
		t.Fatalf("ghi file cũ: %v", err)
	}

	settings, err := NewStore(path).Load(filepath.Join(dir, "không-có.ini"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Misa == nil {
		t.Error("Misa = nil, want map rỗng — frontend cần object thật để render bảng")
	}
	if settings.MisaRouting == nil {
		t.Error("MisaRouting = nil, want map rỗng")
	}
}

func TestStore_SaveLoad_GiữNguyênKhốiMisa(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.bhconfig")
	store := NewStore(path)

	want := Settings{
		Misa:        map[string]string{"sid_url": "https://script.google.com/x", "db_htla": "Long An"},
		MisaRouting: map[string]string{"Lotte": "htla", "BigC/MT": "ha_thanh"},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(filepath.Join(dir, "không-có.ini"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Misa["sid_url"] != want.Misa["sid_url"] {
		t.Errorf("Misa[sid_url] = %q, want %q", got.Misa["sid_url"], want.Misa["sid_url"])
	}
	if got.MisaRouting["BigC/MT"] != "ha_thanh" {
		t.Errorf("MisaRouting[BigC/MT] = %q, want %q", got.MisaRouting["BigC/MT"], "ha_thanh")
	}
}
```

- [ ] **Step 2: Chạy test để chắc chắn nó đỏ**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./internal/appsettings/ -run Misa
```
Expected: FAIL — `settings.Misa undefined`, `settings.MisaRouting undefined`.

- [ ] **Step 3: Thêm 2 field và vá `ensureMaps`**

Trong `GO/internal/appsettings/store.go`, thêm vào cuối struct `Settings` (ngay sau field `Haravan`):

```go
	// Misa giữ cấu hình đẩy đơn lên AMIS Kế toán. Ba khoá quy ước:
	//   sid_url      - URL Apps Script cấp phiên mới khi phiên hết hạn
	//   db_ha_thanh  - tên (hoặc database_id) bộ dữ liệu nhánh Hà Thành
	//   db_htla      - tên (hoặc database_id) bộ dữ liệu nhánh HTLA
	// Vẫn là map[string]string như các nhóm khác để popup Cài đặt dùng
	// lại nguyên KeyValueEditor.
	Misa map[string]string `json:"misa"`
	// MisaRouting ánh xạ khoá định tuyến -> nhánh ("ha_thanh"/"htla").
	// Khoá do misapush.RouteKey sinh ra, ví dụ "Lotte", "BigC/GC",
	// "JIT-CHOICE/WH6_HN", "TMĐT-*". Đây là NGUỒN CHÂN LÝ: bảng gieo
	// trong code chỉ điền vào chỗ trống, không bao giờ ghi đè, để sửa
	// hằng số ở bản sau không xê dịch một cài đặt nào đang chạy.
	MisaRouting map[string]string `json:"misa_routing"`
```

Trong hàm `ensureMaps`, thêm ngay trước dấu `}` cuối:

```go
	if s.Misa == nil {
		s.Misa = map[string]string{}
	}
	if s.MisaRouting == nil {
		s.MisaRouting = map[string]string{}
	}
```

Trong `Load`, sửa dòng trả về khi chưa migrate được (`if !migrated { ... }`) thành:

```go
	if !migrated {
		empty := Settings{}
		ensureMaps(&empty)
		return empty, nil
	}
```

- [ ] **Step 4: Chạy test để chắc chắn nó xanh**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./internal/appsettings/ -v
```
Expected: PASS toàn bộ, gồm cả các test cũ.

- [ ] **Step 5: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/internal/appsettings/store.go GO/internal/appsettings/store_test.go
git commit -m "feat(appsettings): thêm khối cấu hình misa và misa_routing

File .bhconfig của bản cũ không có hai khoá này vẫn đọc lên chạy được —
encoding/json để nil, ensureMaps vá lại thành map rỗng, nên không cần
migration. Nhân tiện thay ba map viết tay trong nhánh chưa-migrate bằng
chính ensureMaps, để thêm nhóm mới lần sau không phải sửa hai chỗ."
```

---

### Task 6: Binding định tuyến — `MisaResolveRoutes` và `MisaRouteOptions`

**Files:**
- Create: `GO/app_misa.go`
- Modify: `GO/app.go` (thêm field vào struct `App`; gieo bảng định tuyến trong `NewApp`)
- Test: `GO/app_misa_test.go`

**Interfaces:**
- Consumes: `misapush.RouteKey`, `misapush.Label`, `misapush.Lookup`, `misapush.SeedRouting`, `misapush.ApplySeed` (Task 2); `appsettings.Settings.MisaRouting` (Task 5).
- Produces:
  - `type MisaRouteInput struct { System string \`json:"system"\`; CustomerCode string \`json:"customerCode"\`; ShipTo string \`json:"shipTo"\` }`
  - `type MisaRouteInfo struct { Key string \`json:"key"\`; Label string \`json:"label"\`; Branch string \`json:"branch"\` }`
  - `func (a *App) MisaResolveRoutes(rows []MisaRouteInput) ([]MisaRouteInfo, error)`
  - `func (a *App) MisaRouteOptions() ([]MisaRouteInfo, error)`

- [ ] **Step 1: Viết test đỏ**

Create `GO/app_misa_test.go`:

```go
package main

import (
	"path/filepath"
	"testing"

	"order-processor/internal/appsettings"
	"order-processor/internal/misapush"
)

func newTestAppForMisa(t *testing.T, settings appsettings.Settings) *App {
	t.Helper()
	store := appsettings.NewStore(filepath.Join(t.TempDir(), "settings.bhconfig"))
	if err := store.Save(settings); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	return &App{appSettingsStore: store}
}

func TestMisaResolveRoutes_TáchĐúngBigCVàJIT(t *testing.T) {
	app := newTestAppForMisa(t, appsettings.Settings{MisaRouting: misapush.SeedRouting()})

	got, err := app.MisaResolveRoutes([]MisaRouteInput{
		{System: "BigC", CustomerCode: "MB_GC_BIGC"},
		{System: "BigC", CustomerCode: "MN_MT_BIGCAC"},
		{System: "JIT-CHOICE", CustomerCode: "MN_JIT_01512", ShipTo: "WH6_HTLA"},
		{System: "JIT-CHOICE", CustomerCode: "MN_JIT_01512", ShipTo: "WH6_HN"},
	})
	if err != nil {
		t.Fatalf("MisaResolveRoutes: %v", err)
	}

	want := []MisaRouteInfo{
		{Key: "BigC/GC", Label: "BigC · gia công", Branch: misapush.BranchHTLA},
		{Key: "BigC/MT", Label: "BigC · modern trade", Branch: misapush.BranchHaThanh},
		{Key: "JIT-CHOICE/WH6_HTLA", Label: "JIT · kho WH6_HTLA", Branch: misapush.BranchHTLA},
		{Key: "JIT-CHOICE/WH6_HN", Label: "JIT · kho WH6_HN", Branch: misapush.BranchHaThanh},
	}
	if len(got) != len(want) {
		t.Fatalf("số phần tử = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestMisaResolveRoutes_SànTMĐTChưaTừngThấyVẫnRaHTLA(t *testing.T) {
	app := newTestAppForMisa(t, appsettings.Settings{MisaRouting: misapush.SeedRouting()})

	got, err := app.MisaResolveRoutes([]MisaRouteInput{{System: "TMĐT-Sàn Mới", CustomerCode: "MN_TMDT_00015"}})
	if err != nil {
		t.Fatalf("MisaResolveRoutes: %v", err)
	}
	if got[0].Branch != misapush.BranchHTLA {
		t.Errorf("Branch = %q, want %q (phải rơi về khoá tiền tố TMĐT-*)", got[0].Branch, misapush.BranchHTLA)
	}
	if got[0].Key != "TMĐT-Sàn Mới" {
		t.Errorf("Key = %q, want %q (khoá giữ nguyên tên sàn để ghi nhớ được)", got[0].Key, "TMĐT-Sàn Mới")
	}
}

func TestMisaResolveRoutes_KhoáChưaMapTrảNhánhRỗng(t *testing.T) {
	app := newTestAppForMisa(t, appsettings.Settings{MisaRouting: map[string]string{}})

	got, err := app.MisaResolveRoutes([]MisaRouteInput{{System: "Lotte", CustomerCode: "MN_MT_LOT1001"}})
	if err != nil {
		t.Fatalf("MisaResolveRoutes: %v", err)
	}
	if got[0].Branch != "" {
		t.Errorf("Branch = %q, want chuỗi rỗng — không được đoán bừa một nhánh", got[0].Branch)
	}
}

func TestMisaResolveRoutes_CấuHìnhNgườiDùngThắngBảngGieo(t *testing.T) {
	routing := misapush.SeedRouting()
	routing["Lotte"] = misapush.BranchHaThanh // người dùng đã đổi
	app := newTestAppForMisa(t, appsettings.Settings{MisaRouting: routing})

	got, err := app.MisaResolveRoutes([]MisaRouteInput{{System: "Lotte", CustomerCode: "MN_MT_LOT1001"}})
	if err != nil {
		t.Fatalf("MisaResolveRoutes: %v", err)
	}
	if got[0].Branch != misapush.BranchHaThanh {
		t.Errorf("Branch = %q, want %q", got[0].Branch, misapush.BranchHaThanh)
	}
}

func TestMisaRouteOptions_GộpSeedVàCấuHìnhSắpTheoNhãn(t *testing.T) {
	app := newTestAppForMisa(t, appsettings.Settings{
		MisaRouting: map[string]string{"Lotte": misapush.BranchHaThanh, "SànLạ": misapush.BranchHTLA},
	})

	got, err := app.MisaRouteOptions()
	if err != nil {
		t.Fatalf("MisaRouteOptions: %v", err)
	}

	byKey := map[string]MisaRouteInfo{}
	for _, o := range got {
		byKey[o.Key] = o
	}
	if byKey["Lotte"].Branch != misapush.BranchHaThanh {
		t.Errorf("Lotte = %q, want %q (giá trị đã lưu phải thắng bảng gieo)", byKey["Lotte"].Branch, misapush.BranchHaThanh)
	}
	if _, ok := byKey["SànLạ"]; !ok {
		t.Error("thiếu khoá lạ đã lưu trong cấu hình")
	}
	if byKey["BigC/GC"].Label != "BigC · gia công" {
		t.Errorf("BigC/GC label = %q, want %q", byKey["BigC/GC"].Label, "BigC · gia công")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Label > got[i].Label {
			t.Fatalf("danh sách chưa sắp theo nhãn: %q đứng trước %q", got[i-1].Label, got[i].Label)
		}
	}
}
```

- [ ] **Step 2: Chạy test để chắc chắn nó đỏ**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test . -run TestMisa
```
Expected: FAIL — `undefined: MisaRouteInput`, `undefined: MisaRouteInfo`, `app.MisaResolveRoutes undefined`, `app.MisaRouteOptions undefined`.

- [ ] **Step 3: Viết bản cài đặt tối thiểu**

Create `GO/app_misa.go`:

```go
package main

import (
	"sort"

	"order-processor/internal/misapush"
)

// MisaRouteInput là dòng ĐẦU của một nhóm đơn trên bảng kết quả — đủ để
// tính khoá định tuyến của cả nhóm.
type MisaRouteInput struct {
	System       string `json:"system"`
	CustomerCode string `json:"customerCode"`
	ShipTo       string `json:"shipTo"`
}

// MisaRouteInfo là khoá định tuyến đã phân giải: khoá máy đọc, nhãn cho
// người đọc, và nhánh mặc định ("" khi chưa map).
type MisaRouteInfo struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Branch string `json:"branch"`
}

// MisaResolveRoutes phân giải khoá + nhãn + nhánh mặc định cho từng nhóm
// đơn. Modal push gọi đúng một lần khi mở, cho cả lô.
//
// Quy tắc định tuyến CHỈ tồn tại ở đây. Viết lại nó bằng TypeScript sẽ
// tạo ra hai bản sao của một quy tắc kế toán, và bản nào lệch thì đơn vào
// nhầm sổ trong khi test của bên kia vẫn xanh.
func (a *App) MisaResolveRoutes(rows []MisaRouteInput) ([]MisaRouteInfo, error) {
	settings, err := a.GetAppSettings()
	if err != nil {
		return nil, err
	}
	out := make([]MisaRouteInfo, 0, len(rows))
	for _, r := range rows {
		key := misapush.RouteKey(r.System, r.CustomerCode, r.ShipTo)
		out = append(out, MisaRouteInfo{
			Key:    key,
			Label:  misapush.Label(key),
			Branch: misapush.Lookup(settings.MisaRouting, key),
		})
	}
	return out, nil
}

// MisaRouteOptions liệt kê mọi khoá định tuyến đã biết — hợp của bảng
// gieo và những khoá đã lưu trong cấu hình — sắp theo nhãn để danh sách
// trong Cài đặt không nhảy chỗ khi có khoá mới.
func (a *App) MisaRouteOptions() ([]MisaRouteInfo, error) {
	settings, err := a.GetAppSettings()
	if err != nil {
		return nil, err
	}

	branches := misapush.SeedRouting()
	// Giá trị đã lưu ĐÈ LÊN bảng gieo, kể cả khi người dùng đã đổi khác
	// mặc định — đây là điểm cả tính năng dựa vào.
	for k, v := range settings.MisaRouting {
		branches[k] = v
	}

	out := make([]MisaRouteInfo, 0, len(branches))
	for k, v := range branches {
		out = append(out, MisaRouteInfo{Key: k, Label: misapush.Label(k), Branch: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out, nil
}
```

- [ ] **Step 4: Gieo bảng định tuyến lúc khởi động**

Trong `GO/app.go`, ngay sau khối `settings, err := appSettingsStore.Load(...)` và trước dòng `excelPath := resolveRepoFile("dondathang.xlsx")`, chèn:

```go
	// Vật chất hoá bảng định tuyến mặc định xuống settings.bhconfig ngay
	// lần chạy đầu, chỉ điền khoá còn thiếu. Xem misapush.ApplySeed cho
	// lý do đầy đủ: nếu bảng gieo chỉ sống trong code như giá trị dự
	// phòng, một lần sửa hằng số ở bản sau sẽ lặng lẽ đổi nhánh của mọi
	// mục người dùng chưa từng chạm vào. Lỗi ghi đĩa KHÔNG chặn khởi
	// động — app vẫn chạy được đầy đủ, chỉ là lần sau gieo lại.
	if misapush.ApplySeed(settings.MisaRouting) {
		_ = appSettingsStore.Save(settings)
	}
```

Thêm `"order-processor/internal/misapush"` vào khối import của `GO/app.go`.

- [ ] **Step 5: Chạy test để chắc chắn nó xanh**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test . -run TestMisa -v
```
Expected: PASS cả 5 test.

- [ ] **Step 6: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/app_misa.go GO/app.go GO/app_misa_test.go
git commit -m "feat(app): binding phân giải khoá định tuyến MISA

MisaResolveRoutes cho modal push (một lời gọi cho cả lô),
MisaRouteOptions cho tab Cài đặt. Quy tắc định tuyến chỉ tồn tại ở Go —
viết lại bằng TypeScript sẽ tạo hai bản sao của một quy tắc kế toán, bản
nào lệch thì đơn vào nhầm sổ mà test bên kia vẫn xanh.

NewApp gieo bảng mặc định xuống settings.bhconfig ngay lần chạy đầu, chỉ
điền chỗ trống. Lỗi ghi đĩa không chặn khởi động."
```

---

### Task 7: `App.PushMisa` — điều phối đẩy theo nhánh

**Files:**
- Modify: `GO/app_misa.go`
- Modify: `GO/app.go` (thêm `misaPusher`, `pushing`, `misaSessionPath` vào struct `App`; khởi tạo trong `NewApp`)
- Test: `GO/app_misa_push_test.go`

**Interfaces:**
- Consumes: `misapush.Pusher`, `misapush.Request`, `misapush.SplitWorkbook`, `misapush.BranchHaThanh`, `misapush.BranchHTLA` (Task 2–4); `Emitter` (app.go hiện có).
- Produces:
  - `type MisaPushJob struct { PO string \`json:"po"\`; RouteKey string \`json:"routeKey"\`; Branch string \`json:"branch"\`; ExcelRows []int \`json:"excelRows"\` }`
  - `func (a *App) PushMisa(jobs []MisaPushJob)`
  - `func (a *App) runMisaPush(emitter Emitter, jobs []MisaPushJob)`
  - `App` thêm field `misaPusher misapush.Pusher`, `pushing atomic.Bool`, `misaSessionPath string`.

- [ ] **Step 1: Viết test đỏ**

Create `GO/app_misa_push_test.go`:

```go
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/appsettings"
	"order-processor/internal/misa"
	"order-processor/internal/misapush"
)

type fakePusher struct {
	mu       sync.Mutex
	requests []misapush.Request
	rowsSeen [][]string // số đơn hàng (cột B) đọc được trong file mỗi lần đẩy
	failOn   string     // Database nào thì trả lỗi
	result   *misa.ImportResult
}

func (f *fakePusher) Push(_ context.Context, req misapush.Request) (*misa.ImportResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	f.rowsSeen = append(f.rowsSeen, readPOColumn(req.FilePath))
	if req.Database == f.failOn {
		return nil, errors.New("giả lập lỗi")
	}
	if f.result != nil {
		return f.result, nil
	}
	return &misa.ImportResult{Committed: true, Valid: 1}, nil
}

func readPOColumn(path string) []string {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	rows, err := f.GetRows(misapush.SheetName)
	if err != nil {
		return nil
	}
	var out []string
	for r := misapush.FirstDataRow; r <= len(rows); r++ {
		v, _ := f.GetCellValue(misapush.SheetName, "B"+strconv.Itoa(r))
		out = append(out, v)
	}
	return out
}

// seedPushWorkbook dựng dondathang.xlsx với n đơn, dòng r mang số đơn "PO-r".
func seedPushWorkbook(t *testing.T, path string, n int) {
	t.Helper()
	f := excelize.NewFile()
	idx, err := f.NewSheet(misapush.SheetName)
	if err != nil {
		t.Fatalf("NewSheet: %v", err)
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")
	if err := f.SetCellValue(misapush.SheetName, "A8", "Ngày đơn hàng (*)"); err != nil {
		t.Fatalf("SetCellValue A8: %v", err)
	}
	for i := 0; i < n; i++ {
		r := misapush.FirstDataRow + i
		if err := f.SetCellValue(misapush.SheetName, "B"+strconv.Itoa(r), "PO-"+strconv.Itoa(r)); err != nil {
			t.Fatalf("SetCellValue B%d: %v", r, err)
		}
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func newTestAppForPush(t *testing.T, pusher misapush.Pusher, misaCfg map[string]string) (*App, *fakeEmitter) {
	t.Helper()
	dir := t.TempDir()
	store := appsettings.NewStore(filepath.Join(dir, "settings.bhconfig"))
	if err := store.Save(appsettings.Settings{Misa: misaCfg, MisaRouting: misapush.SeedRouting()}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	xlsx := filepath.Join(dir, "dondathang.xlsx")
	seedPushWorkbook(t, xlsx, 6) // r9..r14

	emitter := &fakeEmitter{}
	app := &App{
		appSettingsStore: store,
		excelPath:        xlsx,
		misaPusher:       pusher,
		misaSessionPath:  filepath.Join(dir, "misa-session.json"),
		emitter:          emitter,
	}
	return app, emitter
}

func defaultMisaCfg() map[string]string {
	return map[string]string{"db_ha_thanh": "HÀ THÀNH", "db_htla": "Long An", "sid_url": "https://script/x"}
}

func pushedEvents(t *testing.T, events []emittedEvent) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range events {
		if e.name == "misa:pushed" {
			data, ok := e.data[0].(map[string]any)
			if !ok {
				t.Fatalf("misa:pushed data không phải map[string]any: %#v", e.data)
			}
			out = append(out, data)
		}
	}
	return out
}

func TestRunMisaPush_MỗiNhánhMộtLầnĐẩyĐúngDòng(t *testing.T) {
	pusher := &fakePusher{}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9, 10}},
		{PO: "B", RouteKey: "Emart", Branch: misapush.BranchHaThanh, ExcelRows: []int{11}},
		{PO: "C", RouteKey: "Satra", Branch: misapush.BranchHTLA, ExcelRows: []int{13, 12}},
	})

	if len(pusher.requests) != 2 {
		t.Fatalf("số lần Push = %d, want 2 (một nhánh một lần, không phải một đơn một lần)", len(pusher.requests))
	}

	byDB := map[string][]string{}
	for i, req := range pusher.requests {
		byDB[req.Database] = pusher.rowsSeen[i]
	}
	if want := []string{"PO-9", "PO-10", "PO-12", "PO-13"}; !reflect.DeepEqual(byDB["Long An"], want) {
		t.Errorf("file nhánh HTLA chứa %v, want %v", byDB["Long An"], want)
	}
	if want := []string{"PO-11"}; !reflect.DeepEqual(byDB["HÀ THÀNH"], want) {
		t.Errorf("file nhánh Hà Thành chứa %v, want %v", byDB["HÀ THÀNH"], want)
	}
}

func TestRunMisaPush_KhôngGọiChoNhánhRỗng(t *testing.T) {
	pusher := &fakePusher{}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
	})

	if len(pusher.requests) != 1 {
		t.Fatalf("số lần Push = %d, want 1", len(pusher.requests))
	}
	if pusher.requests[0].Database != "Long An" {
		t.Errorf("Database = %q, want %q", pusher.requests[0].Database, "Long An")
	}
}

func TestRunMisaPush_LoạiTrùngVàSắpTăngDần(t *testing.T) {
	pusher := &fakePusher{}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{11, 9}},
		{PO: "B", RouteKey: "Satra", Branch: misapush.BranchHTLA, ExcelRows: []int{9, 10}},
	})

	if want := []string{"PO-9", "PO-10", "PO-11"}; !reflect.DeepEqual(pusher.rowsSeen[0], want) {
		t.Errorf("dòng đã đẩy = %v, want %v", pusher.rowsSeen[0], want)
	}
}

func TestRunMisaPush_ThiếuTênBộDữLiệuThìBỏNhánhĐóThôi(t *testing.T) {
	pusher := &fakePusher{}
	app, emitter := newTestAppForPush(t, pusher, map[string]string{"db_ha_thanh": "HÀ THÀNH"})

	app.runMisaPush(emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
		{PO: "B", RouteKey: "Emart", Branch: misapush.BranchHaThanh, ExcelRows: []int{10}},
	})

	if len(pusher.requests) != 1 || pusher.requests[0].Database != "HÀ THÀNH" {
		t.Fatalf("requests = %#v, want đúng một lần cho HÀ THÀNH", pusher.requests)
	}
	events := pushedEvents(t, emitter.events)
	if len(events) != 2 {
		t.Fatalf("số misa:pushed = %d, want 2 (cả nhánh hỏng cũng phải báo)", len(events))
	}
	for _, e := range events {
		if e["branch"] == misapush.BranchHTLA && e["ok"] != false {
			t.Errorf("nhánh HTLA ok = %v, want false", e["ok"])
		}
	}
}

func TestRunMisaPush_NhánhLỗiKhôngChặnNhánhCònLại(t *testing.T) {
	pusher := &fakePusher{failOn: "Long An"}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
		{PO: "B", RouteKey: "Emart", Branch: misapush.BranchHaThanh, ExcelRows: []int{10}},
	})

	if len(pusher.requests) != 2 {
		t.Fatalf("số lần Push = %d, want 2 — nhánh lỗi không được chặn nhánh kia", len(pusher.requests))
	}
	byBranch := map[string]bool{}
	for _, e := range pushedEvents(t, emitter.events) {
		byBranch[e["branch"].(string)] = e["ok"].(bool)
	}
	if byBranch[misapush.BranchHTLA] != false || byBranch[misapush.BranchHaThanh] != true {
		t.Errorf("kết quả từng nhánh = %#v, want htla=false ha_thanh=true", byBranch)
	}
}

func TestRunMisaPush_XoáFileTạmKểCảKhiLỗi(t *testing.T) {
	pusher := &fakePusher{failOn: "Long An"}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
	})

	if len(pusher.requests) != 1 {
		t.Fatalf("số lần Push = %d, want 1", len(pusher.requests))
	}
	if _, err := os.Stat(pusher.requests[0].FilePath); !os.IsNotExist(err) {
		t.Errorf("file tạm %s còn sót lại sau khi Push lỗi", pusher.requests[0].FilePath)
	}
}

func TestPushMisa_TừChốiKhiĐangXửLýLô(t *testing.T) {
	pusher := &fakePusher{}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())
	app.processing.Store(true)

	app.PushMisa([]MisaPushJob{{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}}})

	if len(pusher.requests) != 0 {
		t.Error("đã đẩy dù đang có lô xử lý chạy — workbook lúc đó đang bị ghi dở")
	}
	if len(emitter.events) == 0 {
		t.Error("không báo gì cho người dùng khi từ chối")
	}
}

func TestPushMisa_TừChốiLờiGọiThứHaiKhiĐangĐẩy(t *testing.T) {
	pusher := &fakePusher{}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())
	app.pushing.Store(true)

	app.PushMisa([]MisaPushJob{{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}}})

	if len(pusher.requests) != 0 {
		t.Error("đã đẩy dù đang có lượt đẩy khác chạy")
	}
	if len(emitter.events) == 0 {
		t.Error("không báo gì cho người dùng khi từ chối")
	}
}

func TestRunMisaPush_TruyềnPhiênVàSidURLXuốngPusher(t *testing.T) {
	pusher := &fakePusher{}
	app, _ := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(app.emitter, []MisaPushJob{
		{PO: "A", RouteKey: "Lotte", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
	})

	req := pusher.requests[0]
	if req.SessionPath != app.misaSessionPath {
		t.Errorf("SessionPath = %q, want %q", req.SessionPath, app.misaSessionPath)
	}
	if req.SidURL != "https://script/x" {
		t.Errorf("SidURL = %q, want %q", req.SidURL, "https://script/x")
	}
}
```

- [ ] **Step 2: Chạy test để chắc chắn nó đỏ**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test . -run TestRunMisaPush
```
Expected: FAIL — `undefined: MisaPushJob`, `app.misaPusher undefined`, `app.pushing undefined`, `app.misaSessionPath undefined`, `app.runMisaPush undefined`.

- [ ] **Step 3: Thêm field vào struct `App`**

Trong `GO/app.go`, thêm vào cuối struct `App` (ngay trước dấu `}` đóng struct):

```go
	// misaPusher thực hiện một lần đẩy cho một nhánh. Thay được trong
	// test để không phải chạm mạng — cùng khuôn với zaloSender.
	misaPusher misapush.Pusher
	// pushing khoá lượt đẩy MISA, đúng vai trò a.sending làm cho Zalo:
	// hai lượt đẩy chồng nhau sẽ đọc cùng một workbook trong lúc file
	// tạm của nhau đang được cắt.
	pushing atomic.Bool
	// misaSessionPath là file phiên đăng nhập MISA, nằm cạnh
	// settings.bhconfig. Nó thay được mật khẩu trong 24h nên đã được
	// .gitignore loại ra.
	misaSessionPath string
```

Trong `NewApp`, thêm 2 dòng vào literal khởi tạo `app := &App{...}` (sau `tmdtResolve: ...`):

```go
		misaPusher:      &misapush.HTTPPusher{},
		misaSessionPath: filepath.Join(resolveRepoDir("settings.ini"), "misa-session.json"),
```

- [ ] **Step 4: Viết phần điều phối**

Thêm vào cuối `GO/app_misa.go`:

```go
// MisaPushJob là một nhóm đơn mà người dùng đã tick và đã gán nhánh.
type MisaPushJob struct {
	PO        string `json:"po"`
	RouteKey  string `json:"routeKey"`
	Branch    string `json:"branch"`
	ExcelRows []int  `json:"excelRows"`
}

// misaBranchOrder cố định thứ tự đẩy, để log và màn hình kết quả không
// đảo chỗ giữa hai lần chạy giống hệt nhau.
var misaBranchOrder = []string{misapush.BranchHaThanh, misapush.BranchHTLA}

// misaBranchLabel là tên hiển thị của nhánh; khoá lưu trữ vẫn là chuỗi
// máy đọc trong misapush.
var misaBranchLabel = map[string]string{
	misapush.BranchHaThanh: "Hà Thành",
	misapush.BranchHTLA:    "HTLA",
}

// misaDatabaseKey là khoá tra tên bộ dữ liệu trong Cài đặt > MISA.
var misaDatabaseKey = map[string]string{
	misapush.BranchHaThanh: "db_ha_thanh",
	misapush.BranchHTLA:    "db_htla",
}

// PushMisa đẩy các nhóm đơn đã chọn lên AMIS Kế toán trong một goroutine
// nền, phát misa:log/misa:pushed/misa:done — cùng khuôn
// SendZaloMessages/runZaloBatch.
func (a *App) PushMisa(jobs []MisaPushJob) {
	if len(jobs) == 0 {
		return
	}
	// Lô xử lý đang ghi vào CHÍNH workbook mà bước tách sắp đọc; cắt file
	// giữa lúc nó đang được ghi là đẩy đi một bản dở dang.
	if a.processing.Load() {
		a.emitter.Emit("misa:log", "⚠️ Đang xử lý đơn hàng, vui lòng đợi hoàn tất rồi đẩy lên MISA.")
		a.emitter.Emit("misa:done", nil)
		return
	}
	if !a.pushing.CompareAndSwap(false, true) {
		a.emitter.Emit("misa:log", "⚠️ Đã có một lượt đẩy MISA đang chạy, vui lòng đợi hoàn tất.")
		return
	}
	go a.runMisaPush(a.emitter, jobs)
}

func (a *App) runMisaPush(emitter Emitter, jobs []MisaPushJob) {
	defer func() {
		if r := recover(); r != nil {
			emitter.Emit("misa:log", fmt.Sprintf("❌ Lỗi không mong muốn: %v", r))
		}
		a.pushing.Store(false)
		emitter.Emit("misa:done", nil)
	}()

	settings, err := a.GetAppSettings()
	if err != nil {
		emitter.Emit("misa:log", fmt.Sprintf("❌ Không đọc được cấu hình: %v", err))
		return
	}

	rowsByBranch := map[string][]int{}
	for _, job := range jobs {
		rowsByBranch[job.Branch] = append(rowsByBranch[job.Branch], job.ExcelRows...)
	}

	for _, branch := range misaBranchOrder {
		rows := dedupSorted(rowsByBranch[branch])
		if len(rows) == 0 {
			continue
		}
		a.pushOneBranch(emitter, settings.Misa, branch, rows)
	}
}

// pushOneBranch tách workbook cho đúng một nhánh rồi đẩy. Mọi lỗi ở đây
// chỉ dừng nhánh này — nhánh còn lại vẫn chạy, vì người dùng thà vào sổ
// được một nửa còn hơn phải làm lại cả hai.
func (a *App) pushOneBranch(emitter Emitter, misaCfg map[string]string, branch string, rows []int) {
	label := misaBranchLabel[branch]

	fail := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		emitter.Emit("misa:log", fmt.Sprintf("❌ %s: %s", label, msg))
		emitter.Emit("misa:pushed", map[string]any{
			"branch": branch, "ok": false, "valid": 0, "invalid": 0, "message": msg,
		})
	}

	database := strings.TrimSpace(misaCfg[misaDatabaseKey[branch]])
	if database == "" {
		fail("chưa khai bộ dữ liệu kế toán (Cài đặt > MISA > %s)", misaDatabaseKey[branch])
		return
	}

	tmp, err := os.CreateTemp("", "misa-*.xlsx")
	if err != nil {
		fail("không tạo được file tạm: %v", err)
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()
	// Xoá dù đi ra bằng đường nào: file này là bản sao của sổ đặt hàng,
	// để lại trong thư mục tạm là rò dữ liệu khách hàng.
	defer os.Remove(tmpPath)

	if err := misapush.SplitWorkbook(a.excelPath, tmpPath, rows); err != nil {
		fail("không tách được đơn của nhánh: %v", err)
		return
	}

	emitter.Emit("misa:log", fmt.Sprintf("📤 %s: đang đẩy %d dòng lên %q…", label, len(rows), database))

	res, err := a.misaPusher.Push(context.Background(), misapush.Request{
		SessionPath: a.misaSessionPath,
		SidURL:      strings.TrimSpace(misaCfg["sid_url"]),
		Database:    database,
		FilePath:    tmpPath,
		Log:         func(line string) { emitter.Emit("misa:log", fmt.Sprintf("   %s: %s", label, line)) },
	})

	if err != nil {
		// Liệt kê ĐỦ dòng hỏng, không chỉ dòng đầu nằm trong thông điệp
		// lỗi — sửa được hết trong một lượt thay vì lặp lại từng dòng.
		if res != nil {
			for _, e := range res.RowErrors {
				emitter.Emit("misa:log", fmt.Sprintf("   %s: ✗ %s", label, e))
			}
		}
		fail("%v", err)
		return
	}

	emitter.Emit("misa:log", fmt.Sprintf("✅ %s: đã ghi vào sổ %d chứng từ hợp lệ, %d lỗi, %d bỏ qua",
		label, res.Valid, res.Invalid, res.Skipped))
	emitter.Emit("misa:pushed", map[string]any{
		"branch": branch, "ok": true, "valid": res.Valid, "invalid": res.Invalid,
		"message": fmt.Sprintf("đã ghi %d chứng từ", res.Valid),
	})
}

// dedupSorted loại trùng và sắp tăng dần. Trùng là chuyện thường: hai
// nhóm đơn khác nhau vẫn có thể trỏ vào cùng một dòng Excel khi một dòng
// mang nhiều mã hàng.
func dedupSorted(rows []int) []int {
	if len(rows) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(rows))
	out := make([]int, 0, len(rows))
	for _, r := range rows {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	sort.Ints(out)
	return out
}
```

Cập nhật khối import của `GO/app_misa.go` thành:

```go
import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"order-processor/internal/misapush"
)
```

- [ ] **Step 5: Chạy test để chắc chắn nó xanh**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test . -run "TestMisa|TestRunMisaPush|TestPushMisa" -v
```
Expected: PASS toàn bộ 14 test.

- [ ] **Step 6: Chạy lại toàn bộ test Go**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./...
```
Expected: mọi package PASS, **trừ** đúng 5 test JIT air-waybill trong `order-processor/internal/processing` vốn đã đỏ từ trước vì thiếu fixture PDF (`đơn hàng/air_waybill_WH6_HTLA_24082026.pdf`). Đây là lỗi đã biết, ngoài phạm vi — không sửa, không xoá, không điều tra. Nếu có bất kỳ test nào KHÁC đỏ thì phải sửa trước khi commit.

- [ ] **Step 7: Thêm `misa-session.json` vào `.gitignore`**

Trong `.gitignore` ở gốc repo, thêm ngay dưới khối `zalo_profile/`:

```
# ===== Phiên đăng nhập MISA (không chứa mật khẩu nhưng THAY ĐƯỢC mật khẩu
# trong 24h — không commit, không gửi qua chat) =====
misa-session.json
```

- [ ] **Step 8: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/app_misa.go GO/app.go GO/app_misa_push_test.go .gitignore
git commit -m "feat(app): đẩy đơn lên MISA theo nhánh, mỗi nhánh một lần

Gom ExcelRows của mọi đơn đã tick theo nhánh, tách một file tạm cho mỗi
nhánh rồi đẩy tuần tự — tối đa hai lần gọi MISA cho cả lô, không phải mỗi
đơn một lần.

Nhánh lỗi không chặn nhánh còn lại: thà vào sổ được một nửa còn hơn phải
làm lại cả hai. File tạm bị xoá dù đi ra bằng đường nào — nó là bản sao
sổ đặt hàng, để lại trong thư mục tạm là rò dữ liệu khách hàng.

Từ chối khi đang có lô xử lý chạy: lô đó đang ghi vào chính workbook mà
bước tách sắp đọc.

misa-session.json vào .gitignore — không chứa mật khẩu nhưng thay được
mật khẩu trong 24h."
```

---

### Task 8: `SegmentedControl` dùng chung

**Files:**
- Create: `GO/frontend/src/components/SegmentedControl.tsx`
- Modify: `GO/frontend/src/components/JITPeriodMenu.tsx`

**Interfaces:**
- Consumes: không có.
- Produces: `export function SegmentedControl(props: { options: readonly { value: string; label: string }[]; value: string; disabled?: boolean; onChange: (value: string) => void; ariaLabel: string }): JSX.Element`

- [ ] **Step 1: Tạo component dùng chung**

Create `GO/frontend/src/components/SegmentedControl.tsx`:

```tsx
interface SegmentedControlProps {
  options: readonly { value: string; label: string }[]
  value: string
  disabled?: boolean
  onChange: (value: string) => void
  ariaLabel: string
}

// Vài lựa chọn loại trừ nhau, tất cả đều ngắn, và người dùng đổi ngay sau
// khi nhìn kết quả - đó là mô tả của một segmented control, không phải
// của một menu bật/tắt. Các nút liền khối cho thấy cả những lựa chọn còn
// lại lẫn cái đang chọn cùng lúc, và bỏ được một cú bấm khỏi thao tác
// thường gặp nhất.
//
// role="group" chứ không phải "radiogroup": ở bộ chọn buổi JIT, các nút
// này KHÔNG chỉ đổi trạng thái cục bộ mà gửi thẳng một lệnh ghi Excel cho
// cả file PDF, nên chúng là nút hành động, và aria-pressed nói đúng điều
// đó. Một radiogroup sẽ hứa với trình đọc màn hình rằng mũi tên
// trái/phải di chuyển lựa chọn mà không ghi gì - lời hứa mà thành phần
// này không giữ.
//
// value không khớp lựa chọn nào (chuỗi rỗng) là trạng thái HỢP LỆ: bảng
// định tuyến MISA dùng nó cho khoá chưa map, và "không nút nào sáng" là
// đúng thứ cần hiện ra để người dùng thấy ngay chỗ phải bấm.
export function SegmentedControl({ options, value, disabled, onChange, ariaLabel }: SegmentedControlProps) {
  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className={`inline-flex shrink-0 overflow-hidden rounded-md border border-border bg-bg ${
        disabled ? 'opacity-50' : ''
      }`}
    >
      {options.map((option, index) => {
        const isActive = value === option.value
        return (
          <button
            key={option.value}
            type="button"
            disabled={disabled}
            aria-pressed={isActive}
            onClick={() => {
              if (!isActive) onChange(option.value)
            }}
            className={`px-2.5 py-1 font-sans text-xs transition-colors disabled:cursor-not-allowed ${
              index > 0 ? 'border-l border-border' : ''
            } ${
              isActive
                ? 'bg-accent/[0.16] font-semibold text-accent'
                : 'font-medium text-muted hover:bg-white/[0.04] hover:text-ink'
            }`}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
```

- [ ] **Step 2: Thu `JITPeriodMenu` thành lớp bọc mỏng**

Replace toàn bộ nội dung `GO/frontend/src/components/JITPeriodMenu.tsx` bằng:

```tsx
import { JIT_PERIOD_OPTIONS, type JITPeriod } from '../lib/jitPeriodMenu'
import { SegmentedControl } from './SegmentedControl'

interface JITPeriodMenuProps {
  value: string
  disabled: boolean
  onChange: (period: JITPeriod) => void
  ariaLabel: string
}

// Ba buổi giao loại trừ nhau - dùng chung SegmentedControl với bảng định
// tuyến MISA. Dùng chung ĐÚNG MỘT component chứ không chép lại style là
// cách duy nhất giữ hai chỗ giống nhau về lâu dài.
export function JITPeriodMenu({ value, disabled, onChange, ariaLabel }: JITPeriodMenuProps) {
  return (
    <SegmentedControl
      options={JIT_PERIOD_OPTIONS}
      value={value}
      disabled={disabled}
      onChange={(next) => onChange(next as JITPeriod)}
      ariaLabel={ariaLabel}
    />
  )
}
```

- [ ] **Step 3: Kiểm tra kiểu và build frontend**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO/frontend"
npx tsc --noEmit
```
Expected: không in ra lỗi nào.

- [ ] **Step 4: Chạy test frontend hiện có**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO/frontend"
npm test
```
Expected: PASS toàn bộ (`isActiveJITPeriod` vẫn còn trong `lib/jitPeriodMenu.ts` và test của nó không đổi — component không còn gọi nó, nhưng hàm và test giữ nguyên, không xoá).

- [ ] **Step 5: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/frontend/src/components/SegmentedControl.tsx GO/frontend/src/components/JITPeriodMenu.tsx
git commit -m "refactor(ui): tách SegmentedControl dùng chung khỏi bộ chọn buổi JIT

Bảng định tuyến MISA cần đúng kiểu điều khiển này. Dùng chung một
component chứ không chép lại style là cách duy nhất giữ hai chỗ giống
nhau về lâu dài — buổi JIT không đổi một pixel vì class giữ nguyên từng
chữ.

Thêm một trạng thái hợp lệ mới: value không khớp lựa chọn nào thì không
nút nào sáng. Định tuyến MISA dùng nó cho khoá chưa map, để người dùng
thấy ngay chỗ phải bấm."
```

---

### Task 9: `lib/misaBranch.ts` — logic thuần của modal

**Files:**
- Create: `GO/frontend/src/lib/misaBranch.ts`
- Test: `GO/frontend/src/lib/misaBranch.test.ts`
- Modify: `GO/frontend/package.json` (thêm file test vào script `test`)

**Interfaces:**
- Consumes: kiểu `OrderRow` từ `types.ts`. **Không** import `groupKeyFor` — nó được
  truyền vào qua tham số (xem ghi chú dưới).
- Produces:
  - `export const MISA_BRANCH_OPTIONS: readonly [{ value: 'ha_thanh'; label: 'Hà Thành' }, { value: 'htla'; label: 'HTLA' }]`
  - `export interface MisaGroupSeed { key: string; po: string; system: string; customerCode: string; shipTo: string; excelRows: number[] }`
  - `export interface MisaGroup extends MisaGroupSeed { routeKey: string; routeLabel: string; branch: string; selected: boolean }`
  - `export function buildMisaGroups(rows: OrderRow[], groupKey: (row: OrderRow) => string): MisaGroupSeed[]`
  - `export function branchTotals(groups: MisaGroup[]): Record<string, { orders: number; rows: number }>`
  - `export function pendingGroups(groups: MisaGroup[], pushedBranches: string[]): MisaGroup[]`
  - `export function canPush(groups: MisaGroup[], pushedBranches: string[]): boolean`
  - `export function rememberRouting(groups: MisaGroup[]): Record<string, string>`

**Vì sao `groupKey` là tham số chứ không phải import.** `node --experimental-strip-types`
**không** phân giải được import không có đuôi khi đó là import GIÁ TRỊ — đã dựng thử và
xác nhận (`ERR_MODULE_NOT_FOUND` tại `src/lib/zaloMessage`). Chuỗi sẽ là
`misaBranch.ts` → `./zaloGrouping` → `./zaloMessage`, mắt xích thứ hai là import giá trị
(`TMDT_SOURCE_PREFIX`), nên file test chết ngay lúc nạp module. Thêm đuôi `.ts` vào
`zaloGrouping.ts` thì `npx tsc --noEmit` đỏ, vì `tsconfig.json` đang để
`moduleResolution: "Node"`. Chép lại quy tắc gom nhóm thì thành hai bản của một quy
tắc — đúng thứ spec cấm. Nên `misaBranch.ts` **chỉ giữ `import type`** (bị xoá lúc
chạy) và nhận hàm gom nhóm qua tham số; `MisaPushModal` truyền thẳng `groupKeyFor`,
vẫn đúng một định nghĩa và chữ ký ép phải truyền.

- [ ] **Step 1: Viết test đỏ**

Create `GO/frontend/src/lib/misaBranch.test.ts`:

```ts
import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  MISA_BRANCH_OPTIONS,
  branchTotals,
  buildMisaGroups,
  canPush,
  pendingGroups,
  rememberRouting,
  type MisaGroup,
} from './misaBranch.ts'
import type { OrderRow } from '../types.ts'

// Bản sao tối giản của groupKeyFor (lib/zaloGrouping.ts) CHỈ dùng trong
// test. Bản thật được MisaPushModal truyền vào lúc chạy — ở đây chỉ cần
// một hàm có cùng hình dạng để kiểm cơ chế gom nhóm.
const testGroupKey = (r: OrderRow): string =>
  r.system === 'JIT-CHOICE' || r.sourceId.startsWith('tmdt|') ? r.sourceId : r.po

function row(over: Partial<OrderRow>): OrderRow {
  return {
    fileName: '', sourceId: '', page: '', system: 'Lotte', maKhachHang: 'MN_MT_LOT1001',
    po: 'PO-1', resultKey: '', maVanDon: '', donGia: '', status: '', statusKind: '',
    excelRows: [9], jitPeriod: '', driveUrl: '', priceMismatchCount: 0,
    priceMismatchDetails: [], shipTo: '', entryDate: '', cancelDate: '',
    totalWeightKg: '', totalPackages: 0, totalQty: 0, skus: [], totalOrders: 0,
    promoItems: [], ...over,
  }
}

function group(over: Partial<MisaGroup>): MisaGroup {
  return {
    key: 'PO-1', po: 'PO-1', system: 'Lotte', customerCode: 'MN_MT_LOT1001', shipTo: '',
    excelRows: [9], routeKey: 'Lotte', routeLabel: 'Lotte', branch: 'htla', selected: true,
    ...over,
  }
}

test('hai nhánh, đúng khoá lưu trữ và nhãn hiển thị', () => {
  assert.deepEqual(
    MISA_BRANCH_OPTIONS.map((o) => [o.value, o.label]),
    [['ha_thanh', 'Hà Thành'], ['htla', 'HTLA']],
  )
})

test('gom nhóm theo cùng khoá mà bảng kết quả và nút Zalo đang dùng', () => {
  const groups = buildMisaGroups([
    row({ po: 'PO-A', excelRows: [9, 10] }),
    row({ po: 'PO-A', excelRows: [11] }),
    row({ po: 'PO-B', excelRows: [12] }),
  ], testGroupKey)
  assert.equal(groups.length, 2)
  assert.deepEqual(groups[0].excelRows, [9, 10, 11])
  assert.deepEqual(groups[1].excelRows, [12])
})

test('gom JIT theo file chứ không theo từng trang', () => {
  const groups = buildMisaGroups([
    row({ system: 'JIT-CHOICE', sourceId: 'awb.pdf', po: 'PO-1', excelRows: [9], shipTo: 'WH6_HN' }),
    row({ system: 'JIT-CHOICE', sourceId: 'awb.pdf', po: 'PO-2', excelRows: [10], shipTo: 'WH6_HN' }),
  ], testGroupKey)
  assert.equal(groups.length, 1)
  assert.deepEqual(groups[0].excelRows, [9, 10])
  assert.equal(groups[0].shipTo, 'WH6_HN')
})

test('bỏ qua dòng không ghi được vào Excel', () => {
  // Dòng hỏng (không trích xuất được) không có excelRows — không có gì để
  // đẩy, và đưa vào modal chỉ tổ khiến người dùng tưởng nó sẽ vào sổ.
  const groups = buildMisaGroups([row({ po: 'PO-A', excelRows: [] }), row({ po: 'PO-B', excelRows: [9] })], testGroupKey)
  assert.equal(groups.length, 1)
  assert.equal(groups[0].po, 'PO-B')
})

test('giữ thông tin định tuyến của dòng đầu mỗi nhóm', () => {
  const groups = buildMisaGroups([
    row({ po: 'PO-A', system: 'BigC', maKhachHang: 'MB_GC_BIGC', excelRows: [9] }),
  ], testGroupKey)
  assert.equal(groups[0].system, 'BigC')
  assert.equal(groups[0].customerCode, 'MB_GC_BIGC')
})

test('đếm đơn và dòng theo nhánh, chỉ tính đơn đang tick', () => {
  const totals = branchTotals([
    group({ key: 'a', branch: 'htla', excelRows: [9, 10], selected: true }),
    group({ key: 'b', branch: 'htla', excelRows: [11], selected: false }),
    group({ key: 'c', branch: 'ha_thanh', excelRows: [12], selected: true }),
  ])
  assert.deepEqual(totals.htla, { orders: 1, rows: 2 })
  assert.deepEqual(totals.ha_thanh, { orders: 1, rows: 1 })
})

test('không cho đẩy khi còn đơn đã tick mà chưa có nhánh', () => {
  assert.equal(canPush([group({ branch: '' })], []), false)
})

test('không cho đẩy khi không tick đơn nào', () => {
  assert.equal(canPush([group({ selected: false })], []), false)
})

test('cho đẩy khi mọi đơn đã tick đều có nhánh', () => {
  assert.equal(canPush([group({ branch: 'htla' }), group({ key: 'b', selected: false, branch: '' })], []), true)
})

test('nhánh đã vào sổ bị loại khỏi lượt đẩy sau, nhánh lỗi thì không', () => {
  const groups = [
    group({ key: 'a', branch: 'htla' }),
    group({ key: 'b', branch: 'ha_thanh' }),
  ]
  const pending = pendingGroups(groups, ['htla'])
  assert.deepEqual(pending.map((g) => g.key), ['b'])
  assert.equal(canPush(groups, ['htla']), true)
  assert.equal(canPush(groups, ['htla', 'ha_thanh']), false)
})

test('ghi nhớ dựng map khoá định tuyến -> nhánh', () => {
  const remembered = rememberRouting([
    group({ key: 'a', routeKey: 'Lotte', branch: 'htla' }),
    group({ key: 'b', routeKey: 'BigC/MT', branch: 'ha_thanh' }),
  ])
  assert.deepEqual(remembered, { Lotte: 'htla', 'BigC/MT': 'ha_thanh' })
})

test('không ghi nhớ khoá bị đặt hai nhánh khác nhau trong cùng một lượt', () => {
  // Người dùng cố tình cho hai đơn Lotte vào hai sổ khác nhau lần này.
  // Ghi lại một trong hai là đoán bừa cho lần sau — thà để hỏi lại.
  const remembered = rememberRouting([
    group({ key: 'a', routeKey: 'Lotte', branch: 'htla' }),
    group({ key: 'b', routeKey: 'Lotte', branch: 'ha_thanh' }),
    group({ key: 'c', routeKey: 'Emart', branch: 'ha_thanh' }),
  ])
  assert.deepEqual(remembered, { Emart: 'ha_thanh' })
})

test('không ghi nhớ đơn chưa tick hoặc chưa có nhánh', () => {
  const remembered = rememberRouting([
    group({ key: 'a', routeKey: 'Lotte', branch: 'htla', selected: false }),
    group({ key: 'b', routeKey: 'Satra', branch: '' }),
  ])
  assert.deepEqual(remembered, {})
})
```

- [ ] **Step 2: Thêm file test vào script `test`**

Trong `GO/frontend/package.json`, sửa dòng `"test": ...` — thêm ` src/lib/misaBranch.test.ts` vào **cuối** danh sách file, ngay trước dấu nháy đóng:

```
"test": "node --experimental-strip-types --test src/lib/orderMismatchScope.test.ts src/lib/batchProgress.test.ts src/lib/jitFileGroups.test.ts src/lib/jitPeriodMenu.test.ts src/lib/jitPeriodState.test.ts src/lib/orderRowUpsert.test.ts src/lib/poPriceWarning.test.ts src/lib/tmdtDateRange.test.ts src/lib/tmdtMissing.test.ts src/lib/zaloMessage.test.ts src/lib/misaBranch.test.ts",
```

- [ ] **Step 3: Chạy test để chắc chắn nó đỏ**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO/frontend"
npm test
```
Expected: FAIL — `Cannot find module './misaBranch.ts'`.

- [ ] **Step 4: Viết bản cài đặt tối thiểu**

Create `GO/frontend/src/lib/misaBranch.ts`:

```ts
import type { OrderRow } from '../types'

// Khoá lưu trữ của hai nhánh — PHẢI khớp misapush.BranchHaThanh /
// BranchHTLA phía Go. Nhãn là thứ hiện ra, khoá là thứ ghi xuống file.
export const MISA_BRANCH_OPTIONS = [
  { value: 'ha_thanh', label: 'Hà Thành' },
  { value: 'htla', label: 'HTLA' },
] as const

export interface MisaGroupSeed {
  key: string
  po: string
  system: string
  customerCode: string
  shipTo: string
  excelRows: number[]
}

export interface MisaGroup extends MisaGroupSeed {
  // routeKey/routeLabel/branch do Go phân giải (MisaResolveRoutes) — quy
  // tắc định tuyến chỉ tồn tại ở đó, không viết lại ở đây.
  routeKey: string
  routeLabel: string
  branch: string
  selected: boolean
}

// buildMisaGroups gom các dòng kết quả thành đơn vị "1 đơn".
//
// groupKey được TRUYỀN VÀO chứ không import: file này phải nạp được bằng
// `node --experimental-strip-types` (bộ test của repo), mà runner đó không
// phân giải nổi import không đuôi khi là import giá trị - và
// ./zaloGrouping có đúng một import như vậy. Chỗ gọi duy nhất
// (MisaPushModal) truyền thẳng groupKeyFor, nên vẫn chỉ có MỘT định nghĩa
// khoá nhóm cho cả bảng kết quả, nút Zalo lẫn modal này; chữ ký ép phải
// truyền một cái gì đó, không có đường quên.
//
// Dòng không có excelRows bị bỏ hẳn: không trích xuất được thì không có
// gì trong sổ đặt hàng để đẩy, và để nó hiện lên chỉ tổ khiến người dùng
// tưởng nó sẽ vào sổ.
export function buildMisaGroups(rows: OrderRow[], groupKey: (row: OrderRow) => string): MisaGroupSeed[] {
  const order: string[] = []
  const byKey = new Map<string, MisaGroupSeed>()

  for (const row of rows) {
    if (!row.excelRows || row.excelRows.length === 0) continue
    const key = groupKey(row)
    const existing = byKey.get(key)
    if (existing) {
      existing.excelRows.push(...row.excelRows)
      continue
    }
    order.push(key)
    byKey.set(key, {
      key,
      po: row.po,
      // Thông tin định tuyến lấy từ dòng ĐẦU của nhóm: mọi dòng trong
      // một đơn đều cùng hệ thống, cùng mã khách hàng, cùng kho giao.
      system: row.system,
      customerCode: row.maKhachHang,
      shipTo: row.shipTo,
      excelRows: [...row.excelRows],
    })
  }

  return order.map((key) => byKey.get(key)!)
}

// branchTotals đếm số đơn và số dòng Excel của từng nhánh, chỉ tính các
// đơn đang tick.
export function branchTotals(groups: MisaGroup[]): Record<string, { orders: number; rows: number }> {
  const totals: Record<string, { orders: number; rows: number }> = {}
  for (const option of MISA_BRANCH_OPTIONS) {
    totals[option.value] = { orders: 0, rows: 0 }
  }
  for (const g of groups) {
    if (!g.selected || !g.branch) continue
    const bucket = totals[g.branch] ?? (totals[g.branch] = { orders: 0, rows: 0 })
    bucket.orders += 1
    bucket.rows += g.excelRows.length
  }
  return totals
}

// pendingGroups là các đơn còn phải đẩy: đã tick, và thuộc nhánh CHƯA vào
// sổ thành công. Nhánh đã ghi xong bị loại hẳn - bấm đẩy lại chỉ gửi
// nhánh còn lỗi, không có đường nào ghi trùng nhánh đã vào sổ.
export function pendingGroups(groups: MisaGroup[], pushedBranches: string[]): MisaGroup[] {
  const done = new Set(pushedBranches)
  return groups.filter((g) => g.selected && !done.has(g.branch))
}

// canPush khoá nút đẩy cho tới khi mọi đơn đang tick đều có nhánh. Không
// đoán bừa một nhánh cho khoá chưa map: đoán sai là đơn vào sổ của pháp
// nhân khác.
export function canPush(groups: MisaGroup[], pushedBranches: string[]): boolean {
  const pending = groups.filter((g) => g.selected)
  if (pending.length === 0) return false
  if (pending.some((g) => !g.branch)) return false
  return pendingGroups(groups, pushedBranches).length > 0
}

// rememberRouting dựng map khoá định tuyến -> nhánh để lưu vào Cài đặt.
//
// Khoá bị đặt HAI nhánh khác nhau trong cùng một lượt thì bỏ hẳn: người
// dùng cố tình cho hai đơn cùng loại vào hai sổ khác nhau lần này, ghi
// lại một trong hai là đoán bừa cho lần sau. Thà để lần sau hỏi lại.
export function rememberRouting(groups: MisaGroup[]): Record<string, string> {
  const seen = new Map<string, string>()
  const conflicting = new Set<string>()

  for (const g of groups) {
    if (!g.selected || !g.branch || !g.routeKey) continue
    const previous = seen.get(g.routeKey)
    if (previous !== undefined && previous !== g.branch) {
      conflicting.add(g.routeKey)
      continue
    }
    seen.set(g.routeKey, g.branch)
  }

  const out: Record<string, string> = {}
  for (const [key, branch] of seen) {
    if (!conflicting.has(key)) out[key] = branch
  }
  return out
}
```

- [ ] **Step 5: Chạy test để chắc chắn nó xanh**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO/frontend"
npm test
```
Expected: PASS toàn bộ, gồm 13 test mới.

- [ ] **Step 6: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/frontend/src/lib/misaBranch.ts GO/frontend/src/lib/misaBranch.test.ts GO/frontend/package.json
git commit -m "feat(ui): logic gom nhóm và khoá nút cho modal push MISA

Gom nhóm bằng đúng groupKeyFor mà bảng kết quả và nút Zalo đang dùng, nên
một dòng trên modal là đúng một đơn người dùng nhìn thấy. Dòng không có
excelRows bị bỏ hẳn — không có gì trong sổ để đẩy.

rememberRouting bỏ hẳn khoá bị đặt hai nhánh khác nhau trong cùng một
lượt: ghi lại một trong hai là đoán bừa cho lần sau.

Thêm file test vào script test của package.json — repo dùng test runner
của Node và liệt kê tường minh từng file, quên thêm là test không bao giờ
chạy."
```

---

### Task 10: Hai tab MISA trong Cài đặt

**Files:**
- Create: `GO/frontend/src/components/MisaRoutingEditor.tsx`
- Modify: `GO/frontend/src/components/SettingsModal.tsx`
- Modify: `GO/frontend/src/types.ts` (kiểu `AppSettings`)

**Interfaces:**
- Consumes: `SegmentedControl` (Task 8), `MISA_BRANCH_OPTIONS` (Task 9), binding `MisaRouteOptions` (Task 6).
- Produces: `export function MisaRoutingEditor(props: { entries: Record<string, string>; onChange: (entries: Record<string, string>) => void }): JSX.Element`

- [ ] **Step 1: Sinh lại binding Wails cho frontend**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
wails generate module
```
Expected: `frontend/wailsjs/go/main/App.d.ts`, `App.js` và `models.ts` được cập nhật, có `MisaRouteOptions`, `MisaResolveRoutes`, `PushMisa`, và `appsettings.Settings` có thêm `misa` / `misa_routing`.

Nếu `wails` không có trong PATH, cài bằng `go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0` rồi chạy lại.

- [ ] **Step 2: Thêm 2 khoá vào kiểu `AppSettings`**

Trong `GO/frontend/src/types.ts`, tìm `interface AppSettings` và thêm 2 dòng vào cuối:

```ts
  misa: Record<string, string>
  misa_routing: Record<string, string>
```

- [ ] **Step 3: Viết `MisaRoutingEditor`**

Create `GO/frontend/src/components/MisaRoutingEditor.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { MisaRouteOptions } from '../../wailsjs/go/main/App'
import { MISA_BRANCH_OPTIONS } from '../lib/misaBranch'
import { SegmentedControl } from './SegmentedControl'

interface MisaRoutingEditorProps {
  entries: Record<string, string>
  onChange: (entries: Record<string, string>) => void
}

interface RouteRow {
  key: string
  label: string
  branch: string
}

// MisaRoutingEditor là bảng "đơn của hệ thống này vào sổ của pháp nhân
// nào". Khác KeyValueEditor ở chỗ KHÔNG gõ tay khoá: khoá do Go sinh ra
// (misapush.RouteKey) nên gõ sai một ký tự là dòng đó không bao giờ khớp
// đơn nào, mà không có gì báo.
//
// Danh sách là hợp của bảng gieo mặc định và mọi khoá đã lưu — Go lo phần
// gộp đó trong MisaRouteOptions, ở đây chỉ hiển thị.
export function MisaRoutingEditor({ entries, onChange }: MisaRoutingEditorProps) {
  const [rows, setRows] = useState<RouteRow[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    MisaRouteOptions()
      .then((options) =>
        setRows(
          options.map((o) => ({
            key: o.key,
            label: o.label,
            // Giá trị đang sửa dở trong popup thắng giá trị Go đọc từ
            // đĩa: người dùng có thể đã bấm đổi vài dòng rồi mới chuyển
            // tab, chưa bấm Lưu.
            branch: entries[o.key] ?? o.branch ?? '',
          })),
        ),
      )
      .catch((err) => setError(String(err)))
    // Chỉ nạp một lần khi mở tab; những lần đổi sau đã nằm trong `rows`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (error) {
    return <p className="font-sans text-xs text-danger">Không tải được bảng định tuyến: {error}</p>
  }
  if (!rows) {
    return <p className="font-sans text-xs text-muted">Đang tải…</p>
  }

  function setBranch(key: string, branch: string) {
    const next = rows!.map((r) => (r.key === key ? { ...r, branch } : r))
    setRows(next)
    const result: Record<string, string> = {}
    for (const r of next) {
      if (r.branch) result[r.key] = r.branch
    }
    onChange(result)
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="px-1 font-sans text-[11px] leading-relaxed text-muted">
        Đơn của mỗi hệ thống sẽ vào sổ kế toán nào. Đổi bất cứ lúc nào — bấm Lưu là áp dụng
        cho lượt đẩy kế tiếp.
      </p>
      <div className="grid grid-cols-[1fr_auto] gap-2 px-1 font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
        <span>Hệ thống</span>
        <span>Nhánh</span>
      </div>
      {rows.map((r) => (
        <div key={r.key} className="grid grid-cols-[1fr_auto] items-center gap-2">
          <span className="font-mono text-xs text-ink">{r.label}</span>
          <SegmentedControl
            options={MISA_BRANCH_OPTIONS}
            value={r.branch}
            onChange={(branch) => setBranch(r.key, branch)}
            ariaLabel={`Nhánh kế toán cho ${r.label}`}
          />
        </div>
      ))}
    </div>
  )
}
```

- [ ] **Step 4: Nối 2 tab vào `SettingsModal`**

Trong `GO/frontend/src/components/SettingsModal.tsx`:

1. Thêm import: `import { MisaRoutingEditor } from './MisaRoutingEditor'`
2. Sửa `type SettingsTab` thành:
```tsx
type SettingsTab = 'gid' | 'zalo' | 'reminder' | 'haravan' | 'misa' | 'misaRouting'
```
3. Sửa `useState` của `dupState` thành:
```tsx
  const [dupState, setDupState] = useState({ gid: false, zalo: false, reminder: false, haravan: false, misa: false })
```
4. Sửa `hasDuplicates` thành:
```tsx
  const hasDuplicates = dupState.gid || dupState.zalo || dupState.reminder || dupState.haravan || dupState.misa
```
5. Thêm 2 mục vào mảng `tabs`, sau `haravan`:
```tsx
    { key: 'misa', label: 'MISA' },
    { key: 'misaRouting', label: 'MISA – Nhánh' },
```
6. Thêm 2 khối render, ngay sau khối `{tab === 'haravan' && (...)}`:
```tsx
          {tab === 'misa' && (
            <KeyValueEditor
              entries={settings.misa}
              onChange={(misa) => setSettings({ ...settings, misa })}
              onDuplicateChange={(hasDup) => setDupState((d) => ({ ...d, misa: hasDup }))}
              keyLabel="Khoá"
              valueLabel="Giá trị"
              valueType="text"
              // sid_url mang secret trên query string — ai có nó lấy được
              // phiên MISA. Tên bộ dữ liệu thì hiện bình thường để đọc lại
              // được mình đang đẩy vào sổ nào.
              secretKeys={['sid_url']}
            />
          )}
          {tab === 'misaRouting' && (
            <MisaRoutingEditor
              entries={settings.misa_routing}
              onChange={(misa_routing) => setSettings({ ...settings, misa_routing })}
            />
          )}
```

- [ ] **Step 5: Kiểm tra kiểu**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO/frontend"
npx tsc --noEmit
```
Expected: không in ra lỗi nào.

- [ ] **Step 6: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/frontend/src/components/MisaRoutingEditor.tsx GO/frontend/src/components/SettingsModal.tsx GO/frontend/src/types.ts GO/frontend/wailsjs
git commit -m "feat(ui): hai tab MISA trong Cài đặt

Tab MISA dùng lại KeyValueEditor cho sid_url/db_ha_thanh/db_htla, sid_url
được che vì nó mang secret trên query string.

Tab MISA - Nhánh là bảng riêng: KHÔNG gõ tay khoá, vì khoá do
misapush.RouteKey sinh ra nên gõ sai một ký tự là dòng đó không bao giờ
khớp đơn nào mà chẳng có gì báo. Mỗi dòng một SegmentedControl, dùng
chung component với bộ chọn buổi JIT."
```

---

### Task 11: Modal push và nút "Push MISA"

**Files:**
- Create: `GO/frontend/src/components/MisaPushModal.tsx`
- Modify: `GO/frontend/src/store/appStore.ts`
- Modify: `GO/frontend/src/hooks/useWailsEvents.ts`
- Modify: `GO/frontend/src/components/ControlPanel.tsx`

**Interfaces:**
- Consumes: `buildMisaGroups`, `branchTotals`, `canPush`, `pendingGroups`, `rememberRouting`, `MISA_BRANCH_OPTIONS`, `MisaGroup` (Task 9); `SegmentedControl` (Task 8); binding `MisaResolveRoutes`, `PushMisa`, `GetAppSettings`, `SaveAppSettings` (Task 6, 7).
- Produces: `export function MisaPushModal(props: { onClose: () => void }): JSX.Element`; store thêm `isPushing`, `setPushing`, `misaResults`, `appendMisaResult`, `clearMisaResults`.

- [ ] **Step 1: Thêm trạng thái vào store**

Trong `GO/frontend/src/store/appStore.ts`:

1. Thêm kiểu trước `interface AppState`:
```ts
export interface MisaPushResult {
  branch: string
  ok: boolean
  valid: number
  invalid: number
  message: string
}
```
2. Thêm vào `interface AppState`, ngay sau `lockStatus: LockStatus`:
```ts
  isPushing: boolean
  misaResults: MisaPushResult[]
  setPushing: (pushing: boolean) => void
  appendMisaResult: (result: MisaPushResult) => void
  clearMisaResults: () => void
```
3. Thêm vào phần khởi tạo store, ngay sau `lockStatus: 'checking',`:
```ts
  isPushing: false,
  // Kết quả từng nhánh của lượt đẩy MISA hiện tại. Modal KHÔNG tự đóng
  // khi xong - nó chuyển sang màn hình kết quả, và nhánh đã vào sổ bị
  // khoá lại để bấm đẩy lại chỉ gửi nhánh còn lỗi.
  misaResults: [],
```
4. Thêm vào phần cài đặt hàm, ngay sau `setLockStatus: (lockStatus) => set({ lockStatus }),`:
```ts
  setPushing: (isPushing) => set({ isPushing }),
  appendMisaResult: (result) =>
    set((state) => ({ misaResults: [...state.misaResults, result] })),
  clearMisaResults: () => set({ misaResults: [] }),
```

- [ ] **Step 2: Bắt 3 sự kiện MISA**

Trong `GO/frontend/src/hooks/useWailsEvents.ts`:

1. Thêm 3 selector, sau `const setTMDTMissing = ...`:
```ts
  const setPushing = useAppStore((s) => s.setPushing)
  const appendMisaResult = useAppStore((s) => s.appendMisaResult)
```
2. Thêm 3 đăng ký, ngay trước `OnFileDrop(() => {}, false)`:
```ts
    const offMisaLog = EventsOn('misa:log', (line: string) => appendLog(line))
    // misa:pushed báo theo NHÁNH chứ không theo đơn: một nhánh là một
    // lần đẩy nguyên khối, MISA không trả kết quả riêng cho từng đơn.
    const offMisaPushed = EventsOn('misa:pushed', (result: MisaPushResult) => appendMisaResult(result))
    const offMisaDone = EventsOn('misa:done', () => setPushing(false))
```
3. Thêm vào hàm cleanup (mảng `return () => { ... }`), trước `OnFileDropOff()`:
```ts
      offMisaLog()
      offMisaPushed()
      offMisaDone()
```
4. Thêm `setPushing, appendMisaResult` vào mảng dependency cuối `useEffect`.
5. Thêm import kiểu ở đầu file:
```ts
import { useAppStore, type LockStatus, type MisaPushResult } from '../store/appStore'
```
(thay dòng import `useAppStore` hiện có).

- [ ] **Step 3: Viết `MisaPushModal`**

Create `GO/frontend/src/components/MisaPushModal.tsx`:

```tsx
import { useEffect, useRef, useState } from 'react'
import { FaXmark, FaCloudArrowUp, FaSpinner } from 'react-icons/fa6'
import { GetAppSettings, MisaResolveRoutes, PushMisa, SaveAppSettings } from '../../wailsjs/go/main/App'
import { useAppStore } from '../store/appStore'
import {
  MISA_BRANCH_OPTIONS,
  branchTotals,
  buildMisaGroups,
  canPush,
  pendingGroups,
  rememberRouting,
  type MisaGroup,
} from '../lib/misaBranch'
import { groupKeyFor } from '../lib/zaloGrouping'
import { SegmentedControl } from './SegmentedControl'
import { useModalEntrance } from '../lib/useModalEntrance'

interface MisaPushModalProps {
  onClose: () => void
}

export function MisaPushModal({ onClose }: MisaPushModalProps) {
  const rows = useAppStore((s) => s.rows)
  const isPushing = useAppStore((s) => s.isPushing)
  const setPushing = useAppStore((s) => s.setPushing)
  const misaResults = useAppStore((s) => s.misaResults)
  const clearMisaResults = useAppStore((s) => s.clearMisaResults)
  const appendLog = useAppStore((s) => s.appendLog)

  const [groups, setGroups] = useState<MisaGroup[] | null>(null)
  const [remember, setRemember] = useState(true)
  const [error, setError] = useState('')
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef, [!!groups])

  useEffect(() => {
    // groupKeyFor truyền vào đây là chỗ DUY NHẤT nối modal với định
    // nghĩa khoá nhóm dùng chung của bảng kết quả và nút gửi Zalo.
    const seeds = buildMisaGroups(rows, groupKeyFor)
    MisaResolveRoutes(
      seeds.map((s) => ({ system: s.system, customerCode: s.customerCode, shipTo: s.shipTo })),
    )
      .then((infos) =>
        setGroups(
          seeds.map((s, i) => ({
            ...s,
            routeKey: infos[i]?.key ?? s.system,
            routeLabel: infos[i]?.label ?? s.system,
            branch: infos[i]?.branch ?? '',
            selected: true,
          })),
        ),
      )
      .catch((err) => setError(String(err)))
    // Chụp một lần lúc mở modal: bảng kết quả không đổi trong lúc modal
    // đang mở (nút Push đã bị khoá khi đang xử lý lô).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const pushedBranches = misaResults.filter((r) => r.ok).map((r) => r.branch)

  function setGroupBranch(key: string, branch: string) {
    setGroups((cur) => cur && cur.map((g) => (g.key === key ? { ...g, branch } : g)))
  }

  function toggleGroup(key: string) {
    setGroups((cur) => cur && cur.map((g) => (g.key === key ? { ...g, selected: !g.selected } : g)))
  }

  async function handlePush() {
    if (!groups) return
    const jobs = pendingGroups(groups, pushedBranches).map((g) => ({
      po: g.po,
      routeKey: g.routeKey,
      branch: g.branch,
      excelRows: g.excelRows,
    }))
    if (jobs.length === 0) return

    if (remember) {
      try {
        const settings = await GetAppSettings()
        await SaveAppSettings({
          ...settings,
          misa_routing: { ...settings.misa_routing, ...rememberRouting(groups) },
        })
      } catch (err) {
        // Không chặn việc đẩy: ghi nhớ chỉ là tiện lợi cho lần sau.
        appendLog(`⚠️ Không lưu được nhánh đã chọn: ${String(err)}`)
      }
    }

    clearMisaResults()
    setPushing(true)
    try {
      await PushMisa(jobs)
    } catch (err) {
      appendLog(`❌ Lỗi đẩy MISA: ${String(err)}`)
      setPushing(false)
    }
  }

  const totals = groups ? branchTotals(groups) : null
  const ready = groups ? canPush(groups, pushedBranches) : false

  return (
    <div ref={backdropRef} className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div
        ref={cardRef}
        className="flex max-h-[80vh] w-[720px] flex-col rounded-xl border border-border bg-panel p-4"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-sans text-sm font-bold text-ink">
            Push MISA{groups ? ` — ${groups.length} đơn` : ''}
          </h2>
          <button type="button" onClick={onClose} className="text-muted hover:text-ink">
            <FaXmark size={16} />
          </button>
        </div>

        {error && <p className="font-sans text-xs text-danger">Không phân giải được nhánh: {error}</p>}
        {!groups && !error && <p className="font-sans text-xs text-muted">Đang tải…</p>}

        {groups && (
          <>
            <div className="flex-1 overflow-y-auto">
              <div className="grid grid-cols-[auto_1fr_1fr_auto_auto] items-center gap-2 px-1 pb-1 font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
                <span />
                <span>Số đơn hàng</span>
                <span>Hệ thống</span>
                <span>Dòng</span>
                <span>Nhánh</span>
              </div>
              {groups.map((g) => {
                const locked = pushedBranches.includes(g.branch)
                return (
                  <div key={g.key} className="grid grid-cols-[auto_1fr_1fr_auto_auto] items-center gap-2 py-1">
                    <input
                      type="checkbox"
                      checked={g.selected && !locked}
                      disabled={locked || isPushing}
                      onChange={() => toggleGroup(g.key)}
                      className="h-4 w-4 accent-accent"
                    />
                    <span className="truncate font-mono text-xs text-ink" title={g.po}>{g.po}</span>
                    <span className="truncate font-sans text-xs text-muted" title={g.routeLabel}>{g.routeLabel}</span>
                    <span className="tabular-nums font-mono text-xs text-muted">{g.excelRows.length}</span>
                    <SegmentedControl
                      options={MISA_BRANCH_OPTIONS}
                      value={g.branch}
                      disabled={locked || isPushing}
                      onChange={(branch) => setGroupBranch(g.key, branch)}
                      ariaLabel={`Nhánh kế toán cho đơn ${g.po}`}
                    />
                  </div>
                )
              })}
            </div>

            {misaResults.length > 0 && (
              <div className="mt-3 flex flex-col gap-1 border-t border-border pt-3">
                {misaResults.map((r) => (
                  <p
                    key={r.branch}
                    className={`font-sans text-xs ${r.ok ? 'text-success' : 'text-danger'}`}
                  >
                    {MISA_BRANCH_OPTIONS.find((o) => o.value === r.branch)?.label ?? r.branch}:{' '}
                    {r.ok ? `đã ghi ${r.valid} chứng từ vào sổ` : r.message}
                  </p>
                ))}
              </div>
            )}

            <div className="mt-3 flex items-center justify-between gap-3 border-t border-border pt-3">
              <label className="flex items-center gap-2 font-sans text-xs text-muted">
                <input
                  type="checkbox"
                  checked={remember}
                  onChange={(e) => setRemember(e.target.checked)}
                  className="h-3.5 w-3.5 accent-accent"
                />
                Ghi nhớ nhánh đã chọn
              </label>
              <span className="ml-auto font-sans text-xs text-muted">
                {MISA_BRANCH_OPTIONS.map((o) => {
                  const t = totals?.[o.value] ?? { orders: 0, rows: 0 }
                  return `${o.label}: ${t.orders} đơn / ${t.rows} dòng`
                }).join(' · ')}
              </span>
              <button
                type="button"
                onClick={handlePush}
                disabled={!ready || isPushing}
                className="inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2 font-sans text-xs font-bold text-[#0a1620] transition-opacity disabled:cursor-not-allowed disabled:opacity-40"
              >
                {isPushing ? <FaSpinner className="animate-spin" /> : <FaCloudArrowUp />}
                {isPushing ? 'ĐANG ĐẨY…' : 'ĐẨY LÊN MISA'}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Bật nút "Push MISA" trong `ControlPanel`**

Trong `GO/frontend/src/components/ControlPanel.tsx`:

1. Thêm import: `import { MisaPushModal } from './MisaPushModal'`
2. Thêm 2 selector, sau `const jitPeriodState = ...`:
```tsx
  const isPushing = useAppStore((s) => s.isPushing)
  const clearMisaResults = useAppStore((s) => s.clearMisaResults)
```
3. Thêm state, cạnh `pendingTMDT`:
```tsx
  const [isMisaOpen, setIsMisaOpen] = useState(false)
```
4. Thay **toàn bộ** khối `<div title="Sẽ có ở giai đoạn sau" ...>…</div>` bằng:
```tsx
        <button
          type="button"
          onClick={() => {
            clearMisaResults()
            setIsMisaOpen(true)
          }}
          disabled={rows.length === 0 || isProcessing || isPushing}
          title={rows.length === 0 ? 'Xử lý đơn hàng trước đã' : 'Đẩy đơn vừa xử lý lên AMIS Kế toán'}
          className="inline-flex items-center gap-2 rounded-lg border border-border px-3 py-2 font-sans text-xs font-semibold text-muted transition-colors hover:border-accent hover:text-accent disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:border-border disabled:hover:text-muted"
        >
          <FaCloudArrowUp /> Push MISA
        </button>
```
5. Thêm modal vào cuối fragment, ngay sau khối `{pendingTMDT && (...)}`:
```tsx
      {isMisaOpen && <MisaPushModal onClose={() => setIsMisaOpen(false)} />}
```

- [ ] **Step 5: Kiểm tra kiểu và chạy test**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO/frontend"
npx tsc --noEmit && npm test
```
Expected: `tsc` không in lỗi nào, `npm test` PASS toàn bộ.

- [ ] **Step 6: Build frontend để chắc chắn Vite đóng gói được**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO/frontend"
npm run build
```
Expected: build thành công, ghi ra `dist/`.

- [ ] **Step 7: Commit**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git add GO/frontend/src/components/MisaPushModal.tsx GO/frontend/src/components/ControlPanel.tsx GO/frontend/src/store/appStore.ts GO/frontend/src/hooks/useWailsEvents.ts
git commit -m "feat(ui): modal push MISA và bật nút Push MISA

Nút thay chỗ nhãn SẮP RA MẮT, chỉ bật khi bảng kết quả có đơn và không có
lô nào đang chạy — đẩy lại lô cũ nằm sẵn trên đĩa là đường ngắn nhất tới
việc ghi trùng vào sổ kế toán.

Modal không tự đóng khi xong: nó chuyển sang màn hình kết quả từng nhánh,
và nhánh đã vào sổ bị khoá lại (bỏ tick, tắt segmented control) nên bấm
đẩy lại chỉ gửi nhánh còn lỗi."
```

---

### Task 12: Chạy toàn bộ, kiểm tra thật trên app

**Files:**
- Modify: `GO/frontend/wailsjs/**` (nếu `wails generate module` sinh thêm thay đổi)

**Interfaces:**
- Consumes: mọi thứ từ Task 1–11.
- Produces: không có API mới.

- [ ] **Step 1: Chạy toàn bộ test Go**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
go test ./...
```
Expected: mọi package PASS, **trừ** đúng 5 test JIT air-waybill trong `order-processor/internal/processing` đã đỏ từ trước vì thiếu fixture PDF. Bất kỳ test nào khác đỏ đều phải sửa.

- [ ] **Step 2: Chạy toàn bộ test frontend**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO/frontend"
npm test
```
Expected: PASS toàn bộ.

- [ ] **Step 3: Khẳng định không thêm dependency Go nào**

`1d70886e` là commit spec cuối cùng, tức là trạng thái repo ngay trước Task 1 —
mốc cố định, không lệ thuộc vào việc trong lúc thực thi có phát sinh thêm commit
sửa lỗi hay không.

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git diff 1d70886e..HEAD --stat -- GO/go.mod GO/go.sum
```
Expected: **không in ra gì**.

- [ ] **Step 4: Khẳng định `misa/` gốc và luồng xử lý không bị đụng**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git diff 1d70886e..HEAD --stat -- misa/ GO/internal/processing/
```
Expected: **không in ra gì** cho `misa/`.

Lưu ý: repo này đang được một agent khác (codex) dùng song song và nó **có** sửa
file trong `GO/internal/processing/`. Nếu lệnh trên in ra thay đổi ở thư mục đó,
đối chiếu `git log --oneline -- GO/internal/processing/` xem commit đó có thuộc
kế hoạch này không. Không thuộc thì bỏ qua, **không hoàn tác**.

- [ ] **Step 5: Chạy app thật và soát bằng mắt**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng/GO"
wails dev
```

Soát đủ 6 điểm:
1. Mở Cài đặt (icon bánh răng) → có 2 tab mới **MISA** và **MISA – Nhánh**.
2. Tab **MISA – Nhánh** hiện đủ 16 dòng, `BigC · gia công` sáng ở HTLA, `BigC · modern trade` sáng ở Hà Thành, `JIT · kho WH6_HN` sáng ở Hà Thành, `JIT · kho WH6_HTLA` sáng ở HTLA.
3. Đổi một dòng bất kỳ → bấm Lưu → đóng và mở lại Cài đặt → giá trị mới vẫn còn.
4. Bấm điều khiển buổi giao của một đơn JIT trên bảng kết quả → trông y hệt như trước khi đổi.
5. Khi bảng kết quả trống, nút **Push MISA** mờ và không bấm được.
6. Xử lý một lô rồi bấm **Push MISA** → modal liệt kê đúng số đơn, mỗi đơn có nhãn khoá định tuyến và nhánh đã chọn sẵn.

**Chưa bấm "ĐẨY LÊN MISA"** ở bước soát này — nó ghi thật vào sổ kế toán. Việc nghiệm thu đẩy thật do người dùng tự làm khi đã khai `sid_url` và tên 2 bộ dữ liệu.

- [ ] **Step 6: Kiểm tra `settings.bhconfig` đã được gieo**

Run:
```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
python -c "import json,io; d=json.load(io.open('settings.bhconfig',encoding='utf-8')); print(json.dumps(d.get('misa_routing'), ensure_ascii=False, indent=2))"
```
Expected: in ra 16 khoá với nhánh tương ứng.

- [ ] **Step 7: Commit phần còn lại (nếu có)**

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng"
git status --short -- GO/frontend/wailsjs
# Nếu có thay đổi chưa commit:
git add GO/frontend/wailsjs
git commit -m "chore(wails): cập nhật binding sinh tự động cho các method MISA"
```
