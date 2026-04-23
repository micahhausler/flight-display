package opensky

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	baseURL  = "https://opensky-network.org/api"
	tokenURL = "https://auth.opensky-network.org/auth/realms/opensky-network/protocol/openid-connect/token"
)

// Client fetches aircraft state vectors from the OpenSky Network API.
type Client struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewClient creates a new OpenSky client. If clientID and clientSecret are empty,
// the client operates anonymously.
func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (c *Client) isAuthenticated() bool {
	return c.clientID != "" && c.clientSecret != ""
}

func (c *Client) ensureToken() error {
	if !c.isAuthenticated() {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	resp, err := c.httpClient.PostForm(tokenURL, map[string][]string{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	})
	if err != nil {
		return fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decoding token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	// Refresh 30 seconds before expiry
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-30) * time.Second)
	return nil
}

// FetchStates retrieves state vectors within the given bounding box.
// Returns nil, nil on rate-limit (429) — caller should treat as a skipped poll.
func (c *Client) FetchStates(latMin, lonMin, latMax, lonMax float64) (*StateResponse, error) {
	if err := c.ensureToken(); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	url := fmt.Sprintf("%s/states/all?lamin=%s&lomin=%s&lamax=%s&lomax=%s&extended=1",
		baseURL,
		strconv.FormatFloat(latMin, 'f', 4, 64),
		strconv.FormatFloat(lonMin, 'f', 4, 64),
		strconv.FormatFloat(latMax, 'f', 4, 64),
		strconv.FormatFloat(lonMax, 'f', 4, 64),
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if c.isAuthenticated() {
		c.mu.Lock()
		token := c.accessToken
		c.mu.Unlock()
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("X-Rate-Limit-Retry-After-Seconds")
		log.Printf("OpenSky rate limited, retry after %s seconds", retryAfter)
		return nil, nil // signal skipped poll, not an error
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenSky returned %d: %s", resp.StatusCode, string(body))
	}

	var stateResp StateResponse
	if err := json.NewDecoder(resp.Body).Decode(&stateResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &stateResp, nil
}
