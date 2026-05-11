package tg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/gotd/td/session"
)

type fileSession struct {
	inner session.FileStorage
}

func NewFileSession(path string) *fileSession {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	return &fileSession{inner: session.FileStorage{Path: path}}
}

func (s *fileSession) LoadSession(ctx context.Context) ([]byte, error) {
	data, err := s.inner.LoadSession(ctx)
	if errors.Is(err, session.ErrNotFound) {
		return nil, nil
	}
	return data, err
}

func (s *fileSession) StoreSession(ctx context.Context, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(s.inner.Path), 0700); err != nil {
		return err
	}
	return s.inner.StoreSession(ctx, data)
}
