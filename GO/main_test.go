package main

import (
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

// App chi duoc chay MOT ban: mo lan thu hai phai danh thuc cua so dang co
// chu khong tao them mot cua so nua. Wails lo phan mutex va gui tin nhan
// giua hai tien trinh, nhung chi khi appOptions thuc su gan chot vao.
func TestAppOptions_ChotMotBanChay(t *testing.T) {
	opts := appOptions(&App{})

	if opts.SingleInstanceLock == nil {
		t.Fatal("SingleInstanceLock = nil, want da gan - thieu no thi moi lan mo la mot cua so moi")
	}
	if opts.SingleInstanceLock.UniqueId == "" {
		t.Error("UniqueId rong - Wails dung no lam ten mutex, rong thi khong khoa duoc gi")
	}
	if opts.SingleInstanceLock.OnSecondInstanceLaunch == nil {
		t.Error("OnSecondInstanceLaunch = nil - ban thu hai se tat lang le ma cua so cu khong hien len")
	}
}

// Ban thu hai co the gui tin den TRUOC khi ban dau chay xong startup, luc do
// a.ctx van con nil. runtime.WindowShow(nil) se panic, nen callback phai tu
// bo qua thay vi keo sap ban dang chay.
func TestOnSecondInstanceLaunch_ChuaCoCtxThiBoQua(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic khi ctx con nil: %v", r)
		}
	}()

	(&App{}).onSecondInstanceLaunch(options.SecondInstanceData{})
}
