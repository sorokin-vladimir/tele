package tg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/dcs"
)

// savedDCList is the list of data-centre addresses a client starts from: the
// ones Telegram sent the last time this session connected, kept in the session
// file. gotd writes them there but starts from its own static list when it
// restores a session, and that list can name addresses Telegram no longer
// serves, so a session that logged in fine could never connect again (#312).
// Workaround: drop it once gotd restores the saved config itself, see
// docs/gotd-workarounds.md.
//
// The zero List, when nothing usable is saved, is gotd's static list.
func savedDCList(ctx context.Context, s session.Storage) dcs.List {
	loader := session.Loader{Storage: s}
	data, err := loader.Load(ctx)
	if err != nil || len(data.Config.DCOptions) == 0 {
		return dcs.List{}
	}
	return dcs.List{Options: data.Config.DCOptions, Domains: dcs.Prod().Domains}
}

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
