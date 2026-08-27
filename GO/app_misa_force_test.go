package main

import (
	"context"
	"errors"
	"testing"

	"order-processor/internal/misa"
	"order-processor/internal/misapush"
)

// pusherConDongHong gia lap dung tinh huong nguoi dung gap: MISA doc duoc
// file, chi ra 40/60 chung tu hong, va CHAN ghi so vi Force tat.
type pusherConDongHong struct {
	sawForce []bool
}

func (p *pusherConDongHong) Push(_ context.Context, req misapush.Request) (*misa.ImportResult, error) {
	p.sawForce = append(p.sawForce, req.Force)
	res := &misa.ImportResult{
		RowsParsed: 60,
		ValidRows:  20,
		RowErrors: []misa.RowError{
			{Row: 1, RefNo: "ĐĐHCOOP-103617493-00", Description: "Số đơn hàng đã tồn tại trên phần mềm"},
			{Row: 2, RefNo: "ĐĐHCOOP-103617494-00", Description: "Số đơn hàng đã tồn tại trên phần mềm"},
		},
	}
	if req.Force {
		res.Committed, res.Valid, res.Skipped = true, 20, 40
		return res, nil
	}
	return res, errors.New("40/60 chứng từ không hợp lệ, không ghi sổ")
}

func pushedEvent(t *testing.T, events []emittedEvent) map[string]any {
	t.Helper()
	for _, e := range events {
		if e.name == "misa:pushed" {
			m, ok := e.data[0].(map[string]any)
			if !ok {
				t.Fatalf("misa:pushed data khong phai map: %#v", e.data)
			}
			return m
		}
	}
	t.Fatal("khong co su kien misa:pushed nao")
	return nil
}

func TestPushMisa_ConDongHongThiDeNghiGhiPhanHopLe(t *testing.T) {
	pusher := &pusherConDongHong{}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(emitter, []MisaPushJob{
		{PO: "A", RouteKey: "COOPFOOD", Branch: misapush.BranchHTLA, ExcelRows: []int{9, 10}},
	}, false)

	ev := pushedEvent(t, emitter.events)
	if ev["ok"] != false {
		t.Errorf("ok = %v, want false", ev["ok"])
	}
	if ev["canForce"] != true {
		t.Errorf("canForce = %v, want true — nguoi dung phai duoc de nghi ghi phan hop le", ev["canForce"])
	}
	if ev["valid"] != 20 || ev["invalid"] != 2 {
		t.Errorf("valid/invalid = %v/%v, want 20/2 — giao dien can con so de ghi tren nut", ev["valid"], ev["invalid"])
	}
	if len(pusher.sawForce) != 1 || pusher.sawForce[0] {
		t.Errorf("Request.Force = %v, want false o luot dau", pusher.sawForce)
	}
}

func TestPushMisa_ForceGhiPhanHopLeVaKhongDeNghiLai(t *testing.T) {
	pusher := &pusherConDongHong{}
	app, emitter := newTestAppForPush(t, pusher, defaultMisaCfg())

	app.runMisaPush(emitter, []MisaPushJob{
		{PO: "A", RouteKey: "COOPFOOD", Branch: misapush.BranchHTLA, ExcelRows: []int{9, 10}},
	}, true)

	if len(pusher.sawForce) != 1 || !pusher.sawForce[0] {
		t.Fatalf("Request.Force = %v, want true", pusher.sawForce)
	}
	ev := pushedEvent(t, emitter.events)
	if ev["ok"] != true {
		t.Errorf("ok = %v, want true", ev["ok"])
	}
	if ev["canForce"] == true {
		t.Error("canForce = true sau khi da Force — se moi nguoi dung bam mai mot viec da lam")
	}
}

func TestPushMisa_LoiKhongPhaiDongHongThiKhongDeNghiForce(t *testing.T) {
	// Thieu ten bo du lieu: Force khong cuu duoc gi, de nghi o day chi
	// khien nguoi dung bam vo ich.
	pusher := &fakePusher{}
	app, emitter := newTestAppForPush(t, pusher, map[string]string{"db_ha_thanh": "HÀ THÀNH"})

	app.runMisaPush(emitter, []MisaPushJob{
		{PO: "A", RouteKey: "COOPFOOD", Branch: misapush.BranchHTLA, ExcelRows: []int{9}},
	}, false)

	ev := pushedEvent(t, emitter.events)
	if ev["canForce"] == true {
		t.Error("canForce = true cho loi thieu bo du lieu — Force khong lien quan")
	}
}
