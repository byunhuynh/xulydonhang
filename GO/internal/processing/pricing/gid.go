package pricing

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var gidBlockPattern = regexp.MustCompile(`(?s)<gid>(.*?)</gid>`)

// LoadGidMap reads the <gid>...</gid> block from settings.ini — a
// bespoke tag, not real XML or an INI section — and returns a map of
// sheet name -> Google Sheets gid, mirroring xulydonhang.py's get_gid.
// A line inside the block with more than one "=" is an error, matching
// Python's `key, value = line.split("=")` unpacking failure.
func LoadGidMap(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("pricing: read %s: %w", path, err)
	}

	match := gidBlockPattern.FindStringSubmatch(string(content))
	if match == nil {
		return map[string]string{}, nil
	}

	gidMap := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(match[1]), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.Split(line, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("pricing: malformed <gid> line (expected exactly one '='): %q", line)
		}
		gidMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return gidMap, nil
}
