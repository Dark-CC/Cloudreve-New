package explorer

import (
	"context"
	"fmt"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type (
	SetFilePermissionParameterCtx struct{}
	SetFilePermissionService      struct {
		URIs    []string                         `json:"uris" binding:"required,min=1,max=100"`
		Setting *inventory.FilePermissionSetting `json:"setting" binding:"required"`
	}

	DeleteFilePermissionParameterCtx struct{}
	DeleteFilePermissionService      struct {
		URIs []string `json:"uris" binding:"required,min=1,max=100"`
	}
)

// Set validates and applies fine grained permissions to every requested file.
func (s *SetFilePermissionService) Set(c *gin.Context) error {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	if user.Edges.Group == nil {
		return serializer.NewError(serializer.CodeGroupNotAllowed, "User group is not loaded", nil)
	}

	if err := inventory.ValidatePermissionSetting(s.Setting,
		user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionSetAnonymousPermission)),
		user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionSetExplicitUser))); err != nil {
		return serializer.NewError(serializer.CodeParamErr, err.Error(), err)
	}

	fileIDs, err := s.resolveFileIDs(c, dep, user.ID)
	if err != nil {
		return err
	}

	if err := dep.FilePermissionClient().Set(c, fileIDs, s.Setting); err != nil {
		for _, fileID := range fileIDs {
			audit.Record(c, audit.EventSetFilePermission, &audit.LogEntry{
				Failed: true,
				Error:  err.Error(),
			}, audit.WithFile(fileID))
		}
		return serializer.NewError(serializer.CodeDBError, "Failed to update file permissions", err)
	}
	for _, fileID := range fileIDs {
		audit.Record(c, audit.EventSetFilePermission, &audit.LogEntry{}, audit.WithFile(fileID))
	}
	return nil
}

// Delete clears fine grained permissions so files inherit from ancestors.
func (s *DeleteFilePermissionService) Delete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	fileIDs, err := s.resolveFileIDs(c, dep, user.ID)
	if err != nil {
		return err
	}

	if err := dep.FilePermissionClient().Delete(c, fileIDs); err != nil {
		for _, fileID := range fileIDs {
			audit.Record(c, audit.EventSetFilePermission, &audit.LogEntry{
				Failed: true,
				Error:  err.Error(),
				Reason: "delete",
			}, audit.WithFile(fileID))
		}
		return serializer.NewError(serializer.CodeDBError, "Failed to clear file permissions", err)
	}
	for _, fileID := range fileIDs {
		audit.Record(c, audit.EventSetFilePermission, &audit.LogEntry{Reason: "delete"}, audit.WithFile(fileID))
	}
	return nil
}

func (s *SetFilePermissionService) resolveFileIDs(c *gin.Context, dep dependency.Dep, userID int) ([]int, error) {
	return resolvePermissionFileIDs(c, dep, userID, s.URIs)
}

func (s *DeleteFilePermissionService) resolveFileIDs(c *gin.Context, dep dependency.Dep, userID int) ([]int, error) {
	return resolvePermissionFileIDs(c, dep, userID, s.URIs)
}

func resolvePermissionFileIDs(c context.Context, dep dependency.Dep, userID int, rawURIs []string) ([]int, error) {
	uris, err := fs.NewUriFromStrings(rawURIs...)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "unknown uri", err)
	}

	m := manager.NewFileManager(dep, inventory.UserFromContext(c))
	defer m.Recycle()

	ids := make([]int, 0, len(uris))
	for _, uri := range uris {
		file, err := m.Get(c, uri, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get file %s: %w", uri.String(), err)
		}
		if file.OwnerID() != userID {
			return nil, serializer.NewError(serializer.CodeOwnerOnly, "Only owner can change file permissions", nil)
		}
		ids = append(ids, file.ID())
	}
	return lo.Uniq(ids), nil
}
