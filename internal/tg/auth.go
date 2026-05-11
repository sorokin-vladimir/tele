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
}

func NewAuthFlow() *AuthFlow {
	return &AuthFlow{
		Requests:  make(chan AuthRequest),
		Responses: make(chan AuthResponse),
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

func (af *AuthFlow) Phone(ctx context.Context) (string, error) {
	return af.ask(ctx, AuthStepPhone)
}

func (af *AuthFlow) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
	return af.ask(ctx, AuthStepCode)
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
