package oneyunpan

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
)

// TestOneYunPanRoundTrip exercises the real 1yunpan HTTP API. It is skipped
// unless credentials are supplied through the environment, so CI never touches
// a live account:
//
//	ONEYUNPAN_TEST_USERNAME=... ONEYUNPAN_TEST_PASSWORD=... go test ./pkg/filemanager/driver/oneyunpan/ -run TestOneYunPanRoundTrip -v
func TestOneYunPanRoundTrip(t *testing.T) {
	username := os.Getenv("ONEYUNPAN_TEST_USERNAME")
	password := os.Getenv("ONEYUNPAN_TEST_PASSWORD")
	if username == "" || password == "" {
		t.Skip("ONEYUNPAN_TEST_USERNAME/ONEYUNPAN_TEST_PASSWORD not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := NewClient("", "", username, password)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Logout(context.Background())

	folder := fmt.Sprintf("codex-integration-%d", time.Now().UnixNano())
	if err := client.CreateFolder(ctx, "", folder); err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}
	if err := client.CreateFolder(ctx, folder, "nested"); err != nil {
		t.Fatalf("CreateFolder(nested) error = %v", err)
	}
	defer func() {
		if err := client.Delete(context.Background(), folder); err != nil {
			t.Logf("cleanup Delete(%q) error = %v", folder, err)
		}
	}()

	content := make([]byte, 3*1024*1024+517)
	if _, err := io.ReadFull(rand.Reader, content); err != nil {
		t.Fatalf("failed to generate payload: %v", err)
	}

	// Exercise multi-part chaining plus encodeURIComponent handling of spaces,
	// "+" and non-ASCII characters in both the path and file name.
	fileName := "测试 文件+1.bin"
	remotePath := "/" + folder + "/" + fileName
	uploader := client.uploader("")
	const partSize = 1024 * 1024
	if err := uploader.Upload(ctx, remotePath, bytes.NewReader(content), int64(len(content)), partSize); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	// 1yunpan's directory listing is eventually consistent; the upload
	// callback can succeed before the new entry becomes visible.
	var found *FileEntry
	for attempt := 0; attempt < 20; attempt++ {
		entries, err := client.List(ctx, folder)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		for i := range entries {
			if entries[i].Name == fileName {
				found = &entries[i]
				break
			}
		}
		if found != nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if found == nil {
		t.Fatalf("uploaded file %q did not appear in the 1yunpan listing", fileName)
	}
	if found.Size != int64(len(content)) {
		t.Fatalf("uploaded size = %d, want %d", found.Size, len(content))
	}
	downloadURL, err := client.DownloadURL(ctx, found.Path)
	if err != nil {
		t.Fatalf("DownloadURL() error = %v", err)
	}

	res, err := http.Get(downloadURL)
	if err != nil {
		t.Fatalf("download request error = %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d", res.StatusCode)
	}
	downloaded, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read download body: %v", err)
	}
	if !bytes.Equal(downloaded, content) {
		t.Fatalf("downloaded content does not match uploaded content (%d vs %d bytes)", len(downloaded), len(content))
	}

	// Thumbnail / media-metadata generation reads the entity through a Range
	// request, so make sure the direct link supports partial content.
	rangeReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		t.Fatalf("failed to build range request: %v", err)
	}
	rangeReq.Header.Set("Range", "bytes=1024-2047")
	rangeRes, err := http.DefaultClient.Do(rangeReq)
	if err != nil {
		t.Fatalf("range request error = %v", err)
	}
	defer rangeRes.Body.Close()
	partial, err := io.ReadAll(rangeRes.Body)
	if err != nil {
		t.Fatalf("failed to read range body: %v", err)
	}
	if rangeRes.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", rangeRes.StatusCode)
	}
	if !bytes.Equal(partial, content[1024:2048]) {
		t.Fatalf("range content mismatch (%d bytes)", len(partial))
	}

	nestedContent := []byte("nested payload")
	nestedUploader := client.uploader("")
	if err := nestedUploader.Upload(ctx, "/"+folder+"/nested/inner.txt", bytes.NewReader(nestedContent), int64(len(nestedContent)), 0); err != nil {
		t.Fatalf("nested Upload() error = %v", err)
	}

	lister := &Driver{
		policy:   &ent.StoragePolicy{Settings: &types.PolicySetting{}},
		client:   client,
		partSize: defaultUploadPartSize,
	}
	scoped, err := lister.List(ctx, folder, nil, true)
	if err != nil {
		t.Fatalf("scoped List() error = %v", err)
	}
	var scopedNested *fs.PhysicalObject
	for i := range scoped {
		if scoped[i].Source == folder+"/nested/inner.txt" {
			scopedNested = &scoped[i]
			break
		}
	}
	if scopedNested == nil {
		t.Fatalf("nested file not found in scoped listing (%d objects)", len(scoped))
	}
	if scopedNested.RelativePath != "nested/inner.txt" {
		t.Fatalf("scoped RelativePath = %q, want %q", scopedNested.RelativePath, "nested/inner.txt")
	}
}
