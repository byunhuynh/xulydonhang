from playwright.sync_api import sync_playwright, Browser, BrowserContext, Page
import os
import re
import time as t
import json
STATE_FILE = "zalo_state.json"




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
# 1️⃣ HÀM LẤY GIÁ TRỊ TRONG <zalo>
# ==========================
def get_zalo_value(key, filename="settings.ini"):
    with open(filename, "r", encoding="utf-8") as f:
        content = f.read()

    # Tìm block <zalo>...</zalo>
    match = re.search(r"<zalo>(.*?)</zalo>", content, re.DOTALL)
    if not match:
        return None

    zalo_block = match.group(1)

    # Tìm dòng KEY = VALUE
    pattern = rf"^{key}\s*=\s*(.+)$"
    value_match = re.search(pattern, zalo_block, re.MULTILINE)

    if value_match:
        return value_match.group(1).strip()
    
    return None

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





def get_zalo_value_auto(ma_khach_hang, vendor, filename="settings.ini"):
    """
    Tạo key từ Mã Khách hàng[:2] + vendor, sau đó lấy giá trị trong <zalo>
    Ví dụ:
    ma_khach_hang = 'MB_MT_BIGC'
    vendor = 'BigC'
    key = 'MBBIGC'
    """
    # Chuẩn hoá vendor → bỏ khoảng trắng, viết hoa
    vendor_clean = vendor.replace(" ", "").upper()

    # Lấy 2 ký tự đầu của mã khách hàng
    prefix = ma_khach_hang[:2].upper()

    # Tạo key
    key = prefix + vendor_clean

    return get_zalo_value(key, filename)



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

def gui_tinnhan():
    groups = read_message_groups_with_raw("message.txt")
    if not groups:
        return []

    chrome_path = find_chrome()
    summary = []

    with sync_playwright() as p:

        browser, context, page = open_zalo(p, chrome_path)

        prev = None

        for g in groups:
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
            url = (data.get("url") or "").replace("/view?usp=drivesdk", "")
            khung_gio = data.get("khung_gio")

            zalo_key_value = get_zalo_value_auto(ma_kh, vendor)

            # ===== BUILD MESSAGE =====
            message = ""
            if vendor == 'JV-Mart':
                message = (
                    f"🔔 TIN NHẮN TỰ ĐỘNG\n"
                    f"🏬 Hệ thống: JupViec\n"
                    f"⏱️ Xử lý lúc: {start_time} (Thời gian: {time_})\n"
                    f"📍 Khu vực: {khu_vuc}\n"
                    f"📦 Tổng số đơn: {tong_don}\n"
                    f"📝 Danh sách đơn hàng:\n{text}"
                )

            elif vendor == 'JIT':
                message = (
                    f"🔔 TIN NHẮN TỰ ĐỘNG\n"
                    f"🏬 Hệ thống: TopValue - JIT\n"
                    f"⏱️ Xử lý lúc: {start_time} (Thời gian: {time_})\n"
                    f"📍 Khu vực: {khu_vuc}\n"
                    f"🌅 Buổi: {khung_gio}\n"
                    f"📦 Tổng số đơn: {tong_don}\n"
                )

            else:
                lines = ["🔔 TIN NHẮN TỰ ĐỘNG"]

                if po: lines.append(f"🎫 Đơn hàng: {po}")
                if store: lines.append(f"🏪 Store: {store}")
                if start_time: lines.append(f"⏱️ Xử lý lúc: {start_time}")
                if vendor: lines.append(f"🏬 Hệ thống: {vendor}")
                if url:
                    lines.append(f"🔗 Link đơn hàng: {url}")
                    lines.append("⏳ Link chỉ tồn tại trong 90 ngày")
                if tong_tien: lines.append(f"💰 Tổng tiền: {tong_tien}")
                if tong_kienhang: lines.append(f"📦 Tổng số kiện: {tong_kienhang}")
                if tong_trongluong: lines.append(f"⚖️ Tổng trọng lượng: {tong_trongluong}")

                message = "\n".join(lines)

                if int(sai_gia) > 0:
                    formatted_text = format_sai_gia_list(text)
                    message += (
                        f"\n\n❗ Số mã sai giá: {number_to_emoji(sai_gia)}\n"
                        f"📝 Danh sách mã sai giá:\n{formatted_text}"
                    )

            # ===== KHÔNG CÓ NHÓM =====
            if not zalo_key_value:
                summary.append({
                    "zalo": None,
                    "message": message,
                    "status": "no_group"
                })
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
                    summary.append({
                        "zalo": zalo_key_value,
                        "message": message,
                        "status": "wrong_chat"
                    })
                    continue

            # ===== GỬI TIN =====
            try:
                t.sleep(0.3)  # nghỉ 1s trước khi gửi
                send_message(page, message)
                remove_processed_block(raw)
                t.sleep(0.3)

                summary.append({
                    "zalo": zalo_key_value,
                    "message": message,
                    "status": "success"
                })

            except Exception as e:
                summary.append({
                    "zalo": zalo_key_value,
                    "message": message,
                    "status": f"error: {str(e)}"
                })

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

    