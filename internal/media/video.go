package media

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// VideoMeta is the subset of video metadata Telegram needs for an uploaded video
// document. Duration is whole seconds (truncated), matching the inbound parser.
type VideoMeta struct {
	Duration int
	Width    int
	Height   int
}

// HasFFprobe reports whether ffprobe is available on PATH.
func HasFFprobe() bool {
	_, err := exec.LookPath("ffprobe")
	return err == nil
}

// HasFFmpeg reports whether ffmpeg is available on PATH.
func HasFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// ProbeVideo runs ffprobe on path and returns its duration/width/height. It
// returns an error if ffprobe is missing or the command fails; callers treat
// that as "no metadata" and send a bare DocumentAttributeVideo.
func ProbeVideo(ctx context.Context, path string) (VideoMeta, error) {
	// One ffprobe call emits width/height (first video stream) and the container
	// duration as key=value lines, which parseFFprobeOutput consumes.
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height:format=duration",
		"-of", "default=noprint_wrappers=1:nokey=0",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return VideoMeta{}, err
	}
	return parseFFprobeOutput(string(out)), nil
}

// parseFFprobeOutput reads ffprobe's "key=value" default output. Unknown or
// unparsable lines are ignored; duration is truncated to whole seconds.
func parseFFprobeOutput(out string) VideoMeta {
	var m VideoMeta
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "width":
			if n, err := strconv.Atoi(v); err == nil {
				m.Width = n
			}
		case "height":
			if n, err := strconv.Atoi(v); err == nil {
				m.Height = n
			}
		case "duration":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				m.Duration = int(f)
			}
		}
	}
	return m
}

// ExtractThumbnail runs ffmpeg to write a single JPEG frame (sampled near the
// start, scaled to a 320px width with an even height) to outPath. It returns an
// error if ffmpeg is missing or the command fails; callers then send without a
// client thumbnail and let Telegram generate one.
func ExtractThumbnail(ctx context.Context, path, outPath string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-ss", "0.5",
		"-i", path,
		"-frames:v", "1",
		"-vf", "scale=320:-2",
		"-f", "mjpeg",
		outPath,
	)
	return cmd.Run()
}
