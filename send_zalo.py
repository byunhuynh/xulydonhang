from playwright.sync_api import sync_playwright
import os
import re
import time as t
STATE_FILE = "zalo_state.json"


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
def open_zalo(headless = True):
    playwright = sync_playwright().start()
    browser = playwright.chromium.launch(headless=headless)

    # Nếu có sẵn cookie
    if os.path.exists(STATE_FILE):
        print("🔐 Đã có trạng thái đăng nhập → mở Zalo...")
        context = browser.new_context(storage_state=STATE_FILE)
        page = context.new_page()
        page.goto("https://chat.zalo.me/")
        page.wait_for_selector("#contact-search-input")
        print("✅ Login OK – không cần quét QR")
        return page

    # Nếu chưa có cookie → yêu cầu login lần đầu
    print("🔓 Chưa có login. Vui lòng quét QR...")
    context = browser.new_context()
    page = context.new_page()
    page.goto("https://id.zalo.me/account?continue=https%3A%2F%2Fchat.zalo.me%2F")

    input("⏳ Đăng nhập xong, nhấn ENTER để lưu trạng thái... ")

    context.storage_state(path=STATE_FILE)
    print("💾 Đã lưu cookie → tự động login lần sau.")

    return page


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



def send_message(page, tinnhan):
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

    # 6) Nhấn nút gửi
    send_btn = page.locator('xpath=//*[@id="chat-input-container-id"]/div[2]/div[2]/div[2]/i')
    send_btn.wait_for(timeout=5000)
    send_btn.click()

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




def gui_tinnhan():
    # 👉 Mở Zalo
    groups = read_message_groups_with_raw("message.txt")
    print(groups)
    if not groups:
        return

    
    
    page = open_zalo()
    prev = None
    

    for g in groups:
        raw = g["raw"]
        data = g["data"]


        po              = data.get("PO")
        vendor          = data.get("vendor")
        ma_kh           = data.get("Mã Khách hàng")
        start_time      = data.get("start_time")
        text            = data.get("text")              # ĐÃ GỘP TẤT CẢ TEXT
        tong_tien       = data.get("tong_tien")
        tong_trongluong = data.get("tong_trongluong")
        tong_kienhang = data.get("tong_kienhang")
        sai_gia         = data.get("sai_gia")
        end_time        = data.get("end_time")
        time            = data.get("time")
        store           = data.get("store")
        tong_don        = data.get("tong_don")
        khu_vuc         = data.get("khu_vuc")
        url         = data.get("url")
        khung_gio   = data.get("khung_gio") 


        print("PO:", po)
        print("Vendor:", vendor)
        print("Mã KH:", ma_kh)
        print("Start:", start_time)
        print("Text:\n", text)
        print("Tổng tiền:", tong_tien)
        print("Tổng trọng lượng:", tong_trongluong)
        print("Sai giá:", sai_gia)
        print("End:", end_time)
        print("Time:", time)
        print("url:", url)
        print("khung_gio:", khung_gio)
        print("-----")

        zalo_key_value = get_zalo_value_auto(ma_kh, vendor)
        print(f"zalo key: {zalo_key_value}")
        

        if not zalo_key_value:
            print("⚠️ Không tìm thấy group Zalo → bỏ qua nhóm này")
            continue
        

        if vendor == 'JV-Mart':
            
            message = (
        f"🔔 TIN NHẮN TỰ ĐỘNG\n"
        f"🏬 Hệ thống: JupViec\n"
        f"⏱️ Xử lý lúc: {start_time} (Thời gian: {time})\n"
        f"📍 Khu vực: {khu_vuc}\n"
        f"📦 Tổng số đơn: {tong_don}\n"
        f"📝 Danh sách đơn hàng:\n{text}"
    )


        elif vendor == 'JIT':
            message = (
        f"🔔 TIN NHẮN TỰ ĐỘNG\n"
        f"🏬 Hệ thống: TopValue - JIT\n"
        f"⏱️ Xử lý lúc: {start_time} (Thời gian: {time})\n"
        f"📍 Khu vực: {khu_vuc}\n"
        f"🌅 Buổi: {khung_gio}\n"
        f"📦 Tổng số đơn: {tong_don}\n"
    )


    

        else:
            lines = ["🔔 TIN NHẮN TỰ ĐỘNG"]

            if po:
                lines.append(f"🎫 Đơn hàng: {po}")
            if store:
                lines.append(f"🏪 Store: {store}")
            if start_time:
                lines.append(f"⏱️ Xử lý lúc: {start_time}" + (f" (Thời gian: {time})" if time else ""))
            if vendor:
                lines.append(f"🏬 Hệ thống: {vendor}")
            if url:
                lines.append(f"🔗‍️ Link đơn hàng: {url}")
                lines.append("⏳ Link chỉ tồn tại trong 90 ngày")
            if tong_tien:
                lines.append(f"💰 Tổng tiền: {tong_tien}")
            if tong_kienhang:
                lines.append(f"📦 Tổng số kiện: {tong_kienhang} kiện")
            if tong_trongluong:
                lines.append(f"⚖️ Tổng trọng lượng: {tong_trongluong}")

            message = "\n".join(lines)

            



            if int(sai_gia) > 0:
                message += (
                    f"\n\n❗ Số mã sai giá: {sai_gia}\n"
                    f"📝 Danh sách mã sai giá:\n{text}"
                )


        if prev != zalo_key_value:
            search_in_zalo(page, zalo_key_value)
            t.sleep(1)
            
        print(message)
        send_message(page, message)

        remove_processed_block(raw, "message.txt")
        prev = zalo_key_value






    t.sleep(3)
    page.close()


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

    