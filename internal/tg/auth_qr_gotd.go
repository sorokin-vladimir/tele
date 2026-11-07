package tg

import (
	"context"
	"errors"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

// gotdLogin is the login's way to Telegram: gotd's auth client for the number,
// the code and the password, and its QR helper for the login token (#330).
type gotdLogin struct {
	*auth.Client
	qr  qrlogin.QR
	api *tg.Client
}

func newGotdLogin(tc *telegram.Client, appID int, appHash string, log *zap.Logger) gotdLogin {
	// The QR helper is built here rather than taken from tc.QR(), so a move to
	// the data centre the account lives on is logged.
	migrate := func(ctx context.Context, dc int) error {
		log.Info("login token migrates", zap.Int("dc", dc))
		return tc.MigrateTo(ctx, dc)
	}
	return gotdLogin{
		Client: tc.Auth(),
		qr:     qrlogin.NewQR(tc.API(), appID, appHash, qrlogin.Options{Migrate: migrate}),
		api:    tc.API(),
	}
}

// ExportLoginToken issues a token. A token accepted on another data centre is
// accepted all the same: importing it makes the move.
func (g gotdLogin) ExportLoginToken(ctx context.Context) (qrlogin.Token, error) {
	token, err := g.qr.Export(ctx)
	var migrate *qrlogin.MigrationNeededError
	if errors.As(err, &migrate) {
		return qrlogin.Token{}, nil
	}
	return token, err
}

func (g gotdLogin) ImportLoginToken(ctx context.Context) error {
	_, err := g.qr.Import(ctx)
	return err
}

func (g gotdLogin) CancelCode(ctx context.Context, phone, hash string) error {
	_, err := g.api.AuthCancelCode(ctx, &tg.AuthCancelCodeRequest{PhoneNumber: phone, PhoneCodeHash: hash})
	return err
}

// loginTokenWatch tells seen when Telegram reports a login token scanned, and
// hands every update on as it came.
type loginTokenWatch struct {
	next telegram.UpdateHandler
	seen func()
}

// watchLoginToken catches the scan on its way in: it comes before the login
// ends, while the updates manager does not run yet.
func watchLoginToken(next telegram.UpdateHandler, seen func()) loginTokenWatch {
	return loginTokenWatch{next: next, seen: seen}
}

func (w loginTokenWatch) Handle(ctx context.Context, u tg.UpdatesClass) error {
	var list []tg.UpdateClass
	switch v := u.(type) {
	case *tg.UpdateShort:
		list = []tg.UpdateClass{v.Update}
	case *tg.Updates:
		list = v.Updates
	case *tg.UpdatesCombined:
		list = v.Updates
	}
	for _, upd := range list {
		if _, ok := upd.(*tg.UpdateLoginToken); ok {
			w.seen()
			break
		}
	}
	return w.next.Handle(ctx, u)
}
