package oneyunpan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs/mime"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
)

var features = &boolset.BooleanSet{}

// ListPathRealRootCtx carries the list root through recursive calls so
// RelativePath stays relative to the user-selected directory.
type ListPathRealRootCtx struct{}

type Driver struct {
	policy   *ent.StoragePolicy
	settings setting.Provider
	config   conf.ConfigProvider
	l        logging.Logger
	mime     mime.MimeDetector
	client   *Client
	partSize int64
}

func New(
	ctx context.Context,
	policy *ent.StoragePolicy,
	settings setting.Provider,
	config conf.ConfigProvider,
	l logging.Logger,
	mimeDetector mime.MimeDetector,
) (*Driver, error) {
	var (
		partSize     int64
		downloadHost string
	)
	if policy.Settings != nil {
		partSize = policy.Settings.ChunkSize
		downloadHost = policy.Settings.OneyunpanDownloadEndpoint
	}
	if partSize <= 0 {
		partSize = defaultUploadPartSize
	}

	client, err := NewClient(policy.Server, downloadHost, policy.AccessKey, policy.SecretKey)
	if err != nil {
		return nil, err
	}

	return &Driver{
		policy:   policy,
		settings: settings,
		config:   config,
		l:        l,
		mime:     mimeDetector,
		client:   client,
		partSize: partSize,
	}, nil
}

func (d *Driver) List(ctx context.Context, base string, onProgress driver.ListProgressFunc, recursive bool) ([]fs.PhysicalObject, error) {
	base = strings.Trim(base, "/")
	root := base
	if realRoot, ok := ctx.Value(ListPathRealRootCtx{}).(string); ok {
		root = realRoot
	} else {
		ctx = context.WithValue(ctx, ListPathRealRootCtx{}, base)
	}

	entries, err := d.client.List(ctx, base)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(entries))
	visited := make(map[string]struct{}, len(entries))
	res := make([]fs.PhysicalObject, 0, len(entries))
	children := make([]string, 0, len(entries))
	for _, entry := range entries {
		entryPath := path.Clean(strings.Trim(entry.Path, "/"))
		if entryPath == "" || entryPath == base {
			continue
		}

		relative := relativePath(root, entry.Path)
		if relative == "" || relative == "." {
			continue
		}
		if _, ok := seen[entryPath]; ok {
			continue
		}
		seen[entryPath] = struct{}{}
		if _, ok := visited[entryPath]; ok {
			continue
		}
		visited[entryPath] = struct{}{}

		isDir := entry.Type == "folder"
		res = append(res, fs.PhysicalObject{
			Name:         entry.Name,
			Source:       entryPath,
			RelativePath: relative,
			Size:         entry.Size,
			IsDir:        isDir,
			LastModify:   parseModified(entry),
		})

		// Only descend when the returned folder is a strict child of the
		// current directory. Some responses include the requested folder
		// itself, which would otherwise recurse forever.
		if isDir {
			parent := path.Dir(entryPath)
			if parent == "." {
				parent = ""
			}
			if parent == base && entryPath != base {
				children = append(children, entryPath)
			}
		}
	}

	if recursive {
		for _, child := range children {
			if child == base {
				continue
			}
			sub, err := d.List(ctx, child, nil, true)
			if err != nil {
				d.l.Warning("Failed to list 1yunpan child folder %q: %s", child, err)
				continue
			}
			res = append(res, sub...)
		}
	}

	if onProgress != nil {
		onProgress(len(res))
	}
	return res, nil
}

func (d *Driver) Open(ctx context.Context, remotePath string) (*os.File, error) {
	return nil, errors.New("1yunpan driver does not support inbound file access")
}

func (d *Driver) LocalPath(ctx context.Context, remotePath string) string {
	return ""
}

func (d *Driver) Put(ctx context.Context, file *fs.UploadRequest) error {
	defer file.Close()

	if file.Offset != 0 {
		return errors.New("1yunpan only supports relayed uploads and cannot accept partial chunks")
	}
	if file.Props == nil {
		return errors.New("1yunpan upload request is missing properties")
	}

	savePath := strings.Trim(file.Props.SavePath, "/")
	if savePath == "" {
		return errors.New("1yunpan upload path is empty")
	}
	overwrite := file.Mode&fs.ModeOverwrite == fs.ModeOverwrite

	// Internal callers such as thumbnail generation and entity relocation can
	// omit Props.Size. Spool those streams to disk so the size is known before
	// the chunked upload starts.
	source := io.Reader(file)
	size := file.Props.Size
	if size <= 0 {
		spooled, spooledSize, err := spoolToTemp(file)
		if err != nil {
			return err
		}
		defer func() {
			_ = spooled.Close()
			_ = os.Remove(spooled.Name())
		}()
		source = spooled
		size = spooledSize
	}
	if size < 2 {
		return errFileTooSmall
	}

	parent := path.Dir(savePath)
	if parent == "." {
		parent = ""
	}
	if parent != "" {
		if err := d.ensureFolder(ctx, parent); err != nil {
			return err
		}
	}

	if !overwrite {
		if exists, err := d.exists(ctx, savePath); err == nil && exists {
			return fs.ErrFileExisted
		}
	}

	uploadEndpoint := ""
	if d.policy.Settings != nil {
		uploadEndpoint = d.policy.Settings.OneyunpanUploadEndpoint
	}

	uploader := d.client.uploader(uploadEndpoint)
	if err := uploader.Upload(ctx, "/"+savePath, source, size, d.partSize); err != nil {
		return fmt.Errorf("failed to upload file to 1yunpan: %w", err)
	}
	return nil
}

func spoolToTemp(content io.Reader) (*os.File, int64, error) {
	tmp, err := os.CreateTemp("", "cloudreve-1yunpan-*")
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create upload spool file: %w", err)
	}

	size, err := io.Copy(tmp, content)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, fmt.Errorf("failed to spool upload content: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, fmt.Errorf("failed to rewind upload spool file: %w", err)
	}

	return tmp, size, nil
}

func (d *Driver) Delete(ctx context.Context, files ...string) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}

	normalized := make([]string, 0, len(files))
	for _, file := range files {
		normalized = append(normalized, strings.Trim(file, "/"))
	}

	if err := d.client.Delete(ctx, normalized...); err != nil {
		return normalized, err
	}
	return nil, nil
}

func (d *Driver) Thumb(ctx context.Context, expire *time.Time, ext string, e fs.Entity) (string, error) {
	return "", nil
}

func (d *Driver) Source(ctx context.Context, e fs.Entity, args *driver.GetSourceArgs) (string, error) {
	return d.client.DownloadURL(ctx, e.Source())
}

// ForgetSession drops the cached login so the next call re-authenticates.
func (d *Driver) ForgetSession() {
	if d.client != nil {
		d.client.Invalidate("")
	}
}

// Session returns a valid 1yunpan session, logging in again if needed.
func (d *Driver) Session(ctx context.Context) (string, string, error) {
	return d.client.Session(ctx)
}

func (d *Driver) Token(ctx context.Context, uploadSession *fs.UploadSession, file *fs.UploadRequest) (*fs.UploadCredential, error) {
	edge := ResolveEdgeConfig(ctx, d.settings, d.policy)
	if !edge.Enabled() {
		return nil, errors.New("1yunpan driver only supports relayed upload unless an edge endpoint is configured")
	}

	key, err := DecodeTicketKey(edge.TicketKey)
	if err != nil {
		return nil, err
	}

	// Fetch a fresh session at issue time. The edge never stores credentials;
	// it only receives the short-lived sealed ticket below.
	sessionID, _, err := d.client.Session(ctx)
	if err != nil {
		return nil, err
	}

	refreshKey, err := NewRefreshKey()
	if err != nil {
		return nil, err
	}

	expires := uploadSession.Props.ExpireAt
	ticket, err := SealTicketWithSite(key, edge.Site, &EdgeTicket{
		Session:        sessionID,
		Path:           strings.Trim(uploadSession.Props.SavePath, "/"),
		Size:           uploadSession.Props.Size,
		UploadEndpoint: UploadEndpointForPolicy(d.policy),
		Expires:        expires.Unix(),
		RefreshKey:     refreshKey,
	})
	if err != nil {
		return nil, err
	}

	uploadURL := strings.TrimRight(edge.Endpoint, "/")
	res := &fs.UploadCredential{
		ChunkSize:       d.partSize,
		UploadURLs:      []string{uploadURL},
		Credential:      ticket,
		EdgeTicket:      ticket,
		Callback:        "",
		Uri:             uploadSession.Props.SavePath,
		CallbackSecret:  uploadSession.CallbackSecret,
		Expires:         expires.Unix(),
		MimeType:        uploadSession.Props.MimeType,
		EncryptMetadata: uploadSession.EncryptMetadata,
	}
	// The edge enforces the same 88 MiB ceiling as the direct driver.
	if res.ChunkSize > maxUploadPartSize {
		res.ChunkSize = maxUploadPartSize
	}

	return res, nil
}

func (d *Driver) CancelToken(ctx context.Context, uploadSession *fs.UploadSession) error {
	return nil
}

func (d *Driver) CompleteUpload(ctx context.Context, session *fs.UploadSession) error {
	if session == nil || session.Props == nil {
		return errors.New("1yunpan upload session is missing properties")
	}
	if session.Importing {
		return nil
	}

	// The edge reports completion, so verify the object really landed with the
	// declared size before the placeholder is promoted to a real file. The
	// worker may finalize asynchronously, hence the short retry window.
	expected := session.Props.Size
	target := strings.Trim(session.Props.SavePath, "/")
	parent := path.Dir(target)
	if parent == "." {
		parent = ""
	}
	name := path.Base(target)

	var lastErr error
	// A multi-gigabyte object can take a while to appear in the upstream
	// listing after the final part, so the window has to scale with the file
	// rather than stay at a few seconds that only suit small uploads.
	attempts := 10
	if session.Props.Size > 256<<20 {
		attempts = 20
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(min(5, attempt)) * time.Second):
			}
		}

		entries, err := d.client.List(ctx, parent)
		if err != nil {
			lastErr = err
			continue
		}
		for _, entry := range entries {
			if entry.Name != name || entry.Type == "folder" {
				continue
			}
			if entry.Size == expected {
				return nil
			}
			lastErr = fmt.Errorf("1yunpan object size mismatch: expected %d, got %d", expected, entry.Size)
		}
		if lastErr == nil {
			lastErr = errors.New("1yunpan object was not found after upload")
		}
	}

	return lastErr
}

func (d *Driver) Capabilities() *driver.Capabilities {
	return &driver.Capabilities{
		StaticFeatures: features,
		ThumbProxy:     true,
		MediaMetaProxy: true,
	}
}

func (d *Driver) MediaMeta(ctx context.Context, remotePath, ext, language string) ([]driver.MediaMeta, error) {
	return nil, nil
}

func (d *Driver) ensureFolder(ctx context.Context, folder string) error {
	parts := strings.Split(strings.Trim(folder, "/"), "/")
	current := ""

	for _, part := range parts {
		if part == "" {
			continue
		}

		next := part
		if current != "" {
			next = current + "/" + part
		}

		if exists, err := d.exists(ctx, next); err == nil && exists {
			current = next
			continue
		}

		if err := d.client.CreateFolder(ctx, current, part); err != nil {
			return err
		}
		current = next
	}

	return nil
}

func (d *Driver) exists(ctx context.Context, remotePath string) (bool, error) {
	parent := path.Dir(remotePath)
	if parent == "." {
		parent = ""
	}
	name := path.Base(remotePath)

	entries, err := d.client.List(ctx, parent)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func relativePath(base, remote string) string {
	base = strings.Trim(base, "/")
	remote = strings.Trim(remote, "/")

	if base == "" {
		return remote
	}
	if remote == base {
		return ""
	}
	if strings.HasPrefix(remote, base+"/") {
		return strings.TrimPrefix(remote, base+"/")
	}
	return remote
}

func parseModified(entry FileEntry) time.Time {
	if entry.ModifiedTimestamp > 0 {
		return time.Unix(entry.ModifiedTimestamp, 0)
	}

	layouts := []string{"2006-01-02 15:04:05", time.RFC3339}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, entry.Modified, time.Local); err == nil {
			return parsed
		}
	}
	return time.Now()
}
