package web

import (
	"context"
	"strings"
	"sync"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// authMachine implements auth.UserAuthenticator over channels, so the login
// flow blocks until the web UI submits phone/code/password.
type authMachine struct {
	mu    sync.Mutex
	state LoginState
	err   string

	// pendingPhone survives a reconnect (e.g. after a wrong login code):
	// the UI may submit the phone while the client is still connecting or
	// showing an error, and Phone() consumes it once the new auth flow
	// starts. reset() deliberately does not clear it.
	pendingPhone string

	phoneCh chan string
	codeCh  chan string
	passCh  chan string
}

func newAuthMachine() *authMachine {
	return &authMachine{
		state:   LoginStateConnecting,
		phoneCh: make(chan string, 1),
		codeCh:  make(chan string, 1),
		passCh:  make(chan string, 1),
	}
}

func (a *authMachine) reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = LoginStateConnecting
	a.err = ""
	// drain stale submissions
	select {
	case <-a.phoneCh:
	default:
	}
	select {
	case <-a.codeCh:
	default:
	}
	select {
	case <-a.passCh:
	default:
	}
}

func (a *authMachine) setState(s LoginState, err string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = s
	a.err = err
}

func (a *authMachine) State() (LoginState, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state, a.err
}

func (a *authMachine) submit(ch chan string, want LoginState, v string) error {
	st, _ := a.State()
	if st != want {
		return errors.Errorf("invalid state: %s, expect %s", st, want)
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("empty value")
	}
	select {
	case ch <- v:
		return nil
	default:
		return errors.New("already submitted")
	}
}

func (a *authMachine) SubmitCode(v string) error     { return a.submit(a.codeCh, LoginStateWaitCode, v) }
func (a *authMachine) SubmitPassword(v string) error { return a.submit(a.passCh, LoginStateWaitPassword, v) }

// clearPendingPhone forgets a remembered phone number once login succeeds.
func (a *authMachine) clearPendingPhone() {
	a.mu.Lock()
	a.pendingPhone = ""
	a.mu.Unlock()
}

// SubmitPhone is also accepted while connecting or after an error: the
// number is remembered and consumed by Phone() when the auth flow starts,
// so users don't have to wait for the reconnect backoff to retype it.
func (a *authMachine) SubmitPhone(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("empty value")
	}

	a.mu.Lock()
	st := a.state
	a.pendingPhone = v
	a.mu.Unlock()

	switch st {
	case LoginStateWaitPhone:
		select {
		case a.phoneCh <- v:
			return nil
		default:
			return errors.New("already submitted")
		}
	case LoginStateConnecting, LoginStateError:
		return nil
	default:
		return errors.Errorf("invalid state: %s", st)
	}
}

// auth.UserAuthenticator implementation

func (a *authMachine) Phone(ctx context.Context) (string, error) {
	a.mu.Lock()
	v := a.pendingPhone
	a.pendingPhone = ""
	a.state = LoginStateWaitPhone
	a.err = ""
	a.mu.Unlock()

	if v != "" {
		return v, nil
	}
	select {
	case v := <-a.phoneCh:
		return v, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (a *authMachine) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
	a.setState(LoginStateWaitCode, "")
	select {
	case v := <-a.codeCh:
		return v, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (a *authMachine) Password(ctx context.Context) (string, error) {
	a.setState(LoginStateWaitPassword, "")
	select {
	case v := <-a.passCh:
		return v, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (a *authMachine) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("sign up is not supported, please register on Telegram first")
}

func (a *authMachine) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{TermsOfService: tos}
}
