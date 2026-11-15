package controllers

import (
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/share"
	"github.com/gin-gonic/gin"
)

// GetShareACL returns the fine grained permissions of a share.
func GetShareACL(c *gin.Context) {
	service := ParametersFromContext[*share.ShareACLService](c, share.ShareACLParamCtx{})
	res, err := service.Get(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// UpdateShareACL replaces the fine grained permissions of a share.
func UpdateShareACL(c *gin.Context) {
	service := ParametersFromContext[*share.ShareACLService](c, share.ShareACLParamCtx{})
	res, err := service.Update(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}
