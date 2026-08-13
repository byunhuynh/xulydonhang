package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const sttKey = "current_row"

// Store đọc/ghi số thứ tự đơn hàng (STT) từ một file dạng key=value
// (`current_row=N`). Đây là cơ chế lưu trữ mới cho bản Go; không có quan hệ
// tương thích với dữ liệu STT của bản Python hiện tại (bản Python đọc STT
// từ ô G1 trong dondathang.xlsx qua cơ chế khác).
type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) GetSTT() (int, error) {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != sttKey {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("config: invalid %s value %q: %w", sttKey, value, err)
		}
		return v, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 1, nil
}

func (s *Store) SetSTT(v int) error {
	content := fmt.Sprintf("%s=%d\n", sttKey, v)
	return os.WriteFile(s.path, []byte(content), 0o644)
}
