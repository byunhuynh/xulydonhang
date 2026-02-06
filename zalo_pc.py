# zalo_controller.py
from pywinauto import Desktop
from pywinauto.keyboard import send_keys
import pyperclip, time

def send_zalo_message(
    keyword: str,
    message: str,
    title: str = "Zalo",
    class_name: str = "Chrome_WidgetWin_1",
    search_auto_id: str = "contact-search-input",
    wait: float = 0.3
) -> bool:
    """
    Gửi tin nhắn qua Zalo PC (Windows) bằng UI Automation.
    - keyword: tên người hoặc nhóm để mở hội thoại
    - message: nội dung tin nhắn cần gửi
    - title, class_name: tiêu đề & class cửa sổ Zalo (xem Inspect)
    - search_auto_id: AutomationId ô tìm kiếm (từ Inspect)
    - wait: độ trễ giữa các thao tác (giây)
    Trả về True nếu gửi thành công, False nếu lỗi.
    """

    try:
        # 🔍 Tìm cửa sổ Zalo
        win = Desktop(backend="uia").window(title=title, class_name=class_name)
        win.wait("exists", timeout=10)

        # 🔁 Restore nếu đang thu nhỏ
        try:
            win.restore()
        except Exception:
            pass
        win.set_focus()

        # 🎯 Tìm ô tìm kiếm
        search = win.child_window(auto_id=search_auto_id, control_type="Edit")
        search.wait("exists enabled visible", timeout=5)
        try:
            search.click_input()
        except Exception:
            search.set_focus()

        # 🔎 Nhập từ khóa tìm hội thoại
        pyperclip.copy(keyword)
        send_keys("^v")
        time.sleep(wait)
        send_keys("{ENTER}")
        time.sleep(wait)

        # 💬 Gửi nội dung tin nhắn
        pyperclip.copy(message)
        send_keys("^v")
        time.sleep(wait)
        send_keys("{ENTER}")

        print(f"✅ Đã gửi tin nhắn tới [{keyword}]")
        return True

    except Exception as e:
        print(f"❌ Gửi tin thất bại: {e}")
        return False


# --- Ví dụ sử dụng ---
if __name__ == "__main__":
    send_zalo_message(
        keyword="Cloud của tôi",
        message="⚠️ Đơn hàng có sai giá, vui lòng kiểm tra lại nhé!"
    )
