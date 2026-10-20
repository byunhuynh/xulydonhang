package processing

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf16"

	"github.com/richardlehane/mscfb"
	"golang.org/x/text/encoding/charmap"
)

// readDocText lay chu tu mot file Word 97-2003 (.doc).
//
// Coop co dot gui bao cao JDA luu bang Word doi cu (.doc) thay vi .docx -
// cung bao cao, cung font Courier New, chi khac dinh dang luu. Day la file
// nhi phan OLE, khong phai zip, nen readDocxText khong doc duoc.
//
// Chu cua tai lieu nam trong luong WordDocument, chia thanh cac "piece" ma
// bang vi tri cua chung (CLX/PlcPcd) nam o luong 0Table hoac 1Table tuy co
// fWhichTblStm cua FIB. Moi piece la ky tu 8-bit (cp1252) hoac UTF-16LE.
// Chi lay phan van ban chinh (ccpText ky tu dau) - phan sau la header,
// footer, chu thich..., khong thuoc bao cao.
func readDocText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("khong mo duoc file Word: %w", err)
	}
	defer f.Close()

	doc, err := mscfb.New(f)
	if err != nil {
		return "", fmt.Errorf("file Word .doc bi hong hoac khong dung dinh dang: %w", err)
	}
	streams := map[string][]byte{}
	for entry, err := doc.Next(); err == nil; entry, err = doc.Next() {
		switch entry.Name {
		case "WordDocument", "0Table", "1Table":
			data, readErr := io.ReadAll(entry)
			if readErr != nil {
				return "", fmt.Errorf("khong doc duoc noi dung file Word: %w", readErr)
			}
			streams[entry.Name] = data
		}
	}

	wd := streams["WordDocument"]
	const (
		fibFlagsOffset   = 0x0A
		fibCcpTextOffset = 0x4C
		fibFcClxOffset   = 0x01A2
		fibLcbClxOffset  = 0x01A6
		fEncrypted       = 0x0100
		fWhichTblStm     = 0x0200
	)
	if len(wd) < fibLcbClxOffset+4 || binary.LittleEndian.Uint16(wd) != 0xA5EC {
		return "", fmt.Errorf("file .doc khong phai tai lieu Word 97-2003")
	}
	flags := binary.LittleEndian.Uint16(wd[fibFlagsOffset:])
	if flags&fEncrypted != 0 {
		return "", fmt.Errorf("file Word .doc dang dat mat khau")
	}
	tableName := "0Table"
	if flags&fWhichTblStm != 0 {
		tableName = "1Table"
	}
	table := streams[tableName]
	ccpText := int(binary.LittleEndian.Uint32(wd[fibCcpTextOffset:]))
	fcClx := int(binary.LittleEndian.Uint32(wd[fibFcClxOffset:]))
	lcbClx := int(binary.LittleEndian.Uint32(wd[fibLcbClxOffset:]))
	if fcClx < 0 || lcbClx <= 0 || fcClx+lcbClx > len(table) {
		return "", fmt.Errorf("file Word .doc thieu bang vi tri van ban")
	}

	plc, err := docPieceTable(table[fcClx : fcClx+lcbClx])
	if err != nil {
		return "", err
	}
	raw, err := docPieceText(wd, plc, ccpText)
	if err != nil {
		return "", err
	}
	return cleanDocText(raw), nil
}

// docPieceTable tim PlcPcd trong CLX: bo qua cac Prc (0x01, kich thuoc
// int16) dung truoc, lay Pcdt (0x02, kich thuoc uint32).
func docPieceTable(clx []byte) ([]byte, error) {
	for i := 0; i < len(clx); {
		switch clx[i] {
		case 0x01:
			if i+3 > len(clx) {
				return nil, fmt.Errorf("bang vi tri van ban cua file .doc bi hong")
			}
			i += 3 + int(int16(binary.LittleEndian.Uint16(clx[i+1:])))
		case 0x02:
			if i+5 > len(clx) {
				return nil, fmt.Errorf("bang vi tri van ban cua file .doc bi hong")
			}
			n := int(binary.LittleEndian.Uint32(clx[i+1:]))
			if i+5+n > len(clx) {
				return nil, fmt.Errorf("bang vi tri van ban cua file .doc bi hong")
			}
			return clx[i+5 : i+5+n], nil
		default:
			return nil, fmt.Errorf("bang vi tri van ban cua file .doc bi hong")
		}
	}
	return nil, fmt.Errorf("file .doc khong co bang vi tri van ban")
}

// docPieceText ghep chu cua tung piece theo thu tu, dung lai sau limit ky
// tu. PlcPcd la (n+1) moc CP 4 byte roi n Pcd 8 byte; fc cua Pcd co bit
// 0x40000000 la piece 8-bit nam o fc/2, khong co la UTF-16LE nam o fc.
func docPieceText(wd, plc []byte, limit int) (string, error) {
	if len(plc) < 4 || (len(plc)-4)%12 != 0 {
		return "", fmt.Errorf("bang vi tri van ban cua file .doc bi hong")
	}
	n := (len(plc) - 4) / 12
	var b strings.Builder
	written := 0
	for k := 0; k < n && written < limit; k++ {
		cpStart := int(binary.LittleEndian.Uint32(plc[4*k:]))
		cpEnd := int(binary.LittleEndian.Uint32(plc[4*(k+1):]))
		count := cpEnd - cpStart
		if count <= 0 {
			continue
		}
		if written+count > limit {
			count = limit - written
		}
		fc := binary.LittleEndian.Uint32(plc[4*(n+1)+8*k+2:])
		if fc&0x40000000 != 0 {
			off := int((fc &^ 0x40000000) / 2)
			if off+count > len(wd) {
				return "", fmt.Errorf("van ban cua file .doc bi cat cut")
			}
			s, err := charmap.Windows1252.NewDecoder().Bytes(wd[off : off+count])
			if err != nil {
				return "", fmt.Errorf("khong giai ma duoc van ban file .doc: %w", err)
			}
			b.Write(s)
		} else {
			off := int(fc)
			if off+2*count > len(wd) {
				return "", fmt.Errorf("van ban cua file .doc bi cat cut")
			}
			units := make([]uint16, count)
			for i := range units {
				units[i] = binary.LittleEndian.Uint16(wd[off+2*i:])
			}
			b.WriteString(string(utf16.Decode(units)))
		}
		written += count
	}
	return b.String(), nil
}

// cleanDocText doi ky tu dieu khien cua Word ve van ban thuong, giong cach
// documentXMLText cho ra moi doan van mot dong: het doan (\r), xuong dong
// (\v) va ngat trang (\f) deu thanh \n. Ma truong (giua 0x13 va 0x14) bi bo,
// chi giu ket qua hien thi cua truong.
func cleanDocText(raw string) string {
	var b strings.Builder
	var fieldInCode []bool
	for _, r := range raw {
		switch r {
		case 0x13:
			fieldInCode = append(fieldInCode, true)
			continue
		case 0x14:
			if len(fieldInCode) > 0 {
				fieldInCode[len(fieldInCode)-1] = false
			}
			continue
		case 0x15:
			if len(fieldInCode) > 0 {
				fieldInCode = fieldInCode[:len(fieldInCode)-1]
			}
			continue
		}
		if len(fieldInCode) > 0 && fieldInCode[len(fieldInCode)-1] {
			continue
		}
		switch {
		case r == '\r' || r == '\v' || r == '\f':
			b.WriteByte('\n')
		case r == '\t' || r >= 0x20:
			b.WriteRune(r)
		}
	}
	return b.String()
}
