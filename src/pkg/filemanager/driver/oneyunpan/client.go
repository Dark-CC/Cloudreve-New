package oneyunpan

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultAPIEndpoint is the only endpoint 1yunpan exposes.
	DefaultAPIEndpoint = "https://api.1yunpan.com/api"
	// DefaultDownloadEndpoint is the root host of 1yunpan direct links.
	DefaultDownloadEndpoint = "1yunpan.com"
	// DefaultUploadEndpoint is the worker that accepts chunked uploads.
	DefaultUploadEndpoint = "https://upload1.1yunpan.com"

	defaultAPIEndpoint      = DefaultAPIEndpoint
	defaultDownloadEndpoint = DefaultDownloadEndpoint
	defaultUploadEndpoint   = DefaultUploadEndpoint
	sessionLifetime         = 6 * 24 * time.Hour
)

var errUnauthorized = errors.New("1yunpan session unauthorized")

// clientCache keeps one client (and therefore one login session) per policy
// credential set. Cloudreve constructs a driver for every operation, so
// without this each list/download/upload would perform a fresh login.
var clientCache sync.Map // map[string]*Client

type Client struct {
	apiEndpoint      string
	downloadEndpoint string
	username         string
	password         string

	httpClient *http.Client

	mu        sync.Mutex
	sessionID string
	userID    string
	expiresAt time.Time
}

type FileEntry struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Type              string `json:"type"`
	Path              string `json:"path"`
	Size              int64  `json:"size"`
	Modified          string `json:"modified"`
	ModifiedTimestamp int64  `json:"modified_timestamp"`
	Extension         string `json:"extension"`
	Share             int    `json:"share"`
}

type listResponse struct {
	Success    bool        `json:"success"`
	Message    string      `json:"message"`
	Files      []FileEntry `json:"files"`
	Pagination struct {
		HasMore bool `json:"has_more"`
	} `json:"pagination"`
}

type baseResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type loginResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	PHPSessID   string `json:"phpsessid"`
	Requires2FA bool   `json:"requires_2fa"`
	User        struct {
		ID          int    `json:"id"`
		SnowflakeID string `json:"snowflake_id"`
	} `json:"user"`
}

func NewClient(apiEndpoint, downloadEndpoint, username, password string) (*Client, error) {
	apiEndpoint = strings.TrimRight(strings.TrimSpace(apiEndpoint), "/")
	if apiEndpoint == "" {
		apiEndpoint = defaultAPIEndpoint
	}

	downloadEndpoint = strings.TrimSpace(downloadEndpoint)
	downloadEndpoint = strings.TrimPrefix(downloadEndpoint, "https://")
	downloadEndpoint = strings.TrimPrefix(downloadEndpoint, "http://")
	downloadEndpoint = strings.TrimRight(downloadEndpoint, "/")
	if downloadEndpoint == "" {
		downloadEndpoint = defaultDownloadEndpoint
	}

	if strings.TrimSpace(username) == "" || password == "" {
		return nil, errors.New("1yunpan username and password are required")
	}

	key := clientCacheKey(apiEndpoint, downloadEndpoint, username, password)
	if cached, ok := clientCache.Load(key); ok {
		return cached.(*Client), nil
	}

	client := &Client{
		apiEndpoint:      apiEndpoint,
		downloadEndpoint: downloadEndpoint,
		username:         username,
		password:         password,
		httpClient:       &http.Client{},
	}

	actual, _ := clientCache.LoadOrStore(key, client)
	return actual.(*Client), nil
}

func clientCacheKey(apiEndpoint, downloadEndpoint, username, password string) string {
	sum := sha256.Sum256([]byte(apiEndpoint + "\x00" + downloadEndpoint + "\x00" + username + "\x00" + password))
	return hex.EncodeToString(sum[:])
}

// ForgetCredentials drops any cached session for the given credential set. It
// is called when a storage policy is updated or deleted so a rotated password
// or endpoint takes effect immediately instead of reusing a stale login.
func ForgetCredentials(apiEndpoint, downloadEndpoint, username, password string) {
	apiEndpoint = strings.TrimRight(strings.TrimSpace(apiEndpoint), "/")
	if apiEndpoint == "" {
		apiEndpoint = defaultAPIEndpoint
	}

	downloadEndpoint = strings.TrimSpace(downloadEndpoint)
	downloadEndpoint = strings.TrimPrefix(downloadEndpoint, "https://")
	downloadEndpoint = strings.TrimPrefix(downloadEndpoint, "http://")
	downloadEndpoint = strings.TrimRight(downloadEndpoint, "/")
	if downloadEndpoint == "" {
		downloadEndpoint = defaultDownloadEndpoint
	}

	if cached, ok := clientCache.LoadAndDelete(clientCacheKey(apiEndpoint, downloadEndpoint, username, password)); ok {
		if client, ok := cached.(*Client); ok {
			client.reset()
		}
	}
}

// reset clears the cached login session without contacting the remote service.
func (c *Client) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = ""
	c.userID = ""
	c.expiresAt = time.Time{}
}

func (c *Client) Session(ctx context.Context) (string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionID != "" && time.Now().Before(c.expiresAt) {
		return c.sessionID, c.userID, nil
	}

	sum := md5.Sum([]byte(c.password))
	payload, err := json.Marshal(map[string]string{
		"identifier": c.username,
		"password":   hex.EncodeToString(sum[:]),
	})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiEndpoint+"/login.php", bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("1yunpan login request failed: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", "", err
	}

	var login loginResponse
	if err := json.Unmarshal(body, &login); err != nil {
		return "", "", fmt.Errorf("failed to decode 1yunpan login response: %w", err)
	}

	if !login.Success || login.PHPSessID == "" {
		message := strings.TrimSpace(login.Message)
		if message == "" {
			message = strings.TrimSpace(string(body))
		}
		return "", "", fmt.Errorf("1yunpan login rejected: %s", message)
	}

	if login.Requires2FA {
		return "", "", errors.New("1yunpan account requires 2FA which is not supported by this storage policy")
	}

	c.sessionID = login.PHPSessID
	c.userID = login.User.SnowflakeID
	c.expiresAt = time.Now().Add(sessionLifetime)
	return c.sessionID, c.userID, nil
}

func (c *Client) Invalidate(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// An empty sessionID means "drop whatever is cached".
	if sessionID == "" || c.sessionID == sessionID {
		c.sessionID = ""
		c.userID = ""
		c.expiresAt = time.Time{}
	}
}

func (c *Client) UserID(ctx context.Context) (string, error) {
	_, userID, err := c.Session(ctx)
	return userID, err
}

func (c *Client) doAPI(ctx context.Context, method, endpoint string, query url.Values, payload []byte, contentType string) ([]byte, error) {
	u, err := url.Parse(c.apiEndpoint + "/" + strings.TrimLeft(endpoint, "/"))
	if err != nil {
		return nil, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusUnauthorized {
		return nil, errUnauthorized
	}

	if res.StatusCode >= 400 {
		var base baseResponse
		_ = json.Unmarshal(data, &base)
		message := strings.TrimSpace(base.Message)
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return nil, fmt.Errorf("1yunpan API %s failed (status %d): %s", endpoint, res.StatusCode, message)
	}

	return data, nil
}

func (c *Client) withSession(ctx context.Context, fn func(sessionID string) error) error {
	sessionID, _, err := c.Session(ctx)
	if err != nil {
		return err
	}

	err = fn(sessionID)
	if errors.Is(err, errUnauthorized) {
		c.Invalidate(sessionID)
		sessionID, _, err = c.Session(ctx)
		if err != nil {
			return err
		}
		return fn(sessionID)
	}
	return err
}

func (c *Client) List(ctx context.Context, base string) ([]FileEntry, error) {
	base = strings.Trim(base, "/")
	entries := make([]FileEntry, 0, 128)
	seen := make(map[string]struct{}, 128)

	// 1yunpan paginates folders and files independently when load_type=all;
	// page 1 of "all" can therefore contain only folders and push every file
	// to later pages. Query both kinds explicitly so no entry is skipped.
	for _, loadType := range []string{"folders", "files"} {
		// Track what this kind has returned so "no new entries" only stops
		// this kind's pagination, not the other kind's.
		kindSeen := make(map[string]struct{}, 128)
		for page := 1; page <= 10000; page++ {
			added := 0
			var data []byte
			err := c.withSession(ctx, func(sessionID string) error {
				query := url.Values{}
				query.Set("path", base)
				query.Set("load_type", loadType)
				query.Set("page", fmt.Sprintf("%d", page))
				query.Set("per_page", "100")
				query.Set("phpsessid", sessionID)

				res, err := c.doAPI(ctx, http.MethodGet, "filesystem.php", query, nil, "")
				if err != nil {
					return err
				}
				data = res
				return nil
			})
			if err != nil {
				return nil, err
			}

			var parsed listResponse
			if err := json.Unmarshal(data, &parsed); err != nil {
				return nil, fmt.Errorf("failed to decode 1yunpan file list: %w", err)
			}
			if !parsed.Success {
				return nil, fmt.Errorf("1yunpan file list failed: %s", parsed.Message)
			}

			for _, entry := range parsed.Files {
				key := strings.Trim(entry.Path, "/")
				if _, ok := kindSeen[key]; ok {
					continue
				}
				kindSeen[key] = struct{}{}
				added++
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				entries = append(entries, entry)
			}

			// The API can keep reporting has_more on empty trailing pages;
			// stop as soon as a page contributes nothing new.
			if !parsed.Pagination.HasMore || added == 0 {
				break
			}
		}
	}

	return entries, nil
}

func (c *Client) CreateFolder(ctx context.Context, base, name string) error {
	base = strings.Trim(base, "/")
	form := url.Values{}
	form.Set("action", "create_folder")
	form.Set("folder_name", name)

	return c.withSession(ctx, func(sessionID string) error {
		query := url.Values{}
		query.Set("path", base)
		query.Set("phpsessid", sessionID)
		_, err := c.doAPI(ctx, http.MethodPost, "filesystem.php", query, []byte(form.Encode()), "application/x-www-form-urlencoded")
		return err
	})
}

func (c *Client) Delete(ctx context.Context, remotePaths ...string) error {
	if len(remotePaths) == 0 {
		return nil
	}

	payload, err := json.Marshal(map[string][]string{"items": remotePaths})
	if err != nil {
		return err
	}

	return c.withSession(ctx, func(sessionID string) error {
		query := url.Values{}
		query.Set("phpsessid", sessionID)
		_, err := c.doAPI(ctx, http.MethodDelete, "filesystem.php", query, payload, "application/json")
		return err
	})
}

func (c *Client) DownloadURL(ctx context.Context, remotePath string) (string, error) {
	userID, err := c.UserID(ctx)
	if err != nil {
		return "", err
	}
	if userID == "" {
		return "", errors.New("1yunpan user id is unavailable")
	}

	parts := strings.Split(strings.Trim(remotePath, "/"), "/")
	for i, part := range parts {
		parts[i] = encodeURIComponent(part)
	}

	return fmt.Sprintf("https://id%s.%s/%s", userID, c.downloadEndpoint, strings.Join(parts, "/")), nil
}

func (c *Client) Logout(ctx context.Context) {
	c.mu.Lock()
	sessionID := c.sessionID
	c.sessionID = ""
	c.userID = ""
	c.expiresAt = time.Time{}
	c.mu.Unlock()

	if sessionID == "" {
		return
	}

	query := url.Values{}
	query.Set("phpsessid", sessionID)
	_, _ = c.doAPI(ctx, http.MethodPost, "logout.php", query, nil, "")
}
