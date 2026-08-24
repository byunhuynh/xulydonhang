package processing

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"time"
)

// Processor biến một file đầu vào thành một hoặc nhiều OrderRow (một file
// Coop PDF có thể chứa nhiều đơn hàng trên cùng một trang). Phase 1 chỉ có
// MockProcessor; RealProcessor implement cùng interface này để parse PDF
// thật theo từng vendor — App.ProcessFiles và frontend không cần đổi khi
// đó xảy ra.
type Processor interface {
	Process(ctx context.Context, filePath string, stt int) ([]OrderRow, error)
}

// StreamingProcessor optionally reports completed rows while processing is
// still in progress. Processor remains the required contract so existing
// processors continue to work unchanged.
type StreamingProcessor interface {
	ProcessStreaming(ctx context.Context, filePath string, stt int, emit func(OrderRow)) ([]OrderRow, error)
}

var mockVendors = []string{
	"Coop", "BigC", "Lotte", "Satra", "Emart", "Kingfood", "Winmart",
	"Fujimart", "BHX", "Farmer", "CN-HCM", "MR.DIY", "JIT", "JV-Mart", "JMART", "BC MART",
}

// mockOutcome ghép một chuỗi Status hiển thị (giữ emoji cho người dùng)
// với StatusKind kiểu (typed) tương ứng để frontend phân loại màu/icon.
type mockOutcome struct {
	status string
	kind   string
}

var mockOutcomes = []mockOutcome{
	{StatusDone, StatusKindDone},
	{StatusDone, StatusKindDone},
	{StatusDone, StatusKindDone},
	{StatusWarning, StatusKindWarning},
	{StatusFailed, StatusKindFailed},
}

// MockProcessor giả lập xử lý đơn hàng: delay ngắn + dữ liệu mẫu ngẫu
// nhiên, để dựng và xác minh pipeline UI/event trước khi có parser PDF
// thật.
type MockProcessor struct {
	Rand  *rand.Rand
	Delay time.Duration
	mu    sync.Mutex
}

func NewMockProcessor() *MockProcessor {
	return &MockProcessor{
		Rand:  rand.New(rand.NewSource(time.Now().UnixNano())),
		Delay: 800 * time.Millisecond,
	}
}

func (m *MockProcessor) Process(ctx context.Context, filePath string, stt int) ([]OrderRow, error) {
	select {
	case <-time.After(m.Delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	system := mockVendors[m.Rand.Intn(len(mockVendors))]
	outcome := mockOutcomes[m.Rand.Intn(len(mockOutcomes))]

	return []OrderRow{{
		FileName:    filepath.Base(filePath),
		Page:        "1",
		System:      system,
		MaKhachHang: fmt.Sprintf("MN_KH%04d", m.Rand.Intn(9999)),
		PO:          fmt.Sprintf("PO%06d", stt),
		DonGia:      fmt.Sprintf("%d", 10000+m.Rand.Intn(90000)),
		Status:      outcome.status,
		StatusKind:  outcome.kind,
	}}, nil
}
