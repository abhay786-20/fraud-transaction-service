// Package authclient calls fraud-auth-service's internal (service-to-
// service) API. This is app-specific, not a generic pkg/ utility — it
// knows the shape of fraud-auth-service's response, which is coupling
// that's fine to have here, not something worth hiding behind a fake
// "generic" abstraction.
package authclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var ErrUserNotFound = errors.New("user not found")

// User is only the subset of fraud-auth-service's user data this service
// actually needs — a separate type from fraud-auth-service's own dto.User,
// necessarily, since these are two different Go modules/repos; there's no
// way to share a type between them even if we wanted to.
type User struct {
	ID       string `json:"id"`
	IsActive bool   `json:"is_active"`
}

// Client calls fraud-auth-service's /internal/* routes.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		// A timeout is not optional for any outbound HTTP call — without
		// one, a slow or hanging fraud-auth-service would hang THIS
		// service's request forever, waiting on it.
		http: &http.Client{Timeout: 5 * time.Second},
	}
}

// GetUser looks up a user by ID via fraud-auth-service's internal API.
// Returns ErrUserNotFound if fraud-auth-service reports no such user.
func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	url := fmt.Sprintf("%s/internal/users/%s", c.baseURL, id)

	// NewRequestWithContext, not http.Get — this lets the CALLER's context
	// (e.g. tied to the original incoming HTTP request, or a deadline)
	// cancel this outbound call too, same ctx-propagation idea as
	// everywhere else in this codebase.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("X-Internal-Api-Key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling auth service: %w", err)
	}
	defer resp.Body.Close() // always close a response body — leaks the connection otherwise

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrUserNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth service returned unexpected status %d", resp.StatusCode)
	}

	// json.NewDecoder(resp.Body).Decode — decodes directly from the
	// response's byte STREAM, unlike json.Unmarshal, which needs the
	// whole []byte already in memory upfront. Same underlying job,
	// better fit when what you have is an io.Reader (like an HTTP body)
	// rather than a complete byte slice.
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &user, nil
}
