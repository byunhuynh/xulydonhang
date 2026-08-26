package processing

import (
	"context"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func TestMockProcessor_ReturnsRowWithKnownVendorAndPO(t *testing.T) {
	p := &MockProcessor{Rand: rand.New(rand.NewSource(1)), Delay: 0}

	rows, err := p.Process(context.Background(), "/tmp/order1.pdf")
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.FileName != "order1.pdf" {
		t.Fatalf("FileName = %q, want %q", row.FileName, "order1.pdf")
	}
	// Số PO giả nay đếm bằng bộ đếm riêng của MockProcessor, không còn
	// mượn tham số stt của Processor (đã bỏ cùng bộ đếm config.txt).
	if row.PO != "PO000001" {
		t.Fatalf("PO = %q, want %q", row.PO, "PO000001")
	}

	next, err := p.Process(context.Background(), "/tmp/order2.pdf")
	if err != nil {
		t.Fatalf("lần gọi thứ hai lỗi: %v", err)
	}
	if next[0].PO != "PO000002" {
		t.Fatalf("PO lần hai = %q, want %q — mỗi dòng giả phải mang một PO khác nhau", next[0].PO, "PO000002")
	}

	found := false
	for _, v := range mockVendors {
		if v == row.System {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("System = %q, not in known vendor list", row.System)
	}

	switch row.StatusKind {
	case StatusKindDone, StatusKindWarning, StatusKindFailed:
	default:
		t.Fatalf("StatusKind = %q, want one of done/warning/failed", row.StatusKind)
	}
}

func TestMockProcessor_ContextCancelledReturnsError(t *testing.T) {
	p := &MockProcessor{Rand: rand.New(rand.NewSource(1)), Delay: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := p.Process(ctx, "/tmp/order1.pdf"); err == nil {
		t.Fatal("Process expected error when context is already cancelled, got nil")
	}
}

func TestProcessor_ProcessStreamingEmitsReturnedRows(t *testing.T) {
	p := &RealProcessor{}
	var emitted []OrderRow

	rows, err := p.ProcessStreaming(context.Background(), "missing.pdf", func(row OrderRow) {
		emitted = append(emitted, row)
	})
	if err != nil {
		t.Fatalf("ProcessStreaming returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ProcessStreaming returned %d rows, want 1", len(rows))
	}
	if !reflect.DeepEqual(emitted, rows) {
		t.Fatalf("emitted rows = %#v, want %#v", emitted, rows)
	}
}
