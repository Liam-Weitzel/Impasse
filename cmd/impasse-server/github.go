package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHub sign in, so that one person means one character rather than one
// keypair meaning one character. Keys are free, GitHub accounts are not.
//
// This uses the device flow rather than the usual redirect flow. There is no
// browser here and no callback to redirect to, so the server asks GitHub for a
// short code, shows it to the player, and polls until they have entered it. No
// HTTP listener, no TLS and no client secret, only outbound requests.

const (
	defaultDeviceCodeURL  = "https://github.com/login/device/code"
	defaultAccessTokenURL = "https://github.com/login/oauth/access_token"
	defaultUserURL        = "https://api.github.com/user"

	// GitHub says to wait this long between polls unless it asks for more.
	defaultPollInterval = 5 * time.Second
)

// DeviceCode is what the player has to type into GitHub.
type DeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	Interval        time.Duration
	ExpiresAt       time.Time
}

// GitHubUser is the identity behind an account. The numeric ID is what we key
// on, because a login can be renamed and then reused by somebody else.
type GitHubUser struct {
	ID    int64
	Login string
}

// oauth is the piece that talks to GitHub, kept behind an interface so tests
// never touch the network.
type oauth interface {
	// Start asks for a code for the player to enter.
	Start(ctx context.Context) (DeviceCode, error)
	// Wait blocks until the player has authorised, the code expires, or the
	// context is cancelled.
	Wait(ctx context.Context, code DeviceCode) (GitHubUser, error)
}

// errAuthorisationPending is GitHub saying the player has not finished yet.
var errAuthorisationPending = errors.New("authorisation pending")

// githubOAuth is the real client.
type githubOAuth struct {
	clientID string

	// Endpoints, overridable so tests can point at a local server.
	deviceCodeURL  string
	accessTokenURL string
	userURL        string

	client *http.Client
}

func newGitHubOAuth(clientID string) *githubOAuth {
	return &githubOAuth{
		clientID:       clientID,
		deviceCodeURL:  defaultDeviceCodeURL,
		accessTokenURL: defaultAccessTokenURL,
		userURL:        defaultUserURL,
		client:         &http.Client{Timeout: 15 * time.Second},
	}
}

func (g *githubOAuth) Start(ctx context.Context) (DeviceCode, error) {
	form := url.Values{}
	form.Set("client_id", g.clientID)
	// No scopes. All that is needed is who they are, and asking for less
	// makes the consent screen an easier yes.
	form.Set("scope", "")

	var out struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Interval        int    `json:"interval"`
		ExpiresIn       int    `json:"expires_in"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
	}
	if err := g.post(ctx, g.deviceCodeURL, form, &out); err != nil {
		return DeviceCode{}, err
	}
	if out.Error != "" {
		return DeviceCode{}, fmt.Errorf("github: %s", describe(out.Error, out.ErrorDesc))
	}
	if out.DeviceCode == "" || out.UserCode == "" {
		return DeviceCode{}, errors.New("github returned no device code")
	}

	interval := time.Duration(out.Interval) * time.Second
	if interval <= 0 {
		interval = defaultPollInterval
	}
	expires := time.Now().Add(15 * time.Minute)
	if out.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	}

	return DeviceCode{
		DeviceCode:      out.DeviceCode,
		UserCode:        out.UserCode,
		VerificationURI: out.VerificationURI,
		Interval:        interval,
		ExpiresAt:       expires,
	}, nil
}

func (g *githubOAuth) Wait(ctx context.Context, code DeviceCode) (GitHubUser, error) {
	interval := code.Interval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	for {
		token, err := g.poll(ctx, code)
		switch {
		case err == nil:
			return g.user(ctx, token)

		case errors.Is(err, errAuthorisationPending):
			// Keep waiting.

		default:
			return GitHubUser{}, err
		}

		if !code.ExpiresAt.IsZero() && time.Now().After(code.ExpiresAt) {
			return GitHubUser{}, errors.New("the code expired, start again")
		}

		select {
		case <-ctx.Done():
			return GitHubUser{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// poll asks once whether the player has authorised yet.
func (g *githubOAuth) poll(ctx context.Context, code DeviceCode) (string, error) {
	form := url.Values{}
	form.Set("client_id", g.clientID)
	form.Set("device_code", code.DeviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := g.post(ctx, g.accessTokenURL, form, &out); err != nil {
		return "", err
	}

	switch out.Error {
	case "":
		if out.AccessToken == "" {
			return "", errors.New("github returned no access token")
		}
		return out.AccessToken, nil

	case "authorization_pending":
		return "", errAuthorisationPending

	case "slow_down":
		// Treated as pending. The next wait is already at least the
		// interval GitHub asked for.
		return "", errAuthorisationPending

	default:
		return "", fmt.Errorf("github: %s", describe(out.Error, out.ErrorDesc))
	}
}

func (g *githubOAuth) user(ctx context.Context, token string) (GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.userURL, nil)
	if err != nil {
		return GitHubUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.client.Do(req)
	if err != nil {
		return GitHubUser{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GitHubUser{}, fmt.Errorf("github user lookup: %s", resp.Status)
	}

	var out struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return GitHubUser{}, err
	}
	if out.ID == 0 {
		return GitHubUser{}, errors.New("github returned no user id")
	}

	return GitHubUser{ID: out.ID, Login: out.Login}, nil
}

func (g *githubOAuth) post(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github %s: %s", endpoint, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// describe turns GitHub's error codes into something a player can act on.
func describe(code, desc string) string {
	switch code {
	case "device_flow_disabled":
		return "device flow is not enabled on this server's GitHub app"
	case "expired_token":
		return "the code expired, start again"
	case "access_denied":
		return "sign in was cancelled"
	case "incorrect_client_credentials":
		return "this server's GitHub client id is wrong"
	}
	if desc != "" {
		return desc
	}
	return code
}
