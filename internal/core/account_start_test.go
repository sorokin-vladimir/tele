package core

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

type failingAccountSetupConn struct{ stubConn }

func (c *failingAccountSetupConn) Connect(
	_ context.Context,
	_ *config.Config,
	_ *internaltg.AuthFlow,
	_ chan<- struct{},
	onAuth func(int64, string) error,
) error {
	if err := onAuth(200, "new-owner"); err != nil {
		return errors.New("prepare authenticated account: " + err.Error())
	}
	return nil
}

func TestOwner_StartReportsAccountResetFailure(t *testing.T) {
	st := store.NewMemory()
	o := New(&config.Config{}, zap.NewNop(), state.New(st), &failingAccountSetupConn{}, nopNotifier{})
	sqliteStore := st.(*store.SQLiteStore)
	t.Cleanup(func() { _ = sqliteStore.Close() })
	_, err := sqliteStore.DB().Exec(`INSERT INTO metadata(key, value) VALUES ('owner_id', 'invalid')`)
	require.NoError(t, err)

	err = o.Start(context.Background())
	require.Error(t, err)

	select {
	case text := <-o.AuthFlow().Errors:
		assert.Contains(t, text, "prepare authenticated account")
	default:
		t.Fatal("account reset failure was not reported to the login UI")
	}
}
