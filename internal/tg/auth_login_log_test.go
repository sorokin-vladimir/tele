package tg

import (
	"context"
	"fmt"
	"testing"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// loggedFlow is a login whose log is kept for the test to read.
func loggedFlow() (*AuthFlow, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return NewAuthFlow(zap.New(core)), logs
}

// A report that the code never came is read from the log alone: each step
// asked, how the code was sent and how the next one would be, and what
// Telegram refused, with its own error (#330).
func TestLogin_LogsEachStep(t *testing.T) {
	af, logs := loggedFlow()
	client := &scriptedLogin{
		signIn:  []error{refused(400, "PHONE_CODE_INVALID")},
		next:    &tg.AuthCodeTypeSMS{},
		timeout: 60,
	}
	playScreen(t, af, []ask{
		turnToPhone,
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "11111"},
		{step: AuthStepCode, reason: reasonCodeInvalid, answer: "12345"},
	})

	require.NoError(t, af.login(context.Background(), client))

	asked := logs.FilterMessage("login step asked").AllUntimed()
	require.Len(t, asked, 4)
	for i, step := range []string{"qr", "phone", "code", "code"} {
		assert.Equal(t, zapcore.InfoLevel, asked[i].Level)
		assert.Equal(t, step, asked[i].ContextMap()["step"])
	}
	assert.Equal(t, true, asked[3].ContextMap()["again"], "a step asked again says so")

	sent := logs.FilterMessage("login code sent").AllUntimed()
	require.Len(t, sent, 1)
	assert.Equal(t, zapcore.InfoLevel, sent[0].Level)
	assert.Equal(t, map[string]any{
		"type":      "auth.sentCodeTypeApp",
		"next_type": "auth.codeTypeSms",
		"timeout":   int64(60),
	}, sent[0].ContextMap())

	refusals := logs.FilterMessage("login refused").AllUntimed()
	require.Len(t, refusals, 1)
	assert.Equal(t, zapcore.InfoLevel, refusals[0].Level)
	assert.Equal(t, "code", refusals[0].ContextMap()["step"])
	assert.Equal(t, "PHONE_CODE_INVALID", refusals[0].ContextMap()["rpc_error"])
	assert.Equal(t, int64(400), refusals[0].ContextMap()["rpc_code"])
}

// A code with no other way to come says so rather than leaving the field out,
// so a missing next type is not mistaken for a log that lost it.
func TestLogin_LogsNoNextTypeAsNone(t *testing.T) {
	af, logs := loggedFlow()
	playScreen(t, af, []ask{
		turnToPhone,
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
	})

	require.NoError(t, af.login(context.Background(), &scriptedLogin{}))

	sent := logs.FilterMessage("login code sent").AllUntimed()
	require.Len(t, sent, 1)
	assert.Equal(t, "none", sent[0].ContextMap()["next_type"])
	assert.Equal(t, int64(0), sent[0].ContextMap()["timeout"])
}

// Refusals end in the log whichever step they meet and whether or not they end
// the login: a refused number, and gotd's own word for a wrong password.
func TestLogin_LogsRefusalsOfEveryStep(t *testing.T) {
	af, logs := loggedFlow()
	client := &scriptedLogin{
		sendCode: []error{refused(400, "PHONE_NUMBER_INVALID")},
		signIn:   []error{auth.ErrPasswordAuthNeeded},
		password: []error{auth.ErrPasswordInvalid},
	}
	playScreen(t, af, []ask{
		turnToPhone,
		{step: AuthStepPhone, answer: "12"},
		{step: AuthStepPhone, reason: reasonNumberInvalid, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
		{step: AuthStepPassword, answer: "wrong"},
		{step: AuthStepPassword, reason: reasonPasswordInvalid, answer: "right"},
	})

	require.NoError(t, af.login(context.Background(), client))

	refusals := logs.FilterMessage("login refused").AllUntimed()
	require.Len(t, refusals, 2)
	assert.Equal(t, "phone", refusals[0].ContextMap()["step"])
	assert.Equal(t, "PHONE_NUMBER_INVALID", refusals[0].ContextMap()["rpc_error"])
	assert.Equal(t, "password", refusals[1].ContextMap()["step"])
	assert.Contains(t, refusals[1].ContextMap()["error"], auth.ErrPasswordInvalid.Error())
}

// What the person types never reaches the log: not the number, not the code,
// not the password, in any message or field.
func TestLogin_LogNamesNothingTyped(t *testing.T) {
	af, logs := loggedFlow()
	client := &scriptedLogin{
		signIn:   []error{refused(400, "PHONE_CODE_INVALID"), auth.ErrPasswordAuthNeeded},
		password: []error{auth.ErrPasswordInvalid},
	}
	typed := []string{"+19995550123", "48151", "62342", "hunter2wrong", "hunter2right"}
	playScreen(t, af, []ask{
		turnToPhone,
		{step: AuthStepPhone, answer: typed[0]},
		{step: AuthStepCode, answer: typed[1]},
		{step: AuthStepCode, reason: reasonCodeInvalid, answer: typed[2]},
		{step: AuthStepPassword, answer: typed[3]},
		{step: AuthStepPassword, reason: reasonPasswordInvalid, answer: typed[4]},
	})

	require.NoError(t, af.login(context.Background(), client))

	require.NotZero(t, logs.Len())
	for _, e := range logs.AllUntimed() {
		line := fmt.Sprint(e.Message, " ", e.ContextMap())
		for _, secret := range typed {
			assert.NotContains(t, line, secret)
		}
	}
}
