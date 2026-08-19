package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixture copies the comment-heavy config into a temp directory and returns its
// path. Every test here edits a real file, because what is being tested is what
// the file looks like afterwards.
func fixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "comment-heavy.yml"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, raw, 0600))
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(raw)
}

// The whole reason writes go through a node tree rather than a marshal of
// Config: the first launch writes a file that is mostly documentation, and a
// setting changed in the overlay must not cost the person that documentation
// (ADR 0009). This is the test that would catch it.
func TestEditFile_ChangingOneValueLeavesEverythingElseAlone(t *testing.T) {
	path := fixture(t)
	before := read(t, path)

	require.NoError(t, editFile(path, "ui.history_limit", 200))
	after := read(t, path)

	assert.Equal(t,
		strings.Replace(before, "history_limit: 50", "history_limit: 200", 1),
		after,
		"the file is byte-for-byte what it was, except the value that changed")
}

// The alignment of a comment beside a value is somebody's, and it survives the
// value beside it changing width.
func TestEditFile_KeepsTheSpacingBeforeAnInlineComment(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "ui.history_limit", 7))

	assert.Contains(t, read(t, path), "history_limit: 7   # messages fetched when a chat is opened")
}

func TestEditFile_KeepsCommentsBlankLinesAndOrder(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "ui.history_limit", 200))
	after := read(t, path)

	assert.Contains(t, after, "# tele configuration.", "the head comment")
	assert.Contains(t, after, "# state_dir: ~/.local/state/tele", "a commented-out setting is still writing")
	assert.Contains(t, after, "# messages fetched when a chat is opened", "the comment beside the value that changed")
	assert.Contains(t, after, "# docs/themes.md.", "a comment inside a section")
	assert.Contains(t, after, "\n\nui:", "the blank line before a section")
	assert.Contains(t, after, "\n\nphotos:", "and every other blank line")

	assert.Less(t, strings.Index(after, "telegram:"), strings.Index(after, "ui:"), "order kept")
	assert.Less(t, strings.Index(after, "ui:"), strings.Index(after, "photos:"), "order kept")
}

// A key this binary does not know is somebody else's - a newer tele, a typo
// worth keeping, a section written in advance. Rewriting the file from Config
// would silently drop it.
func TestEditFile_KeepsKeysTheAppDoesNotKnow(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "ui.history_limit", 200))
	after := read(t, path)

	assert.Contains(t, after, "experimental:")
	assert.Contains(t, after, "telepathy: true")
	assert.Contains(t, after, "# Something a future version of tele knows about")
}

// The zone keys live in a section the file does not have at all, so writing one
// has to create ui.toasts and put it where the registry says it goes.
func TestEditFile_CreatesAMissingSection(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "ui.toasts.error_zone", "top-right"))
	after := read(t, path)

	assert.Contains(t, after, "toasts:")
	assert.Contains(t, after, "error_zone: top-right")

	cfg, err := Load(path, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "top-right", cfg.UI.Toasts.ErrorZone, "and it reads back")
	assert.Equal(t, 50, cfg.UI.HistoryLimit, "beside what was already there")
}

// A new key goes where a person reading the file would look for it, not at the
// bottom. history_limit is declared before notification_preview, so an inserted
// notification_preview belongs after it.
func TestEditFile_InsertsInRegistryOrder(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "ui.notification_preview", false))
	after := read(t, path)

	assert.Less(t, strings.Index(after, "history_limit:"), strings.Index(after, "notification_preview:"))
	assert.Less(t, strings.Index(after, "notification_preview:"), strings.Index(after, "theme:"))
}

// An inserted key joins the block it belongs to. It must not land between the
// comment block above theme: and the theme: it describes, and it must not push
// itself past the blank line that separates them either.
func TestEditFile_InsertsWithoutSplittingACommentFromItsKey(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "ui.notification_preview", false))
	after := read(t, path)

	assert.Contains(t, after,
		"  history_limit: 50   # messages fetched when a chat is opened\n"+
			"  notification_preview: false\n"+
			"\n"+
			"  # Themes follow the terminal background")
}

// Resetting a setting deletes its key. Absence is the value, so a file does not
// accumulate an explicit copy of every default one edit at a time.
func TestEditFile_NilDeletesTheKey(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "ui.history_limit", nil))
	after := read(t, path)

	assert.NotContains(t, after, "history_limit")
	assert.Contains(t, after, "ui:", "the section it was in stays, because it holds other keys")
	assert.Contains(t, after, "theme:")

	cfg, err := Load(path, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, 50, cfg.UI.HistoryLimit, "and the default is what it is worth again")
}

// Deleting the only key in a section leaves nothing behind - an empty section is
// a leftover of an edit, not something anybody wrote.
func TestEditFile_PrunesASectionItEmptied(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "photos.eager_full_quality", nil))
	after := read(t, path)

	assert.NotContains(t, after, "photos:")
	assert.Contains(t, after, "experimental:", "and stops there")
}

// Unless the section carries a comment, which is somebody's writing. "Reset this
// setting" did not ask for it to be deleted.
func TestEditFile_KeepsAnEmptiedSectionThatCarriesAComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(""+
		"# Everything about pictures of people.\n"+
		"avatars:\n"+
		"  disk_cache_size: 1024\n"), 0600))

	require.NoError(t, editFile(path, "avatars.disk_cache_size", nil))
	after := read(t, path)

	assert.Contains(t, after, "# Everything about pictures of people.")
	assert.Contains(t, after, "avatars:")
}

// Deleting a key that is not there is what "reset a setting the file never
// named" does, and it is not an error - the setting is already at its default.
func TestEditFile_DeletingAnAbsentKeyChangesNothing(t *testing.T) {
	path := fixture(t)
	before := read(t, path)

	require.NoError(t, editFile(path, "ui.toasts.max_visible", nil))

	assert.Equal(t, before, read(t, path))
}

// A file the app cannot parse is the one it understands least. It is reported
// and left exactly as it is; the write never truncates what it could not read.
func TestEditFile_RefusesAFileItCannotParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	broken := "ui:\n  history_limit: 50\n   theme: nord\n"
	require.NoError(t, os.WriteFile(path, []byte(broken), 0600))

	err := editFile(path, "ui.history_limit", 200)

	require.Error(t, err)
	assert.Equal(t, broken, read(t, path), "left alone")
}

func TestEditFile_RefusesAFileWhoseTopLevelIsNotAMapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("- one\n- two\n"), 0600))

	require.Error(t, editFile(path, "ui.history_limit", 200))
}

// An empty file is not a broken file: it is what a person left behind when they
// deleted everything, and writing a setting into it should work.
func TestEditFile_WritesIntoAnEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, nil, 0600))

	require.NoError(t, editFile(path, "ui.history_limit", 120))

	assert.Contains(t, read(t, path), "history_limit: 120")
}

// The file holds an API hash. An edit must not be the moment it becomes
// world-readable, and the temporary file must not be either.
func TestEditFile_KeepsTheFileMode(t *testing.T) {
	path := fixture(t)
	require.NoError(t, os.Chmod(path, 0600))

	require.NoError(t, editFile(path, "ui.history_limit", 200))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// Nothing is left beside the config when a write succeeds.
func TestEditFile_LeavesNoTemporaryFileBehind(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "ui.history_limit", 200))

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "config.yml", entries[0].Name())
}

// A value written in a spelling the person chose stays in that spelling. Turning
// "deadbeef" into deadbeef is a change they did not ask for, in a file they read.
func TestEditFile_KeepsQuotingStyle(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "ui.history_limit", 200))

	assert.Contains(t, read(t, path), `api_hash: "deadbeef"`)
}

func TestSiblingOrder(t *testing.T) {
	assert.Equal(t,
		[]string{"telegram", "state_dir", "ui", "photos", "avatars", "keybindings"},
		siblingOrder(nil),
		"the file's own order, exclusions included")
	assert.Equal(t, []string{"history_limit", "notification_preview", "theme", "toasts", "date_format"}, siblingOrder([]string{"ui"}))
	assert.Equal(t, []string{"error_zone", "notify_zone", "max_visible"}, siblingOrder([]string{"ui", "toasts"}))
}

// Two settings are called disk_cache_size, in different sections. Ordering must
// follow the path rather than the last segment of it.
func TestSiblingOrder_DistinguishesSectionsWithTheSameKeyName(t *testing.T) {
	assert.Equal(t, []string{"eager_full_quality", "mode", "kitty_placement_cap", "max_long_side_px", "disk_cache_size"}, siblingOrder([]string{"photos"}))
	assert.Equal(t, []string{"disk_cache_size"}, siblingOrder([]string{"avatars"}))
}

// Text was spliced where a parser said to splice it, so the result is handed
// back to the parser before it is handed to the disk.
func TestEditFile_VerifiesTheResultReadsBack(t *testing.T) {
	path := fixture(t)

	require.NoError(t, editFile(path, "ui.theme", "nord"))
	after := read(t, path)

	assert.Contains(t, after, "theme: nord", "a mapping value replaced by a scalar")
	assert.NotContains(t, after, "gruvbox-dark")
	assert.NotContains(t, after, "seoul256-light")

	cfg, err := Load(path, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, ThemeSlots{Dark: "nord", Light: "nord"}, cfg.UI.ThemeSlots)
}

// Writing a value that is already there changes nothing, so the file's mtime
// does not move and nothing downstream believes it did.
func TestEditFile_WritingTheSameValueIsNotAWrite(t *testing.T) {
	path := fixture(t)
	before, err := os.Stat(path)
	require.NoError(t, err)

	require.NoError(t, editFile(path, "ui.history_limit", 50))

	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime())
}

// A file that ends without a newline is unusual and is still the person's.
func TestEditFile_KeepsAMissingTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("ui:\n  history_limit: 50"), 0600))

	require.NoError(t, editFile(path, "ui.history_limit", 60))

	assert.Equal(t, "ui:\n  history_limit: 60", read(t, path))
}

// Resetting twice must not leave a growing run of blank lines where sections
// used to be.
func TestEditFile_DoesNotAccumulateBlankLines(t *testing.T) {
	path := fixture(t)
	require.NoError(t, editFile(path, "photos.eager_full_quality", nil))
	after := read(t, path)

	assert.NotContains(t, after, "\n\n\n")
	assert.NotContains(t, after, "photos:")
}
