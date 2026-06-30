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
