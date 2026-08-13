# Thiết kế: Khung sườn ứng dụng Go + Wails + React (Phase 1 của dự án refactor)

## Bối cảnh

Ứng dụng hiện tại (`xulydonhang.py`, 10.406 dòng + `App.py`, `send_zalo.py`,
`uploadmisa.py`...) là một app desktop PySide6 xử lý đơn đặt hàng PDF/XLSX từ
~20 hệ thống bán lẻ khác nhau (Coop, BigC, Lotte, Satra, Fujimart, Farmer, BHX,
Kingfood, Winmart, Emart, CN-HCM, JIT, Jupviec, KOC, TMDT...), ghi kết quả vào
`dondathang.xlsx`, và tích hợp tự động hoá Zalo (Playwright) + MISA (Selenium).

Đây là hệ thống quá lớn để viết một spec/kế hoạch duy nhất. Dự án được chia
thành nhiều sub-project độc lập, làm tuần tự. Tài liệu này là spec cho
**Phase 1 duy nhất**: dựng khung sườn ứng dụng Go + Wails + React, tái hiện
luồng UI hiện có với dữ liệu xử lý đơn hàng ở dạng mock/stub.

## Lộ trình tổng thể (tham khảo, không phải scope của spec này)

1. **Phase 1 (spec này):** Khung sườn Wails + React — UI, quản lý file, STT,
   log/bảng kết quả realtime qua event, xử lý đơn hàng bằng `MockProcessor`.
2. **Phase 2:** Engine parse PDF + ghi Excel thật, bắt đầu với 2-3 vendor tiêu
   biểu (Coop, BigC), chứng minh pattern trước khi làm tiếp.
3. **Phase 3:** Mở rộng `RealProcessor` cho toàn bộ ~20 vendor còn lại.
4. **Phase 4:** Zalo automation (thay Playwright Python bằng Go, ví dụ
   `playwright-go` hoặc `chromedp`).
5. **Phase 5:** MISA automation — đồng thời sửa lỗi bảo mật đang hardcode
   email/mật khẩu đăng nhập MISA trong `uploadmisa.py`, chuyển sang đọc từ
   config/env khi viết lại bằng Go.

Mỗi phase sẽ có brainstorm + spec + plan riêng khi bắt đầu.

## Phạm vi Phase 1

### Làm thật (real)

- Chọn file qua dialog native của Wails (multi-select, lọc `.pdf/.xlsx/.txt`).
- Kéo-thả file vào cửa sổ (native drag-and-drop của Wails).
- Quét thư mục `đơn hàng/MM-YYYY` (tự tạo nếu chưa có), liệt kê file hợp lệ —
  tương đương `ensure_monthly_order_folder` + `load_files_from_folder` hiện tại.
- Đọc/ghi số thứ tự đơn hàng (STT) từ file config, tương đương `config.txt`
  (`current_row`) hiện tại.
- Luồng sự kiện Go → React thật: log realtime + cập nhật bảng kết quả từng
  dòng, dùng `runtime.EventsEmit`/`EventsOn` của Wails — đây là cơ chế sẽ giữ
  nguyên xuyên suốt các phase sau, chỉ có nguồn dữ liệu (mock → thật) thay đổi.

### Stub (giả lập, thay thế ở phase sau)

- Xử lý đơn hàng: `MockProcessor` — với mỗi file, delay giả (1-2s), trả về một
  `OrderRow` với tên hệ thống lấy ngẫu nhiên từ danh sách vendor thật trong
  `settings.ini` (Coop, BigC, Lotte...) để bảng kết quả nhìn chân thực, PO/mã
  khách hàng/đơn giá là dữ liệu mẫu, trạng thái random trong
  {Hoàn Thành, Thất bại, Hoàn Thành (cảnh báo)} để xem đủ 3 màu trạng thái.
- Nút "Gửi thông báo Zalo" và "Push MISA": hiển thị nhưng `disabled`, có
  tooltip "Sẽ có ở giai đoạn sau".
- Kiểm tra khoá bản quyền qua Google Apps Script (`check_lock` hiện tại): bỏ
  qua hoàn toàn trong Phase 1, coi như luôn `active = 1`.
- Đồng bộ Google Sheets (`export_table_to_log` gửi POST lên Apps Script,
  đánh dấu PO trùng lặp): không làm ở Phase 1.

### Ngoài phạm vi (không đụng tới trong Phase 1)

- Toàn bộ logic parse PDF/OCR/fuzzy-matching theo từng vendor.
- Ghi `dondathang.xlsx` với format màu/comment thật.
- Zalo automation, MISA automation, Google Drive upload.
- Xoá hoặc thay thế code Python hiện có — Python vẫn là bản chạy production
  cho tới khi Go version thay thế được ở phase cuối cùng.

## Kiến trúc

**Wails v2** (ổn định) thay vì v3 (alpha). Toàn bộ code Go mới nằm trong thư
mục `GO/` ở gốc repo, tách biệt hoàn toàn với code Python hiện tại.

```
GO/
  main.go                     # wails.Run bootstrap
  app.go                      # struct App + các method bind cho frontend
  wails.json
  go.mod
  internal/
    config/
      config.go               # đọc/ghi STT (tương đương config.txt)
    fileset/
      fileset.go               # quét "đơn hàng/MM-YYYY", lọc đuôi file hợp lệ
    processing/
      processor.go             # interface Processor + MockProcessor (Phase 1)
      types.go                 # struct OrderRow
  frontend/
    package.json, vite.config.ts, tsconfig.json
    src/
      main.tsx, App.tsx
      components/
        FileListPanel.tsx      # danh sách file, nút "Tải lại", xoá bằng Delete
        ControlPanel.tsx        # ô STT, nút Xử lý / Gửi Zalo (disabled) / Push MISA (disabled)
        LogPanel.tsx             # nhật ký hệ thống, realtime, auto-scroll
        ResultTable.tsx          # bảng kết quả, màu theo trạng thái
        InfoTab.tsx               # tab "Thông tin": giới thiệu, QR, liên hệ
      hooks/
        useWailsEvents.ts        # subscribe process:log / process:row / process:done
      store/
        appStore.ts               # Zustand: file list, stt, processing flag, log, rows
```

### Backend (Go) — các method bind cho frontend

- `SelectFiles() ([]string, error)` — mở dialog chọn nhiều file, lọc đuôi.
- `ScanOrderFolder() ([]string, error)` — quét thư mục tháng-năm hiện tại,
  tự tạo thư mục nếu thiếu.
- `GetSTT() (int, error)` / `SetSTT(v int) error` — đọc/ghi config STT.
- `ProcessFiles(files []string, stt int) error` — chạy trong goroutine riêng,
  với mỗi file gọi `processing.Processor.Process(...)`; Phase 1 dùng
  `MockProcessor`. Emit `process:log` cho mỗi bước, `process:row` mỗi khi có
  kết quả một file, `process:done` khi xong toàn bộ. Có `recover()` quanh
  goroutine để lỗi trong xử lý 1 file không làm sập cả luồng.

`processing.Processor` là interface duy nhất:

```go
type Processor interface {
    Process(ctx context.Context, filePath string, stt int) (OrderRow, error)
}
```

Phase 1 implement `MockProcessor`. Phase 2 sẽ thêm `RealProcessor` cùng
implement interface này — phần còn lại của hệ thống (event contract, UI)
không cần đổi.

### Frontend (React + TypeScript)

- 2 tab: "Xử lý Đơn hàng" (FileListPanel + ControlPanel + LogPanel +
  ResultTable) và "Thông tin" (InfoTab).
- State toàn cục qua Zustand (`appStore`): danh sách file, STT, cờ đang xử lý
  (để khoá UI như `_lock_ui_for_processing` hiện tại), danh sách dòng log,
  danh sách dòng kết quả.
- Gọi Go qua binding tự sinh (`wailsjs/go/main/App`), nhận event qua
  `wailsjs/runtime` (`EventsOn`).
- Icon: `react-icons/fa6` (Font Awesome).
- Style: Tailwind CSS, thiết kế lại hoàn toàn theo hướng dashboard hiện đại
  (dùng skill `frontend-design`), không giữ giao diện Qt cũ, nhưng giữ nguyên
  nghiệp vụ/bố cục chức năng đã có.

### Cảm giác desktop app (không phải trang web)

Webview của Wails mặc định vẫn có hành vi trình duyệt (bôi đen text khi kéo
chuột, menu chuột phải mặc định, kéo-thả ảnh, zoom bằng Ctrl+cuộn...). Để
UI cảm giác như app desktop thật:

- `user-select: none` áp dụng toàn cục cho UI chrome (nút, nhãn, tab, header,
  panel tiêu đề...). Ngoại lệ: vùng nội dung `LogPanel` và các ô dữ liệu
  trong `ResultTable` vẫn cho phép bôi đen/copy (giữ đúng hành vi
  `QTextEdit`/`QTableWidget` hiện tại — người dùng cần copy số PO, copy log
  lỗi để báo cáo).
- Tắt context menu mặc định của webview (chuột phải) trên toàn bộ ứng dụng.
- Tắt kéo-thả ảnh/element mặc định của trình duyệt (`draggable="false"` cho
  ảnh QR, ngăn `dragstart` mặc định ngoài vùng file-drop hợp lệ).
- Tắt zoom bằng Ctrl+cuộn/Ctrl+/-/= trong webview.
- Build production tắt DevTools/chuột-phải-inspect (Wails mặc định đã tắt
  DevTools khi build không kèm `-debug`, chỉ cần đảm bảo không bật lại).

### Data flow

```
User click "Xử lý đơn hàng"
  → React gọi App.ProcessFiles(files, stt)
  → Go goroutine: với mỗi file → MockProcessor.Process()
      → EventsEmit("process:log", "...")
      → EventsEmit("process:row", OrderRow{...})
  → EventsEmit("process:done")
  → React: useWailsEvents cập nhật Zustand store → LogPanel/ResultTable re-render
```

### Error handling

- Mọi method Go bind trả `error`; frontend hiển thị lỗi thành dòng log màu đỏ
  (không dùng dialog chặn, giữ đúng tinh thần "log liên tục" của app hiện tại).
- `ProcessFiles` bọc `recover()` quanh xử lý từng file — một file lỗi chỉ ghi
  trạng thái "Thất bại" cho file đó, không dừng toàn bộ batch.

### Testing

- Go: unit test cho `internal/config` (đọc/ghi STT round-trip) và
  `internal/fileset` (lọc đuôi file, logic tạo thư mục tháng-năm) bằng
  `testing` chuẩn + thư mục tạm (`t.TempDir()`).
- `MockProcessor` không cần test sâu (là code giả lập tạm thời).
- Frontend: không viết test tự động cho Phase 1 (YAGNI cho khung sườn); xác
  minh thủ công bằng cách chạy `wails dev` và thao tác qua các luồng chính.

## Rủi ro / lưu ý

- Wails v2 cần WebView2 runtime trên Windows (thường có sẵn từ Win10 1803+/
  Win11) — không cần cài thêm trên máy đa số người dùng.
- Thư mục `GO/` là dự án Go/Node độc lập, không ảnh hưởng tới `xulydonhang.py`
  đang chạy production; hai bản tồn tại song song cho tới khi các phase sau
  hoàn tất.
