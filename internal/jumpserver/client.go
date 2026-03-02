package jumpserver

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Host represents a JumpServer host asset with metadata fields.
type Host struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Platform Platform          `json:"platform"`
	Labels   []Label           `json:"labels"`
	IsActive bool              `json:"is_active"`
	Comment  string            `json:"comment"`
	OrgID    string            `json:"org_id"`
	OrgName  string            `json:"org_name"`
	Nodes    []Node            `json:"nodes"`
	Accounts []any             `json:"accounts"`
	Spec     map[string]any    `json:"spec_info"`
}

// Platform describes the host platform in JumpServer.
type Platform struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Label is a key-value pair used for tagging assets in JumpServer.
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Node represents an asset tree node in JumpServer.
type Node struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_value"`
}

// PaginatedResponse wraps the JumpServer paginated API response.
type PaginatedResponse struct {
	Count    int             `json:"count"`
	Next     string          `json:"next"`
	Previous string          `json:"previous"`
	Results  json.RawMessage `json:"results"`
}

// Client is a JumpServer API client using access key authentication.
type Client struct {
	baseURL    string
	orgID      string
	httpClient *http.Client
	signer     *Signer
	logger     *slog.Logger
	pageSize   int
	activeOnly bool
}

// ClientOption configures the JumpServer client.
type ClientOption func(*Client)

// WithTimeout sets the HTTP request timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithInsecureSkipVerify disables TLS certificate verification.
func WithInsecureSkipVerify(skip bool) ClientOption {
	return func(c *Client) {
		if t, ok := c.httpClient.Transport.(*http.Transport); ok {
			t.TLSClientConfig = &tls.Config{InsecureSkipVerify: skip} //nolint:gosec
		}
	}
}

// WithOrgID sets the JumpServer organization ID header.
func WithOrgID(orgID string) ClientOption {
	return func(c *Client) {
		c.orgID = orgID
	}
}

// WithPageSize sets the number of results per API page.
func WithPageSize(size int) ClientOption {
	return func(c *Client) {
		c.pageSize = size
	}
}

// WithActiveOnly controls whether to fetch only active hosts.
func WithActiveOnly(active bool) ClientOption {
	return func(c *Client) {
		c.activeOnly = active
	}
}

// WithLogger sets a custom logger for the client.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// NewClient creates a new JumpServer API client.
func NewClient(baseURL, keyID, keySecret string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		orgID:   "",
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{},
		},
		signer:     NewSigner(keyID, keySecret),
		logger:     slog.Default(),
		pageSize:   100,
		activeOnly: true,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// FetchAllHosts retrieves all active host assets from JumpServer with pagination.
func (c *Client) FetchAllHosts(ctx context.Context) ([]Host, error) {
	var allHosts []Host
	offset := 0

	for {
		hosts, total, err := c.fetchHostsPage(ctx, offset, c.pageSize)
		if err != nil {
			return nil, fmt.Errorf("fetch hosts page offset=%d: %w", offset, err)
		}
		allHosts = append(allHosts, hosts...)
		c.logger.Debug("Fetched hosts page",
			"offset", offset,
			"page_count", len(hosts),
			"total", total,
			"accumulated", len(allHosts),
		)

		offset += c.pageSize
		if offset >= total {
			break
		}
	}

	c.logger.Info("Fetched all hosts from JumpServer",
		"total", len(allHosts),
	)
	return allHosts, nil
}

// fetchHostsPage retrieves a single page of hosts.
func (c *Client) fetchHostsPage(ctx context.Context, offset, limit int) ([]Host, int, error) {
	params := url.Values{}
	params.Set("format", "json")
	if c.activeOnly {
		params.Set("is_active", "true")
	}
	params.Set("limit", strconv.Itoa(limit))
	params.Set("offset", strconv.Itoa(offset))

	endpoint := fmt.Sprintf("%s/api/v1/assets/hosts/?%s", c.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.orgID != "" {
		req.Header.Set("X-JMS-ORG", c.orgID)
	}

	if err := c.signer.Sign(req); err != nil {
		return nil, 0, fmt.Errorf("sign request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var paginated PaginatedResponse
	if err := json.Unmarshal(body, &paginated); err != nil {
		return nil, 0, fmt.Errorf("decode paginated response: %w", err)
	}

	var hosts []Host
	if err := json.Unmarshal(paginated.Results, &hosts); err != nil {
		return nil, 0, fmt.Errorf("decode hosts: %w", err)
	}

	return hosts, paginated.Count, nil
}
