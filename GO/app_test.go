package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"order-processor/internal/config"
	"order-processor/internal/processing"
)

type fakeEmitter struct {
	events []emittedEvent
}

type emittedEvent struct {
	name string
	data []interface{}
}

func (f *fakeEmitter) Emit(name string, data ...interface{}) {
	f.events = append(f.events, emittedEvent{name: name, data: data})
}

type stubProcessor struct {
	failOn string
}

func (s *stubProcessor) Process(ctx context.Context, filePath string, stt int) (processing.OrderRow, error) {
	if filePath == s.failOn {
		return processing.OrderRow{}, errors.New("stub failure")
	}
	return processing.OrderRow{FileName: filePath, PO: "PO1", Status: processing.StatusDone}, nil
}

func TestRunBatch_EmitsLogRowPerFileThenDone(t *testing.T) {
	cfg := config.NewStore(filepath.Join(t.TempDir(), "config.txt"))
	a := &App{cfg: cfg, processor: &stubProcessor{}}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"a.pdf", "b.pdf"}, 10)

	wantNames := []string{"process:log", "process:row", "process:log", "process:row", "process:done"}
	if len(emitter.events) != len(wantNames) {
		t.Fatalf("got %d events, want %d: %+v", len(emitter.events), len(wantNames), emitter.events)
	}
	for i, want := range wantNames {
		if emitter.events[i].name != want {
			t.Fatalf("event[%d] = %q, want %q", i, emitter.events[i].name, want)
		}
	}

	gotSTT, err := cfg.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if gotSTT != 12 {
		t.Fatalf("STT after batch = %d, want 12", gotSTT)
	}
}

func TestRunBatch_FileErrorEmitsLogAndContinues(t *testing.T) {
	cfg := config.NewStore(filepath.Join(t.TempDir(), "config.txt"))
	a := &App{cfg: cfg, processor: &stubProcessor{failOn: "bad.pdf"}}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"bad.pdf", "good.pdf"}, 1)

	wantNames := []string{"process:log", "process:log", "process:log", "process:row", "process:done"}
	if len(emitter.events) != len(wantNames) {
		t.Fatalf("got %d events, want %d: %+v", len(emitter.events), len(wantNames), emitter.events)
	}

	gotSTT, err := cfg.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if gotSTT != 3 {
		t.Fatalf("STT after batch = %d, want 3", gotSTT)
	}
}
