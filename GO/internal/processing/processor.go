package processing

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"time"
)

// Processor biến một file đầu vào thành một OrderRow. Phase 1 chỉ có
// MockProcessor; phase sau sẽ thêm RealProcessor implement cùng interface
// này để parse PDF thật theo từng vendor — App.ProcessFiles và frontend
// không cần đổi khi đó xảy ra.
type Processor interface {
	Process(ctx context.Context, filePath string, stt int) (OrderRow, error)
}

var mockVendors = []string{
	"Coop", "BigC", "Lotte", "Satra", "Emart", "Kingfood", "Winmart",
	"Fujimart", "BHX", "Farmer", "CN-HCM", "MR.DIY", "JIT", "JV-Mart", "JMART", "BC MART",
}

var mockStatuses = []string{StatusDone, StatusDone, StatusDone, StatusWarning, StatusFailed}

// MockProcessor giả lập xử lý đơn hàng: delay ngắn + dữ liệu mẫu ngẫu
// nhiên, để dựng và xác minh pipeline UI/event trước khi có parser PDF
// thật.
type MockProcessor struct {
	Rand  *rand.Rand
	Delay time.Duration
}

func NewMockProcessor() *MockProcessor {
	return &MockProcessor{
		Rand:  rand.New(rand.NewSource(time.Now().UnixNano())),
		Delay: 800 * time.Millisecond,
	}
}

func (m *MockProcessor) Process(ctx context.Context, filePath string, stt int) (OrderRow, error) {
	select {
	case <-time.After(m.Delay):
	case <-ctx.Done():
		return OrderRow{}, ctx.Err()
	}

	system := mockVendors[m.Rand.Intn(len(mockVendors))]
	status := mockStatuses[m.Rand.Intn(len(mockStatuses))]

	return OrderRow{
		FileName:    filepath.Base(filePath),
		Page:        "1",
		System:      system,
		MaKhachHang: fmt.Sprintf("MN_KH%04d", m.Rand.Intn(9999)),
		PO:          fmt.Sprintf("PO%06d", stt),
		DonGia:      fmt.Sprintf("%d", 10000+m.Rand.Intn(90000)),
		Status:      status,
	}, nil
}
