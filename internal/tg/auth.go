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
}

type AuthResponse struct {
	Value string
	Err   error
}

// AuthFlow bridges gotd's blocking auth.UserAuthenticator callbacks
// with bubbletea's event loop via unbuffered channels.
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

func (af *AuthFlow) ask(ctx context.Context, step AuthStep) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case af.Requests <- AuthRequest{Step: step}:
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case resp := <-af.Responses:
		return resp.Value, resp.Err
	}
}

func (af *AuthFlow) askWithHint(ctx context.Context, step AuthStep, hint string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case af.Requests <- AuthRequest{Step: step, Hint: hint}:
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case resp := <-af.Responses:
		return resp.Value, resp.Err
	}
}

func (af *AuthFlow) Phone(ctx context.Context) (string, error) {
	return af.ask(ctx, AuthStepPhone)
}

func (af *AuthFlow) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	if _, ok := sentCode.Type.(*tg.AuthSentCodeTypeSetUpEmailRequired); ok {
		af.Errors <- "Login requires email verification, which is not yet supported.\nPlease sign in via the official Telegram app first, then relaunch tele."
		return "", fmt.Errorf("authSentCodeTypeSetUpEmailRequired: email verification required")
	}
	return af.askWithHint(ctx, AuthStepCode, codeHint(sentCode.Type))
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
	return af.ask(ctx, AuthStepPassword)
}

func (af *AuthFlow) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

func (af *AuthFlow) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign up not supported — create account in official Telegram app")
}
