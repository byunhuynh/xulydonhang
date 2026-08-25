// GO/internal/appsettings/migrate.go
package appsettings

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// migrateFromOldIni đọc settings.ini cũ (định dạng
// <tag>\nKEY = VALUE\n...\n</tag>, xem pricing/gid.go's LoadGidMap bản
// gốc — hàm này thay thế nó, tổng quát hóa tên tag thành tham số thay
// vì chỉ đọc riêng <gid>) và trả về Settings + true nếu file tồn tại
// và đọc được, hoặc Settings{} + false nếu file không tồn tại (KHÔNG
// phải lỗi — app có thể chưa từng có settings.ini, ví dụ cài mới hoàn
// toàn).
func migrateFromOldIni(path string) (Settings, bool, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: read %s: %w", path, err)
	}

	gid, err := parseTagBlock(string(content), "gid")
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: migrate <gid>: %w", err)
	}
	zalo, err := parseTagBlock(string(content), "zalo")
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: migrate <zalo>: %w", err)
	}
	reminder, err := parseTagBlock(string(content), "reminder")
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: migrate <reminder>: %w", err)
	}
	haravan, err := parseTagBlock(string(content), "haravan")
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: migrate <haravan>: %w", err)
	}
	return Settings{Gid: gid, Zalo: zalo, Reminder: reminder, Haravan: haravan}, true, nil
}

// parseTagBlock đọc khối <tagName>...</tagName>, mỗi dòng bên trong
// dạng "KEY = VALUE" — logic giống hệt pricing.LoadGidMap bản gốc,
// tổng quát hoá tên tag thành tham số. Dòng không có dấu "=" bị bỏ qua
// (comment, ví dụ 2 dòng "# MAKH/SANPHAM..." đã có sẵn trong <gid>);
// dòng có NHIỀU HƠN 1 dấu "=" là lỗi rõ ràng, không âm thầm lấy phần
// đầu/cuối. Không tìm thấy khối tag → trả map rỗng, không lỗi (ví dụ
// file cũ không có <reminder> vì được thêm sau).
func parseTagBlock(content, tagName string) (map[string]string, error) {
	pattern := regexp.MustCompile(`(?s)<` + tagName + `>(.*?)</` + tagName + `>`)
	match := pattern.FindStringSubmatch(content)
	if match == nil {
		return map[string]string{}, nil
	}

	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(match[1]), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.Split(line, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed <%s> line (expected exactly one '='): %q", tagName, line)
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result, nil
}
