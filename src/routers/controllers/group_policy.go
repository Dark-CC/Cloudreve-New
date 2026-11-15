package controllers

import (
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/admin"
	"github.com/gin-gonic/gin"
)

// AdminGetGroupPolicies returns the storage policies bound to a group.
func AdminGetGroupPolicies(c *gin.Context) {
	service := ParametersFromContext[*admin.GroupPolicyService](c, admin.GroupPolicyParamCtx{})
	res, err := service.Get(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// AdminUpdateGroupPolicies replaces the storage policies bound to a group.
func AdminUpdateGroupPolicies(c *gin.Context) {
	service := ParametersFromContext[*admin.GroupPolicyService](c, admin.GroupPolicyParamCtx{})
	res, err := service.Update(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// AdminGetLoadBalanceTargets returns the weighted targets of a load balance policy.
func AdminGetLoadBalanceTargets(c *gin.Context) {
	service := ParametersFromContext[*admin.LoadBalanceService](c, admin.LoadBalanceParamCtx{})
	res, err := service.Get(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// AdminUpdateLoadBalanceTargets replaces the weighted targets of a load balance policy.
func AdminUpdateLoadBalanceTargets(c *gin.Context) {
	service := ParametersFromContext[*admin.LoadBalanceService](c, admin.LoadBalanceParamCtx{})
	res, err := service.Update(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}
