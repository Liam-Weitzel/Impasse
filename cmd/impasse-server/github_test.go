package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGitHub stands in for github.com. The endpoints are overridable precisely
// so these tests never reach the network.
type fakeGitHub struct {
	server *httptest.Server

	// pendingPolls is how many times the token endpoint says "not yet"
	// before succeeding.
	pendingPolls atomic.Int32
	// tokenError, when set, is returned instead of a token.
	tokenError string
	// polls counts how many times the token endpoint was hit.
	polls atomic.Int32

	login string
	id    int64
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()

	f := &fakeGitHub{login: "liam", id: 4242}

	mux := http.NewServeMux()

	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"device_code": "dev-code",
			"user_code": "ABCD-1234",
			"verification_uri": "https://github.com/login/device",
			"interval": 1,
			"expires_in": 900
		}`))
	})

	mux.HandleFunc("/access_token", func(w http.ResponseWriter, r *http.Request) {
		f.polls.Add(1)
		w.Header().Set("Content-Type", "application/json")

		if f.tokenError != "" {
			w.Write([]byte(`{"error":"` + f.tokenError + `"}`))
			return
		}
		if f.pendingPolls.Add(-1) >= 0 {
			w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		w.Write([]byte(`{"access_token":"gho_test"}`))
	})

	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":4242,"login":"liam"}`))
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	return f
}

func (f *fakeGitHub) client() *githubOAuth {
	return &githubOAuth{
		clientID:       "test-client",
		deviceCodeURL:  f.server.URL + "/device/code",
		accessTokenURL: f.server.URL + "/access_token",
		userURL:        f.server.URL + "/user",
		client:         f.server.Client(),
	}
}

func TestDeviceFlowHappyPath(t *testing.T) {
	f := newFakeGitHub(t)
	g := f.client()

	code, err := g.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if code.UserCode != "ABCD-1234" {
		t.Errorf("user code %q", code.UserCode)
	}
	if code.VerificationURI == "" {
		t.Error("no verification uri to show the player")
	}
	if code.Interval != time.Second {
		t.Errorf("interval %v, want the one GitHub asked for", code.Interval)
	}

	user, err := g.Wait(context.Background(), code)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if user.ID != 4242 || user.Login != "liam" {
		t.Errorf("got %+v, want id 4242 login liam", user)
	}
}

// The player takes a while to type the code, so pending is the normal case and
// must not be treated as a failure.
func TestDeviceFlowWaitsThroughPending(t *testing.T) {
	f := newFakeGitHub(t)
	f.pendingPolls.Store(3)

	g := f.client()

	code, err := g.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	code.Interval = 10 * time.Millisecond

	user, err := g.Wait(context.Background(), code)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if user.ID != 4242 {
		t.Errorf("got %+v", user)
	}
	if got := f.polls.Load(); got < 4 {
		t.Errorf("polled %d times, expected to wait through the pending ones", got)
	}
}

// slow_down is GitHub asking for patience, not an error.
func TestDeviceFlowHandlesSlowDown(t *testing.T) {
	f := newFakeGitHub(t)
	g := f.client()

	code, err := g.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	code.Interval = 10 * time.Millisecond

	f.tokenError = "slow_down"
	go func() {
		time.Sleep(60 * time.Millisecond)
		f.tokenError = ""
	}()

	if _, err := g.Wait(context.Background(), code); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestDeviceFlowCancelledSignIn(t *testing.T) {
	f := newFakeGitHub(t)
	f.tokenError = "access_denied"

	g := f.client()
	code, err := g.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	code.Interval = 10 * time.Millisecond

	_, err = g.Wait(context.Background(), code)
	if err == nil {
		t.Fatal("want an error when sign in is refused")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error %q, want something a player can act on", err)
	}
}

// A misconfigured app is the likeliest deployment mistake, so it needs to say
// so rather than surfacing a bare code.
func TestDeviceFlowExplainsDisabledDeviceFlow(t *testing.T) {
	f := newFakeGitHub(t)
	f.tokenError = "device_flow_disabled"

	g := f.client()
	code, _ := g.Start(context.Background())
	code.Interval = 10 * time.Millisecond

	_, err := g.Wait(context.Background(), code)
	if err == nil || !strings.Contains(err.Error(), "device flow is not enabled") {
		t.Errorf("error %v, want it to name the misconfiguration", err)
	}
}

func TestDeviceFlowExpiredCode(t *testing.T) {
	f := newFakeGitHub(t)
	f.pendingPolls.Store(1000)

	g := f.client()
	code, err := g.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	code.Interval = 5 * time.Millisecond
	code.ExpiresAt = time.Now().Add(20 * time.Millisecond)

	_, err = g.Wait(context.Background(), code)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("error %v, want it to say the code expired", err)
	}
}

func TestDeviceFlowRespectsContext(t *testing.T) {
	f := newFakeGitHub(t)
	f.pendingPolls.Store(1000)

	g := f.client()
	code, err := g.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	code.Interval = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	if _, err := g.Wait(ctx, code); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %v, want the context deadline", err)
	}
}

func TestDescribeNamesKnownFailures(t *testing.T) {
	for code, want := range map[string]string{
		"device_flow_disabled":         "device flow",
		"access_denied":                "cancelled",
		"expired_token":                "expired",
		"incorrect_client_credentials": "client id",
	} {
		if got := describe(code, ""); !strings.Contains(got, want) {
			t.Errorf("describe(%q) = %q, want it to mention %q", code, got, want)
		}
	}

	// Anything unrecognised falls back to what GitHub said.
	if got := describe("weird_thing", "something went wrong"); got != "something went wrong" {
		t.Errorf("got %q, want the description passed through", got)
	}
	if got := describe("weird_thing", ""); got != "weird_thing" {
		t.Errorf("got %q, want the raw code", got)
	}
}
