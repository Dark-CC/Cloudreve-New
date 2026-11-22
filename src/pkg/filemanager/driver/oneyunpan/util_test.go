package oneyunpan

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestHexToBase62(t *testing.T) {
	// sha256("hello world")
	const digest = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	const expected = "hwJv5HX24TB0CGVfFeR48VzqDhD86wniVmEGyVQSgcj"

	if got := hexToBase62(digest); got != expected {
		t.Fatalf("hexToBase62() = %q, want %q", got, expected)
	}
	if got := base62Hash(digest); got != expected {
		t.Fatalf("base62Hash() = %q, want %q", got, expected)
	}
}

func TestBase62HashPadsTo43(t *testing.T) {
	got := base62Hash("1")
	if len(got) != 43 {
		t.Fatalf("base62Hash() length = %d, want 43", len(got))
	}
	if got[42] != base62Alphabet[1] {
		t.Fatalf("base62Hash() = %q, want zero padding ending in 1", got)
	}
}

func TestEncodeURIComponentMatchesJavascript(t *testing.T) {
	cases := map[string]string{
		"/folder/a b.txt":    "%2Ffolder%2Fa%20b.txt",
		"/folder/中文.txt":     "%2Ffolder%2F%E4%B8%AD%E6%96%87.txt",
		"/folder/a+b(c).txt": "%2Ffolder%2Fa%2Bb(c).txt",
	}

	for input, expected := range cases {
		if got := encodeURIComponent(input); got != expected {
			// url.PathEscape leaves "/" unescaped, matching encodeURIComponent.
			t.Fatalf("encodeURIComponent(%q) = %q, want %q", input, got, expected)
		}
	}

	if _, err := url.PathUnescape(encodeURIComponent("/a b/中文.txt")); err != nil {
		t.Fatalf("encodeURIComponent produced invalid escape: %v", err)
	}
}

func TestRelativePath(t *testing.T) {
	cases := []struct {
		base, remote, expected string
	}{
		{"", "a/b.txt", "a/b.txt"},
		{"a", "a/b.txt", "b.txt"},
		{"a/", "/a/b.txt", "b.txt"},
		{"a", "a", ""},
		{"a", "ab/c.txt", "ab/c.txt"},
	}

	for _, tc := range cases {
		if got := relativePath(tc.base, tc.remote); got != tc.expected {
			t.Fatalf("relativePath(%q, %q) = %q, want %q", tc.base, tc.remote, got, tc.expected)
		}
	}
}

func TestDownloadPathEncoding(t *testing.T) {
	// 1yunpan treats an unescaped "+" as a space when resolving download
	// paths, so every segment must be encodeURIComponent-encoded.
	segments := []string{"测试 文件+1.bin", "a&b.txt"}
	encoded := make([]string, len(segments))
	for i, segment := range segments {
		encoded[i] = encodeURIComponent(segment)
	}

	const expected = "%E6%B5%8B%E8%AF%95%20%E6%96%87%E4%BB%B6%2B1.bin/a%26b.txt"
	if got := strings.Join(encoded, "/"); got != expected {
		t.Fatalf("encoded download path = %q, want %q", got, expected)
	}
}

func TestSpoolToTemp(t *testing.T) {
	content := []byte("spooled upload payload")
	file, size, err := spoolToTemp(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("spoolToTemp() error = %v", err)
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()

	if size != int64(len(content)) {
		t.Fatalf("spoolToTemp() size = %d, want %d", size, len(content))
	}

	got := make([]byte, len(content))
	if _, err := io.ReadFull(file, got); err != nil {
		t.Fatalf("failed to read spool file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("spool content = %q, want %q", got, content)
	}
}
