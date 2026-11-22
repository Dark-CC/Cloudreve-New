package oneyunpan

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// EdgeTicket is the sealed payload handed to the browser for edge uploads. It
// carries the 1yunpan session so the edge never has to store an account, and it
// is encrypted so the session cannot be lifted out of the browser.
type EdgeTicket struct {
	// Session is the 1yunpan phpsessid, refreshed when the ticket was issued.
	Session string `json:"s"`
	// Path is the remote object path the browser is allowed to write.
	Path string `json:"p"`
	// Size is the declared file size, used to reject oversized uploads.
	Size int64 `json:"z"`
	// UploadEndpoint is the 1yunpan worker the edge must forward chunks to.
	UploadEndpoint string `json:"u,omitempty"`
	// Expires is a Unix timestamp (seconds) after which the ticket is invalid.
	Expires int64 `json:"e"`
	// RefreshKey authenticates the refresh request the edge makes when the
	// embedded session is rejected mid-upload.
	RefreshKey string `json:"r"`
}

var (
	// ErrTicketKeyInvalid means the configured ticket key is unusable.
	ErrTicketKeyInvalid = errors.New("1yunpan ticket key must be 32 bytes (base64 or raw)")
	// ErrTicketInvalid means the ticket is malformed, forged, or truncated.
	ErrTicketInvalid = errors.New("1yunpan upload ticket is invalid")
	// ErrTicketExpired means the ticket is past its expiry.
	ErrTicketExpired = errors.New("1yunpan upload ticket has expired")
)

// DecodeTicketKey accepts a base64 (standard or URL, padded or not) 32-byte
// key. A raw 32-byte string is accepted as well so operators can paste one in.
func DecodeTicketKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrTicketKeyInvalid
	}

	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if decoded, err := enc.DecodeString(raw); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}

	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, ErrTicketKeyInvalid
}

// SealTicket encrypts a ticket with AES-256-GCM. The output is URL-safe so it
// can travel in a request header without further escaping.
func SealTicket(key []byte, ticket *EdgeTicket) (string, error) {
	return SealTicketWithSite(key, "", ticket)
}

// SealTicketWithSite prefixes the sealed payload with a plaintext site id so a
// shared edge can pick the right key before decrypting anything. The site id is
// not secret; it only selects which ticket key to use.
func SealTicketWithSite(key []byte, site string, ticket *EdgeTicket) (string, error) {
	if len(key) != 32 {
		return "", ErrTicketKeyInvalid
	}
	if ticket == nil {
		return "", ErrTicketInvalid
	}

	plain, err := json.Marshal(ticket)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := aead.Seal(nonce, nonce, plain, nil)
	encoded := base64.RawURLEncoding.EncodeToString(sealed)
	site = strings.TrimSpace(site)
	if site == "" {
		return encoded, nil
	}
	if strings.ContainsAny(site, ".") {
		return "", errors.New("1yunpan edge site id must not contain a dot")
	}
	return site + "." + encoded, nil
}

// SplitTicket separates the optional plaintext site id from the sealed payload.
func SplitTicket(encoded string) (site, payload string) {
	encoded = strings.TrimSpace(encoded)
	if idx := strings.IndexByte(encoded, '.'); idx > 0 && idx < 64 {
		return encoded[:idx], encoded[idx+1:]
	}
	return "", encoded
}

// OpenTicket decrypts and validates a ticket. The caller is expected to pass
// the same key that was used to seal it.
func OpenTicket(key []byte, encoded string, now time.Time) (*EdgeTicket, error) {
	if len(key) != 32 {
		return nil, ErrTicketKeyInvalid
	}

	_, encoded = SplitTicket(encoded)
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		if padded, padErr := base64.URLEncoding.DecodeString(strings.TrimSpace(encoded)); padErr == nil {
			raw = padded
		} else {
			return nil, ErrTicketInvalid
		}
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < aead.NonceSize() {
		return nil, ErrTicketInvalid
	}

	nonce, ciphertext := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrTicketInvalid
	}

	var ticket EdgeTicket
	if err := json.Unmarshal(plain, &ticket); err != nil {
		return nil, ErrTicketInvalid
	}
	if ticket.Session == "" || ticket.Path == "" {
		return nil, ErrTicketInvalid
	}
	if ticket.Expires > 0 && now.Unix() > ticket.Expires {
		return nil, ErrTicketExpired
	}

	return &ticket, nil
}

// NewRefreshKey returns a random key identifying one refreshable upload. It is
// stored inside the ticket so the edge can prove ownership when asking for a
// fresh session.
func NewRefreshKey() (string, error) {
	buf := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("failed to generate refresh key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
