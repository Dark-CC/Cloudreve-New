package uploadlink

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

const (
	// maxLinkTTL caps how long a public upload link can stay valid.
	maxLinkTTL = 30 * 24 * time.Hour
	// defaultLinkTTL is used when the caller does not pick an expiry.
	defaultLinkTTL = 24 * time.Hour
)

var (
	ErrLinkInvalid = serializer.NewError(serializer.CodeInvalidSign, "upload link is invalid", nil)
	ErrLinkExpired = serializer.NewError(serializer.CodeSignExpired, "upload link has expired", nil)
)

// claims is the encrypted content of an upload link. It never leaves the
// server in plaintext, so a link reveals neither the folder path nor the owner.
type claims struct {
	// Owner is the user whose storage and quota receive the upload.
	Owner int `json:"o"`
	// Path is the destination folder, relative to the owner's file root.
	Path string `json:"p"`
	// Policy optionally pins the storage policy. Zero lets the owner's group
	// policy decide, which keeps links working after a policy migration.
	Policy int `json:"s,omitempty"`
	// Expires is a Unix timestamp.
	Expires int64 `json:"e"`
	// MaxSize limits a single file. Zero means the policy maximum.
	MaxSize int64 `json:"m,omitempty"`
	// Note is an optional operator-facing label, e.g. "客户资料".
	Note string `json:"n,omitempty"`
}

// linkAEAD derives a domain-separated AES-256-GCM key from the site secret.
func linkAEAD(dep dependency.Dep) (cipher.AEAD, error) {
	seed := dep.SettingProvider().SecretKey(nil)
	if seed == "" {
		return nil, errors.New("site secret key is empty")
	}

	sum := sha256.Sum256([]byte("Cloudreve/upload-link/v1|" + seed))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// sealLink encrypts the claims into a URL-safe token.
func sealLink(dep dependency.Dep, c *claims) (string, error) {
	aead, err := linkAEAD(dep)
	if err != nil {
		return "", err
	}

	plain, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := aead.Seal(nonce, nonce, plain, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// openLink decrypts and validates a token.
func openLink(dep dependency.Dep, token string) (*claims, error) {
	return openLinkAt(dep, token, false)
}

// openLinkAt decrypts a token. When allowExpired is set the expiry is not
// enforced, which lets an upload that started before the link expired still
// finish. Only completion and ticket refresh use that escape hatch.
func openLinkAt(dep dependency.Dep, token string, allowExpired bool) (*claims, error) {
	c, err := decodeLink(dep, token)
	if err != nil {
		return nil, err
	}
	if !allowExpired && time.Now().Unix() > c.Expires {
		return nil, ErrLinkExpired
	}
	return c, nil
}

// decodeLink verifies the AEAD tag and shape of a token without checking its
// expiry.
func decodeLink(dep dependency.Dep, token string) (*claims, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 4096 {
		return nil, ErrLinkInvalid
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		if padded, padErr := base64.URLEncoding.DecodeString(token); padErr == nil {
			raw = padded
		} else {
			return nil, ErrLinkInvalid
		}
	}

	aead, err := linkAEAD(dep)
	if err != nil {
		return nil, ErrLinkInvalid
	}
	if len(raw) <= aead.NonceSize() {
		return nil, ErrLinkInvalid
	}

	nonce, ciphertext := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrLinkInvalid
	}

	var c claims
	if err := json.Unmarshal(plain, &c); err != nil {
		return nil, ErrLinkInvalid
	}
	if c.Owner <= 0 || c.Expires == 0 {
		return nil, ErrLinkInvalid
	}

	return &c, nil
}
