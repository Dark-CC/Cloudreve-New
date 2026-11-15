package controllers

import (
	"net/http"
	"strconv"

	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/sso"
	"github.com/gin-gonic/gin"
)

// SSOOIDCStart begins the OIDC authorization flow.
func SSOOIDCStart(c *gin.Context) {
	service := ParametersFromContext[*sso.OIDCStartService](c, sso.OIDCStartParamCtx{})
	target, err := service.Start(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}

	c.JSON(http.StatusOK, serializer.Response{Data: map[string]string{"url": target}})
}

// SSOOIDCCallback completes the OIDC flow and redirects to the frontend.
func SSOOIDCCallback(c *gin.Context) {
	service := ParametersFromContext[*sso.OIDCCallbackService](c, sso.OIDCCallbackParamCtx{})
	target, err := service.Callback(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}

	c.Redirect(http.StatusFound, target)
}

// SSOOIDCProviderCallback accepts Pro's `/callback/openid/:provider` route.
// The one-time state selects the actual provider; the path id is only a hint
// and is checked against the stored session when present.
func SSOOIDCProviderCallback(c *gin.Context) {
	provider := c.Param("provider")
	if provider == "oidc" {
		SSOOIDCCallback(c)
		return
	}

	providerID, err := strconv.Atoi(provider)
	if err != nil || providerID < 0 {
		c.JSON(http.StatusOK, serializer.Err(c, serializer.NewError(
			serializer.CodeParamErr,
			"Invalid sign-in provider id",
			nil,
		)))
		return
	}

	SSOOIDCCallback(c)
}

// SSOOIDCQQCallback serves the legacy /login/qq redirect used by Pro's first
// QQ provider. Provider resolution still comes from the stored state.
func SSOOIDCQQCallback(c *gin.Context) {
	SSOOIDCCallback(c)
}

// SSOOIDCExchange exchanges a one-time ticket for a token pair.
func SSOOIDCExchange(c *gin.Context) {
	service := ParametersFromContext[*sso.OIDCExchangeService](c, sso.OIDCExchangeParamCtx{})
	token, err := service.Exchange(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}

	c.JSON(http.StatusOK, serializer.Response{Data: token})
}

// SSOOIDCStatus reports OIDC availability for the current user.
func SSOOIDCStatus(c *gin.Context) {
	service := &sso.OIDCLinkService{}
	res, err := service.Status(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}

	c.JSON(http.StatusOK, serializer.Response{Data: res})
}

// SSOOIDCLink starts linking the current user's OIDC identity.
func SSOOIDCLink(c *gin.Context) {
	service := ParametersFromContext[*sso.OIDCLinkService](c, sso.OIDCLinkParamCtx{})
	target, err := service.Link(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}

	c.JSON(http.StatusOK, serializer.Response{Data: map[string]string{"url": target}})
}

// SSOOIDCUnlink removes the current user's OIDC identity.
func SSOOIDCUnlink(c *gin.Context) {
	service := &sso.OIDCLinkService{}
	if err := service.Unlink(c); err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}

	c.JSON(http.StatusOK, serializer.Response{})
}

// SSOOIDCUnlinkByID removes the linked OIDC identity named in the request path.
func SSOOIDCUnlinkByID(c *gin.Context) {
	service := ParametersFromContext[*sso.OIDCUnlinkService](c, sso.OIDCUnlinkParamCtx{})
	if err := sso.UnlinkIdentityByID(c, service.ID); err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}
	c.JSON(http.StatusOK, serializer.Response{})
}

// ProOpenIDStart starts a Pro-style OpenID flow. The Pro frontend expects the
// authorization URL itself in the response body.
func ProOpenIDStart(c *gin.Context) {
	service := ParametersFromContext[*sso.ProOpenIDStartService](c, sso.ProOpenIDStartParamCtx{})
	target, err := service.Start(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}

	c.JSON(http.StatusOK, serializer.Response{Data: target})
}

// ProOpenIDExchange completes a Pro-style OpenID sign-in.
func ProOpenIDExchange(c *gin.Context) {
	service := ParametersFromContext[*sso.ProOpenIDExchangeService](c, sso.ProOpenIDExchangeParamCtx{})
	res, err := service.Exchange(c)
	if err != nil {
		c.JSON(http.StatusOK, serializer.Err(c, err))
		return
	}

	c.JSON(http.StatusOK, serializer.Response{Data: res})
}
