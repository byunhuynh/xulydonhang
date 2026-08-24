"""
Tu dong gui tin nhan tren chat.zalo.me bang Playwright (dieu khien UI that,
khong goi thang API noi bo vi API do dung payload ma hoa rieng cua Zalo).

Cach dung:
    python send_message.py "Ten lien he hoac ten nhom" "Noi dung tin nhan"

    python send_message.py "Ten lien he" "**Dam** *nghieng* {red:mau do}" --rich

Tuy chon:
    --headless        Chay an (khong hien cua so trinh duyet)
    --rich            Dien giai noi dung theo cu phap dinh dang trong
                       RICH_TEXT_SYNTAX.md (dam/nghieng/gach chan/gach
                       ngang/mau chu/danh sach) truoc khi gui, bang cach
                       dan (paste) 1 lan - xem rich_paste_engine.py.
                       Khong ho tro thut le (list long nhau).

Lan dau chay se can dang nhap bang QR code (session duoc luu lai o
./zalo_profile nen cac lan sau tu dong dang nhap).

LUU Y: Chi nen dung voi tai khoan cua chinh ban, cho muc dich ca nhan.
Khong dung de spam / gui hang loat vi co the vi pham dieu khoan Zalo va
bi khoa tai khoan.
"""

import argparse
import asyncio
from pathlib import Path

from playwright.async_api import async_playwright, TimeoutError as PlaywrightTimeoutError

from rich_paste_engine import send_pasted_message

BASE_DIR = Path(__file__).parent
USER_DATA_DIR = BASE_DIR / "zalo_profile"


async def open_conversation(page, contact_query: str) -> bool:
    search_box = page.locator("#contact-search-input")
    await search_box.click()
    await search_box.fill("")
    await page.keyboard.type(contact_query, delay=40)
    await page.wait_for_timeout(1500)

    # Tim ket qua dau tien trong danh sach ket qua tim kiem (doan text duoc highlight)
    rect = await page.evaluate(
        """
        () => {
            const el = document.querySelector('.txt-highlight');
            if (!el) return null;
            const r = el.getBoundingClientRect();
            return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
        }
        """
    )
    if rect is None:
        print(f'Khong tim thay ket qua nao cho "{contact_query}".')
        return False

    await page.mouse.click(rect["x"], rect["y"])
    await page.wait_for_timeout(1500)

    # Xac nhan da mo dung hoi thoai: placeholder cua o nhap se chua ten lien he
    rich_input = page.locator("#richInput")
    try:
        await rich_input.wait_for(state="visible", timeout=8000)
    except PlaywrightTimeoutError:
        print("Khong thay o nhap tin nhan sau khi chon ket qua tim kiem.")
        return False

    return True


async def send_message(page, message_text: str) -> bool:
    rich_input = page.locator("#richInput")
    await rich_input.click()

    lines = message_text.split("\n")
    for i, line in enumerate(lines):
        if i > 0:
            await page.keyboard.press("Shift+Enter")
        await page.keyboard.type(line, delay=20)

    try:
        async with page.expect_response(
            lambda r: "message/sms" in r.url, timeout=10000
        ) as resp_info:
            await page.keyboard.press("Enter")
        resp = await resp_info.value
        body = await resp.text()
        ok = resp.status == 200 and '"error_code":0' in body
        print("Gui tin: ", "THANH CONG" if ok else "THAT BAI")
        print("  Response:", body[:300])
        return ok
    except PlaywrightTimeoutError:
        print("Khong nhan duoc phan hoi tu server sau khi gui (co the van gui duoc, kiem tra lai giao dien).")
        return False


async def main(contact_query: str, message_text: str, headless: bool, rich: bool = False) -> None:
    USER_DATA_DIR.mkdir(exist_ok=True)
    async with async_playwright() as p:
        context = await p.chromium.launch_persistent_context(
            user_data_dir=str(USER_DATA_DIR),
            headless=headless,
            viewport={"width": 1280, "height": 900},
        )
        page = await context.new_page()
        await page.goto("https://chat.zalo.me/", wait_until="domcontentloaded", timeout=30000)
        await page.wait_for_timeout(4000)

        if "id.zalo.me" in page.url:
            print("Chua dang nhap. Vui long quet QR code trong cua so trinh duyet (chay lai voi --headless=False).")
            print("Dang cho dang nhap (toi da 120s)...")
            try:
                await page.wait_for_url("https://chat.zalo.me/**", timeout=120000)
                await page.wait_for_timeout(3000)
            except PlaywrightTimeoutError:
                print("Het thoi gian cho dang nhap.")
                await context.close()
                return

        opened = await open_conversation(page, contact_query)
        if not opened:
            await context.close()
            return

        if rich:
            ok, body = await send_pasted_message(page, message_text)
            print("Gui tin (rich): ", "THANH CONG" if ok else "THAT BAI")
            print("  Response:", body[:300])
        else:
            await send_message(page, message_text)
        await page.wait_for_timeout(1000)
        await context.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Gui tin nhan tu dong tren chat.zalo.me")
    parser.add_argument("contact", help="Ten lien he hoac nhom can gui (theo dung nhu hien trong Zalo)")
    parser.add_argument("message", help="Noi dung tin nhan can gui")
    parser.add_argument("--headless", action="store_true", help="Chay an, khong hien cua so trinh duyet")
    parser.add_argument("--rich", action="store_true", help="Dien giai noi dung theo cu phap dinh dang (xem RICH_TEXT_SYNTAX.md)")
    args = parser.parse_args()

    asyncio.run(main(args.contact, args.message, args.headless, args.rich))
