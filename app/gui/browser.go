package gui

import (
	"os/exec"
	"runtime"
)

func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// OpenInFileManager reveals dir in the native file manager
// (Finder / Explorer / the default Linux file manager).
//
// Note: on Windows explorer.exe returns a non-zero exit code even when it
// successfully opens a window, so we only check that the process starts
// (Start, not Run) and never interpret its exit status.
func OpenInFileManager(dir string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", dir).Start()
	case "windows":
		return exec.Command("explorer.exe", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}
