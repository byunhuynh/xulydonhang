package zalosend

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"order-processor/internal/zalosend/richtext"
)

func TestFindBrowserExecutableFallsBackToEdge(t *testing.T) {
	programFiles := `C:\Program Files`
	edgePath := filepath.Join(programFiles, "Microsoft", "Edge", "Application", "msedge.exe")

	got, err := findBrowserExecutable(
		func(key string) string {
			if key == "ProgramFiles" {
				return programFiles
			}
			return ""
		},
		func(path string) bool { return path == edgePath },
		func(string) (string, error) { return "", filepath.ErrBadPattern },
	)
	if err != nil {
		t.Fatalf("findBrowserExecutable returned error: %v", err)
	}
	if got != edgePath {
		t.Fatalf("findBrowserExecutable = %q, want Edge at %q", got, edgePath)
	}
}

func TestWaitForPastedContentWaitsUntilAllLinesArePresent(t *testing.T) {
	lines := richtext.ParseDocument("**ĐƠN HÀNG BIGC**\n- Mã A\n- Mã B")
	reads := []string{"ĐƠN HÀNG BIGC\nMã A", "ĐƠN HÀNG BIGC\n• Mã A\n• Mã B"}
	readIndex := 0
	var sleeps []time.Duration

	err := waitForPastedContent(
		lines,
		func() (string, error) {
			value := reads[readIndex]
			if readIndex < len(reads)-1 {
				readIndex++
			}
			return value, nil
		},
		func(delay time.Duration) error {
			sleeps = append(sleeps, delay)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if readIndex != 1 {
		t.Fatalf("readIndex = %d, want 1 (must re-check after incomplete paste)", readIndex)
	}
	wantSleeps := []time.Duration{time.Second, 200 * time.Millisecond}
	if !reflect.DeepEqual(sleeps, wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", sleeps, wantSleeps)
	}
}

func TestStartFirstWorkingBrowserFallsBackWhenChromeCannotStart(t *testing.T) {
	chromePath := `C:\Program Files\Google\Chrome\Application\chrome.exe`
	edgePath := `C:\Program Files\Microsoft\Edge\Application\msedge.exe`
	var attempts []string

	got, err := startFirstWorkingBrowser([]string{chromePath, edgePath}, func(path string) error {
		attempts = append(attempts, path)
		if path == chromePath {
			return errors.New("chrome failed to start")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("startFirstWorkingBrowser returned error: %v", err)
	}
	if got != edgePath {
		t.Fatalf("startFirstWorkingBrowser = %q, want %q", got, edgePath)
	}
	if want := []string{chromePath, edgePath}; !reflect.DeepEqual(attempts, want) {
		t.Fatalf("browser attempts = %#v, want %#v", attempts, want)
	}
}

func TestResolveProfileDirAnchorsRelativePathBesideExecutable(t *testing.T) {
	got, err := resolveProfileDir("zalo_profile", func() (string, error) {
		return `D:\OrderApp\order-processor.exe`, nil
	})
	if err != nil {
		t.Fatalf("resolveProfileDir returned error: %v", err)
	}
	want := filepath.Join(`D:\OrderApp`, "zalo_profile")
	if got != want {
		t.Fatalf("resolveProfileDir = %q, want %q", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("resolveProfileDir returned relative path %q", got)
	}
}
