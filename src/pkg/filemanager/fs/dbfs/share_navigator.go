package dbfs

import (
	"context"
	"fmt"

	"github.com/cloudreve/Cloudreve/v4/application/constants"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
)

var (
	ErrShareNotFound = serializer.NewError(serializer.CodeNotFound, "Shared file does not exist", nil)
	ErrNotPurchased  = serializer.NewError(serializer.CodePurchaseRequired, "You need to purchased this share", nil)
)

const (
	PurchaseTicketHeader = constants.CrHeaderPrefix + "Purchase-Ticket"
)

var shareNavigatorCapability = &boolset.BooleanSet{}

// NewShareNavigator creates a navigator for user's "shared" file system.
func NewShareNavigator(u *ent.User, fileClient inventory.FileClient, shareClient inventory.ShareClient,
	l logging.Logger, config *setting.DBFS, hasher hashid.Encoder, vasClient inventory.VASClient) Navigator {
	return NewShareNavigatorWithACL(u, fileClient, shareClient, l, config, hasher, vasClient, nil, nil)
}

// NewShareNavigatorWithACL creates a share navigator that enforces Pro share
// ACLs and inherits file level permissions from the shared subtree.
func NewShareNavigatorWithACL(u *ent.User, fileClient inventory.FileClient, shareClient inventory.ShareClient,
	l logging.Logger, config *setting.DBFS, hasher hashid.Encoder, vasClient inventory.VASClient,
	shareACLClient inventory.ShareACLClient, filePermissionClient inventory.FilePermissionClient) Navigator {
	n := &shareNavigator{
		user:                 u,
		l:                    l,
		fileClient:           fileClient,
		shareClient:          shareClient,
		vasClient:            vasClient,
		shareACLClient:       shareACLClient,
		filePermissionClient: filePermissionClient,
		config:               config,
	}
	n.baseNavigator = newBaseNavigator(fileClient, defaultFilter, u, hasher, config)
	return n
}

type (
	shareNavigator struct {
		l                    logging.Logger
		user                 *ent.User
		fileClient           inventory.FileClient
		shareClient          inventory.ShareClient
		vasClient            inventory.VASClient
		config               *setting.DBFS
		shareACLClient       inventory.ShareACLClient
		filePermissionClient inventory.FilePermissionClient

		*baseNavigator
		shareRoot       *File
		singleFileShare bool
		ownerRoot       *File
		share           *ent.Share
		owner           *ent.User
		disableRecycle  bool
		persist         func()
		permission      inventory.SharePermission
		aclLoaded       bool
	}

	shareNavigatorState struct {
		ShareRoot       *File
		OwnerRoot       *File
		SingleFileShare bool
		Share           *ent.Share
		Owner           *ent.User
	}
)

func (n *shareNavigator) PersistState(kv cache.Driver, key string) {
	n.disableRecycle = true
	n.persist = func() {
		kv.Set(key, shareNavigatorState{
			ShareRoot:       n.shareRoot,
			OwnerRoot:       n.ownerRoot,
			SingleFileShare: n.singleFileShare,
			Share:           n.share,
			Owner:           n.owner,
		}, ContextHintTTL)
	}
}

func (n *shareNavigator) RestoreState(s State) error {
	n.disableRecycle = true
	if state, ok := s.(shareNavigatorState); ok {
		n.shareRoot = state.ShareRoot
		n.ownerRoot = state.OwnerRoot
		n.singleFileShare = state.SingleFileShare
		n.share = state.Share
		n.owner = state.Owner
		// Share permissions must be re-evaluated on every navigator restore.
		// Caching them in the context hint would keep a revoked grant alive
		// until the hint expires.
		n.permission = inventory.SharePermission{}
		n.aclLoaded = false
		return nil
	}

	return fmt.Errorf("invalid state type: %T", s)
}

func (n *shareNavigator) Recycle() {
	if n.persist != nil {
		n.persist()
		n.persist = nil
	}

	if !n.disableRecycle {
		if n.ownerRoot != nil {
			n.ownerRoot.Recycle()
		} else if n.shareRoot != nil {
			n.shareRoot.Recycle()
		}
	}
}

func (n *shareNavigator) Root(ctx context.Context, path *fs.URI) (*File, error) {
	ctx = context.WithValue(ctx, inventory.LoadShareUser{}, true)
	ctx = context.WithValue(ctx, inventory.LoadUserGroup{}, true)
	ctx = context.WithValue(ctx, inventory.LoadShareFile{}, true)
	share, err := n.shareClient.GetByHashID(ctx, path.ID(hashid.EncodeUserID(n.hasher, n.user.ID)))
	if err != nil {
		return nil, ErrShareNotFound.WithError(err)
	}

	if err := inventory.IsValidShare(share); err != nil {
		return nil, ErrShareNotFound.WithError(err)
	}

	n.owner = share.Edges.User

	// Check password
	if share.Password != "" && share.Password != path.Password() {
		return nil, ErrShareIncorrectPassword
	}

	if err := n.checkPurchase(ctx, share); err != nil {
		return nil, err
	}

	// Share permission setting should overwrite root folder's permission
	n.shareRoot = newFile(nil, share.Edges.File)

	// Find the user side root of the file.
	ownerRoot, err := n.findRoot(ctx, n.shareRoot)
	if err != nil {
		return nil, err
	}

	if n.shareRoot.Type() == types.FileTypeFile {
		n.singleFileShare = true
		n.shareRoot = n.shareRoot.Parent
	}

	n.shareRoot.Path[pathIndexUser] = path.Root()
	n.shareRoot.OwnerModel = n.owner
	n.shareRoot.IsUserRoot = true
	n.shareRoot.disableView = (share.Props == nil || !share.Props.ShareView) && n.user.ID != n.owner.ID
	n.applyPermission(ctx, share)
	n.shareRoot.CapabilitiesBs = n.Capabilities(false).Capability

	// Check if any ancestors is deleted
	if ownerRoot.Name() != inventory.RootFolderName {
		return nil, ErrShareNotFound
	}

	if n.user.ID != n.owner.ID && !n.user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionShareDownload)) {
		if inventory.IsAnonymousUser(n.user) {
			return nil, serializer.NewError(
				serializer.CodeAnonymouseAccessDenied,
				fmt.Sprintf("You don't have permission to access share links"),
				err,
			)
		}

		return nil, serializer.NewError(
			serializer.CodeNoPermissionErr,
			fmt.Sprintf("You don't have permission to access share links"),
			err,
		)
	}

	n.ownerRoot = ownerRoot
	n.ownerRoot.Path[pathIndexRoot] = newMyIDUri(hashid.EncodeUserID(n.hasher, n.owner.ID))
	n.share = share
	return n.shareRoot, nil
}

// applyPermission resolves the effective share ACL for the current viewer. When
// no ACL is configured, CE keeps its historical read/list/enter behavior.
func (n *shareNavigator) applyPermission(ctx context.Context, share *ent.Share) {
	n.aclLoaded = false
	n.permission = inventory.SharePermission{CanList: true, CanRead: true}

	if n.shareACLClient == nil || n.owner == nil || share == nil {
		return
	}
	if n.user != nil && n.owner != nil && n.user.ID == n.owner.ID {
		n.permission = inventory.SharePermission{
			CanList: true, CanRead: true, CanUpdate: true,
			CanCreate: true, CanDelete: true, CanShare: true,
			Discoverable: true,
		}
		n.aclLoaded = true
		return
	}

	acl, err := n.shareACLClient.List(ctx, share.ID)
	if err != nil {
		n.l.Warning("Failed to load share ACL for share %d: %s", share.ID, err)
		// An existing but unreadable ACL must not fall back to the historical
		// read/list defaults. Fail closed until the ACL can be read again.
		n.permission = inventory.SharePermission{}
		n.aclLoaded = true
		return
	}
	if len(acl.Builtin)+len(acl.Group)+len(acl.User) == 0 {
		return
	}

	userGroupID := 0
	if n.user != nil && n.user.Edges.Group != nil {
		userGroupID = n.user.Edges.Group.ID
	}
	ownerGroupID := 0
	if n.owner.Edges.Group != nil {
		ownerGroupID = n.owner.Edges.Group.ID
	}

	anonymous := inventory.IsAnonymousUser(n.user)
	permission, matched := acl.EffectivePermission(n.user.ID, userGroupID, ownerGroupID, anonymous)
	if !matched {
		n.permission = inventory.SharePermission{}
	} else {
		n.permission = permission
	}
	n.aclLoaded = true
}

func (n *shareNavigator) currentPermission(ctx context.Context) inventory.SharePermission {
	if !n.aclLoaded && n.share != nil {
		n.applyPermission(ctx, n.share)
	}
	return n.permission
}

// PrepareCapabilities loads the share and its ACL before getNavigator checks
// required capabilities. Without this, every share would be treated as read-only
// because the navigator has not visited its root yet.
func (n *shareNavigator) PrepareCapabilities(ctx context.Context, path *fs.URI) error {
	if n.shareRoot == nil {
		_, err := n.Root(ctx, path)
		return err
	}
	if !n.aclLoaded && n.share != nil {
		n.applyPermission(ctx, n.share)
	}
	return nil
}

func (n *shareNavigator) capabilitySet(ctx context.Context, isSearching bool) *boolset.BooleanSet {
	permission := n.currentPermission(ctx)
	capabilities := &boolset.BooleanSet{}
	if permission.CanList {
		boolset.Set(NavigatorCapabilityListChildren, true, capabilities)
	}
	if permission.CanRead {
		boolset.Set(NavigatorCapabilityDownloadFile, true, capabilities)
		boolset.Set(NavigatorCapabilityEnterFolder, true, capabilities)
		boolset.Set(NavigatorCapabilityInfo, true, capabilities)
		boolset.Set(NavigatorCapabilityGenerateThumb, true, capabilities)
	}
	if permission.CanCreate {
		boolset.Set(NavigatorCapabilityCreateFile, true, capabilities)
		boolset.Set(NavigatorCapabilityUploadFile, true, capabilities)
	}
	if permission.CanUpdate {
		boolset.Set(NavigatorCapabilityRenameFile, true, capabilities)
		boolset.Set(NavigatorCapabilityUpdateMetadata, true, capabilities)
		boolset.Set(NavigatorCapabilityModifyProps, true, capabilities)
		boolset.Set(NavigatorCapabilityVersionControl, true, capabilities)
	}
	if permission.CanDelete {
		boolset.Set(NavigatorCapabilityDeleteFile, true, capabilities)
		boolset.Set(NavigatorCapabilityRestore, true, capabilities)
	}
	if permission.CanShare {
		boolset.Set(NavigatorCapabilityShare, true, capabilities)
	}
	boolset.Set(NavigatorCapabilityLockFile, true, capabilities)
	return capabilities
}

// AllowNonOwnerOperation lets ACL-authorized viewers perform operations that
// are otherwise restricted to the file owner.
func (n *shareNavigator) AllowNonOwnerOperation(capability NavigatorCapability) bool {
	return n.capabilitySet(context.Background(), false).Enabled(int(capability))
}

// checkPurchase enforces the Pro pay-per-access share price.
func (n *shareNavigator) checkPurchase(ctx context.Context, share *ent.Share) error {
	if n.vasClient == nil || n.user == nil {
		return nil
	}

	if share.Edges.User != nil && n.user.ID == share.Edges.User.ID {
		return nil
	}
	if n.user.Edges.Group != nil &&
		n.user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionShareFree)) {
		return nil
	}

	price, err := n.shareClient.GetPrice(ctx, share.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to get share price", err)
	}
	if price <= 0 {
		return nil
	}

	if ticket, ok := ctx.Value(PurchaseTicketCtxKey{}).(string); ok && ticket != "" {
		purchase, err := n.vasClient.GetSharePurchaseByTicket(ctx, ticket)
		if err == nil && purchase.ShareID != nil && *purchase.ShareID == share.ID {
			if purchase.UserID == nil || n.user.ID == 0 || *purchase.UserID == n.user.ID {
				return nil
			}
		}
	}

	if n.user.ID > 0 {
		owned, err := n.vasClient.HasSharePurchase(ctx, share.ID, n.user.ID)
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to check share purchase", err)
		}
		if owned {
			return nil
		}
	}

	return ErrNotPurchased
}

func (n *shareNavigator) To(ctx context.Context, path *fs.URI) (*File, error) {
	if n.shareRoot == nil {
		root, err := n.Root(ctx, path)
		if err != nil {
			return nil, err
		}

		n.shareRoot = root
	}

	current, lastAncestor := n.shareRoot, n.shareRoot
	elements := path.Elements()

	// If target is root of single file share, the root itself is the target.
	if len(elements) == 1 && n.singleFileShare {
		file, err := n.latestSharedSingleFile(ctx)
		if err != nil {
			return nil, err
		}

		if len(elements) == 1 && file.Name() != elements[0] {
			return nil, fs.ErrPathNotExist
		}

		return file, nil
	}

	var err error
	for index, element := range elements {
		lastAncestor = current
		current, err = n.walkNext(ctx, current, element, index == len(elements)-1)
		if err != nil {
			return lastAncestor, fmt.Errorf("failed to walk into %q: %w", element, err)
		}
	}

	return current, nil
}

func (n *shareNavigator) walkNext(ctx context.Context, root *File, next string, isLeaf bool) (*File, error) {
	nextFile, err := n.baseNavigator.walkNext(ctx, root, next, isLeaf)
	if err != nil {
		return nil, err
	}

	return nextFile, nil
}

func (n *shareNavigator) Children(ctx context.Context, parent *File, args *ListArgs) (*ListResult, error) {
	if n.singleFileShare {
		file, err := n.latestSharedSingleFile(ctx)
		if err != nil {
			return nil, err
		}

		return &ListResult{
			Files:          []*File{file},
			Pagination:     &inventory.PaginationResults{},
			SingleFileView: true,
		}, nil
	}

	return n.baseNavigator.children(ctx, parent, args)
}

func (n *shareNavigator) latestSharedSingleFile(ctx context.Context) (*File, error) {
	if n.singleFileShare {
		file, err := n.fileClient.GetByID(ctx, n.share.Edges.File.ID)
		if err != nil {
			return nil, err
		}

		f := newFile(n.shareRoot, file)
		f.OwnerModel = n.shareRoot.OwnerModel

		return f, nil
	}

	return nil, fs.ErrPathNotExist
}

func (n *shareNavigator) Capabilities(isSearching bool) *fs.NavigatorProps {
	res := &fs.NavigatorProps{
		Capability:            n.capabilitySet(context.Background(), isSearching),
		OrderDirectionOptions: fullOrderDirectionOption,
		OrderByOptions:        fullOrderByOption,
		MaxPageSize:           n.config.MaxPageSize,
	}

	if isSearching {
		res.OrderByOptions = nil
		res.OrderDirectionOptions = nil
	}

	return res
}

func (n *shareNavigator) FollowTx(ctx context.Context) (func(), error) {
	if _, ok := ctx.Value(inventory.TxCtx{}).(*inventory.Tx); !ok {
		return nil, fmt.Errorf("navigator: no inherited transaction found in context")
	}
	newFileClient, _, _, err := inventory.WithTx(ctx, n.fileClient)
	if err != nil {
		return nil, err
	}

	newSharClient, _, _, err := inventory.WithTx(ctx, n.shareClient)

	oldFileClient, oldShareClient := n.fileClient, n.shareClient
	revert := func() {
		n.fileClient = oldFileClient
		n.shareClient = oldShareClient
		n.baseNavigator.fileClient = oldFileClient
	}

	n.fileClient = newFileClient
	n.shareClient = newSharClient
	n.baseNavigator.fileClient = newFileClient
	return revert, nil
}

func (n *shareNavigator) ExecuteHook(ctx context.Context, hookType fs.HookType, file *File) error {
	switch hookType {
	case fs.HookTypeBeforeDownload:
		return n.shareClient.Downloaded(ctx, n.share)
	}
	return nil
}

func (n *shareNavigator) Walk(ctx context.Context, levelFiles []*File, limit, depth int, f WalkFunc) error {
	return n.baseNavigator.walk(ctx, levelFiles, limit, depth, f)
}

func (n *shareNavigator) GetView(ctx context.Context, file *File) *types.ExplorerView {
	return file.View()
}
