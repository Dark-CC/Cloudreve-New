package oneyunpan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultUploadPartSize = int64(25 << 20)
	maxUploadPartSize     = int64(88 << 20)
	minUploadPartSize     = int64(1 << 20)
	maxUploadRetries      = 5
	uploadRequestTimeout  = 10 * time.Minute
)

var errFileTooSmall = errors.New("1yunpan requires files to be at least 2 bytes")

type Uploader struct {
	client     *Client
	endpoint   string
	httpClient *http.Client
}

type uploadResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Next    string `json:"next"`
}

func (c *Client) uploader(endpoint string) *Uploader {
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimRight(endpoint, "/")
	if endpoint == "" {
		endpoint = defaultUploadEndpoint
	}

	return &Uploader{
		client:     c,
		endpoint:   endpoint,
		httpClient: &http.Client{Timeout: uploadRequestTimeout},
	}
}

func (u *Uploader) Upload(ctx context.Context, remotePath string, content io.Reader, size, partSize int64) error {
	if size < 2 {
		return errFileTooSmall
	}

	sessionID, _, err := u.client.Session(ctx)
	if err != nil {
		return err
	}

	return u.upload(ctx, remotePath, content, size, partSize, sessionID)
}

// upload streams the file to 1yunpan without computing a client-side hash.
// The /v1/upload endpoint lets the worker hash the file server-side, so no
// seekable source or temporary file is required.
func (u *Uploader) upload(
	ctx context.Context,
	remotePath string,
	content io.Reader,
	size, partSize int64,
	sessionID string,
) error {
	if size < 2 {
		return errFileTooSmall
	}
	if partSize <= 0 {
		partSize = defaultUploadPartSize
	}
	if partSize < minUploadPartSize {
		partSize = minUploadPartSize
	}
	if partSize > maxUploadPartSize {
		partSize = maxUploadPartSize
	}

	// The first part must be exactly one byte.
	first := make([]byte, 1)
	if _, err := io.ReadFull(content, first); err != nil {
		return fmt.Errorf("failed to read first byte: %w", err)
	}

	nextToken, err := u.sendChunk(ctx, first, 0, 1, size, "", false, sessionID, remotePath)
	if err != nil {
		return err
	}

	buffer := make([]byte, partSize)
	offset := int64(1)

	for offset < size {
		want := partSize
		if remaining := size - offset; remaining < want {
			want = remaining
		}

		if _, err := io.ReadFull(content, buffer[:want]); err != nil {
			return fmt.Errorf("failed to read upload chunk at offset %d: %w", offset, err)
		}

		start := offset
		end := offset + want
		isFinal := end == size

		next, err := u.sendChunk(ctx, buffer[:want], start, end, size, nextToken, isFinal, sessionID, remotePath)
		if err != nil {
			return err
		}
		if isFinal {
			return nil
		}

		nextToken = next
		offset = end
	}

	return errors.New("1yunpan upload ended before the final part")
}

func (u *Uploader) sendChunk(
	ctx context.Context,
	chunk []byte,
	start, end, total int64,
	nextToken string,
	isFinal bool,
	sessionID string,
	remotePath string,
) (string, error) {
	var lastErr error

	for attempt := 0; attempt < maxUploadRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.endpoint+"/v1/upload", bytes.NewReader(chunk))
		if err != nil {
			return "", err
		}

		req.Header.Set("Content-Range", fmt.Sprintf("%d-%d", start, end-1))
		if nextToken != "" {
			req.Header.Set("X-Next-Token", nextToken)
		}
		req.Header.Set("Content-Type", "application/octet-stream")

		if isFinal {
			req.Header.Set("X-Upload-URI", encodeURIComponent(remotePath))
			req.Header.Set("X-Phpsessid", sessionID)
			// /v1/upload asks the worker to calculate the hash; "0" is the
			// explicit "no client hash" marker used by the web client.
			req.Header.Set("X-File-Hash", "0")
			req.Header.Set("X-File-Size", fmt.Sprintf("%d", total))
			req.Header.Set("X-File-Name", encodeURIComponent(pathBase(remotePath)))
		}

		res, err := u.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			lastErr = fmt.Errorf(
				"upload chunk %d-%d failed (status %d): %s",
				start, end-1, res.StatusCode, strings.TrimSpace(string(body)),
			)
			continue
		}
		var parsed uploadResponse
		decoded := json.Unmarshal(body, &parsed) == nil
		if decoded && !parsed.Status && strings.TrimSpace(parsed.Message) != "" {
			lastErr = fmt.Errorf("upload rejected: %s", strings.TrimSpace(parsed.Message))
			continue
		}

		if isFinal {
			// The worker may return an empty or non-JSON body once the final
			// part succeeds; the HTTP status is authoritative there.
			return "", nil
		}
		if !decoded {
			lastErr = errors.New("failed to decode 1yunpan upload response")
			continue
		}

		return parsed.Next, nil
	}

	if lastErr == nil {
		lastErr = errors.New("upload failed with an unknown error")
	}
	return "", lastErr
}
