//go:build darwin

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// newOSClipboardImageReader returns the osascript-backed reader on macOS.
func newOSClipboardImageReader() clipboardImageReader { return osascriptImageReader{} }

// osascriptImageReader reads a clipboard image via AppleScript. osascript ships
// with macOS, so it needs no install (unlike pngpaste). pbpaste is unusable here
// because it only emits text representations, not raw image bytes.
type osascriptImageReader struct{}

// osascriptInfoCmd asks for the clipboard's available types, used to tell "no
// image" (silent text paste) from "extract failed" (toast).
func osascriptInfoCmd() *exec.Cmd {
	return exec.Command("osascript", "-e", "clipboard info")
}

// osascriptExtractCmd coerces the clipboard to PNG and writes it to path,
// truncating first. The try/close guard avoids leaving a dangling file handle
// when coercion fails (e.g. clipboard is not an image).
func osascriptExtractCmd(path string) *exec.Cmd {
	return exec.Command("osascript",
		"-e", "set p to POSIX file "+applescriptQuote(path),
		"-e", "try",
		"-e", "set png to (the clipboard as «class PNGf»)",
		"-e", "set fp to open for access p with write permission",
		"-e", "set eof fp to 0",
		"-e", "write png to fp",
		"-e", "close access fp",
		"-e", "on error errMsg",
		"-e", "try",
		"-e", "close access p",
		"-e", "end try",
		"-e", "error errMsg",
		"-e", "end try",
	)
}

// applescriptQuote wraps a path in AppleScript double quotes, escaping backslash
// and quote so a crafted filename cannot break out of the string literal.
func applescriptQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func (osascriptImageReader) ReadImage() ([]byte, string, error) {
	info, err := osascriptInfoCmd().Output()
	if err != nil {
		// osascript itself failed; on macOS it should always exist, so treat this
		// as "no image" rather than spamming a toast.
		return nil, "", nil
	}
	types := string(info)
	if !strings.Contains(types, "PNGf") && !strings.Contains(types, "TIFF") {
		return nil, "", nil // no image on the clipboard
	}

	f, err := os.CreateTemp("", "tele-clip-*.png")
	if err != nil {
		return nil, "", err
	}
	path := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()

	if err := osascriptExtractCmd(path).Run(); err != nil {
		return nil, "", fmt.Errorf("osascript extract: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("clipboard image was empty")
	}
	return data, ".png", nil
}
