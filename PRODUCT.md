# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Xác nhận qua phỏng vấn 2026-08-24: **chủ doanh nghiệp cùng vài nhân viên**, không phải một người dùng đơn lẻ. Trình độ không đồng đều — người viết ra hệ thống và người chỉ được giao thao tác cùng dùng chung một màn hình. Hệ quả bắt buộc: nhãn phải đọc là hiểu, lỗi phải nói rõ chuyện gì xảy ra và làm gì tiếp, và các nút phá hoại phải khó bấm nhầm.

App được đóng gói đem sang máy khác (thư mục `phat-hanh-may-khac/`), nên không thể giả định người ngồi trước máy biết nội tình hệ thống.

## Product Purpose

Biến file đơn đặt hàng PDF/Excel từ 10 chuỗi siêu thị thành các dòng đã chuẩn hoá trong `dondathang.xlsx`, rồi hỗ trợ bốn việc tiếp theo mà con người phải quyết.

Thành công không phải là "đổ xong đơn". Xác nhận qua phỏng vấn: **cả bốn việc sau khi đổ đơn đều nặng ngang nhau** — rà đơn lệch giá, gửi Zalo cho khách, đổi buổi giao JIT, và xử lý file lỗi. Bảng kết quả vì thế là **điểm bắt đầu của bốn luồng công việc**, không phải màn hình kết thúc.

## Positioning

Hiểu được định dạng PO riêng của từng chuỗi siêu thị Việt Nam — Coop, Lotte, Satra, BigC, WinMart, Emart, FujiMart, KingFood, JMart, JIT-Choice (Top Value) — kể cả những file PDF hỏng mà trình đọc PDF thông thường từ chối mở. Đây là thứ một phần mềm kế toán mua sẵn không có: nó gắn với đúng bộ nhà cung cấp mà doanh nghiệp này bán hàng.

## Operating Context

- App desktop Windows (Wails + Go + React), chạy cục bộ, ghi thẳng vào `dondathang.xlsx` trên máy.
- Nguồn dữ liệu sống: bảng khách hàng/sản phẩm và bảng giá lấy từ Google Sheets lúc khởi động.
- Gửi tin nhắn Zalo qua trình duyệt tự động (chromedp), cần quét QR đăng nhập.
- Nhịp làm việc **thay đổi theo lô**: xác nhận qua phỏng vấn — vài file thì ngồi nhìn chạy, cả trăm file thì bấm rồi đi làm việc khác. Giao diện phải phục vụ cả hai: theo dõi trực tiếp lúc đang chạy, và một bản tổng kết đọc được khi quay lại.
- Xử lý theo lô là realtime: mỗi đơn hiện lên ngay khi xong, không đợi hết file (đã có sẵn trong bản hiện tại).
- File nguồn nằm trong thư mục `đơn hàng/`, cỡ 163 file tại thời điểm ghi nhận.

## Capabilities and Constraints

- 10 nhà cung cấp, mỗi nhà có bộ luật trích xuất riêng; 151/151 golden fixture của Coop hiện đang khớp.
- Ghi Excel theo lô một lần cho JIT và BigC; các nhà khác ghi theo từng đoạn.
- Đối soát giá so PO với bảng giá hệ thống, trừ JIT-Choice (không đối soát).
- Đổi buổi giao JIT áp cho toàn bộ đơn trong một file PDF, khoá chung với việc ghi Excel.
- Ràng buộc cứng: **font phải phủ đủ dấu tiếng Việt**. Be Vietnam Pro + JetBrains Mono đang dùng; Geist, Satoshi, Cabinet Grotesk không đủ dấu.
- `dondathang.xlsx` phải nằm ở gốc repo; app mở và ghi trực tiếp lên nó.
- **Quan hệ file ↔ PO (xác nhận 2026-08-24): một PO chỉ nằm trong đúng một file PDF; một file có thể chứa nhiều PO.** Đây là quan hệ cha–con thật, không phải nhiều–nhiều. Hệ quả: số PO là khoá duy nhất trong một lô, cây file → PO → dòng luôn dựng được, và `resultKey = sourceID|vị trí|PO` hiện tại là đủ.
- Một PO có thể trải ra nhiều dòng: BigC dùng chung một PO cho 23 cửa hàng.
- Chưa quyết: có cần lịch sử/nhật ký các lô đã chạy hay không.

## Brand Commitments

- Tên: **Blue Hà Thành**. Nhãn phụ trong app: "Order System".
- Logo: `blue_logo_vector.svg` — khối tím `#5453A1` làm nền, nét quét cyan `#28C5F2` bên dưới, chữ "Blue" trắng nét dày bụng tròn. Người dùng đã nêu rõ (2026-08-24) rằng giao diện phải đi theo phong cách logo này; màu tím là màu thương hiệu, không phải màu mặc định cần loại bỏ.
- Toàn bộ giao diện bằng tiếng Việt.

## Evidence on Hand

- 151 golden fixture Coop + fixture của 9 nhà cung cấp còn lại, trong `GO/internal/processing/*/testdata/`.
- PDF thật trong `đơn hàng/`: BigC 23 cửa hàng (`806_SOUTHDC_Q06_3005382_2634058273095.pdf`), vận đơn JIT (`air_waybill_WH6_*`).
- Không có: số liệu doanh thu, danh sách khách hàng công khai, cam kết SLA. Không được bịa ra những thứ này.

## Product Principles

1. **Bảng kết quả là ngã tư, không phải đích đến.** Bốn việc tiếp theo đều nặng ngang nhau, nên màn hình phải dẫn được vào cả bốn chứ không chỉ trưng kết quả.
2. **Hai nhịp làm việc, một màn hình.** Vừa xem được từng đơn chạy realtime, vừa đọc được tổng kết khi quay lại sau nửa tiếng.
3. **Không giả định người dùng biết nội tình.** Nhân viên và máy khác cũng dùng bản này.
4. **Tiền là dữ liệu nhạy cảm.** Số tiền và mã hàng phải thẳng hàng, dễ so, không được để font tỉ lệ làm lệch cột.
5. **Lỗi là việc cần làm, không phải thông báo.** Một file không đọc được là một đầu việc, phải đưa được người dùng tới bước xử lý.
