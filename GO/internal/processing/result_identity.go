package processing

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"strings"
)

// SourceIDForPath returns an opaque, stable identity for one physical input
// path. FileName remains the display basename; this value exists solely to
// keep equal basenames from different directories independent.
func SourceIDForPath(filePath string) string {
	if absolute, err := filepath.Abs(filePath); err == nil {
		filePath = absolute
	}
	filePath = filepath.Clean(filePath)
	if runtime.GOOS == "windows" {
		filePath = strings.ToLower(filePath)
	}
	digest := sha256.Sum256([]byte(filePath))
	return hex.EncodeToString(digest[:])
}

func orderResultKey(sourceID, position, po string) string {
	return sourceID + "|" + position + "|" + po
}

func emitIdentifiedOrderRow(emit func(OrderRow), filePath, position string, row OrderRow) OrderRow {
	row.SourceID = SourceIDForPath(filePath)
	row.ResultKey = orderResultKey(row.SourceID, position, row.PO)
	return emitOrderRow(emit, row)
}
