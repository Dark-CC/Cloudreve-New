package dbfs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/constants"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/samber/lo"
)

var sharedWithMeNavigatorCapability = &boolset.BooleanSet{}

// NewSharedWithMeNavigator creates a navigator for user's "shared with me" file system.
func NewSharedWithMeNavigator(u *ent.User, fileClient inventory.FileClient, shareClient inventory.ShareClient,
	shareACLClient inventory.ShareACLClient, settingClient setting.Provider, l logging.Logger,
	config *setting.DBFS, hasher hashid.Encoder) Navigator {
	n := &sharedWithMeNavigator{
		user:           u,
		l:              l,
		fileClient:     fileClient,
		shareClient:    shareClient,
		shareACLClient: shareACLClient,
		settingClient:  settingClient,
		config:         config,
		hasher:         hasher,
	}
	n.baseNavigator = newBaseNavigator(fileClient, defaultFilter, u, hasher, config)
	return n
}

type sharedWithMeNavigator struct {
	l              logging.Logger
	user           *ent.User
	fileClient     inventory.FileClient
	shareClient    inventory.ShareClient
	shareACLClient inventory.ShareACLClient
	settingClient  setting.Provider
	config         *setting.DBFS
	hasher         hashid.Encoder

	root *File
	*baseNavigator
}

func (t *sharedWithMeNavigator) Recycle() {

}

func (n *sharedWithMeNavigator) PersistState(kv cache.Driver, key string) {
}

func (n *sharedWithMeNavigator) RestoreState(s State) error {
	return nil
}

func (t *sharedWithMeNavigator) To(ctx context.Context, path *fs.URI) (*File, error) {
	// Anonymous user does not have a trash folder.
	if inventory.IsAnonymousUser(t.user) {
		return nil, ErrLoginRequired
	}

	elements := path.Elements()
	if len(elements) > 0 {
		// Shared with me folder is a flatten tree, only root can be accessed.
		return nil, fs.ErrPathNotExist.WithError(fmt.Errorf("invalid Path %q", path))
	}

	if t.root == nil {
		rootFile, err := t.fileClient.Root(ctx, t.user)
		if err != nil {
			t.l.Info("User's root folder not found: %s, will initialize it.", err)
			return nil, ErrFsNotInitialized
		}

		t.root = newFile(nil, rootFile)
		rootPath := newSharedWithMeUri("")
		t.root.Path[pathIndexRoot], t.root.Path[pathIndexUser] = rootPath, rootPath
		t.root.OwnerModel = t.user
		t.root.IsUserRoot = true
		t.root.CapabilitiesBs = t.Capabilities(false).Capability
	}

	return t.root, nil
}

func (t *sharedWithMeNavigator) Children(ctx context.Context, parent *File, args *ListArgs) (*ListResult, error) {
	if t.settingClient != nil && t.settingClient.SharedWithMeACL(ctx) &&
		t.shareClient != nil && t.shareACLClient != nil {
		return t.listByACL(ctx, args)
	}

	args.SharedWithMe = true
	res, err := t.baseNavigator.children(ctx, nil, args)
	if err != nil {
		return nil, err
	}

	// Adding user uri for each file.
	for i := 0; i < len(res.Files); i++ {
		res.Files[i].Path[pathIndexUser] = newSharedWithMeUri(hashid.EncodeFileID(t.hasher, res.Files[i].Model.ID))
	}

	return res, nil
}

func (t *sharedWithMeNavigator) listByACL(ctx context.Context, args *ListArgs) (*ListResult, error) {
	shares, err := t.allShares(ctx)
	if err != nil {
		return nil, err
	}

	shareIDs := lo.Map(shares, func(item *ent.Share, _ int) int { return item.ID })
	acls, err := t.shareACLClient.ListByShareIDs(ctx, shareIDs)
	if err != nil {
		t.l.Warning("Failed to load share ACLs for shared-with-me: %s", err)
		return nil, fmt.Errorf("failed to load share ACLs: %w", err)
	}

	userGroupID := 0
	if t.user != nil && t.user.Edges.Group != nil {
		userGroupID = t.user.Edges.Group.ID
	}

	files := make([]*File, 0, len(shares))
	for _, share := range shares {
		if share == nil || share.Edges.User == nil || share.Edges.File == nil {
			continue
		}
		if t.user != nil && share.Edges.User.ID == t.user.ID {
			continue
		}
		if err := inventory.IsValidShare(share); err != nil {
			continue
		}

		ownerGroupID := 0
		if share.Edges.User.Edges.Group != nil {
			ownerGroupID = share.Edges.User.Edges.Group.ID
		}
		permission := effectiveSharePermission(acls[share.ID], t.user, userGroupID, ownerGroupID)
		if !permission.Discoverable {
			continue
		}

		model := *share.Edges.File
		model.Edges.Metadata = append([]*ent.Metadata{
			{Name: MetadataSharedRedirect, Value: fs.NewShareUri(
				hashid.EncodeShareID(t.hasher, share.ID), "")},
			{Name: MetadataSharedOwner, Value: hashid.EncodeUserID(t.hasher, share.Edges.User.ID)},
		}, model.Edges.Metadata...)
		model.Edges.Shares = nil
		file := newFile(nil, &model)
		shareURI, err := fs.NewUriFromString(fs.NewShareUri(
			hashid.EncodeShareID(t.hasher, share.ID), ""))
		if err != nil {
			continue
		}
		file.Path[pathIndexRoot] = shareURI
		file.Path[pathIndexUser] = file.Path[pathIndexRoot]
		file.OwnerModel = share.Edges.User
		file.CapabilitiesBs = sharedWithMeNavigatorCapability
		files = append(files, file)
	}

	files = filterSharedWithMeFiles(ctx, files, args.Search)
	sortSharedWithMeFiles(files, args.Page)

	pageSize := defaultPageSize
	page := 0
	useCursor := false
	pageToken := ""
	if args.Page != nil {
		if args.Page.PageSize > 0 {
			pageSize = args.Page.PageSize
		}
		if args.Page.Page > 0 {
			page = args.Page.Page
		}
		useCursor = args.Page.UseCursorPagination
		pageToken = args.Page.PageToken
	}
	if pageSize > t.config.MaxPageSize && t.config.MaxPageSize > 0 {
		pageSize = t.config.MaxPageSize
	}

	if useCursor {
		start := 0
		if pageToken != "" {
			cursor, err := t.decodeSharedWithMeCursor(pageToken)
			if err != nil {
				return nil, fs.ErrNotSupportedAction.WithError(fmt.Errorf("invalid shared-with-me page token: %w", err))
			}
			start = sort.Search(len(files), func(i int) bool {
				return compareSharedWithMeFile(files[i], cursor, args.Page.OrderBy, args.Page.Order) > 0
			})
		}

		end := start + pageSize
		if end > len(files) {
			end = len(files)
		}

		pagination := &inventory.PaginationResults{
			PageSize: pageSize,
			IsCursor: true,
		}
		if end < len(files) && end > start {
			nextToken, err := t.encodeSharedWithMeCursor(files[end-1], args.Page)
			if err != nil {
				return nil, fmt.Errorf("failed to encode shared-with-me page token: %w", err)
			}
			pagination.NextPageToken = nextToken
		}

		return &ListResult{
			Files:      files[start:end],
			Pagination: pagination,
		}, nil
	}

	start := page * pageSize
	if start > len(files) {
		start = len(files)
	}
	end := start + pageSize
	if end > len(files) {
		end = len(files)
	}

	return &ListResult{
		Files: files[start:end],
		Pagination: &inventory.PaginationResults{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: len(files),
		},
	}, nil
}

func (t *sharedWithMeNavigator) allShares(ctx context.Context) ([]*ent.Share, error) {
	ctx = context.WithValue(ctx, inventory.LoadShareFile{}, true)
	ctx = context.WithValue(ctx, inventory.LoadShareUser{}, true)
	ctx = context.WithValue(ctx, inventory.LoadUserGroup{}, true)
	ctx = context.WithValue(ctx, inventory.LoadFilePublicMetadata{}, true)

	const pageSize = 100
	res := make([]*ent.Share, 0)
	for page := 0; ; page++ {
		list, err := t.shareClient.List(ctx, &inventory.ListShareArgs{
			PaginationArgs: &inventory.PaginationArgs{
				Page:     page,
				PageSize: pageSize,
				Order:    inventory.OrderDirectionAsc,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list shares: %w", err)
		}
		res = append(res, list.Shares...)
		if len(list.Shares) == 0 || list.TotalItems < 0 || len(res) >= list.TotalItems {
			break
		}
	}
	return res, nil
}

func effectiveSharePermission(acl *inventory.ShareACL, user *ent.User, userGroupID, ownerGroupID int) inventory.SharePermission {
	permission := inventory.SharePermission{CanList: true, CanRead: true}
	if acl == nil || len(acl.Builtin)+len(acl.Group)+len(acl.User) == 0 {
		return permission
	}
	anonymous := inventory.IsAnonymousUser(user)
	effective, matched := acl.EffectivePermission(user.ID, userGroupID, ownerGroupID, anonymous)
	if !matched {
		return inventory.SharePermission{}
	}
	return effective
}

func filterSharedWithMeFiles(ctx context.Context, files []*File, search *inventory.SearchFileParameters) []*File {
	_ = ctx
	if search == nil {
		return files
	}

	return lo.Filter(files, func(file *File, _ int) bool {
		if search.Category != "" && !matchSharedWithMeCategory(file, search.Category) {
			return false
		}
		if search.Type != nil && file.Type() != *search.Type {
			return false
		}
		if search.SizeGte > 0 && file.Size() < search.SizeGte {
			return false
		}
		if search.SizeLte > 0 && file.Size() > search.SizeLte {
			return false
		}
		if search.CreatedAtGte != nil && file.CreatedAt().Before(*search.CreatedAtGte) {
			return false
		}
		if search.CreatedAtLte != nil && file.CreatedAt().After(*search.CreatedAtLte) {
			return false
		}
		if search.UpdatedAtGte != nil && file.UpdatedAt().Before(*search.UpdatedAtGte) {
			return false
		}
		if search.UpdatedAtLte != nil && file.UpdatedAt().After(*search.UpdatedAtLte) {
			return false
		}
		for _, filter := range search.Metadata {
			value, exists := file.Metadata()[filter.Key]
			if filter.Exact {
				if !exists || value != filter.Value {
					return false
				}
				continue
			}
			if !exists {
				return false
			}
			if filter.Value != "" && !strings.Contains(strings.ToLower(value), strings.ToLower(filter.Value)) {
				return false
			}
		}
		if len(search.Name) == 0 {
			return true
		}

		name := strings.ToLower(file.Name())
		matches := lo.Map(search.Name, func(pattern string, _ int) bool {
			exact := strings.HasPrefix(pattern, `"`) && strings.HasSuffix(pattern, `"`)
			pattern = strings.ToLower(strings.Trim(pattern, `"`))
			if exact {
				return name == pattern
			}
			if strings.Contains(pattern, inventory.SearchWildcard) {
				return matchSharedWithMePattern(name, pattern)
			}
			if search.CaseFolding {
				return strings.Contains(name, pattern)
			}
			return strings.Contains(file.Name(), strings.Trim(pattern, `"`))
		})
		if search.NameOperatorOr {
			return lo.SomeBy(matches, func(item bool) bool { return item })
		}
		return lo.EveryBy(matches, func(item bool) bool { return item })
	})
}

func matchSharedWithMeCategory(file *File, category string) bool {
	ext := strings.TrimPrefix(strings.ToLower(file.Ext()), ".")
	switch strings.ToLower(category) {
	case "image":
		return lo.Contains([]string{"jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "heic", "heif", "avif", "tiff", "ico"}, ext)
	case "video":
		return lo.Contains([]string{"mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "m4v", "ts", "mpeg", "mpg"}, ext)
	case "audio":
		return lo.Contains([]string{"mp3", "flac", "wav", "aac", "ogg", "m4a", "wma", "opus", "ape"}, ext)
	case "document", "doc":
		return lo.Contains([]string{"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "md", "rtf", "odt", "ods", "odp", "epub"}, ext)
	case "archive", "compressed":
		return lo.Contains([]string{"zip", "rar", "7z", "tar", "gz", "bz2", "xz", "zst", "tgz", "tbz2"}, ext)
	default:
		return ext == strings.TrimPrefix(strings.ToLower(category), ".")
	}
}

func matchSharedWithMePattern(name, pattern string) bool {
	pattern = strings.ReplaceAll(pattern, inventory.SearchWildcard, "*")
	if !strings.HasPrefix(pattern, "*") && !strings.HasSuffix(pattern, "*") {
		pattern = "*" + pattern + "*"
	}
	if ok, err := url.PathUnescape(pattern); err == nil {
		pattern = ok
	}
	parts := strings.Split(pattern, "*")
	current := 0
	for index, part := range parts {
		if part == "" {
			continue
		}
		next := strings.Index(name[current:], part)
		if next < 0 {
			return false
		}
		if index == 0 && !strings.HasPrefix(pattern, "*") && next != 0 {
			return false
		}
		current += next + len(part)
	}
	return strings.HasSuffix(pattern, "*") || current == len(name)
}

func sortSharedWithMeFiles(files []*File, page *inventory.PaginationArgs) {
	orderBy := ""
	order := inventory.OrderDirection("")
	if page != nil {
		orderBy = page.OrderBy
		order = page.Order
	}
	desc := order == inventory.OrderDirectionDesc

	sort.SliceStable(files, func(i, j int) bool {
		left, right := files[i], files[j]
		if left.Type() != right.Type() {
			return left.Type() == types.FileTypeFolder
		}

		var cmp int
		switch orderBy {
		case "name":
			cmp = strings.Compare(strings.ToLower(left.Name()), strings.ToLower(right.Name()))
		case "size":
			cmp = compareInt64(left.Size(), right.Size())
		case "updated_at":
			cmp = compareTime(left.UpdatedAt(), right.UpdatedAt())
		case "created_at":
			cmp = compareTime(left.CreatedAt(), right.CreatedAt())
		default:
			cmp = compareInt64(int64(left.ID()), int64(right.ID()))
		}
		if cmp == 0 {
			cmp = compareInt64(int64(left.ID()), int64(right.ID()))
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

type sharedWithMeCursor struct {
	ID        int
	Name      string
	Size      int64
	CreatedAt time.Time
	UpdatedAt time.Time
	IsFolder  bool
}

type sharedWithMeCursorPayload struct {
	Name      string    `json:"n"`
	Size      int64     `json:"s"`
	CreatedAt time.Time `json:"c"`
	UpdatedAt time.Time `json:"u"`
	IsFolder  bool      `json:"f,omitempty"`
}

func (t *sharedWithMeNavigator) encodeSharedWithMeCursor(file *File, page *inventory.PaginationArgs) (string, error) {
	payload, err := json.Marshal(sharedWithMeCursorPayload{
		Name:      file.Name(),
		Size:      file.Size(),
		CreatedAt: file.CreatedAt(),
		UpdatedAt: file.UpdatedAt(),
		IsFolder:  file.Type() == types.FileTypeFolder,
	})
	if err != nil {
		return "", err
	}

	token := &inventory.PageToken{
		ID:     file.ID(),
		String: string(payload),
	}
	return token.Encode(t.hasher, hashid.EncodeFileID)
}

func (t *sharedWithMeNavigator) decodeSharedWithMeCursor(raw string) (*sharedWithMeCursor, error) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}

	token := &inventory.PageToken{}
	if err := json.Unmarshal(decoded, token); err != nil {
		return nil, err
	}
	id, err := t.hasher.Decode(token.IDHash, hashid.FileID)
	if err != nil {
		return nil, err
	}
	payload := sharedWithMeCursorPayload{}
	if err := json.Unmarshal([]byte(token.String), &payload); err != nil {
		return nil, err
	}

	return &sharedWithMeCursor{
		ID:        id,
		Name:      payload.Name,
		Size:      payload.Size,
		CreatedAt: payload.CreatedAt,
		UpdatedAt: payload.UpdatedAt,
		IsFolder:  payload.IsFolder,
	}, nil
}

func compareSharedWithMeFile(file *File, cursor *sharedWithMeCursor, orderBy string, order inventory.OrderDirection) int {
	fileFolder := file.Type() == types.FileTypeFolder
	if fileFolder != cursor.IsFolder {
		if fileFolder {
			return -1
		}
		return 1
	}

	var cmp int
	switch orderBy {
	case "name":
		cmp = strings.Compare(strings.ToLower(file.Name()), strings.ToLower(cursor.Name))
	case "size":
		cmp = compareInt64(file.Size(), cursor.Size)
	case "updated_at":
		cmp = compareTime(file.UpdatedAt(), cursor.UpdatedAt)
	case "created_at":
		cmp = compareTime(file.CreatedAt(), cursor.CreatedAt)
	default:
		cmp = compareInt64(int64(file.ID()), int64(cursor.ID))
	}
	if cmp == 0 {
		cmp = compareInt64(int64(file.ID()), int64(cursor.ID))
	}
	if order == inventory.OrderDirectionDesc {
		cmp = -cmp
	}
	return cmp
}

func compareInt64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareTime(left, right time.Time) int {
	switch {
	case left.Before(right):
		return -1
	case left.After(right):
		return 1
	default:
		return 0
	}
}

func (t *sharedWithMeNavigator) Capabilities(isSearching bool) *fs.NavigatorProps {
	res := &fs.NavigatorProps{
		Capability:            sharedWithMeNavigatorCapability,
		OrderDirectionOptions: fullOrderDirectionOption,
		OrderByOptions:        fullOrderByOption,
		MaxPageSize:           t.config.MaxPageSize,
	}

	if isSearching {
		res.OrderByOptions = searchLimitedOrderByOption
	}

	return res
}

func (t *sharedWithMeNavigator) Walk(ctx context.Context, levelFiles []*File, limit, depth int, f WalkFunc) error {
	return errors.New("not implemented")
}

func (n *sharedWithMeNavigator) FollowTx(ctx context.Context) (func(), error) {
	if _, ok := ctx.Value(inventory.TxCtx{}).(*inventory.Tx); !ok {
		return nil, fmt.Errorf("navigator: no inherited transaction found in context")
	}
	newFileClient, _, _, err := inventory.WithTx(ctx, n.fileClient)
	if err != nil {
		return nil, err
	}

	oldFileClient := n.fileClient
	revert := func() {
		n.fileClient = oldFileClient
		n.baseNavigator.fileClient = oldFileClient
	}

	n.fileClient = newFileClient
	n.baseNavigator.fileClient = newFileClient
	return revert, nil
}

func (n *sharedWithMeNavigator) ExecuteHook(ctx context.Context, hookType fs.HookType, file *File) error {
	return nil
}

func (n *sharedWithMeNavigator) GetView(ctx context.Context, file *File) *types.ExplorerView {
	if view, ok := n.user.Settings.FsViewMap[string(constants.FileSystemSharedWithMe)]; ok {
		return &view
	}
	return getDefaultView()
}
