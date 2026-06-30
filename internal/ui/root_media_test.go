package ui

import (
	"strings"
	"testing"
)

func TestOpenPathCommand(t *testing.T) {
	tests := []struct {
		goos     string
		wantArgs []string
		contains string
	}{
		{goos: "darwin", wantArgs: []string{"open", "/tmp/file.jpg"}},
		{goos: "linux", wantArgs: []string{"xdg-open", "/tmp/file.jpg"}},
		{goos: "windows", contains: "/tmp/file.jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			cmd := openPathCommand(tt.goos, "/tmp/file.jpg")
			if cmd == nil {
				t.Fatalf("openPathCommand(%q) returned nil", tt.goos)
			}
			if tt.wantArgs != nil {
				if len(cmd.Args) != len(tt.wantArgs) {
					t.Fatalf("args = %v, want %v", cmd.Args, tt.wantArgs)
				}
				for i := range tt.wantArgs {
					if cmd.Args[i] != tt.wantArgs[i] {
						t.Fatalf("args[%d] = %q, want %q", i, cmd.Args[i], tt.wantArgs[i])
					}
				}
			}
			if tt.contains != "" {
				if !strings.Contains(strings.Join(cmd.Args, " "), tt.contains) {
					t.Fatalf("args %v do not contain %q", cmd.Args, tt.contains)
				}
			}
		})
	}
}

func TestOpenPathCommandWindowsUsesStart(t *testing.T) {
	cmd := openPathCommand("windows", `C:\Users\me\pic.jpg`)
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "start") {
		t.Fatalf("windows command %q does not use start", joined)
	}
}
