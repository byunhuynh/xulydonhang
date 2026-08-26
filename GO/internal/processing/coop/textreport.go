package coop

import "strings"

// SplitTextReport cat noi dung mot file .txt bao cao don dat hang cua JDA
// (Coop/Coop Food xuat ra) thanh tung don rieng.
//
// Khac voi PDF - noi thu vien PDF tu chia san thanh trang - file .txt chi
// la mot khoi van ban lien tuc, co the chua HANG CHUC don. Moc phan cach
// duy nhat dang tin la dong mo dau bao cao "POM343"/"POM346", dung dung
// regex ma CountPOsOnPage dung de dem.
//
// Hai khoi LIEN NHAU cung mot "P/O Number" duoc GOP lai lam mot: don dai
// tran sang trang thu hai se lap lai dong POM343 voi "Page: 2" nhung van
// la MOT don. Khong gop thi mot don bi xe doi, va nua sau - vi thieu phan
// dau - vao so dat hang thanh mot chung tu khong co that. Chi gop khi
// LIEN NHAU: cung mot so P/O bi mot don khac chen giua la hai lan dat
// hang khac nhau, gop se tron du lieu cua ca hai.
//
// Van ban truoc khoi POM343 dau tien bi bo: no khong thuoc don nao.
// Khong tim thay khoi nao thi tra ve rong, de ben goi bao loi ro rang
// thay vi day mot don rong xuong duong xu ly.
func SplitTextReport(text string) []string {
	text = pomNormalizer.Replace(text)

	starts := pom34Pattern.FindAllStringIndex(text, -1)
	if len(starts) == 0 {
		return nil
	}

	blocks := make([]string, 0, len(starts))
	for i, loc := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		blocks = append(blocks, text[loc[0]:end])
	}

	merged := make([]string, 0, len(blocks))
	lastPO := ""
	for _, b := range blocks {
		po := ParseInvoiceInfo(b).PONumber
		// So P/O rong (khoi khong doc duoc so don) khong bao gio duoc coi
		// la "trung" voi khoi truoc: hai khoi hong lien nhau se bi gop
		// lam mot cach im lang.
		if po != "" && po == lastPO && len(merged) > 0 {
			merged[len(merged)-1] += b
			continue
		}
		merged = append(merged, b)
		lastPO = po
	}

	for i := range merged {
		merged[i] = strings.TrimRight(merged[i], "\n")
	}
	return merged
}
