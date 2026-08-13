package media

import "github.com/gabriel-vasile/mimetype"

// ExtensionFor names the file extension for a MIME type, leading dot included,
// and reports whether the type was recognized at all.
//
// The two answers are separate because "no extension" arrives two ways and the
// callers do not treat them alike. A recognized type may simply have no
// extension worth the name - application/octet-stream is what the sniffer says
// about bytes it could not place - and an unrecognized one tells us nothing.
// The upload path repairs a name in neither case; the download path
// (extFromMime, in internal/core) falls back to a container extension for video
// only when the type itself was unknown.
//
// The mapping is the mimetype library's rather than the standard library's:
// mime.ExtensionsByType returns every extension registered for a type in an
// order it does not promise, so image/jpeg is as likely to come back .jfif as
// .jpg (#131).
func ExtensionFor(mime string) (ext string, known bool) {
	mt := mimetype.Lookup(NormalizeMIME(mime))
	if mt == nil {
		return "", false
	}
	return mt.Extension(), true
}
