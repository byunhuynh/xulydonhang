// Package richtext dich cu phap markup rieng (giong markdown) sang cau
// truc du lieu (ParsedLine/Span), dung de dung HTML dan (paste) vao o
// soan tin cua chat.zalo.me. Ban port tu rich_paste_engine.py (Python).
//
// Xem quy tac cu phap trong RICH_TEXT_SYNTAX.md o thu muc goc.
package richtext

import "regexp"

// ColorSwatchRGB anh xa ten mau trong markup ({red:...}) sang dung ma
// RGB cua bang mau Zalo - CHI 5 mau nay moi duoc Zalo giu lai khi dan,
// mau tuy y se bi loai bo hoan toan (da xac nhan qua thu nghiem thuc te).
var ColorSwatchRGB = map[string]string{
	"red":    "rgb(219, 52, 46)",
	"orange": "rgb(242, 120, 6)",
	"yellow": "rgb(247, 181, 3)",
	"green":  "rgb(21, 168, 95)",
	"black":  "rgb(5, 10, 25)",
}

var inlinePattern = regexp.MustCompile(
	`\*\*(?P<bold>.+?)\*\*` +
		`|__(?P<underline>.+?)__` +
		`|~~(?P<strike>.+?)~~` +
		`|\{(?P<color>red|orange|yellow|green|black):(?P<colortext>.+?)\}` +
		`|\*(?P<italic>.+?)\*`,
)

var numberedRe = regexp.MustCompile(`^\d+\.\s+(.*)$`)
var bulletRe = regexp.MustCompile(`^-\s+(.*)$`)

// Span la mot vung dinh dang trong plain_text, [Start, End) tinh theo rune.
type Span struct {
	Start int
	End   int
	Kind  string // bold | italic | underline | strike | color
	Color string // chi dung khi Kind == "color"
}

// ParsedLine la ket qua parse 1 dong markup.
type ParsedLine struct {
	Raw       string
	Indent    int
	ListType  string // "bullet" | "numbered" | "" (khong phai list)
	PlainText string
	Spans     []Span
}

// ParseInline tach cac the dinh dang inline (**..**, *..*, __..__, ~~..~~,
// {color:..}) khoi content, tra ve plainText da bo het cac dau markup va
// danh sach Span voi offset tinh tren plainText (theo rune, khong phai byte).
func ParseInline(content string) (string, []Span) {
	matches := inlinePattern.FindAllStringSubmatchIndex(content, -1)
	names := inlinePattern.SubexpNames()

	var plainParts []rune
	var spans []Span
	pos := 0 // byte offset trong content, dung de cat chuoi con truoc match

	for _, m := range matches {
		matchStartByte, matchEndByte := m[0], m[1]

		// phan van ban thuong truoc match
		before := []rune(content[pos:matchStartByte])
		plainParts = append(plainParts, before...)

		var text, kind, color string
		getGroup := func(name string) (string, bool) {
			for gi, gname := range names {
				if gname == name && m[2*gi] != -1 {
					return content[m[2*gi]:m[2*gi+1]], true
				}
			}
			return "", false
		}

		if v, ok := getGroup("bold"); ok {
			text, kind = v, "bold"
		} else if v, ok := getGroup("underline"); ok {
			text, kind = v, "underline"
		} else if v, ok := getGroup("strike"); ok {
			text, kind = v, "strike"
		} else if v, ok := getGroup("color"); ok {
			colorText, _ := getGroup("colortext")
			text, kind, color = colorText, "color", v
		} else if v, ok := getGroup("italic"); ok {
			text, kind = v, "italic"
		} else {
			text = content[matchStartByte:matchEndByte]
		}

		start := len(plainParts)
		textRunes := []rune(text)
		plainParts = append(plainParts, textRunes...)
		end := len(plainParts)
		if kind != "" {
			spans = append(spans, Span{Start: start, End: end, Kind: kind, Color: color})
		}

		pos = matchEndByte
	}

	plainParts = append(plainParts, []rune(content[pos:])...)
	return string(plainParts), spans
}

// ParseLine phan tich mot dong markup don le: thut le (2 khoang trang
// hoac tab moi cap), danh sach (- hoac 1.), roi den dinh dang inline.
func ParseLine(rawLine string) ParsedLine {
	indent := 0
	line := rawLine
	for len(line) >= 2 && line[:2] == "  " {
		indent++
		line = line[2:]
	}
	for len(line) >= 1 && line[:1] == "\t" {
		indent++
		line = line[1:]
	}

	listType := ""
	if m := bulletRe.FindStringSubmatch(line); m != nil {
		listType = "bullet"
		line = m[1]
	} else if m := numberedRe.FindStringSubmatch(line); m != nil {
		listType = "numbered"
		line = m[1]
	}

	plainText, spans := ParseInline(line)
	return ParsedLine{Raw: rawLine, Indent: indent, ListType: listType, PlainText: plainText, Spans: spans}
}

// ParseDocument tach markup thanh cac dong (theo "\n") va parse tung dong.
func ParseDocument(text string) []ParsedLine {
	lines := splitLines(text)
	result := make([]ParsedLine, 0, len(lines))
	for _, l := range lines {
		result = append(result, ParseLine(l))
	}
	return result
}

func splitLines(text string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			lines = append(lines, text[start:i])
			start = i + 1
		}
	}
	lines = append(lines, text[start:])
	return lines
}
