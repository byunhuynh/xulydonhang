package fileset

import (
	"os"
	"path/filepath"
	"strings"
)

var allowedExtensions = map[string]bool{
	".pdf":  true,
	".xlsx": true,
	".txt":  true,
}

// IsAllowed báo file có đuôi được phép xử lý (.pdf, .xlsx, .txt) hay không.
func IsAllowed(path string) bool {
	return allowedExtensions[strings.ToLower(filepath.Ext(path))]
}

// FilterValid giữ lại các đường dẫn có đuôi hợp lệ, giữ nguyên thứ tự.
func FilterValid(paths []string) []string {
	valid := make([]string, 0, len(paths))
	for _, p := range paths {
		if IsAllowed(p) {
			valid = append(valid, p)
		}
	}
	return valid
}

// ListFiles trả về đường dẫn tuyệt đối các file (không gồm thư mục con)
// nằm trực tiếp trong dir có đuôi hợp lệ.
func ListFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !IsAllowed(entry.Name()) {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, abs)
	}
	return files, nil
}
