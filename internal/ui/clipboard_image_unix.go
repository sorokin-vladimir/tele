//go:build linux || freebsd || openbsd || netbsd || dragonfly

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// newOSClipboardImageReader returns the Wayland/X11-backed reader on Linux and
// the BSDs, which share the same clipboard tools (wl-clipboard / xclip).
func newOSClipboardImageReader() clipboardImageReader { return unixClipboardImageReader{} }

// unixClipboardImageReader reads a clipboard image by shelling out to the
// session's clipboard tool: wl-paste on Wayland, xclip on X11. Both emit the raw
// image bytes on stdout, so no temp file is needed. A missing tool degrades to a
// text paste (no toast); an image that is present but fails to extract reports an
// error so the caller can toast.
type unixClipboardImageReader struct{}

// pngMIME is the clipboard target we source, matching the macOS reader's PNG.
const pngMIME = "image/png"

// wlPasteListCmd lists the MIME types currently offered on the Wayland clipboard.
func wlPasteListCmd() *exec.Cmd { return exec.Command("wl-paste", "--list-types") }

// wlPasteExtractCmd reads the clipboard image as PNG. --no-newline is required so
// wl-paste does not append a trailing byte to the binary output.
func wlPasteExtractCmd() *exec.Cmd {
	return exec.Command("wl-paste", "--no-newline", "--type", pngMIME)
}

// xclipTargetsCmd lists the targets (MIME types) on the X11 clipboard selection.
func xclipTargetsCmd() *exec.Cmd {
	return exec.Command("xclip", "-selection", "clipboard", "-t", "TARGETS", "-o")
}

// xclipExtractCmd reads the clipboard image as PNG from the X11 selection.
func xclipExtractCmd() *exec.Cmd {
	return exec.Command("xclip", "-selection", "clipboard", "-t", pngMIME, "-o")
}

// clipboardTypesHavePNG reports whether a newline-separated MIME/target list
// advertises image/png.
func clipboardTypesHavePNG(list string) bool {
	for _, line := range strings.Split(list, "\n") {
		if strings.TrimSpace(line) == pngMIME {
			return true
		}
	}
	return false
}

func (unixClipboardImageReader) ReadImage() ([]byte, string, error) {
	// Prefer Wayland: on a Wayland session DISPLAY may also be set (XWayland), so
	// WAYLAND_DISPLAY takes precedence.
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return readClipboardImagePNG(wlPasteListCmd(), wlPasteExtractCmd())
	}
	if os.Getenv("DISPLAY") != "" {
		return readClipboardImagePNG(xclipTargetsCmd(), xclipExtractCmd())
	}
	return nil, "", nil // no graphical session -> no reader
}

// readClipboardImagePNG runs the list command to check for an image/png target,
// then the extract command to fetch the bytes. A failing list command (e.g. the
// tool is not installed) degrades silently to a text paste; a failing extract of
// an advertised image reports an error so the caller can toast.
func readClipboardImagePNG(list, extract *exec.Cmd) ([]byte, string, error) {
	types, err := list.Output()
	if err != nil {
		// Tool missing or clipboard unavailable: fall back to text, no toast.
		return nil, "", nil //nolint:nilerr // documented above: this degrades to a text paste
	}
	if !clipboardTypesHavePNG(string(types)) {
		return nil, "", nil // no image on the clipboard
	}
	data, err := extract.Output()
	if err != nil {
		return nil, "", fmt.Errorf("clipboard image extract: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("clipboard image was empty")
	}
	return data, ".png", nil
}
