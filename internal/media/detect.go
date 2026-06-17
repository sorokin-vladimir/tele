package media

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

// DetectMIME resolves a local file's MIME type. It first tries the file extension
// via the stdlib mime table; if the extension is unknown it sniffs the first 512
// bytes with http.DetectContentType. No hand-rolled type switch — #131 standardizes
// the reverse (MIME -> ext) direction on the same stdlib mime package.
func DetectMIME(path string) (string, error) {
	if ext := filepath.Ext(path); ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			return NormalizeMIME(t), nil
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("detect mime: %w", err)
	}
	defer func() { _ = f.Close() }()

	var buf [512]byte
	n, _ := f.Read(buf[:]) // a short read (or io.EOF) is fine; sniff what we have
	return NormalizeMIME(http.DetectContentType(buf[:n])), nil
}
