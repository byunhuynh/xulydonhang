// Renders the same markup syntax GO/internal/zalosend/richtext (Go, used
// for the REAL Zalo send) understands, so the preview modal shows the
// message styled the way it will actually look in Zalo instead of raw
// `**`/`{red:...}` markers. See RICH_TEXT_SYNTAX.md for the full syntax
// reference this mirrors.
//
// Deliberately a SMALLER port than the Go package: only inline
// bold/italic/underline/strike/color and bullet/numbered lists with a
// flat (non-nested) indent rendered as margin-left - the only features
// zaloMessage.ts's own message templates actually use. This module is a
// best-effort PREVIEW; the Go richtext package (ChromedpSender's
// SendMessage) is the one actual source of truth for what gets sent.
// Indent specifically can only ever be an APPROXIMATION here: on the Go
// side, indentation is not part of the pasted HTML at all - it's a
// separate step (applyIndents in chromedp_sender.go) that clicks the
// real "Lùi đầu dòng" button per line AFTER pasting, so there is no HTML
// structure to mirror 1:1. This renders the same VISUAL result via
// margin-left instead.

const COLOR_SWATCH_RGB: Record<string, string> = {
  red: 'rgb(219, 52, 46)',
  orange: 'rgb(242, 120, 6)',
  yellow: 'rgb(247, 181, 3)',
  green: 'rgb(21, 168, 95)',
  black: 'rgb(5, 10, 25)',
}

type SpanKind = 'bold' | 'italic' | 'underline' | 'strike' | 'color'

interface Span {
  start: number
  end: number
  kind: SpanKind
  color?: string
}

interface ParsedLine {
  listType: 'bullet' | 'numbered' | null
  indent: number
  plainText: string
  spans: Span[]
}

const INLINE_PATTERN =
  /\*\*(?<bold>.+?)\*\*|__(?<underline>.+?)__|~~(?<strike>.+?)~~|\{(?<color>red|orange|yellow|green|black):(?<colortext>.+?)\}|\*(?<italic>.+?)\*/g

const BULLET_RE = /^-\s+(.*)$/
const NUMBERED_RE = /^\d+\.\s+(.*)$/

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

// parseInline tách các thẻ định dạng inline khỏi content, trả về
// plainText đã bỏ hết dấu markup + danh sách Span với offset tính trên
// plainText - khớp cách GO/internal/zalosend/richtext/parser.go's
// ParseInline hoạt động (không hỗ trợ định dạng lồng nhau, cùng lý do).
function parseInline(content: string): { plainText: string; spans: Span[] } {
  const spans: Span[] = []
  let plainText = ''
  let pos = 0
  INLINE_PATTERN.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = INLINE_PATTERN.exec(content))) {
    plainText += content.slice(pos, m.index)
    const g = m.groups!
    let text: string
    let kind: SpanKind
    let color: string | undefined
    if (g.bold !== undefined) {
      text = g.bold
      kind = 'bold'
    } else if (g.underline !== undefined) {
      text = g.underline
      kind = 'underline'
    } else if (g.strike !== undefined) {
      text = g.strike
      kind = 'strike'
    } else if (g.color !== undefined) {
      text = g.colortext
      kind = 'color'
      color = g.color
    } else {
      text = g.italic
      kind = 'italic'
    }
    const start = plainText.length
    plainText += text
    spans.push({ start, end: plainText.length, kind, color })
    pos = m.index + m[0].length
  }
  plainText += content.slice(pos)
  return { plainText, spans }
}

function parseLine(rawLine: string): ParsedLine {
  let listType: ParsedLine['listType'] = null
  // Bóc từng cặp 2 khoảng trắng đầu dòng (rồi tab) VÔ ĐIỀU KIỆN, kể cả
  // khi dòng không phải list - khớp đúng hành vi
  // GO/internal/zalosend/richtext/parser.go's ParseLine. Đếm lại số cặp
  // đã bóc làm CẤP THỤT LỀ (indent) - dòng list có indent>0 được hiện
  // lùi vào trong preview (xem renderInlineHtml/markupToHtml), xấp xỉ
  // đúng hiệu ứng "Lùi đầu dòng" mà ChromedpSender bấm thật sau khi dán
  // (xem RICH_TEXT_SYNTAX.md mục 3/7) - bản thân bước bấm đó không nằm
  // trong HTML dán ban đầu nên không thể mô phỏng y hệt DOM, chỉ mô
  // phỏng ĐÚNG KẾT QUẢ NHÌN THẤY bằng margin-left.
  let indent = 0
  let line = rawLine
  while (line.startsWith('  ')) {
    indent++
    line = line.slice(2)
  }
  while (line.startsWith('\t')) {
    indent++
    line = line.slice(1)
  }
  const bulletMatch = BULLET_RE.exec(line)
  if (bulletMatch) {
    listType = 'bullet'
    line = bulletMatch[1]
  } else {
    const numberedMatch = NUMBERED_RE.exec(line)
    if (numberedMatch) {
      listType = 'numbered'
      line = numberedMatch[1]
    }
  }
  const { plainText, spans } = parseInline(line)
  return { listType, indent, plainText, spans }
}

function renderInlineHtml(plainText: string, spans: Span[]): string {
  const sorted = [...spans].sort((a, b) => a.start - b.start)
  let out = ''
  let pos = 0
  for (const span of sorted) {
    if (span.start > pos) out += escapeHtml(plainText.slice(pos, span.start))
    const inner = escapeHtml(plainText.slice(span.start, span.end))
    switch (span.kind) {
      case 'bold':
        out += `<b>${inner}</b>`
        break
      case 'italic':
        out += `<i>${inner}</i>`
        break
      case 'underline':
        out += `<u>${inner}</u>`
        break
      case 'strike':
        out += `<s>${inner}</s>`
        break
      case 'color':
        out += `<span style="color: ${COLOR_SWATCH_RGB[span.color as string]}">${inner}</span>`
        break
    }
    pos = span.end
  }
  if (pos < plainText.length) out += escapeHtml(plainText.slice(pos))
  return out
}

// markupToHtml dịch markupText thành 1 khối HTML để hiện trong preview -
// mỗi dòng thường thành 1 <div>, các dòng list LIÊN TIẾP cùng loại gộp
// vào 1 <ul>/<ol> (không xử lý dòng trống xen giữa như bản Go, vì nội
// dung tin nhắn hiện tại không tạo ra tình huống đó).
export function markupToHtml(markupText: string): string {
  const lines = markupText.split('\n').map(parseLine)
  const parts: string[] = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    if (line.listType) {
      const tag = line.listType === 'numbered' ? 'ol' : 'ul'
      const items: string[] = []
      while (i < lines.length && lines[i].listType === line.listType) {
        const style = lines[i].indent > 0 ? ` style="margin-left: ${lines[i].indent * 18}px"` : ''
        items.push(`<li${style}>${renderInlineHtml(lines[i].plainText, lines[i].spans)}</li>`)
        i++
      }
      parts.push(`<${tag}>${items.join('')}</${tag}>`)
    } else {
      parts.push(line.plainText === '' ? '<div><br></div>' : `<div>${renderInlineHtml(line.plainText, line.spans)}</div>`)
      i++
    }
  }
  return parts.join('')
}
