package richtext

import (
	"html"
	"strings"
)

// renderInlineHTML ghep plainText + spans (offset tinh theo rune, da co
// san) thanh 1 doan HTML - khong ho tro dinh dang long nhau (giong v1
// cua ban Python).
func renderInlineHTML(plainText string, spans []Span) string {
	runes := []rune(plainText)
	sorted := make([]Span, len(spans))
	copy(sorted, spans)
	// sort theo Start (danh sach dau vao thuong da theo thu tu, nhung
	// sort lai cho chac chan giong ban Python dung sorted())
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j-1].Start > sorted[j].Start; j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}

	var out strings.Builder
	pos := 0
	for _, span := range sorted {
		if span.Start > pos {
			out.WriteString(html.EscapeString(string(runes[pos:span.Start])))
		}
		inner := html.EscapeString(string(runes[span.Start:span.End]))
		switch span.Kind {
		case "bold":
			out.WriteString("<b>" + inner + "</b>")
		case "italic":
			out.WriteString("<i>" + inner + "</i>")
		case "underline":
			out.WriteString("<u>" + inner + "</u>")
		case "strike":
			out.WriteString("<s>" + inner + "</s>")
		case "color":
			rgb := ColorSwatchRGB[span.Color]
			out.WriteString(`<span style="color: ` + rgb + `">` + inner + "</span>")
		default:
			out.WriteString(inner)
		}
		pos = span.End
	}
	if pos < len(runes) {
		out.WriteString(html.EscapeString(string(runes[pos:])))
	}
	return out.String()
}

// BuildHTML dich danh sach ParsedLine thanh 1 khoi HTML de dan (paste)
// 1 lan. Cac dong list cung loai duoc gom vao 1 the <ul>/<ol>, KE CA khi
// co dong trong xen giua (dong trong chi la khoang cach thi giac trong
// markup - neu dong ke tiep sau (cac) dong trong van cung ListType thi
// van duoc xem la tiep tuc cung 1 danh sach, khong bi ngat thanh nhieu
// danh sach rieng le - moi danh sach rieng le se tu danh so lai tu 1,
// day la ly do can gop dung).
// LUU Y: khong ho tro thut le qua HTML long nhau (bi Zalo lam phang khi
// dan) - xem ApplyIndents trong engine.go cho co che thut le thuc su.
func BuildHTML(lines []ParsedLine) string {
	var parts strings.Builder
	i := 0
	n := len(lines)
	for i < n {
		line := lines[i]
		if line.ListType != "" {
			tag := "ul"
			if line.ListType == "numbered" {
				tag = "ol"
			}
			var items strings.Builder
			for i < n {
				if lines[i].ListType == line.ListType {
					items.WriteString("<li>" + renderInlineHTML(lines[i].PlainText, lines[i].Spans) + "</li>")
					i++
				} else if lines[i].ListType == "" && lines[i].PlainText == "" {
					j := i
					for j < n && lines[j].ListType == "" && lines[j].PlainText == "" {
						j++
					}
					if j < n && lines[j].ListType == line.ListType {
						i = j // bo qua cac dong trong, tiep tuc cung danh sach
					} else {
						break
					}
				} else {
					break
				}
			}
			parts.WriteString("<" + tag + ">" + items.String() + "</" + tag + ">")
		} else {
			if line.PlainText == "" {
				parts.WriteString("<div><br></div>")
			} else {
				parts.WriteString("<div>" + renderInlineHTML(line.PlainText, line.Spans) + "</div>")
			}
			i++
		}
	}
	return parts.String()
}

// MarkupToHTML la ham tien ich: parse markup roi dung HTML trong 1 buoc.
func MarkupToHTML(markupText string) string {
	return BuildHTML(ParseDocument(markupText))
}
