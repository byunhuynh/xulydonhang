package coop

import (
	"regexp"
	"strings"
)

// pom34Pattern and subTotalCountPattern add character-by-character
// whitespace tolerance (via spacedPattern, already used for
// P/O-Number-style fields in invoice.go) on top of a faithful port of
// demsodonhang1trang_coop's two literal regexes. This is NOT a
// behavioral deviation from the Python original — PyMuPDF's text
// extraction never introduces gaps between adjacent letters of a
// single word for these PDFs, so Python's un-tolerant literals always
// matched real "POM343"/"Sub Total" text as intact substrings. But this
// Go port's PDF library sometimes can't fully reconstruct the original
// letter-spacing for certain field labels (confirmed against a real
// archived PDF, 103311304-00: "Sub Total" extracts as "S u b   T o t a
// l", a pure text-extraction-fidelity gap, not a difference in what the
// PDF actually contains) — spacedPattern's \s* between every letter is
// purely additive tolerance that still requires the same letters in
// the same order, so it can only recognize genuine
// "POM343"/"POM346"/"Sub Total" occurrences the original, stricter
// pattern would also have recognized on cleanly-extracted text; it
// cannot introduce a false match that wasn't already the real word.
var (
	pom34Pattern         = regexp.MustCompile(spacedPattern("POM34") + `\s*[36]\b`)
	subTotalCountPattern = regexp.MustCompile(`\b` + spacedPattern("Sub Total") + `\b`)
)

// PageCounts mirrors demsodonhang1trang_coop's returned dict.
type PageCounts struct {
	POM343   int
	SubTotal int
}

// pomNormalizer chuan hoa van ban truoc khi DEM va truoc khi TACH.
// CountPOsOnPage va SplitMultiPO phai nhin thay CUNG mot van ban: truoc
// day mot ben dem tren ban da chuan hoa con ben kia tach tren ban goc,
// nen so dem va so doan tach duoc lech nhau ma khong co gi bao.
var pomNormalizer = strings.NewReplacer(" ", " ", "pom343", "POM343", "pom346", "POM346")

// CountPOsOnPage mirrors demsodonhang1trang_coop.
func CountPOsOnPage(text string) PageCounts {
	text = pomNormalizer.Replace(text)
	return PageCounts{
		POM343:   len(pom34Pattern.FindAllString(text, -1)),
		SubTotal: len(subTotalCountPattern.FindAllString(text, -1)),
	}
}

// SplitMultiPO cat mot trang nhieu PO thanh tung doan, moi doan mot PO.
//
// Ranh gioi mo dau la POM343/POM346, ranh gioi ket thuc la "Sub Total".
// Ca hai deu dung DUNG regex ma CountPOsOnPage dung de dem, tren van ban
// da qua pomNormalizer - nen so doan tra ve luon khop so dem.
//
// Ban dau ham nay port nguyen si mot bug cua ban Python: kiem tra ton tai
// thi khong phan biet hoa thuong, nhung lenh tach lai dung strings.SplitN
// voi chuoi CHU THUONG "pom343" trong khi du lieu Coop that viet HOA. Hau
// qua: moi trang PDF co nhieu hon mot PO chi ra dung don dau tien, cac don
// sau bien mat KHONG BAO GI. Nguoi dung quyet sua ngay 2026-08-26 vi day
// la mat don im lang vao so dat hang.
func SplitMultiPO(text string) []string {
	text = pomNormalizer.Replace(text)

	var segments []string

	before, after, ok := splitOnSubTotal(text)
	if !ok {
		return segments
	}
	segments = append(segments, before)
	text = after

	for subTotalCountPattern.MatchString(text) {
		loc := pom34Pattern.FindStringIndex(text)
		if loc == nil {
			break
		}
		// Giu dung chuoi tu khoa da khop (POM343 hay POM346) thay vi gan
		// cung mot chuoi: moi doan phai noi dung loai chung tu cua chinh no.
		keyword := text[loc[0]:loc[1]]
		text = text[loc[1]:]

		subBefore, subAfter, subOK := splitOnSubTotal(text)
		if !subOK {
			segments = append(segments, keyword+text)
			return segments
		}
		segments = append(segments, keyword+strings.TrimRight(subBefore, "\n"))
		text = subAfter
	}

	return segments
}

// splitOnSubTotal finds the first whitespace-tolerant "Sub Total"
// boundary (per subTotalCountPattern) and returns the text before and
// after it. Mirrors strings.SplitN(text, "Sub Total", 2) but tolerant
// of character-shredded extraction the same way CountPOsOnPage is.
func splitOnSubTotal(text string) (before, after string, ok bool) {
	loc := subTotalCountPattern.FindStringIndex(text)
	if loc == nil {
		return "", "", false
	}
	return text[:loc[0]], text[loc[1]:], true
}
