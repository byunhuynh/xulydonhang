package processing

import "fmt"

func extractPageTexts(path string) ([]string, error) {
	file, r, err := pdfOpen(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	numPages := r.NumPage()
	pages := make([]string, 0, numPages)
	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return nil, fmt.Errorf("trang %d: %w", i, err)
		}
		pages = append(pages, text)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("không đọc được nội dung trang nào")
	}
	return pages, nil
}
