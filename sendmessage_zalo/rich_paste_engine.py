"""
Gui tin nhan dinh dang tren chat.zalo.me bang cach dan (paste) mot khoi
HTML da dung san vao o soan tin, thay vi go+chon+bam nut qua ban phim.

Nhanh (dan 1 lan thay vi hang tram lan nhan phim, ~1s so voi ~36s cho
mot doan van dai nhieu dinh dang) va on dinh hon cach ban phim.

Xem quy tac cu phap markup trong file RICH_TEXT_SYNTAX.md.

Han che da xac nhan qua thu nghiem thuc te tren chat.zalo.me:
- Mau chu CHI giu duoc neu dung dung 5 ma RGB cua bang mau Zalo (xem
  COLOR_SWATCH_RGB) - mau tuy y (vd tu Word) se bi loai bo hoan toan.
- Thut le (list long nhau) KHONG duoc giu qua paste - bi lam phang.
- Co chu chi co hieu qua 3 muc: mac dinh, 18px, 20px (moi gia tri lon
  hon deu bi kep ve 20px) - chua tich hop cu phap rieng cho no.
- Danh sach danh so ma tung muc con kem dinh dang inline khac co the bi
  Zalo hien lai "1." cho moi muc thay vi tu tang - day la hanh vi phia
  Zalo (da xac nhan), khong phai loi cua engine nay.
"""

import html as html_module
import re
from dataclasses import dataclass
from typing import List, Optional

COLOR_SWATCH_RGB = {
    "red": "rgb(219, 52, 46)",
    "orange": "rgb(242, 120, 6)",
    "yellow": "rgb(247, 181, 3)",
    "green": "rgb(21, 168, 95)",
    "black": "rgb(5, 10, 25)",
}

INLINE_PATTERN = re.compile(
    r"\*\*(?P<bold>.+?)\*\*"
    r"|__(?P<underline>.+?)__"
    r"|~~(?P<strike>.+?)~~"
    r"|\{(?P<color>red|orange|yellow|green|black):(?P<colortext>.+?)\}"
    r"|\*(?P<italic>.+?)\*"
)

NUMBERED_RE = re.compile(r"^\d+\.\s+(.*)$")
BULLET_RE = re.compile(r"^-\s+(.*)$")


@dataclass
class Span:
    start: int
    end: int
    kind: str  # bold, italic, underline, strike, color
    color: Optional[str] = None


@dataclass
class ParsedLine:
    raw: str
    indent: int
    list_type: Optional[str]  # 'bullet' | 'numbered' | None
    plain_text: str
    spans: List[Span]


def parse_inline(content: str) -> (str, List[Span]):
    """Tra ve (plain_text, spans) voi offset tinh tren plain_text."""
    plain_parts: List[str] = []
    spans: List[Span] = []
    pos = 0
    out_len = 0

    for m in INLINE_PATTERN.finditer(content):
        plain_parts.append(content[pos:m.start()])
        out_len += m.start() - pos

        if m.group("bold") is not None:
            text, kind, color = m.group("bold"), "bold", None
        elif m.group("underline") is not None:
            text, kind, color = m.group("underline"), "underline", None
        elif m.group("strike") is not None:
            text, kind, color = m.group("strike"), "strike", None
        elif m.group("color") is not None:
            text, kind, color = m.group("colortext"), "color", m.group("color")
        elif m.group("italic") is not None:
            text, kind, color = m.group("italic"), "italic", None
        else:
            text, kind, color = m.group(0), None, None

        start = out_len
        plain_parts.append(text)
        out_len += len(text)
        end = out_len
        if kind:
            spans.append(Span(start=start, end=end, kind=kind, color=color))

        pos = m.end()

    plain_parts.append(content[pos:])
    plain_text = "".join(plain_parts)
    return plain_text, spans


def parse_line(raw_line: str) -> ParsedLine:
    indent = 0
    line = raw_line
    while line.startswith("  "):
        indent += 1
        line = line[2:]
    while line.startswith("\t"):
        indent += 1
        line = line[1:]

    list_type = None
    m = BULLET_RE.match(line)
    if m:
        list_type = "bullet"
        line = m.group(1)
    else:
        m = NUMBERED_RE.match(line)
        if m:
            list_type = "numbered"
            line = m.group(1)

    plain_text, spans = parse_inline(line)
    return ParsedLine(raw=raw_line, indent=indent, list_type=list_type, plain_text=plain_text, spans=spans)


def parse_document(text: str) -> List[ParsedLine]:
    return [parse_line(l) for l in text.split("\n")]


def _render_inline_html(plain_text: str, spans: List[Span]) -> str:
    """Ghep plain_text + spans (offset da tinh san) thanh 1 doan HTML co
    the long nhau dung thu tu (khong ho tro format long nhau, giong v1)."""
    spans_sorted = sorted(spans, key=lambda s: s.start)
    out = []
    pos = 0
    for span in spans_sorted:
        if span.start > pos:
            out.append(html_module.escape(plain_text[pos:span.start]))
        inner = html_module.escape(plain_text[span.start:span.end])
        if span.kind == "bold":
            out.append(f"<b>{inner}</b>")
        elif span.kind == "italic":
            out.append(f"<i>{inner}</i>")
        elif span.kind == "underline":
            out.append(f"<u>{inner}</u>")
        elif span.kind == "strike":
            out.append(f"<s>{inner}</s>")
        elif span.kind == "color":
            rgb = COLOR_SWATCH_RGB[span.color]
            out.append(f'<span style="color: {rgb}">{inner}</span>')
        else:
            out.append(inner)
        pos = span.end
    if pos < len(plain_text):
        out.append(html_module.escape(plain_text[pos:]))
    return "".join(out) if out else ""


def build_html(lines: List[ParsedLine]) -> str:
    """Dich danh sach ParsedLine thanh 1 khoi HTML de dan (paste) 1 lan.
    Cac dong list cung loai duoc gom vao 1 the <ul>/<ol>, KE CA khi co
    dong trong xen giua (dong trong chi la khoang cach thi giac trong
    markup - neu dong ke tiep sau (cac) dong trong van cung list_type
    thi vAn duoc xem la tiep tuc cung 1 danh sach, khong bi ngat thanh
    nhieu danh sach rieng le - moi danh sach rieng le se tu danh so lai
    tu 1, day la ly do can gop dung).
    LUU Y: khong ho tro thut le (xem docstring dau file)."""
    parts = []
    i = 0
    n = len(lines)
    while i < n:
        line = lines[i]
        if line.list_type:
            tag = "ul" if line.list_type == "bullet" else "ol"
            items = []
            while i < n:
                if lines[i].list_type == line.list_type:
                    inner_html = _render_inline_html(lines[i].plain_text, lines[i].spans)
                    items.append(f"<li>{inner_html}</li>")
                    i += 1
                elif lines[i].list_type is None and lines[i].plain_text == "":
                    j = i
                    while j < n and lines[j].list_type is None and lines[j].plain_text == "":
                        j += 1
                    if j < n and lines[j].list_type == line.list_type:
                        i = j  # bo qua cac dong trong, tiep tuc cung danh sach
                    else:
                        break
                else:
                    break
            parts.append(f"<{tag}>{''.join(items)}</{tag}>")
        else:
            if line.plain_text == "":
                parts.append("<div><br></div>")
            else:
                inner_html = _render_inline_html(line.plain_text, line.spans)
                parts.append(f"<div>{inner_html}</div>")
            i += 1
    return "".join(parts)


def markup_to_html(markup_text: str) -> str:
    lines = parse_document(markup_text)
    return build_html(lines)


async def _ensure_format_mode(page) -> None:
    already_on = await page.evaluate(
        """
        () => {
            const btn = document.querySelector('[title="Định dạng tin nhắn (Ctrl + Shift + X)"]');
            return !!(btn && btn.className.includes('focused'));
        }
        """
    )
    if not already_on:
        await page.keyboard.press("Control+Shift+X")
        await page.get_by_title("In đậm", exact=True).wait_for(state="visible", timeout=5000)
        await page.wait_for_timeout(300)


INDENT_CLICKS_PER_LEVEL = {
    # Nut "Lui dau dong" tren <ul> tao 1 muc thut le ro rang chi voi 1 lan
    # bam. Tren <ol> no chi cong don text-indent:10px moi lan bam (kem
    # list-style-position:inside), phai bam nhieu lan moi thay ro - da do
    # dac qua thu nghiem thuc te (xem RICH_TEXT_SYNTAX.md muc 7).
    "bullet": 1,
    "numbered": 3,  # ~30px moi cap, du ro de phan biet voi muc cha
}


async def _apply_indents(page, lines: List[ParsedLine]) -> None:
    """
    Buoc 2 cua co che hybrid: sau khi da dan (paste) noi dung dang list
    PHANG, tim tung dong can thut le (line.indent > 0) trong o soan tin
    bang cach khop text, dat con tro vao dong do, roi bam nut "Lui dau
    dong" dung so lan. Khac voi thut le qua HTML long nhau (bi Zalo lam
    phang khi paste), thut le qua nut bam nay duoc giu nguyen sau khi gui.

    Dung "occurrence index" de xu ly dung khi co nhieu dong trung text.
    """
    seen_counts: dict = {}
    for line in lines:
        if not (line.list_type and line.indent > 0 and line.plain_text):
            continue
        skip = seen_counts.get(line.plain_text, 0)
        seen_counts[line.plain_text] = skip + 1

        coords = await page.evaluate(
            """
            ({ text, skip }) => {
                const root = document.activeElement;
                // Moi dong duoc Zalo bao trong 1 khoi "rtf-block" (DIV hoac LI).
                // Dong khong co dinh dang inline la node la (children.length===0)
                // voi textContent = dung text; dong CO dinh dang inline (vd
                // *nghieng*) bi tach thanh nhieu <span> con ben trong khoi do,
                // nen phai gop textContent CUA CA KHOI (khong chi node la) moi
                // khop dung - neu chi tim node la se bo sot cac dong co dinh dang.
                const blocks = root.querySelectorAll('[data-component="rtf-block"]');
                let matchCount = 0;
                for (const node of blocks) {
                    if (node.textContent.trim() === text) {
                        if (matchCount === skip) {
                            // node la block-level (rong bang ca container), khong
                            // dai dien vi tri thi giac that cua cuoi dong text -
                            // tim span/text-node CUOI CUNG co noi dung ben trong
                            // khoi nay va lay canh phai cua no thay vi cua block.
                            let target = node;
                            const spans = node.querySelectorAll('[data-text="true"]');
                            if (spans.length > 0) {
                                target = spans[spans.length - 1];
                            }
                            // Tin dai co the khien o soan tin cuon noi bo; dong
                            // can thut le co the dang nam ngoai vung nhin thay -
                            // cuon no vao giua man hinh truoc khi lay toa do,
                            // neu khong click se roi vao vi tri sai (vd de len
                            // tin nhan lich su phia tren o soan tin).
                            target.scrollIntoView({ block: "center" });
                            const r = target.getBoundingClientRect();
                            return { x: r.x + r.width - 2, y: r.y + r.height / 2 };
                        }
                        matchCount += 1;
                    }
                }
                return null;
            }
            """,
            {"text": line.plain_text, "skip": skip},
        )
        if coords is None:
            continue
        await page.mouse.click(coords["x"], coords["y"])
        await page.wait_for_timeout(150)
        clicks_per_level = INDENT_CLICKS_PER_LEVEL.get(line.list_type, 1)
        for _ in range(line.indent * clicks_per_level):
            await page.get_by_title("Lùi đầu dòng", exact=True).click()
            await page.wait_for_timeout(150)


async def send_pasted_message(page, markup_text: str, rich_input_selector: str = "#richInput"):
    """
    Dan (paste) toan bo `markup_text` (cu phap RICH_TEXT_SYNTAX.md) vao
    hoi thoai dang mo, roi gui. Neu markup co dong list voi thut le (2
    khoang trang dau dong), se tu dong ap dung thut le bang cach bam nut
    "Lui dau dong" cho tung dong sau khi paste (xem _apply_indents) -
    cham hon mot chut so voi khong co thut le, nhung van nhanh hon nhieu
    so voi go toan bo bang ban phim.
    Yeu cau: da mo dung hoi thoai (#richInput dang hien dien truoc khi goi).
    """
    rich_input = page.locator(rich_input_selector)
    await rich_input.click()
    await page.wait_for_timeout(150)

    await _ensure_format_mode(page)

    lines = parse_document(markup_text)
    html = build_html(lines)
    plain_fallback = "\n".join(l.plain_text for l in lines)

    await page.evaluate(
        """
        ({ html, text }) => {
            const el = document.activeElement;
            const dt = new DataTransfer();
            dt.setData('text/html', html);
            dt.setData('text/plain', text);
            const evt = new ClipboardEvent('paste', {
                clipboardData: dt,
                bubbles: true,
                cancelable: true,
            });
            el.dispatchEvent(evt);
        }
        """,
        {"html": html, "text": plain_fallback},
    )
    await page.wait_for_timeout(300)

    if any(l.list_type and l.indent > 0 for l in lines):
        await _apply_indents(page, lines)

    async with page.expect_response(lambda r: "message/sms" in r.url, timeout=15000) as resp_info:
        await page.keyboard.press("Enter")
    resp = await resp_info.value
    body = await resp.text()
    ok = resp.status == 200 and '"error_code":0' in body
    return ok, body
