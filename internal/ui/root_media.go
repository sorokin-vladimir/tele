package ui

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/audio"
	vmedia "github.com/sorokin-vladimir/tele/internal/media"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// createTempMediaFile creates an empty private (0600) temp file in tmpDir with
// the given extension. The caller owns the returned file and must Close it.
func createTempMediaFile(tmpDir, ext string) (*os.File, error) {
	f, err := os.CreateTemp(tmpDir, "tele-media-*"+ext)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(f.Name(), 0600)
	return f, nil
}

// writeTempMediaFile writes data to a private (0600) temp file in tmpDir with
// the given extension and returns its path.
func writeTempMediaFile(data []byte, tmpDir, ext string) (string, error) {
	f, err := createTempMediaFile(tmpDir, ext)
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// openPathCommand builds the OS-specific command that hands a file to the
// default application for the given GOOS.
func openPathCommand(goos, name string) *exec.Cmd {
	switch goos {
	case "darwin":
		return exec.Command("open", name)
	case "windows":
		// Empty title arg keeps `start` from treating a quoted path as a title.
		return exec.Command("cmd", "/c", "start", "", name)
	default:
		return exec.Command("xdg-open", name)
	}
}

// openPath hands a file to the OS default application. It is a variable so
// tests can stub out the external launch.
var openPath = func(name string) {
	_ = openPathCommand(runtime.GOOS, name).Start()
}

// openURL hands a URL to the OS default handler (typically the web browser),
// reusing the same platform launcher as openPath. Variable so tests can stub it.
var openURL = func(url string) {
	_ = openPathCommand(runtime.GOOS, url).Start()
}

// openURLCmd opens url off the update loop.
func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		openURL(url)
		return nil
	}
}

// handleOpen acts on the "open" action for the selected message: nothing when
// there is no openable target, opens directly for a single target, or presents
// the picker when several targets compete (media plus links).
func (m RootModel) handleOpen() (tea.Model, tea.Cmd) {
	targets := m.chat.SelectedMessageOpenTargets()
	switch len(targets) {
	case 0:
		return m, nil
	case 1:
		return m.openTarget(targets[0])
	default:
		m.openPicker = components.NewOpenPicker(targets, m.chat.PhotoContentCols())
		return m, nil
	}
}

// openTarget acts on a single open-target of the selected message: links go to
// the browser, media to the in-app modal (external-player fallback for video).
func (m RootModel) openTarget(t components.OpenTarget) (tea.Model, tea.Cmd) {
	switch t.Kind {
	case components.OpenTargetLink:
		return m, openURLCmd(t.URL)
	case components.OpenTargetVideo:
		if ref, ok := m.chat.SelectedMessageVideo(); ok {
			if useInAppVideoPlayer(m.imageMode, vmedia.HasFFmpeg()) {
				dur, sender := m.selectedVideoInfo()
				return m.openVideoModal(ref, m.chat.SelectedMessageID(), dur, sender)
			}
			return m.startDocumentOpen(ref, m.chat.SelectedMessageID(), m.selectedDownloadLabel())
		}
	case components.OpenTargetPhoto:
		if ref, ok := m.chat.SelectedMessagePhoto(); ok {
			sender, date := m.selectedPhotoInfo()
			return m.openPhotoModal(ref, m.chat.SelectedMessageID(), sender, date)
		}
	}
	return m, nil
}

// SetURLOpenerForTest swaps the URL opener and returns a restore func, so tests
// can capture opened URLs without launching a browser.
func SetURLOpenerForTest(fn func(string)) func() {
	prev := openURL
	openURL = fn
	return func() { openURL = prev }
}

func openInViewer(img image.Image, tmpDir string) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		return
	}
	name, err := writeTempMediaFile(buf.Bytes(), tmpDir, ".jpg")
	if err != nil {
		return
	}
	openPath(name)
}

// handlePlayVoice toggles or starts in-app playback of the selected voice
// message. Degrades silently when no audio device is available.
func (m RootModel) handlePlayVoice() (RootModel, tea.Cmd) {
	ref, ok := m.chat.SelectedMessageVoice()
	if !ok {
		return m, nil
	}
	if m.voicePlayer == nil {
		pl, err := audio.NewPlayer()
		if err != nil {
			return m, nil // no audio device
		}
		m.voicePlayer = pl
	}
	if m.voicePlayer.Toggle(ref.ID) {
		return m, nil // same message: paused/resumed
	}
	return m, downloadVoiceCmd(m.ctx, m.tgClient, m.currentPeer(), m.chat.SelectedMessageID(), ref)
}
