package processing

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

func TestMockProcessor_ReturnsRowWithKnownVendorAndPO(t *testing.T) {
	p := &MockProcessor{Rand: rand.New(rand.NewSource(1)), Delay: 0}

	rows, err := p.Process(context.Background(), "/tmp/order1.pdf", 108)
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
	if row.PO != "PO000108" {
		t.Fatalf("PO = %q, want %q", row.PO, "PO000108")
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

	if _, err := p.Process(ctx, "/tmp/order1.pdf", 1); err == nil {
		t.Fatal("Process expected error when context is already cancelled, got nil")
	}
}
