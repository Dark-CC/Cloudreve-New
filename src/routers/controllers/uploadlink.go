package controllers

import (
	"net/http"

	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/uploadlink"
	"github.com/gin-gonic/gin"
)

// CreateUploadLink issues a shareable upload link for a folder.
func CreateUploadLink(c *gin.Context) {
	service := ParametersFromContext[*uploadlink.CreateService](c, uploadlink.CreateParamCtx{})
	resp, err := service.Create(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}
	c.JSON(http.StatusOK, serializer.Response{Data: resp})
}

// UploadLinkInfo returns the public metadata of an upload link.
func UploadLinkInfo(c *gin.Context) {
	service := ParametersFromContext[*uploadlink.InfoService](c, uploadlink.InfoParamCtx{})
	resp, err := service.Info(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}
	c.JSON(http.StatusOK, serializer.Response{Data: resp})
}

// CreateUploadLinkSession starts an anonymous upload through a link.
func CreateUploadLinkSession(c *gin.Context) {
	// The link token travels in the path while the file metadata is JSON. URI
	// binding validates the whole struct, so binding both in one pass would
	// reject the body fields (and vice versa). Bind them separately instead.
	// Seed the token from the path before binding: the struct's required tag
	// would otherwise reject the JSON body for a field that only lives in the
	// URL. Re-apply it after binding so a body value can never override it.
	service := &uploadlink.CreateSessionService{Token: c.Param("token")}
	if err := c.ShouldBindJSON(service); err != nil {
		c.JSON(http.StatusOK, ErrorResponse(err))
		return
	}
	service.Token = c.Param("token")
	resp, err := service.Create(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}
	c.JSON(http.StatusOK, serializer.Response{Data: resp})
}

// CompleteUploadLinkUpload finalizes an anonymous upload.
func CompleteUploadLinkUpload(c *gin.Context) {
	service := ParametersFromContext[*uploadlink.CompleteService](c, uploadlink.CompleteParamCtx{})
	if err := service.Complete(c); err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}
	c.JSON(http.StatusOK, serializer.Response{})
}

// RefreshUploadLinkTicket re-issues the edge ticket for a visitor upload.
func RefreshUploadLinkTicket(c *gin.Context) {
	service := ParametersFromContext[*uploadlink.RefreshService](c, uploadlink.RefreshParamCtx{})
	resp, err := service.Refresh(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}
	c.JSON(http.StatusOK, serializer.Response{Data: resp})
}
