# Quy tắc cú pháp định dạng tin nhắn Zalo (qua Playwright)

Cú pháp markup riêng (giống markdown) dùng để soạn tin nhắn có định dạng,
được `rich_paste_engine.py` dịch thành một khối HTML rồi **dán (paste)
một lần** vào ô soạn tin trên `chat.zalo.me`. Đây **không phải** cú pháp
markdown chuẩn của Zalo (gõ trực tiếp `**text**` vào Zalo thật sẽ hiển
thị y nguyên dấu `**`, không tự in đậm) — cú pháp này chỉ là quy ước nội
bộ để `send_pasted_message()` biết cần dựng HTML định dạng gì.

## 1. Định dạng ký tự (inline)

| Cú pháp | Kết quả | Ví dụ |
|---|---|---|
| `**text**` | **In đậm** | `**Quan trọng**` |
| `*text*` | *In nghiêng* | `*ghi chú*` |
| `__text__` | Gạch chân | `__sáng nay__` |
| `~~text~~` | ~~Gạch ngang~~ | `~~bản cũ~~` |
| `{color:text}` | Chữ màu | `{red:Khẩn cấp}` |

Màu hỗ trợ (đúng 5 màu có sẵn trên bảng màu Zalo — **bắt buộc phải là
1 trong 5 màu này**, xem mục 6 vì sao): `red`, `orange`, `yellow`,
`green`, `black`.

Có thể dùng nhiều định dạng khác nhau trên cùng một dòng, kể cả nhiều
đoạn liền kề nhau, nhưng **không lồng nhau** (không hỗ trợ
`**bold *italic* bold**`) — mỗi đoạn `**..**`, `*..*`, `__..__`,
`~~..~~`, `{color:..}` phải là các đoạn tách biệt.

Văn bản tiếng Việt có dấu hoạt động bình thường (Unicode được giữ
nguyên qua paste) — không có giới hạn gì về dấu tiếng Việt.

## 2. Danh sách (block)

Dòng bắt đầu bằng `- ` (dấu gạch ngang + khoảng trắng) → gạch đầu dòng:

```
- Mục một
- Mục hai
```

Dòng bắt đầu bằng số + dấu chấm + khoảng trắng (`1. `, `2. `, ...) →
đánh số, tự tăng đúng 1, 2, 3... Số bạn gõ trong markup chỉ cần đúng
định dạng (`<số>. <nội dung>`), không nhất thiết phải tăng dần đúng vì
Zalo tự đánh số lại khi hiển thị:

```
1. Bước một
2. Bước hai
```

Các dòng list cùng loại được gộp vào **một danh sách duy nhất**, kể cả
khi có dòng trống xen giữa (dòng trống chỉ tạo khoảng cách thị giác,
không làm ngắt danh sách) — miễn là không có dòng nội dung khác (không
phải list) xen vào giữa. Ví dụ hai cách viết sau cho kết quả numbering
giống hệt nhau:

```
1. Bước một
2. Bước hai
```

```
1. Bước một

2. Bước hai
```

## 3. Thụt lề (mục con)

Thêm **2 khoảng trắng** (hoặc 1 tab) ở đầu dòng cho mỗi cấp thụt lề, chỉ
áp dụng cho dòng danh sách (bullet/numbered):

```
- Việc chính
  - Việc con cấp 1
    - Việc con cấp 2
```

Thụt lề được áp dụng bằng một bước xử lý riêng SAU khi dán (xem mục 6),
nên tốn thêm một chút thời gian (~0.3-0.5 giây cho mỗi dòng cần thụt
lề) so với tin không có thụt lề, nhưng vẫn nhanh hơn nhiều so với gõ
toàn bộ bằng bàn phím.

## 4. Xuống dòng trong cùng một tin

Mỗi dòng (`\n`) trong chuỗi markup trở thành một dòng mới **trong cùng
một tin nhắn**. Dòng trống (`\n\n`) tạo một dòng cách quãng (trừ khi
nằm giữa 2 dòng list cùng loại — xem mục 2). Toàn bộ nội dung được dán
và gửi trong 1 tin nhắn duy nhất.

## 5. Ví dụ đầy đủ

```
**Thông báo họp nhóm**
Xin chào cả nhóm, đây là *bản tóm tắt* cuộc họp __sáng nay__.

{red:Việc cần làm gấp}:
- Hoàn thành ~~bản nháp cũ~~ và gửi bản mới trước 17h
- Liên hệ khách hàng
  - Gọi điện xác nhận lịch
  - Gửi email tổng kết

Các bước tiếp theo:
1. Review tài liệu
2. Duyệt ngân sách
3. Triển khai

{green:Cảm ơn mọi người đã cố gắng!}
```

→ đã test gửi thật vào hội thoại "My Documents" và hiển thị đúng: dòng
tiêu đề in đậm, chữ nghiêng, gạch chân, dòng cảnh báo màu đỏ, mục bị
gạch ngang, danh sách gạch đầu dòng có mục con thụt lề, danh sách đánh
số đúng thứ tự, và dòng cảm ơn màu xanh lá.

## 6. Cách dùng

Qua dòng lệnh (đã tích hợp vào `send_message.py`):

```powershell
python send_message.py "My Documents" "**Xin chào** *bạn*" --rich
```

Hoặc gọi trực tiếp trong code:

```python
from playwright.async_api import async_playwright
from rich_paste_engine import send_pasted_message

# ... mo browser, dang nhap, mo dung hoi thoai (xem send_message.py ...
# ham open_conversation) sao cho o soan tin dang hien dien tren trang ...

ok, response_body = await send_pasted_message(page, "**Xin chào** thế *giới*")
```

## 7. Cơ chế hoạt động

**Vì sao dán 1 lần lại nhanh:** bản đầu tiên của công cụ này mô phỏng
thao tác gõ tay (gõ chữ, di chuyển con trỏ bằng `Home`/`ArrowRight`/
`Shift+ArrowRight` để chọn từng vùng, rồi bấm nút định dạng) — cách này
chậm (~36 giây cho một đoạn văn 832 ký tự/18 vùng định dạng, vì mỗi lần
nhấn phím là một round-trip riêng tới trình duyệt) và có rủi ro lỗi
hiếm gặp do thao tác phím tuần tự. Cách hiện tại dựng sẵn **toàn bộ nội
dung dưới dạng 1 khối HTML** (`<b>`, `<i>`, `<u>`, `<s>`,
`<span style="color:...">`, `<ul>/<ol><li>`), rồi giả lập một sự kiện
`paste` — đúng như khi người dùng dán nội dung đã copy từ nơi khác —
thay vì gõ và chọn từng ký tự. Kết quả: cùng nội dung trên chỉ mất
**~1 giây** (nhanh hơn ~34 lần).

**Chi tiết kỹ thuật của paste:** khi bật chế độ định dạng
(`Ctrl+Shift+X`), ô soạn tin của Zalo đổi từ `id="richInput"` sang một
id ngẫu nhiên khác (element cũ bị huỷ và tạo lại), nên engine không dò
theo `#richInput` mà thao tác trên `document.activeElement`. Việc "dán"
được thực hiện bằng cách tạo một `ClipboardEvent('paste')` với
`clipboardData` chứa `text/html` và `text/plain`, rồi bắn thẳng sự kiện
này vào `document.activeElement` — không cần đụng tới clipboard thật
của hệ điều hành.

**Vì sao màu chỉ giữ được với đúng 5 mã RGB swatch:** đã thử nghiệm dán
từ Word (màu RGB tuỳ ý) và màu luôn bị mất, nhưng dán đúng
`rgb(219, 52, 46)` (một trong 5 màu swatch) thì giữ nguyên — Zalo lọc
bỏ mọi màu không khớp chính xác 1 trong 5 giá trị đó.
`COLOR_SWATCH_RGB` trong `rich_paste_engine.py` map đúng 5 màu này để
tránh bị lọc.

**Cơ chế thụt lề (hybrid — 2 bước):** dán `<ul>`/`<ol>` lồng nhau qua
paste bị Zalo làm phẳng hoàn toàn (không giữ được thụt lề), NHƯNG bấm
nút "Lùi đầu dòng" trên giao diện thật sau khi đã có nội dung trong ô
soạn tin thì thụt lề được giữ nguyên sau khi gửi. Vì vậy `_apply_indents()`
làm 2 bước: (1) dán toàn bộ nội dung dạng danh sách PHẲNG như bình
thường, (2) với mỗi dòng cần thụt lề, tìm đúng khối DOM chứa dòng đó
bằng cách khớp text, click vào cuối dòng để đặt con trỏ, rồi bấm nút
"Lùi đầu dòng" một số lần nhất định. Nếu nhiều dòng trùng nội dung y
hệt nhau, dùng chỉ số lần xuất hiện để chọn đúng dòng.

2 lỗi tinh vi đã gặp và sửa khi cài `_apply_indents()`, đáng lưu ý nếu
sau này cần chỉnh sửa lại:
- **Dòng có kèm định dạng inline khác (đậm/nghiêng/màu...) bị bỏ sót:**
  mỗi dòng được Zalo bọc trong 1 khối `[data-component="rtf-block"]`
  (DIV hoặc LI). Nếu dòng không có định dạng gì khác, khối đó là node lá
  (không có phần tử con) nên khớp text trực tiếp dễ dàng; nhưng nếu dòng
  có định dạng inline, nó bị tách thành nhiều `<span>` con bên trong
  cùng khối đó — khớp theo kiểu "chỉ tìm node lá" sẽ KHÔNG tìm thấy gì
  và ÂM THẦM bỏ qua bước thụt lề (không báo lỗi). Phải khớp theo
  `textContent` của CẢ KHỐI `[data-component="rtf-block"]`, không phải
  node lá.
- **Toạ độ click sai khi dòng có định dạng inline:** khối `rtf-block` là
  phần tử block-level nên `width` của nó bằng cả bề ngang khung soạn
  tin (không phải bề ngang thật của dòng chữ) — click vào "mép phải của
  khối" sẽ rơi ra ngoài rìa khung, trật mục tiêu. Phải tìm `<span
  data-text="true">` CUỐI CÙNG bên trong khối rồi click vào mép phải
  của chính span đó.
- **Tin dài khiến dòng cần thụt lề bị cuộn ra ngoài vùng nhìn thấy:** ô
  soạn tin có cuộn nội bộ; với tin nhiều dòng, dòng cần thụt lề (thường
  nằm giữa tin) có thể không nằm trong vùng đang hiển thị tại thời điểm
  xử lý, khiến toạ độ tính được tuy hợp lệ nhưng click không trúng nội
  dung mong muốn. Phải gọi `scrollIntoView({block:"center"})` trên dòng
  đó trước khi lấy toạ độ.

Số lần bấm mỗi cấp thụt lề **khác nhau giữa 2 loại danh sách** (đã đo
đạc qua thử nghiệm thực tế, xem `INDENT_CLICKS_PER_LEVEL`):
- **Bullet (`-`):** 1 lần bấm = 1 cấp thụt lề rõ ràng (margin đẩy hẳn
  cả dòng sang phải).
- **Đánh số (`1.`, `2.`...):** mỗi lần bấm chỉ cộng thêm
  `text-indent: 10px` (kèm dời vị trí số vào bên trong dòng), nhẹ hơn
  nhiều so với bullet, nên cần bấm **3 lần/cấp (~30px)** mới đủ rõ để
  phân biệt mục cha/con bằng mắt thường. Lưu ý: số thứ tự vẫn **tăng
  tuần tự liên tục** qua cả mục cha lẫn mục con (Zalo không hỗ trợ đánh
  số lại từ 1 cho từng cấp con như outline thật) — thụt lề chỉ tạo hiệu
  ứng thị giác phân cấp, không đổi cách đánh số.

Đã test thực tế nhiều cấp lồng nhau (không chỉ 1 cấp): bullet tới cấp 3
(`indent=3`, ví dụ `      - Mục` với 6 khoảng trắng) và đánh số tới cấp
2 — cả hai đều thụt lề đúng theo từng cấp, mỗi cấp lùi thêm rõ ràng so
với cấp trước.

## 8. Giới hạn đã biết

- Không hỗ trợ lồng định dạng (vd chữ vừa đậm vừa màu đỏ).
- **Màu chữ chỉ giữ được nếu đúng 1 trong 5 mã RGB swatch của Zalo**
  (`rgb(219,52,46)` đỏ, `rgb(242,120,6)` cam, `rgb(247,181,3)` vàng,
  `rgb(21,168,95)` xanh lá, `rgb(5,10,25)` đen). Màu tuỳ ý sẽ bị loại
  bỏ hoàn toàn, không hiển thị màu nào cả.
- Cỡ chữ khi paste chỉ có hiệu lực ở khoảng 3 mức thực tế: mặc định,
  18px, 20px (mọi giá trị lớn hơn đều bị kẹp về 20px) — chưa có cú
  pháp riêng trong markup cho việc này.
- Thụt lề tốn thêm thao tác (tìm dòng + click + bấm nút) cho mỗi dòng
  cần thụt lề, nên tin nhắn có nhiều dòng thụt lề sẽ chậm hơn tin
  không có — vẫn nhanh hơn nhiều so với gõ toàn bộ bằng bàn phím.
- **Lỗi xác nhận là hành vi phía Zalo (đã kiểm chứng, kể cả trên bản
  điện thoại):** nếu một danh sách gạch đầu dòng đứng NGAY TRƯỚC một
  danh sách đánh số trong cùng 1 tin nhắn, mục CUỐI CÙNG của danh sách
  đánh số có thể tự bị thụt lề sai dù markup không hề yêu cầu — đã xác
  minh HTML gửi đi và ô soạn tin trước khi gửi đều hoàn toàn phẳng/đúng,
  lỗi chỉ xuất hiện sau khi Zalo xử lý/hiển thị tin đã gửi. Cách né:
  tách bullet-list và numbered-list thành 2 tin nhắn riêng nếu thứ tự
  thụt lề chính xác là quan trọng.
- Đây là tự động hoá dựa trên giao diện web nội bộ (không chính thức)
  của Zalo — nếu Zalo đổi tên nút/tiêu đề (`title` attribute), mã màu
  swatch, hoặc cách xử lý sự kiện paste, engine cần cập nhật lại.
- Vẫn chỉ nên dùng cho tài khoản cá nhân, không gửi hàng loạt/spam.

## 9. Đã sửa — thông tin sai trước đây

Trước đó tài liệu này từng ghi nhầm rằng "danh sách đánh số có kèm định
dạng inline khác thì Zalo tự hiển thị lại 1. cho mọi mục" là **hành vi
phía Zalo**. Sau khi điều tra kỹ hơn, xác nhận đây thực ra là **lỗi
trong `build_html()`**: dữ liệu test ban đầu vô tình có dòng trống giữa
mỗi mục danh sách, khiến hàm gộp list hiểu nhầm thành nhiều danh sách
1-mục riêng lẻ (mỗi cái tự đánh số lại từ 1) thay vì 1 danh sách nhiều
mục. Đã sửa `build_html()` để dòng trống giữa các mục list cùng loại
không còn làm ngắt nhóm (xem mục 2 và mục 7) — số thứ tự giờ tăng đúng
trong mọi trường hợp đã test, kể cả khi từng mục có kèm định dạng
inline khác.
