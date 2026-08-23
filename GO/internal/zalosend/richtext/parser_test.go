package richtext

import "testing"

func TestMarkupToHTML_Basic(t *testing.T) {
	got := MarkupToHTML("**Xin chào** *bạn* __gạch chân__ ~~cũ~~ {red:khẩn cấp}")
	want := `<div><b>Xin chào</b> <i>bạn</i> <u>gạch chân</u> <s>cũ</s> <span style="color: rgb(219, 52, 46)">khẩn cấp</span></div>`
	if got != want {
		t.Fatalf("mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestMarkupToHTML_ListWithBlankLineContinuation(t *testing.T) {
	// dong trong giua 2 muc list cung loai phai duoc gop chung 1 <ol>,
	// khong tach thanh 2 list rieng (moi list rieng se tu danh so lai tu 1)
	got := MarkupToHTML("1. Bước một\n\n2. Bước hai")
	want := "<ol><li>Bước một</li><li>Bước hai</li></ol>"
	if got != want {
		t.Fatalf("mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestMarkupToHTML_BulletList(t *testing.T) {
	got := MarkupToHTML("- Mục một\n- Mục hai")
	want := "<ul><li>Mục một</li><li>Mục hai</li></ul>"
	if got != want {
		t.Fatalf("mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestParseLine_Indent(t *testing.T) {
	l := ParseLine("    - Mục con cấp 2")
	if l.Indent != 2 {
		t.Fatalf("expected indent 2, got %d", l.Indent)
	}
	if l.ListType != "bullet" {
		t.Fatalf("expected bullet, got %s", l.ListType)
	}
	if l.PlainText != "Mục con cấp 2" {
		t.Fatalf("unexpected plain text: %q", l.PlainText)
	}
}

func TestParseInline_SpanOffsetsAreRuneBased(t *testing.T) {
	// "đậm" co dau, dam bao offset tinh theo rune khong bi lech do UTF-8
	_, spans := ParseInline("**đậm** rồi thường")
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Start != 0 || spans[0].End != 3 {
		t.Fatalf("unexpected span bounds: %+v", spans[0])
	}
}
