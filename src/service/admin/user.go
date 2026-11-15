package admin

import (
	"context"
	"strconv"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/samber/lo"
)

// AddUserService 用户添加服务
type AddUserService struct {
	//User     model.User `json:"User" binding:"required"`
	Password string `json:"password"`
}

// UserService 用户ID服务
type UserService struct {
	ID uint `uri:"id" json:"id" binding:"required"`
}

// UserBatchService 用户批量操作服务
type UserBatchService struct {
	ID []uint `json:"id" binding:"min=1"`
}

const (
	userStatusCondition = "user_status"
	userGroupCondition  = "user_group"
	userNickCondition   = "user_nick"
	userEmailCondition  = "user_email"
)

func (service *AdminListService) Users(c *gin.Context) (*ListUserResponse, error) {
	dep := dependency.FromContext(c)
	hasher := dep.HashIDEncoder()
	userClient := dep.UserClient()

	ctx := context.WithValue(c, inventory.LoadUserGroup{}, true)
	ctx = context.WithValue(ctx, inventory.LoadUserPasskey{}, true)

	var (
		err     error
		groupID int
	)
	if service.Conditions[userGroupCondition] != "" {
		groupID, err = strconv.Atoi(service.Conditions[userGroupCondition])
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid group ID", err)
		}
	}

	res, err := userClient.ListUsers(ctx, &inventory.ListUserParameters{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     service.Page - 1,
			PageSize: service.PageSize,
			OrderBy:  service.OrderBy,
			Order:    inventory.OrderDirection(service.OrderDirection),
		},
		Status:  user.Status(service.Conditions[userStatusCondition]),
		GroupID: groupID,
		Nick:    service.Conditions[userNickCondition],
		Email:   service.Conditions[userEmailCondition],
	})

	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list users", err)
	}

	ids := lo.Map(res.Users, func(user *ent.User, _ int) int { return user.ID })
	proFields, err := loadProUserFields(c, dep.DBClient(), ids)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to load user extras", err)
	}
	openIDs, err := loadAdminOpenIDs(c, dep, ids)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to load linked identities", err)
	}

	return &ListUserResponse{
		Pagination: res.PaginationResults,
		Users: lo.Map(res.Users, func(user *ent.User, _ int) GetUserResponse {
			resp := GetUserResponse{
				User:         user,
				HashID:       hashid.EncodeUserID(hasher, user.ID),
				TwoFAEnabled: user.TwoFactorSecret != "",
			}
			if extra := proFields[user.ID]; extra != nil {
				resp.Credit = extra.Credit
				resp.GroupExpires = extra.GroupExpires
				resp.PreviousGroup = extra.PreviousGroup
			}
			if links := openIDs[user.ID]; len(links) > 0 {
				resp.Edges = &AdminUserEdges{OpenID: links}
			}
			return resp
		}),
	}, nil
}

type (
	SingleUserService struct {
		ID int `uri:"id" json:"id" binding:"required"`
	}
	SingleUserParamCtx struct{}
)

func (service *SingleUserService) Get(c *gin.Context) (*GetUserResponse, error) {
	dep := dependency.FromContext(c)
	hasher := dep.HashIDEncoder()
	userClient := dep.UserClient()

	ctx := context.WithValue(c, inventory.LoadUserGroup{}, true)
	ctx = context.WithValue(ctx, inventory.LoadUserPasskey{}, true)

	user, err := userClient.GetByID(ctx, service.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get user", err)
	}

	m := manager.NewFileManager(dep, user)
	capacity, err := m.Capacity(ctx)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeInternalSetting, "Failed to get user capacity", err)
	}

	resp := &GetUserResponse{
		User:         user,
		HashID:       hashid.EncodeUserID(hasher, user.ID),
		TwoFAEnabled: user.TwoFactorSecret != "",
		Capacity:     capacity,
	}
	extra, err := loadProUserFields(c, dep.DBClient(), []int{user.ID})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to load user extras", err)
	}
	if fields := extra[user.ID]; fields != nil {
		resp.Credit = fields.Credit
		resp.GroupExpires = fields.GroupExpires
		resp.PreviousGroup = fields.PreviousGroup
	}
	openIDs, err := loadAdminOpenIDs(c, dep, []int{user.ID})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to load linked identities", err)
	}
	if links := openIDs[user.ID]; len(links) > 0 {
		resp.Edges = &AdminUserEdges{OpenID: links}
	}
	return resp, nil
}

func loadAdminOpenIDs(c *gin.Context, dep dependency.Dep, userIDs []int) (map[int][]OpenIDLink, error) {
	res := make(map[int][]OpenIDLink, len(userIDs))
	providers := dep.SettingProvider().ExternalLoginProviders(c)
	configured := make(map[string]map[int]bool, len(providers))
	firstIDs := make(map[string]int, len(providers))
	for _, provider := range providers {
		if configured[provider.Type] == nil {
			configured[provider.Type] = make(map[int]bool)
			firstIDs[provider.Type] = provider.ID
		}
		configured[provider.Type][provider.ID] = true
	}
	for _, userID := range userIDs {
		identities, err := dep.ExternalIdentityClient().ListByUser(c, userID)
		if err != nil {
			return nil, err
		}
		for _, identity := range identities {
			providerType, storedID := setting.ParseExternalIdentityProviderKey(identity.Provider)
			providerID := 0
			if storedID >= 0 {
				providerID = storedID
			} else if id, ok := firstIDs[providerType]; ok {
				providerID = id
			} else {
				switch providerType {
				case "logto":
					providerID = 0
				case "qq":
					providerID = 1
				case "oidc":
					providerID = 2
				}
			}
			if ids := configured[providerType]; storedID >= 0 && len(ids) > 0 && !ids[storedID] {
				providerID = firstIDs[providerType]
			}
			res[userID] = append(res[userID], OpenIDLink{
				ID:       identity.ID,
				Provider: providerID,
				Sub:      identity.Subject,
				Email:    identity.Email,
				Name:     identity.DisplayName,
				Avatar:   identity.Avatar,
				LinkedAt: identity.CreatedAt,
			})
		}
	}
	return res, nil
}

func (service *SingleUserService) CalibrateStorage(c *gin.Context) (*GetUserResponse, error) {
	dep := dependency.FromContext(c)
	userClient := dep.UserClient()

	ctx := context.WithValue(c, inventory.LoadUserGroup{}, true)
	_, err := userClient.CalculateStorage(ctx, service.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to calculate storage", err)
	}

	subService := &SingleUserService{ID: service.ID}
	return subService.Get(c)
}

type (
	UpsertUserService struct {
		User          *ent.User `json:"user" binding:"required"`
		Password      string    `json:"password" binding:"omitempty,min=6,max=128"`
		TwoFA         string    `json:"two_fa"`
		Credit        *int64    `json:"credit"`
		GroupExpires  *string   `json:"group_expires"`
		PreviousGroup *int      `json:"previous_group"`
	}
	UpsertUserParamCtx struct{}
)

type adminUserEmailValidation struct {
	Email string `binding:"required,email"`
}

func (s *UpsertUserService) validateEmail() error {
	if s.User == nil {
		return serializer.NewError(serializer.CodeParamErr, "Email format error", nil)
	}
	if err := binding.Validator.ValidateStruct(adminUserEmailValidation{Email: s.User.Email}); err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Email format error", err)
	}
	return nil
}

func (s *UpsertUserService) Update(c *gin.Context) (*GetUserResponse, error) {
	if err := s.validateEmail(); err != nil {
		return nil, err
	}
	if s.Credit != nil && *s.Credit < 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Credit cannot be negative", nil)
	}

	dep := dependency.FromContext(c)
	userClient := dep.UserClient()

	ctx := context.WithValue(c, inventory.LoadUserGroup{}, true)
	existing, err := userClient.GetByID(ctx, s.User.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get user", err)
	}

	if s.User.ID == 1 && existing.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		if s.User.GroupUsers != existing.GroupUsers {
			return nil, serializer.NewError(serializer.CodeInvalidActionOnDefaultUser, "Cannot change default user's group", nil)
		}

		if s.User.Status != user.StatusActive {
			return nil, serializer.NewError(serializer.CodeInvalidActionOnDefaultUser, "Cannot change default user's status", nil)
		}

	}

	newUser, err := userClient.Upsert(ctx, s.User, s.Password, s.TwoFA)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update user", err)
	}
	if err := updateProUserFields(c, dep.DBClient(), newUser.ID, s.Credit,
		s.GroupExpires, s.GroupExpires != nil, s.PreviousGroup, s.PreviousGroup != nil); err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Failed to update user extras", err)
	}

	audit.Record(c, audit.EventUserChanged, &audit.LogEntry{
		Name:        newUser.Email,
		GroupID:     newUser.GroupUsers,
		GroupIDFrom: existing.GroupUsers,
		Reason:      "update",
		Exts:        map[string]string{"target_user_id": strconv.Itoa(newUser.ID)},
	}, audit.WithUser(inventory.UserIDFromContext(c)))
	service := &SingleUserService{ID: newUser.ID}
	return service.Get(c)
}

func (s *UpsertUserService) Create(c *gin.Context) (*GetUserResponse, error) {
	if err := s.validateEmail(); err != nil {
		return nil, err
	}
	if s.Credit != nil && *s.Credit < 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Credit cannot be negative", nil)
	}

	if s.Password == "" {
		return nil, serializer.NewError(serializer.CodeParamErr, "Password is required", nil)
	}

	dep := dependency.FromContext(c)
	userClient := dep.UserClient()

	if s.User.ID != 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "ID must be 0", nil)
	}

	user, err := userClient.Upsert(c, s.User, s.Password, s.TwoFA)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create user", err)
	}
	if err := updateProUserFields(c, dep.DBClient(), user.ID, s.Credit,
		s.GroupExpires, s.GroupExpires != nil, s.PreviousGroup, s.PreviousGroup != nil); err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Failed to update user extras", err)
	}

	audit.Record(c, audit.EventUserChanged, &audit.LogEntry{
		Name:    user.Email,
		GroupID: user.GroupUsers,
		Reason:  "create",
		Exts:    map[string]string{"target_user_id": strconv.Itoa(user.ID)},
	}, audit.WithUser(inventory.UserIDFromContext(c)))
	service := &SingleUserService{ID: user.ID}
	return service.Get(c)

}

type (
	BatchUserService struct {
		IDs []int `json:"ids" binding:"min=1"`
	}
	BatchUserParamCtx struct{}
)

func (s *BatchUserService) Delete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	userClient := dep.UserClient()
	fileClient := dep.FileClient()

	current := inventory.UserFromContext(c)
	ae := serializer.NewAggregateError()
	for _, id := range s.IDs {
		if current.ID == id || id == 1 {
			ae.Add(strconv.Itoa(id), serializer.NewError(serializer.CodeInvalidActionOnDefaultUser, "Cannot delete current user", nil))
			continue
		}

		fc, tx, ctx, err := inventory.WithTx(c, fileClient)
		if err != nil {
			ae.Add(strconv.Itoa(id), serializer.NewError(serializer.CodeDBError, "Failed to start transaction", err))
			continue
		}

		uc, _, ctx, err := inventory.WithTx(ctx, userClient)
		if err != nil {
			ae.Add(strconv.Itoa(id), serializer.NewError(serializer.CodeDBError, "Failed to start transaction", err))
			continue
		}

		if err := fc.DeleteByUser(ctx, id); err != nil {
			_ = inventory.Rollback(tx)
			ae.Add(strconv.Itoa(id), serializer.NewError(serializer.CodeDBError, "Failed to delete user files", err))
			continue
		}

		if err := uc.Delete(ctx, id); err != nil {
			_ = inventory.Rollback(tx)
			ae.Add(strconv.Itoa(id), serializer.NewError(serializer.CodeDBError, "Failed to delete user", err))
			continue
		}

		if err := inventory.Commit(tx); err != nil {
			ae.Add(strconv.Itoa(id), serializer.NewError(serializer.CodeDBError, "Failed to commit transaction", err))
			continue
		}
		audit.Record(c, audit.EventUserChanged, &audit.LogEntry{
			Reason: "delete",
			Exts:   map[string]string{"target_user_id": strconv.Itoa(id)},
		}, audit.WithUser(current.ID))
	}

	return ae.Aggregate()
}
