package explorer

import (
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/gin-gonic/gin"
)

type (
	PatchMetadataService struct {
		Uris    []string           `json:"uris" binding:"required"`
		Patches []fs.MetadataPatch `json:"patches" binding:"required,dive"`
	}

	PatchMetadataParameterCtx struct{}
)

func (s *PatchMetadataService) GetUris() []string {
	return s.Uris
}

func (s *PatchMetadataService) Patch(c *gin.Context) error {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	uris, err := fs.NewUriFromStrings(s.Uris...)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "unknown uri", err)
	}

	err = m.PatchMedata(c, uris, s.Patches...)
	for _, uri := range uris {
		entry := &audit.LogEntry{From: uri.String()}
		if err != nil {
			entry.Failed = true
			entry.Error = err.Error()
		}
		audit.Record(c, audit.EventUpdateMetadata, entry)
	}
	return err
}
