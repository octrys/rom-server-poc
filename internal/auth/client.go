// Package auth validates connecting clients against the rom-api region service.
// The game server never checks credentials itself: it trusts the sessionKey the
// client presents on the game socket only if region recognises it and the
// bound userCode matches the accountCode the client sent.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrSessionInvalid means the region service did not recognise the sessionKey
// (unknown, expired, or already consumed).
var ErrSessionInvalid = errors.New("auth: session not recognised by region")

// Session is the record region returns for a valid sessionKey.
type Session struct {
	SessionKey int32  `json:"sessionKey"`
	AccountID  int64  `json:"accountId"`
	UserCode   string `json:"userCode"`
	WorldID    int    `json:"worldId"`
	ExpiresAt  int64  `json:"expiresAt"`
}

// Client calls the region service's /internal/sessions endpoints.
type Client struct {
	baseURL string
	consume bool
	http    *http.Client
}

// NewClient builds a region auth client. When consume is true it uses single-use
// validation (POST .../consume); otherwise the reusable GET.
func NewClient(baseURL string, consume bool, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		consume: consume,
		http:    &http.Client{Timeout: timeout},
	}
}

// Validate confirms that sessionKey is valid and bound to accountCode, returning
// the session on success or ErrSessionInvalid otherwise.
func (c *Client) Validate(ctx context.Context, sessionKey int32, accountCode string) (Session, error) {
	method := http.MethodGet
	url := fmt.Sprintf("%s/internal/sessions/%s", c.baseURL, strconv.FormatInt(int64(sessionKey), 10))
	if c.consume {
		method = http.MethodPost
		url += "/consume"
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return Session{}, fmt.Errorf("auth: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("auth: call region: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Session{}, ErrSessionInvalid
	}
	if resp.StatusCode != http.StatusOK {
		return Session{}, fmt.Errorf("auth: region returned status %d", resp.StatusCode)
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return Session{}, fmt.Errorf("auth: decode region response: %w", err)
	}
	if session.UserCode != accountCode {
		return Session{}, ErrSessionInvalid
	}
	return session, nil
}
