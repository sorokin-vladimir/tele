package tg

import (
	"context"
	"fmt"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

type AuthStep int

const (
	AuthStepPhone AuthStep = iota
	AuthStepCode
	AuthStepPassword
)

type AuthRequest struct {
	Step AuthStep
	Hint string
	// Err says why the step is asked for again: the value given last time was
	// refused. Empty the first time a step is asked (#285).
	Err string
}

type AuthResponse struct {
	Value string
	Err   error
}

// AuthFlow bridges the login's blocking questions (see login) with bubbletea's
// event loop via unbuffered channels. gotd's auth.Flow no longer drives it
// (#285).
type AuthFlow struct {
	Requests  chan AuthRequest
	Responses chan AuthResponse
	// Errors carries fatal auth errors to the UI (buffered 1 so Code() never blocks).
	Errors chan string
}

func NewAuthFlow() *AuthFlow {
	return &AuthFlow{
		Requests:  make(chan AuthRequest),
		Responses: make(chan AuthResponse),
		Errors:    make(chan string, 1),
	}
}

// request puts one step to the login screen and waits for the answer.
func (af *AuthFlow) request(ctx context.Context, req AuthRequest) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case af.Requests <- req:
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case resp := <-af.Responses:
		return resp.Value, resp.Err
	}
}

func (af *AuthFlow) Phone(ctx context.Context) (string, error) {
	return af.request(ctx, AuthRequest{Step: AuthStepPhone})
}

func (af *AuthFlow) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	return af.askCode(ctx, sentCode, "")
}

// askCode asks for the code Telegram sent, saying how it was sent and, when it
// is asked again, why.
func (af *AuthFlow) askCode(ctx context.Context, sentCode *tg.AuthSentCode, reason string) (string, error) {
	if _, ok := sentCode.Type.(*tg.AuthSentCodeTypeSetUpEmailRequired); ok {
		af.Errors <- "Login requires email verification, which is not yet supported.\nPlease log in via the official Telegram app first, then relaunch tele."
		return "", fmt.Errorf("authSentCodeTypeSetUpEmailRequired: email verification required")
	}
	return af.request(ctx, AuthRequest{Step: AuthStepCode, Hint: codeHint(sentCode.Type), Err: reason})
}

func codeHint(t tg.AuthSentCodeTypeClass) string {
	switch v := t.(type) {
	case *tg.AuthSentCodeTypeApp:
		return "Enter the code from your Telegram app:"
	case *tg.AuthSentCodeTypeEmailCode:
		if v.EmailPattern != "" {
			return fmt.Sprintf("Enter the code sent to %s:", v.EmailPattern)
		}
		return "Enter the code sent to your email:"
	case *tg.AuthSentCodeTypeSMS:
		return "Enter the SMS code:"
	case *tg.AuthSentCodeTypeSMSWord:
		return "Enter the word sent to you via SMS:"
	case *tg.AuthSentCodeTypeSMSPhrase:
		return "Enter the phrase sent to you via SMS:"
	case *tg.AuthSentCodeTypeCall:
		return "Answer the incoming call — the code will be read aloud:"
	case *tg.AuthSentCodeTypeFragmentSMS:
		return "Enter the code from Fragment (fragment.com):"
	default:
		return "Enter the login code:"
	}
}

func (af *AuthFlow) Password(ctx context.Context) (string, error) {
	return af.request(ctx, AuthRequest{Step: AuthStepPassword})
}

func (af *AuthFlow) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign up not supported — create account in official Telegram app")
}
