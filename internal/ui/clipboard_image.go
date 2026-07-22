package ui

// clipboardImageReader sources an image off the system clipboard.
//
//	ok=false, err=nil  -> no image on the clipboard (caller does a text paste)
//	err != nil         -> reader/utility failed (caller shows a toast + text paste)
type clipboardImageReader interface {
	// ReadImage returns image bytes and a file extension (e.g. ".png").
	ReadImage() (data []byte, ext string, err error)
}

// noopClipboardImageReader always reports "no image", so platforms without a
// reader degrade silently to the text paste path.
type noopClipboardImageReader struct{}

func (noopClipboardImageReader) ReadImage() ([]byte, string, error) { return nil, "", nil }

// clipboardImageReaderFunc adapts a func to the interface for tests.
type clipboardImageReaderFunc func() ([]byte, string, error)

func (f clipboardImageReaderFunc) ReadImage() ([]byte, string, error) { return f() }

// clipImageReader is the active reader. newOSClipboardImageReader is defined
// per-platform (see clipboard_image_darwin.go / clipboard_image_other.go):
// osascript on darwin, noop elsewhere.
var clipImageReader clipboardImageReader = newOSClipboardImageReader()

// SetClipboardImageReaderForTest swaps the reader and returns a restore func, so
// tests exercise staging without touching the real clipboard.
func SetClipboardImageReaderForTest(fn func() ([]byte, string, error)) func() {
	prev := clipImageReader
	clipImageReader = clipboardImageReaderFunc(fn)
	return func() { clipImageReader = prev }
}

// ReadClipboardImageForTest exposes the active reader to external tests.
func ReadClipboardImageForTest() ([]byte, string, error) { return clipImageReader.ReadImage() }
