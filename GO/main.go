package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app, err := NewApp()
	if err != nil {
		reportFatalError("khởi động ứng dụng", err)
		return
	}

	err = wails.Run(&options.App{
		Title:     "Blue Hà Thành - Order System v3.0",
		Width:     1100,
		Height:    850,
		MinWidth:  900,
		MinHeight: 800,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
	})

	if err != nil {
		reportFatalError("chạy ứng dụng", err)
	}
}

// reportFatalError makes a fatal startup/runtime error visible even on
// a built Windows release, which Wails links with -H windowsgui (no
// console) - println's stderr goes nowhere there, so a user whose
// data.xlsx/settings.ini is missing, locked, or unreachable within
// resolveRepoFile's directory walk previously got a double-click that
// silently did nothing.
//
// There's no Wails ctx/webview yet when NewApp() can fail, so
// runtime.MessageDialog (which requires an active Wails context) isn't
// usable here, and adding a new dependency just for a message box is
// out of scope. This writes the error, timestamped, to a small
// dedicated log file next to the app's other resolved-by-directory-walk
// files (data.xlsx/settings.ini) - not the pre-existing root-level
// log.log, which is xulydonhang.py's own structured per-order run log
// in a completely different table format; appending an unrelated Go
// crash line there would corrupt that log rather than help anyone
// read it - in addition to still printing, so `wails dev`/any
// console-attached run keeps seeing it exactly as before.
func reportFatalError(stage string, err error) {
	msg := fmt.Sprintf("[%s] Error (%s): %v", time.Now().Format("2006-01-02 15:04:05"), stage, err)
	println(msg)

	logPath := filepath.Join(resolveRepoDir("settings.ini"), "go_startup_error.log")
	f, openErr := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		println("Error: could not write startup error log:", openErr.Error())
		return
	}
	defer f.Close()
	fmt.Fprintln(f, msg)
}
