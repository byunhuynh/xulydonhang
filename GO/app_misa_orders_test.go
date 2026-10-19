package main

import (
	"context"
	"errors"
	"testing"

	"order-processor/internal/misa"
	"order-processor/internal/misapush"
)

// pusherTheoKichBan trả về đúng kết quả mà test dựng sẵn.
type pusherTheoKichBan struct {
	respond func(req misapush.Request) (*misa.ImportResult, error)
}

func (p *pusherTheoKichBan) Push(_ context.Context, req misapush.Request) (*misa.ImportResult, error) {
	return p.respond(req)
}

// lastOrderStatuses gom trạng thái CUỐI của từng đơn từ các sự kiện
// misa:orders — đúng cách store phía frontend gộp chúng.
func lastOrderStatuses(t *testing.T, events []emittedEvent) map[string]MisaOrderStatus {
	t.Helper()
	out := map[string]MisaOrderStatus{}
	for _, e := range events {
		if e.name != "misa:orders" {
			continue
		}
		list, ok := e.data[0].([]MisaOrderStatus)
		if !ok {
			t.Fatalf("misa:orders data không phải []MisaOrderStatus: %#v", e.data)
		}
		for _, s := range list {
			out[s.Key] = s
		}
	}
	return out
}

// Số đơn hàng (cột B) của workbook test là "PO-<dòng>" - xem seedPushWorkbook.
func dongHongPO10(force bool) (*misa.ImportResult, error) {
	res := &misa.ImportResult{
		RowsParsed: 3,
		ValidRows:  2,
		RowErrors:  []misa.RowError{{Row: 2, RefNo: "PO-10", Description: "Số đơn hàng đã tồn tại trên phần mềm"}},
	}
	if force {
		res.CommitStarted, res.Committed, res.Valid, res.Skipped = true, true, 2, 1
		return res, nil
	}
	return res, errors.New("1/3 chứng từ không hợp lệ, không ghi sổ")
}

func TestPushMisa_BaoDungDonDaTonTaiConLaiChoGhi(t *testing.T) {
	pusher := &pusherTheoKichBan{respond: func(req misapush.Request) (*misa.ImportResult, error) {
		return dongHongPO10(req.Force)
	}}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(emitter, []MisaPushJob{
		{Key: "A", PO: "A", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
		{Key: "B", PO: "B", Branch: misapush.BranchHTLA, ExcelRows: []int{10}},
		{Key: "C", PO: "C", Branch: misapush.BranchHTLA, ExcelRows: []int{11, 12}},
	}, false)

	got := lastOrderStatuses(t, emitter.events)
	if got["B"].Status != misaOrderExists {
		t.Errorf("B = %+v, want exists — đây là đơn MISA báo trùng", got["B"])
	}
	for _, k := range []string{"A", "C"} {
		if got[k].Status != misaOrderHeld {
			t.Errorf("%s = %+v, want held — đơn hợp lệ, chỉ bị chặn vì B", k, got[k])
		}
	}
}

func TestPushMisa_ForceThiDonHopLeVaoSoDonLoiBiBoQua(t *testing.T) {
	pusher := &pusherTheoKichBan{respond: func(req misapush.Request) (*misa.ImportResult, error) {
		return dongHongPO10(req.Force)
	}}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(emitter, []MisaPushJob{
		{Key: "A", PO: "A", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
		{Key: "B", PO: "B", Branch: misapush.BranchHTLA, ExcelRows: []int{10}},
	}, true)

	got := lastOrderStatuses(t, emitter.events)
	if got["A"].Status != misaOrderOK {
		t.Errorf("A = %+v, want ok", got["A"])
	}
	if got["B"].Status != misaOrderExists {
		t.Errorf("B = %+v, want exists", got["B"])
	}
}

func TestPushMisa_LoiDangNhapThiMoiDonCungBaoThatBai(t *testing.T) {
	pusher := &pusherTheoKichBan{respond: func(misapush.Request) (*misa.ImportResult, error) {
		return nil, errors.New("đăng nhập MISA: phiên hết hạn")
	}}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(emitter, []MisaPushJob{
		{Key: "A", PO: "A", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
		{Key: "B", PO: "B", Branch: misapush.BranchHTLA, ExcelRows: []int{10}},
	}, false)

	got := lastOrderStatuses(t, emitter.events)
	for _, k := range []string{"A", "B"} {
		if got[k].Status != misaOrderFailed || got[k].Message == "" {
			t.Errorf("%s = %+v, want failed kèm lý do", k, got[k])
		}
	}
}

func TestPushMisa_LoiGiuaLucGhiSoThiChuaRo(t *testing.T) {
	pusher := &pusherTheoKichBan{respond: func(misapush.Request) (*misa.ImportResult, error) {
		return &misa.ImportResult{RowsParsed: 1, ValidRows: 1, CommitStarted: true},
			errors.New("step4: quá 5m0s mà tiến trình nền chưa xong")
	}}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(emitter, []MisaPushJob{
		{Key: "A", PO: "A", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
	}, false)

	if got := lastOrderStatuses(t, emitter.events)["A"]; got.Status != misaOrderUnknown {
		t.Errorf("A = %+v, want unknown — MISA có thể đã ghi, không được báo là chưa ghi", got)
	}
}

func TestPushMisa_BaoDangDayTruocKhiCoKetQua(t *testing.T) {
	app, emitter := newTestAppForPush(t, &fakePusher{}, defaultMisaCfg())

	app.runMisaPush(emitter, []MisaPushJob{
		{Key: "A", PO: "A", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
		{Key: "B", PO: "B", Branch: misapush.BranchHaThanh, ExcelRows: []int{10}},
	}, false)

	var first []MisaOrderStatus
	for _, e := range emitter.events {
		if e.name == "misa:orders" {
			first = e.data[0].([]MisaOrderStatus)
			break
		}
	}
	if len(first) != 1 || first[0].Status != misaOrderPushing {
		t.Errorf("sự kiện misa:orders đầu tiên = %+v, want đúng một đơn đang đẩy", first)
	}
	got := lastOrderStatuses(t, emitter.events)
	if got["A"].Status != misaOrderOK || got["B"].Status != misaOrderOK {
		t.Errorf("trạng thái cuối = %+v, want cả hai ok", got)
	}
}
