from playwright.sync_api import sync_playwright, Browser, BrowserContext, Page
import os
import re
import time as t
import json
from datetime import datetime, timedelta
from typing import Callable, Optional
STATE_FILE = "zalo_state.json"

# Công tắc tổng cho tính năng tự tạo nhắc hẹn giao hàng sau khi gửi tin.
# Bật/tắt cho TỪNG nhóm cụ thể thì cấu hình trong block <reminder> của settings.ini
# (cùng key MÃKH+VENDOR như block <zalo>), ví dụ: MNKINGFOOD = 1
ENABLE_REMINDER = True




CHROME_PATHS = [
    r"C:\Program Files\Google\Chrome\Application\chrome.exe",
    r"C:\Program Files (x86)\Google\Chrome\Application\chrome.exe",
]

def find_chrome():
    for path in CHROME_PATHS:
        if os.path.exists(path):
            return path
    raise RuntimeError("❌ Máy chưa cài Google Chrome")




NUMBER_EMOJI = {
    '0': '0️⃣', '1': '1️⃣', '2': '2️⃣', '3': '3️⃣', '4': '4️⃣',
    '5': '5️⃣', '6': '6️⃣', '7': '7️⃣', '8': '8️⃣', '9': '9️⃣'
}

def number_to_emoji(n):
    return ''.join(NUMBER_EMOJI.get(d, d) for d in str(n))


def format_sai_gia_list(text):
    lines = text.splitlines()
    new_lines = []

    for i, line in enumerate(lines, 1):
        # xoá số thứ tự cũ nếu có: 1. / 1)
        clean_line = re.sub(r'^\d+[\.\)]\s*', '', line)

        emoji_index = number_to_emoji(i)
        new_lines.append(f"{emoji_index} {clean_line}")

    return "\n".join(new_lines)


# ==========================
# 1️⃣ HÀM LẤY GIÁ TRỊ TRONG 1 BLOCK <section>...</section>
# ==========================
def get_ini_section_value(key, section, filename="settings.ini"):
    with open(filename, "r", encoding="utf-8") as f:
        content = f.read()

    # Tìm block <section>...</section>
    match = re.search(rf"<{section}>(.*?)</{section}>", content, re.DOTALL)
    if not match:
        return None

    section_block = match.group(1)

    # Tìm dòng KEY = VALUE
    pattern = rf"^{re.escape(key)}\s*=\s*(.+)$"
    value_match = re.search(pattern, section_block, re.MULTILINE)

    if value_match:
        return value_match.group(1).strip()

    return None


def get_zalo_value(key, filename="settings.ini"):
    return get_ini_section_value(key, "zalo", filename)

def read_message_groups_with_raw(filename="message.txt"):
    with open(filename, "r", encoding="utf-8") as f:
        content = f.read().strip()

    groups = []
    current_raw = []
    inside = False

    for line in content.splitlines():
        line_stripped = line.strip()

        # Bắt đầu 1 block
        if line_stripped.startswith("["):
            inside = True
            current_raw = [line_stripped]
            continue

        # Kết thúc block
        if inside and line_stripped.endswith("]"):
            current_raw.append(line_stripped)
            raw_text = "\n".join(current_raw)

            # ========== PARSE DICT =============
            group_dict = {}
            text_lines = []

            # Bỏ dấu [ và ]
            inner = raw_text[1:-1]

            for ln in inner.splitlines():
                if ":" not in ln:
                    continue

                key, value = ln.split(":", 1)
                key = key.strip()
                value = value.strip()

                # Gom nhiều dòng text:
                if key.lower() == "text":
                    text_lines.append(value)
                else:
                    group_dict[key] = value

            # 👉 ĐÁNH SỐ THỨ TỰ CHO CÁC DÒNG TEXT
            if text_lines:
                numbered_text = []
                for idx, line in enumerate(text_lines, start=1):
                    numbered_text.append(f"{idx}. {line}")
                group_dict["text"] = "\n".join(numbered_text)

            groups.append({"raw": raw_text, "data": group_dict})

            # Reset
            inside = False
            current_raw = []
            continue

        # Đang trong block → thêm dòng
        if inside:
            current_raw.append(line_stripped)

    return groups




def remove_processed_block(raw_block, filename="message.txt"):
    with open(filename, "r", encoding="utf-8") as f:
        content = f.read()

    # Xoá đúng đoạn raw block
    new_content = content.replace(raw_block, "").strip()

    with open(filename, "w", encoding="utf-8") as f:
        f.write(new_content)

    print("🧹 Đã xoá block đã xử lý!")


# ==========================
# 2️⃣ HÀM MỞ ZALO + LOAD COOKIE
# ==========================
def open_zalo(p, chrome_path, headless=True) -> tuple[Browser, BrowserContext, Page]:
    browser = p.chromium.launch(
        executable_path=chrome_path,
        headless=headless,
        args=["--disable-blink-features=AutomationControlled"]
    )

    context = None

    if os.path.exists(STATE_FILE):
        try:
            with open(STATE_FILE, "r", encoding="utf-8") as f:
                json.load(f)
        except (json.JSONDecodeError, ValueError):
            print("⚠️ File state JSON bị lỗi → xóa và login lại")
            os.remove(STATE_FILE)

    if os.path.exists(STATE_FILE):
        print("🔐 Có storage_state → thử login...")

        context = browser.new_context(storage_state=STATE_FILE)
        page = context.new_page()
        page.goto("https://chat.zalo.me/")

        try:
            # nếu thấy QR login → cookie hết hạn
            page.wait_for_selector("text=Đăng nhập qua mã QR", timeout=5000)

            print("⚠️ Cookie hết hạn → cần login lại")
            context.close()
            browser.close()

            # Mở lại với headless=False để user quét QR
            browser = p.chromium.launch(
                executable_path=chrome_path,
                headless=False,
                args=["--disable-blink-features=AutomationControlled"]
            )

        except:
            # không thấy QR → login OK
            page.wait_for_selector("#contact-search-input")
            print("✅ Login OK – không cần quét QR")
            return browser, context, page

    print("🔓 Chưa login. Vui lòng quét QR...")

    browser.close()
    browser = p.chromium.launch(
        executable_path=chrome_path,
        headless=False,
        args=["--disable-blink-features=AutomationControlled"]
    )

    context = browser.new_context()
    page = context.new_page()
    page.goto("https://id.zalo.me/account?continue=https://chat.zalo.me/")

    # đợi login thành công
    page.wait_for_selector("#contact-search-input")

    context.storage_state(path=STATE_FILE)
    print("💾 Đã lưu cookie → lần sau auto login")

    return browser, context, page


# ==========================
# KEEP-ALIVE: làm mới cookie
# ==========================
def refresh_zalo_session():
    """
    Mở Zalo nền, kiểm tra session còn sống không.
    Nếu còn → lưu lại cookie mới nhất rồi đóng.
    Nếu hết hạn → mở cửa sổ để user quét QR.
    """
    chrome_path = find_chrome()

    with sync_playwright() as p:
        browser, context, page = open_zalo(p, chrome_path, headless=True)

        # Lưu lại cookie mới nhất (Zalo có thể cấp token mới trong session)
        context.storage_state(path=STATE_FILE)
        print("🔄 Đã làm mới và lưu cookie.")

        page.close()
        context.close()
        browser.close()

# ==========================
# 3️⃣ HÀM TÌM KIẾM TRONG ZALO
# ==========================
def search_in_zalo(page, text):
    print(f"🔍 Đang tìm: {text}")

    # ---------------------------
    # 1) Inject React setter để nhập từ khóa vào ô tìm kiếm
    # ---------------------------
    js = f"""
        (() => {{
            const el = document.querySelector('#contact-search-input');
            if (!el) return;

            const prototype = Object.getPrototypeOf(el);
            const descriptor = Object.getOwnPropertyDescriptor(prototype, 'value');
            const reactSetter = descriptor.set;

            // Gọi React setter → UI update
            reactSetter.call(el, "{text}");

            el.dispatchEvent(new Event('input', {{ bubbles: true }}));
        }})();
    """

    page.evaluate(js)
    print("🎉 Đã gõ vào ô tìm kiếm đúng cách (React render).")

    # ---------------------------
    # 2) Chờ danh sách kết quả update
    # ---------------------------
    page.wait_for_timeout(300)  # thời gian nhỏ để Zalo render

    # ---------------------------
    # 3) Click kết quả đầu tiên (XPath từ Selenium của bạn)
    # ---------------------------
    xpath_first_result = '//*[@id="searchResultList"]/div/div[1]/div/div[1]/div/div/div[2]'

    try:
        first_item = page.locator(f'xpath={xpath_first_result}')
        first_item.wait_for(timeout=5000)
        first_item.click()
        print("🎯 Đã click vào kết quả đầu tiên!")
    except Exception as e:
        print("❌ Không thể click kết quả đầu tiên:", e)


def send_message(page: Page, tinnhan: str):
    if not tinnhan:
        print("⚠️ Không có nội dung → bỏ qua gửi.")
        return

    print("✉️ Đang gửi tin nhắn bằng phương pháp shadow-paste...")

    # 1) Tạo textarea ẩn & gán nội dung
    page.evaluate(f"""
        () => {{
            let ta = document.createElement('textarea');
            ta.id = 'shadow_clipboard';
            ta.style.position = 'fixed';
            ta.style.opacity = '0';
            ta.value = `{tinnhan}`;
            document.body.appendChild(ta);
        }}
    """)

    # 2) Copy nội dung từ textarea
    page.locator("#shadow_clipboard").click()
    page.keyboard.down("Control")
    page.keyboard.press("A")
    page.keyboard.press("C")
    page.keyboard.up("Control")

    # 3) Xoá textarea cho sạch
    page.evaluate("""
        () => {
            let ta = document.getElementById('shadow_clipboard');
            if (ta) ta.remove();
        }
    """)

    # 4) Click vào khung Zalo richInput
    input_box = page.locator('xpath=//*[@id="richInput"]')
    input_box.wait_for(timeout=5000)
    input_box.click()

    # 5) Paste vào Zalo
    page.keyboard.down("Control")
    page.keyboard.press("V")
    page.keyboard.up("Control")

    # 6) Đếm số tin nhắn hiện có trước khi gửi
    msg_info = page.evaluate("""
        () => {
            const selectors = ['[class*="message-item"]', '[class*="msg-item"]', '[class*="chat-msg"]'];
            for (const sel of selectors) {
                const items = document.querySelectorAll(sel);
                if (items.length > 0) return {count: items.length, sel: sel};
            }
            return {count: -1, sel: null};
        }
    """)

    # 7) Nhấn nút gửi
    send_btn = page.locator('xpath=//*[@id="chat-input-container-id"]/div[2]/div[2]/div[2]/i')
    send_btn.wait_for(timeout=5000)
    send_btn.click()

    # Bước 1: Chờ richInput trống → Zalo đã nhận lệnh gửi phía UI
    try:
        page.wait_for_function(
            "() => { const el = document.querySelector('#richInput'); return el && el.innerText.trim() === ''; }",
            timeout=5000
        )
    except Exception:
        page.wait_for_timeout(1000)

    # Bước 2: Chờ tin mới xuất hiện trong chat → server đã xác nhận qua WebSocket
    count = msg_info.get("count", -1)
    sel = msg_info.get("sel")
    if count > 0 and sel:
        try:
            page.wait_for_function(
                f"() => document.querySelectorAll('{sel}').length > {count}",
                timeout=10000
            )
        except Exception:
            page.wait_for_timeout(3000)
    else:
        page.wait_for_timeout(3000)

    print("✅ Đã gửi tin nhắn — KHÔNG LỖI CHỮ, KHÔNG MẤT KÝ TỰ.")





def make_zalo_key(ma_khach_hang, vendor):
    """
    Tạo key từ Mã Khách hàng[:2] + vendor.
    Ví dụ:
    ma_khach_hang = 'MB_MT_BIGC'
    vendor = 'BigC'
    key = 'MBBIGC'
    """
    # Chuẩn hoá vendor → bỏ khoảng trắng, viết hoa
    vendor_clean = vendor.replace(" ", "").upper()

    # Lấy 2 ký tự đầu của mã khách hàng
    prefix = ma_khach_hang[:2].upper()

    return prefix + vendor_clean


def get_zalo_value_auto(ma_khach_hang, vendor, filename="settings.ini"):
    """Tạo key từ Mã Khách hàng[:2] + vendor, sau đó lấy giá trị trong <zalo>"""
    key = make_zalo_key(ma_khach_hang, vendor)
    return get_zalo_value(key, filename)


def is_reminder_enabled(ma_khach_hang, vendor, filename="settings.ini"):
    """
    Kiểm tra xem nhóm này có được bật tính năng tạo nhắc hẹn hay không, dựa vào
    block <reminder> trong settings.ini (dùng cùng key MÃKH+VENDOR như <zalo>).
    Chỉ những key có mặt trong <reminder> và giá trị là 1/true/yes mới được bật.
    """
    key = make_zalo_key(ma_khach_hang, vendor)
    value = get_ini_section_value(key, "reminder", filename)

    if value is None:
        return False

    return value.strip().lower() in ("1", "true", "yes", "on", "bật", "bat")



def is_correct_chat(page, expected_name, timeout=5000):

    try:
        title = page.locator(
            '//*[@id="header"]/div[1]/div[2]/div[1]/div/div-b18'
        )

        title.wait_for(timeout=timeout)

        current_name = " ".join(title.inner_text().split())
        expected_name = " ".join(expected_name.split())

        print("👉 Current chat:", current_name)

        return expected_name.lower() in current_name.lower()

    except:
        return False

# ==========================
# 3.5️⃣ TẠO NHẮC HẸN GIAO HÀNG (mặc định TẮT, bật bằng ENABLE_REMINDER)
# ==========================
REMINDER_LOG_FILE = "reminder_debug.log"


def _log_reminder(msg: str):
    """
    Ghi log ra cả console lẫn file reminder_debug.log.
    Cần thiết vì khi chạy headless (hoặc chạy trong app không có console, ví dụ
    App.py mở như GUI), print() không hiển thị ở đâu cả → không có cách nào biết
    bước nào bị lỗi. Ghi ra file để luôn xem lại được, kể cả khi không thấy trình duyệt.
    """
    line = f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] {msg}"
    print(line)
    try:
        with open(REMINDER_LOG_FILE, "a", encoding="utf-8") as f:
            f.write(line + "\n")
    except Exception:
        pass


def _paste_into_contenteditable(page: Page, locator, text: str):
    """Dán text vào 1 ô contenteditable bất kỳ bằng kỹ thuật shadow-clipboard (giống send_message)."""
    page.evaluate(f"""
        () => {{
            let ta = document.createElement('textarea');
            ta.id = 'shadow_clipboard_reminder';
            ta.style.position = 'fixed';
            ta.style.opacity = '0';
            ta.value = `{text}`;
            document.body.appendChild(ta);
        }}
    """)
    page.locator("#shadow_clipboard_reminder").click()
    page.keyboard.down("Control")
    page.keyboard.press("A")
    page.keyboard.press("C")
    page.keyboard.up("Control")
    page.evaluate("""
        () => {
            let ta = document.getElementById('shadow_clipboard_reminder');
            if (ta) ta.remove();
        }
    """)

    locator.click()
    page.keyboard.down("Control")
    page.keyboard.press("V")
    page.keyboard.up("Control")


def add_line(lines: list[str], label: str, value: Optional[str]):
    """Thêm '{label}: {value}' vào lines, bỏ qua nếu value rỗng/None."""
    if value:
        lines.append(f"{label}: {value}")


def build_reminder_content(data: dict[str, Optional[str]]) -> str:
    """Nội dung nhắc hẹn giao hàng, dựa theo thông tin đơn hàng đã có trong tin nhắn đã gửi."""
    lines = ["🔔 NHẮC HẸN GIAO HÀNG"]
    add_line(lines, "🎫 Đơn hàng", data.get("PO"))
    add_line(lines, "🏬 Hệ thống", data.get("vendor"))
    add_line(lines, "🏪 Store", data.get("store"))
    add_line(lines, "🗓️ Ngày đặt hàng", data.get("entry_date"))
    add_line(lines, "⏳ Hạn giao hàng", data.get("cancle_date"))
    return "\n".join(lines)


def create_delivery_reminder(page: Page, data: dict[str, Optional[str]]):
    """
    Tạo 1 nhắc hẹn Zalo ngay trong nhóm vừa gửi tin (không chuyển nhóm khác):
    - Nội dung: thông tin đơn hàng (PO/store/vendor/entry_date/cancle_date)
    - Thời gian: 9h sáng, trước cancle_date 1 ngày

    Luồng thao tác lấy từ html/Zalo - Thành.html, html/Zalo - Thành - step 3.html
    và html/Zalo - Thành - calender.html (lúc khung lịch đang mở):
    menu "..." -> Tạo nhắc hẹn -> nhập nội dung -> chọn "Khác" -> bấm ô ngày để mở
    khung lịch "#calendar-v3" (popup này gắn ngoài .zl-modal, ngay dưới <body>) ->
    gõ thẳng ngày/giờ vào 2 ô input trong khung lịch (data-id="txt_RMD_Date" và
    "txt_RMD_Time", định dạng DD/MM/YYYY và hh:mm) thay vì click từng ô ngày.
    """
    cancle_date = data.get("cancle_date")
    if not cancle_date:
        _log_reminder("⚠️ Không có cancle_date → bỏ qua tạo nhắc hẹn.")
        return

    try:
        cancle_dt = datetime.strptime(cancle_date.strip(), "%d/%m/%Y")
        # Ngày giao là thứ 2 → nhắc trước 2 ngày (thứ 7) thay vì 1 ngày (rơi vào CN)
        days_before = 2 if cancle_dt.weekday() == 0 else 1
        target_dt = (cancle_dt - timedelta(days=days_before)).replace(hour=9, minute=0)
    except ValueError:
        _log_reminder(f"⚠️ cancle_date không đúng định dạng dd/mm/yyyy: {cancle_date} → bỏ qua tạo nhắc hẹn.")
        return

    # Thời gian nhắc hẹn đã qua (Zalo không cho chọn ngày quá khứ) → bỏ qua luôn,
    # tránh script bị kẹt khi cố gõ 1 ngày mà lịch không chấp nhận.
    if target_dt <= datetime.now():
        _log_reminder(f"⏭️ Thời gian nhắc hẹn ({target_dt.strftime('%H:%M %d/%m/%Y')}) đã ở quá khứ → bỏ qua tạo nhắc hẹn.")
        return

    content = build_reminder_content(data)

    try:
        # 1) Mở menu "Tùy chọn thêm" trong khung soạn tin (data-id="div_More_Menu")
        more_btn = page.locator('[data-id="div_More_Menu"]')
        more_btn.wait_for(timeout=5000)
        more_btn.click()
        page.wait_for_timeout(300)

        # 2) Chọn "Tạo nhắc hẹn" trong menu vừa mở
        reminder_item = page.get_by_text("Tạo nhắc hẹn", exact=True)
        reminder_item.wait_for(timeout=5000)
        reminder_item.click()

        # 3) Chờ modal "Tạo nhắc hẹn" mở (class "zl-modal")
        modal = page.locator(".zl-modal").last
        modal.wait_for(timeout=5000)
        page.wait_for_timeout(300)

        # 4) Nhập nội dung nhắc hẹn
        content_box = modal.locator(".rich-input").first
        content_box.wait_for(timeout=5000)
        _paste_into_contenteditable(page, content_box, content)

        # 5) Chọn chip "Khác" để tự chọn ngày giờ cụ thể (thay vì 15p/30p/9h ngày mai)
        other_chip = modal.get_by_text("Khác", exact=True)
        other_chip.wait_for(timeout=5000)
        other_chip.click()
        page.wait_for_timeout(300)

        # 6) Bấm ô ngày trong modal (data-id="txt_RMD_Date") để mở khung lịch "#calendar-v3"
        date_field = modal.locator('[data-id="txt_RMD_Date"]')
        date_field.wait_for(timeout=5000)
        date_field.click()

        calendar = page.locator("#calendar-v3")
        calendar.wait_for(timeout=5000)
        page.wait_for_timeout(300)

        # 7) Gõ ngày/giờ vào 2 ô input trong khung lịch bằng phím thật (giống người
        # dùng gõ tay) thay vì .fill() — input này có parser theo từng ký tự
        # (mask DD/MM/YYYY, hh:mm) nên .fill() có thể không kích hoạt đúng, nhất
        # là khi chạy headless.
        date_str = target_dt.strftime("%d/%m/%Y")
        time_str = target_dt.strftime("%H:%M")

        date_input = calendar.locator('input[data-id="txt_RMD_Date"]')
        date_input.click()
        date_input.press("Control+A")
        page.keyboard.type(date_str, delay=50)
        page.keyboard.press("Enter")
        page.wait_for_timeout(200)

        time_input = calendar.locator('input[data-id="txt_RMD_Time"]')
        time_input.click()
        time_input.press("Control+A")
        page.keyboard.type(time_str, delay=50)
        page.keyboard.press("Enter")
        page.wait_for_timeout(200)

        # Đối chiếu lại giá trị đã gõ vào ô input có đúng không
        date_ok = time_ok = None
        try:
            date_ok = date_input.input_value() == date_str
            time_ok = time_input.input_value() == time_str
        except Exception:
            pass
        _log_reminder(f"📝 Đã gõ ngày='{date_str}' (khớp={date_ok}) giờ='{time_str}' (khớp={time_ok})")

        # Đóng khung lịch: KHÔNG dùng phím Escape — đã test thực tế thấy Escape kích
        # hoạt luôn hộp thoại "Xác nhận — Bạn muốn huỷ nhắc hẹn đang được tạo?" của
        # modal cha (Escape bị modal bắt trước, không chỉ đóng riêng khung lịch).
        # Cũng không click vào ô nội dung vì khung lịch che ngay phía trên nó.
        # → Click ra ngoài vào dòng "Chọn kiểu lặp lại" (nằm dưới khung lịch, ảnh
        # chụp thực tế cho thấy không bị che).
        try:
            modal.locator('[data-id="div_RMD_Repeat"]').click(timeout=3000)
        except Exception as e:
            _log_reminder(f"⚠️ Không click được ra ngoài để đóng khung lịch (bỏ qua, thử bấm nút tạo luôn): {e}")
        try:
            calendar.wait_for(state="hidden", timeout=2000)
        except Exception:
            pass
        page.wait_for_timeout(300)

        # Kiểm tra lại preview ngày giờ hiển thị trên modal có khớp không
        try:
            preview_text = " ".join(date_field.inner_text().split())
            _log_reminder(f"🗓️ Xem trước ngày giờ nhắc hẹn trên modal: {preview_text}")
        except Exception:
            pass

        # 8) Bấm "Tạo nhắc hẹn" (data-id="btn_RMD_CXL") sau khi nút được kích hoạt
        create_btn = modal.locator('[data-id="btn_RMD_CXL"]')
        create_btn.wait_for(timeout=5000)
        try:
            page.wait_for_function(
                "(el) => el.getAttribute('data-disabled') !== 'disabled'",
                arg=create_btn.element_handle(),
                timeout=5000
            )
        except Exception:
            disabled_now = create_btn.get_attribute("data-disabled")
            _log_reminder(f"⚠️ Nút 'Tạo nhắc hẹn' vẫn có data-disabled='{disabled_now}' sau khi chờ — có thể ngày/giờ chưa được Zalo chấp nhận.")

        create_btn.click()
        _log_reminder(f"⏰ Đã bấm tạo nhắc hẹn giao hàng lúc {target_dt.strftime('%H:%M %d/%m/%Y')}")

    except Exception as e:
        _log_reminder(f"❌ Lỗi khi tạo nhắc hẹn: {e}")
        try:
            screenshot_path = f"reminder_error_{datetime.now().strftime('%Y%m%d_%H%M%S')}.png"
            page.screenshot(path=screenshot_path)
            _log_reminder(f"📸 Đã lưu ảnh chụp màn hình lúc lỗi: {screenshot_path}")
        except Exception:
            pass
        raise


def gui_tinnhan(
    on_progress: Optional[Callable[[dict], None]] = None,
    on_status: Optional[Callable[[int, int], None]] = None,
):
    """
    on_progress: callback tuỳ chọn, được gọi ngay sau khi xử lý xong MỖI nhóm tin
    (thay vì đợi xử lý hết rồi mới trả về), để nơi gọi (ví dụ GUI) có thể cập nhật
    nhật ký ngay lập tức. Nhận vào 1 dict giống các phần tử trong summary trả về.

    on_status: callback tuỳ chọn, báo tiến độ (current, total) để nơi gọi hiển thị
    "đang gửi tin thứ mấy / tổng cộng bao nhiêu tin". Được gọi 1 lần với
    (0, total) trước khi bắt đầu gửi, rồi 1 lần nữa với (current, total) trước
    khi gửi từng tin (current bắt đầu từ 1).
    """
    groups = read_message_groups_with_raw("message.txt")
    if not groups:
        if on_status:
            on_status(0, 0)
        return []

    chrome_path = find_chrome()
    summary = []
    total = len(groups)

    if on_status:
        on_status(0, total)

    with sync_playwright() as p:

        browser, context, page = open_zalo(p, chrome_path)

        prev = None

        for idx, g in enumerate(groups, start=1):
            if on_status:
                on_status(idx, total)

            raw = g["raw"]
            data = g["data"]

            po = data.get("PO")
            vendor = data.get("vendor")
            ma_kh = data.get("Mã Khách hàng")
            start_time = data.get("start_time")
            text = data.get("text")
            tong_tien = data.get("tong_tien")
            tong_trongluong = data.get("tong_trongluong")
            tong_kienhang = data.get("tong_kienhang")
            sai_gia = data.get("sai_gia") or "0"
            time_ = data.get("time")
            store = data.get("store")
            tong_don = data.get("tong_don")
            khu_vuc = data.get("khu_vuc")
            url = f"https://bluedonhang.pages.dev/?po={po}" if po else ""
            khung_gio = data.get("khung_gio")
            entry_date = data.get("entry_date")
            cancle_date = data.get("cancle_date")

            zalo_key_value = get_zalo_value_auto(ma_kh, vendor)

            # ===== BUILD MESSAGE =====
            if vendor == 'JV-Mart':
                lines = ["🔔 TIN NHẮN TỰ ĐỘNG"]
                add_line(lines, "🏬 Hệ thống", "JupViec")
                add_line(lines, "⏱️ Xử lý lúc", f"{start_time} (Thời gian: {time_})")
                add_line(lines, "📍 Khu vực", khu_vuc)
                add_line(lines, "📦 Tổng số đơn", tong_don)
                add_line(lines, "🗓️ Ngày đặt hàng", entry_date)
                add_line(lines, "⏳ Hạn giao hàng", cancle_date)
                lines.append(f"📝 Danh sách đơn hàng:\n{text}")
                message = "\n".join(lines)

            elif vendor == 'JIT':
                lines = ["🔔 TIN NHẮN TỰ ĐỘNG"]
                add_line(lines, "🏬 Hệ thống", "TopValue - JIT")
                add_line(lines, "⏱️ Xử lý lúc", f"{start_time} (Thời gian: {time_})")
                add_line(lines, "📍 Khu vực", khu_vuc)
                add_line(lines, "🌅 Buổi", khung_gio)
                add_line(lines, "📦 Tổng số đơn", tong_don)
                add_line(lines, "🗓️ Ngày đặt hàng", entry_date)
                add_line(lines, "⏳ Hạn giao hàng", cancle_date)
                add_line(lines, "💰 Tổng tiền", tong_tien)
                message = "\n".join(lines)

            else:
                lines = ["🔔 TIN NHẮN TỰ ĐỘNG"]
                add_line(lines, "🎫 Đơn hàng", po)
                add_line(lines, "🏪 Store", store)
                add_line(lines, "⏱️ Xử lý lúc", start_time)
                add_line(lines, "🏬 Hệ thống", vendor)
                add_line(lines, "🗓️ Ngày đặt hàng", entry_date)
                add_line(lines, "⏳ Hạn giao hàng", cancle_date)
                add_line(lines, "🔗 Link đơn hàng", url)
                add_line(lines, "💰 Tổng tiền", tong_tien)
                add_line(lines, "📦 Tổng số kiện", tong_kienhang)
                add_line(lines, "⚖️ Tổng trọng lượng", tong_trongluong)
                message = "\n".join(lines)

                if int(sai_gia) > 0:
                    formatted_text = format_sai_gia_list(text)
                    message += (
                        f"\n\n❗ Số mã sai giá: {number_to_emoji(sai_gia)}\n"
                        f"📝 Danh sách mã sai giá:\n{formatted_text}"
                    )

            # ===== KHÔNG CÓ NHÓM =====
            if not zalo_key_value:
                item = {
                    "zalo": None,
                    "message": message,
                    "status": "no_group"
                }
                summary.append(item)
                if on_progress:
                    on_progress(item)
                continue

            # ===== VÀO ĐÚNG CHAT =====
            if prev != zalo_key_value and not is_correct_chat(page, zalo_key_value):

                for _ in range(3):
                    search_in_zalo(page, zalo_key_value)
                    page.wait_for_timeout(1200)

                    if is_correct_chat(page, zalo_key_value):
                        prev = zalo_key_value
                        break
                else:
                    item = {
                        "zalo": zalo_key_value,
                        "message": message,
                        "status": "wrong_chat"
                    }
                    summary.append(item)
                    if on_progress:
                        on_progress(item)
                    continue

            # ===== GỬI TIN =====
            try:
                t.sleep(0.3)  # nghỉ 1s trước khi gửi
                send_message(page, message)

                if ENABLE_REMINDER and is_reminder_enabled(ma_kh, vendor):
                    try:
                        create_delivery_reminder(page, data)
                    except Exception as e:
                        _log_reminder(f"⚠️ Bỏ qua nhắc hẹn do lỗi: {e}")

                remove_processed_block(raw)
                t.sleep(0.3)

                item = {
                    "zalo": zalo_key_value,
                    "message": message,
                    "status": "success"
                }
                summary.append(item)
                if on_progress:
                    on_progress(item)

            except Exception as e:
                item = {
                    "zalo": zalo_key_value,
                    "message": message,
                    "status": f"error: {str(e)}"
                }
                summary.append(item)
                if on_progress:
                    on_progress(item)

            prev = zalo_key_value
        # Lưu lại cookie mới nhất sau mỗi phiên gửi
        try:
            context.storage_state(path=STATE_FILE)
            print("💾 Đã cập nhật cookie sau phiên gửi.")
        except Exception as e:
            print(f"⚠️ Không lưu được cookie: {e}")

        t.sleep(2)
        page.close()
        context.close()
        browser.close()

    return summary


# ==========================
# 4️⃣ MAIN
# ==========================
if __name__ == "__main__":
    print('hello')
    #page = open_zalo(False)
    #search_in_zalo(page, 'PO')

    #t.sleep(30)

    #page.close()

    gui_tinnhan()

    