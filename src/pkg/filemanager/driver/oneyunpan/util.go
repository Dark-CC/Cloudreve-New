package oneyunpan

import (
	"math/big"
	"strings"
)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func pathBase(remotePath string) string {
	remotePath = strings.Trim(remotePath, "/")
	if idx := strings.LastIndex(remotePath, "/"); idx >= 0 {
		return remotePath[idx+1:]
	}
	return remotePath
}

// encodeURIComponent mirrors JavaScript's encodeURIComponent for the header
// values expected by the 1yunpan upload worker. url.QueryEscape would encode
// spaces as "+", which the worker does not decode in header context.
func encodeURIComponent(value string) string {
	const hexDigits = "0123456789ABCDEF"

	var out strings.Builder
	out.Grow(len(value))

	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '!' || c == '~' || c == '*' ||
			c == '\'' || c == '(' || c == ')' {
			out.WriteByte(c)
			continue
		}

		out.WriteByte('%')
		out.WriteByte(hexDigits[c>>4])
		out.WriteByte(hexDigits[c&0x0f])
	}

	return out.String()
}

// hexToBase62 converts a hex digest to the unpadded base62 representation used
// by 1yunpan's fast-upload hash.
func hexToBase62(hexString string) string {
	if hexString == "" {
		return ""
	}

	num := new(big.Int)
	if _, ok := num.SetString(hexString, 16); !ok {
		return ""
	}
	if num.Sign() == 0 {
		return "0"
	}

	base := new(big.Int).SetInt64(62)
	mod := new(big.Int)
	var out []byte

	for num.Sign() > 0 {
		num.DivMod(num, base, mod)
		out = append(out, base62Alphabet[mod.Int64()])
	}

	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

// base62Hash is the 43-character, zero-padded hash used by the 1yunpan web
// client for fast upload.
func base62Hash(hexDigest string) string {
	encoded := hexToBase62(hexDigest)
	if len(encoded) >= 43 {
		return encoded
	}
	return strings.Repeat("0", 43-len(encoded)) + encoded
}
