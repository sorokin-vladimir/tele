//go:build !darwin

package ui

// newOSClipboardImageReader returns the noop reader on platforms without a
// clipboard image reader yet (Linux/Windows), so paste degrades to text.
func newOSClipboardImageReader() clipboardImageReader { return noopClipboardImageReader{} }
