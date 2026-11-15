package sso

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/auth"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	usersvc "github.com/cloudreve/Cloudreve/v4/service/user"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"
)

const (
	ProviderOIDC = "oidc"

	oidcJWKSPrefix  = "sso_oidc_jwks_"
	oidcStateTTL    = 600
	oidcTicketTTL   = 120
	defaultOIDCName = "SSO"
)

type (
	OIDCStartParamCtx struct{}
	// OIDCStartService starts the OIDC authorization code flow.
	OIDCStartService struct {
		Redirect string `form:"redirect" json:"redirect"`
	}

	OIDCCallbackParamCtx struct{}
	// OIDCCallbackService handles the provider redirect.
	OIDCCallbackService struct {
		Code             string `form:"code" json:"code"`
		State            string `form:"state" json:"state"`
		Error            string `form:"error" json:"error"`
		ErrorDescription string `form:"error_description" json:"error_description"`
	}

	OIDCExchangeParamCtx struct{}
	// OIDCExchangeService exchanges a one-time ticket for a token pair.
	OIDCExchangeService struct {
		Ticket string `json:"ticket" binding:"required"`
	}

	OIDCLinkParamCtx struct{}
	// OIDCLinkService links the current user's OIDC account.
	OIDCLinkService struct {
		Redirect string `form:"redirect" json:"redirect"`
	}

	OIDCUnlinkParamCtx struct{}
	// OIDCUnlinkService removes a linked OIDC identity by provider. The Pro
	// frontend passes the identity ID in the path.
	OIDCUnlinkService struct {
		ID int `uri:"id" binding:"required,min=1"`
	}

	ProOpenIDStartParamCtx struct{}
	// ProOpenIDStartService accepts the Pro frontend's OpenID entry payload.
	ProOpenIDStartService struct {
		Provider *int   `json:"provider"`
		Hint     string `json:"hint"`
		Linking  bool   `json:"linking"`
	}

	ProOpenIDExchangeParamCtx struct{}
	// ProOpenIDExchangeService handles the Pro callback exchange payload.
	ProOpenIDExchangeService struct {
		Provider  *int   `json:"provider_id"`
		Code      string `json:"code"`
		SessionID string `json:"session_id"`
	}

	OIDCStatusResponse struct {
		Enabled    bool   `json:"enabled"`
		Name       string `json:"name"`
		ProviderID int    `json:"provider_id"`
		LoginURL   string `json:"login_url,omitempty"`
		Linked     bool   `json:"linked,omitempty"`
		Provider   string `json:"provider,omitempty"`
		Email      string `json:"email,omitempty"`
		Avatar     string `json:"avatar,omitempty"`
		DirectSSO  string `json:"direct_sso,omitempty"`
	}

	OIDCExchangeResponse struct {
		User  usersvc.User `json:"user"`
		Token auth.Token   `json:"token"`
	}

	OIDCDiscovery struct {
		Issuer                string   `json:"issuer"`
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		UserinfoEndpoint      string   `json:"userinfo_endpoint"`
		JWKSURI               string   `json:"jwks_uri"`
		ScopesSupported       []string `json:"scopes_supported"`
		IDTokenSigningAlgs    []string `json:"id_token_signing_alg_values_supported"`
	}

	oidcState struct {
		Verifier    string `json:"verifier"`
		Redirect    string `json:"redirect"`
		CallbackURL string `json:"callback_url"`
		Link        bool   `json:"link"`
		UserID      int    `json:"user_id"`
		Nonce       string `json:"nonce"`
		DirectLogin bool   `json:"direct_login"`
		ProviderID  int    `json:"provider_id"`
	}

	oidcTokenResponse struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}

	oidcUserInfo struct {
		Sub               string         `json:"sub"`
		Email             string         `json:"email"`
		EmailVerified     bool           `json:"email_verified"`
		Name              string         `json:"name"`
		PreferredUsername string         `json:"preferred_username"`
		Nickname          string         `json:"nickname"`
		Picture           string         `json:"picture"`
		Raw               map[string]any `json:"-"`
	}

	oidcJWKS struct {
		Keys []oidcJWK `json:"keys"`
	}

	oidcJWK struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	}

	oidcIDTokenClaims struct {
		Nonce             string `json:"nonce"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Nickname          string `json:"nickname"`
		Picture           string `json:"picture"`
		jwt.RegisteredClaims
	}

	oidcTicket struct {
		UserID   int    `json:"user_id"`
		Redirect string `json:"redirect"`
	}

	oidcFlowError struct {
		Code        string
		Description string
		Err         error
	}
)

func (e *oidcFlowError) Error() string {
	if e.Description != "" {
		return e.Description
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func newOIDCFlowError(code, description string, err error) error {
	return &oidcFlowError{Code: code, Description: description, Err: err}
}

func oidcFlowErrorParts(err error) (string, string) {
	var flowErr *oidcFlowError
	if errors.As(err, &flowErr) {
		return flowErr.Code, flowErr.Error()
	}
	return "sign_in_failed", err.Error()
}

// Start begins the authorization code + PKCE flow.
func (s *ProOpenIDStartService) Start(c *gin.Context) (string, error) {
	var provider *setting.ExternalLoginProvider
	if s.Provider == nil {
		provider = firstConfiguredProvider(c)
		if provider == nil {
			return "", serializer.NewError(serializer.CodeFeatureNotEnabled,
				"No sign-in provider is configured", nil)
		}
		if !provider.Enabled || provider.ClientID == "" || provider.ClientSecret == "" {
			return "", serializer.NewError(serializer.CodeFeatureNotEnabled,
				"This sign-in provider is not enabled", nil)
		}
	} else {
		var err error
		provider, err = resolveExternalProvider(c, *s.Provider)
		if err != nil {
			return "", err
		}
	}
	return startExternalFlow(c, provider, "", s.Linking)
}

// Exchange completes a Pro-style OpenID callback.
func (s *ProOpenIDExchangeService) Exchange(c *gin.Context) (*OIDCExchangeResponse, error) {
	dep := dependency.FromContext(c)

	// Pro's callback page posts the authorization code and sends the state as
	// session_id. The provider is resolved from the one-time server session, so
	// clients cannot switch providers between authorization and callback.
	if s.SessionID != "" {
		if raw, ok := dep.KV().Get(externalStatePrefix + s.SessionID); ok {
			_ = dep.KV().Delete(externalStatePrefix, s.SessionID)
			session := &oidcState{}
			if err := json.Unmarshal([]byte(raw.(string)), session); err != nil {
				return nil, serializer.NewError(serializer.CodeCredentialInvalid, "Malformed sign-in session", err)
			}
			if s.Code == "" {
				return nil, serializer.NewError(serializer.CodeParamErr, "Missing authorization code", nil)
			}

			if s.Provider != nil {
				if session.ProviderID != *s.Provider {
					return nil, serializer.NewError(serializer.CodeCredentialInvalid,
						"Sign-in session provider does not match", nil)
				}
			}
			provider, err := resolveSessionProvider(c, session)
			if err != nil {
				return nil, err
			}
			expectedUser, err := completeExternalFlow(c, provider, dep, s.Code, session)
			if err != nil {
				code, description := oidcFlowErrorParts(err)
				return nil, serializer.NewError(serializer.CodeCredentialInvalid,
					firstNonEmpty(description, code), err)
			}
			if session.Link {
				return &OIDCExchangeResponse{}, nil
			}
			return exchangeTicket(c, expectedUser.ID)
		}
	}

	// Also accept a one-time community ticket in session_id.
	raw, ok := dep.KV().Get(externalTicketPrefix + s.SessionID)
	if ok {
		_ = dep.KV().Delete(externalTicketPrefix, s.SessionID)
		ticket := &oidcTicket{}
		if err := json.Unmarshal([]byte(raw.(string)), ticket); err != nil {
			return nil, serializer.NewError(serializer.CodeCredentialInvalid, "Malformed sign-in ticket", err)
		}
		return exchangeTicket(c, ticket.UserID)
	}

	return nil, serializer.NewError(serializer.CodeFeatureNotEnabled,
		"This sign-in callback session is invalid or expired", nil)
}

// Start begins the authorization code + PKCE flow.
func (s *OIDCStartService) Start(c *gin.Context) (string, error) {
	provider := firstConfiguredProviderOfType(c, ProviderOIDC)
	if provider == nil {
		provider = firstConfiguredProviderOfType(c, ProviderLogto)
	}
	if provider == nil {
		return "", serializer.NewError(serializer.CodeFeatureNotEnabled,
			"OIDC sign-in is not enabled", nil)
	}
	if !provider.Enabled || provider.ClientID == "" || provider.ClientSecret == "" {
		return "", serializer.NewError(serializer.CodeFeatureNotEnabled,
			"OIDC sign-in is not enabled", nil)
	}
	return startExternalFlowWithCallback(c, provider, s.Redirect, false, legacyOIDCCallbackURL(c))
}

// legacyOIDCCallbackURL preserves the redirect URI used before the
// Pro-compatible provider endpoints were added. Existing IdP registrations
// only whitelist this address.
func legacyOIDCCallbackURL(c *gin.Context) string {
	base := strings.TrimSuffix(dependency.FromContext(c).SettingProvider().SiteURL(c).String(), "/")
	return base + "/api/v4/session/sso/oidc/callback"
}

// startOIDCFlowForProvider starts one OIDC/Logto provider selected from
// external_login_providers.
func startOIDCFlowForProvider(c *gin.Context, provider *setting.ExternalLoginProvider,
	redirect string, link bool, callbackURL string) (string, error) {
	dep := dependency.FromContext(c)
	cfg := providerSetting(provider)

	var loginUserID int
	if link {
		loginUser := inventory.UserFromContext(c)
		if loginUser == nil || loginUser.ID <= 0 {
			return "", serializer.NewError(serializer.CodeCheckLogin, "Please sign in first", nil)
		}
		loginUserID = loginUser.ID
	}

	discovery, err := loadDiscovery(c, dep, cfg)
	if err != nil {
		return "", serializer.NewError(serializer.CodeInternalSetting, "Failed to load sign-in discovery document", err)
	}

	state := randomToken(32)
	verifier := randomToken(48)
	challenge := pkceChallenge(verifier)
	nonce := randomToken(24)

	session := &oidcState{
		Verifier:    verifier,
		Redirect:    sanitizeRedirect(redirect),
		CallbackURL: callbackURL,
		Link:        link,
		UserID:      loginUserID,
		Nonce:       nonce,
		ProviderID:  provider.ID,
	}
	if raw, err := json.Marshal(session); err == nil {
		if err := dep.KV().Set(externalStatePrefix+state, string(raw), oidcStateTTL); err != nil {
			return "", serializer.NewError(serializer.CodeInternalSetting, "Failed to persist OIDC state", err)
		}
	}

	authURL, err := url.Parse(discovery.AuthorizationEndpoint)
	if err != nil {
		return "", serializer.NewError(serializer.CodeInternalSetting, "Invalid OIDC authorization endpoint", err)
	}

	query := authURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", cfg.ClientID)
	query.Set("redirect_uri", callbackURL)
	query.Set("scope", oidcScopes(cfg.Scopes))
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	if cfg.DirectSSO != "" {
		query.Set("direct_sign_in", cfg.DirectSSO)
	}
	authURL.RawQuery = query.Encode()

	return authURL.String(), nil
}

// startQQFlow starts QQ OAuth 2.0. QQ does not support PKCE; state remains a
// one-time server-side session and the callback redirect URI is fixed.
func startQQFlow(c *gin.Context, provider *setting.ExternalLoginProvider,
	redirect string, link bool, callbackURL string) (string, error) {
	dep := dependency.FromContext(c)
	var loginUserID int
	if link {
		loginUser := inventory.UserFromContext(c)
		if loginUser == nil || loginUser.ID <= 0 {
			return "", serializer.NewError(serializer.CodeCheckLogin, "Please sign in first", nil)
		}
		loginUserID = loginUser.ID
	}

	state := randomToken(32)
	session := &oidcState{
		Redirect:    sanitizeRedirect(redirect),
		CallbackURL: callbackURL,
		Link:        link,
		UserID:      loginUserID,
		ProviderID:  provider.ID,
	}
	raw, _ := json.Marshal(session)
	if err := dep.KV().Set(externalStatePrefix+state, string(raw), oidcStateTTL); err != nil {
		return "", serializer.NewError(serializer.CodeInternalSetting, "Failed to persist QQ state", err)
	}

	authURL, err := url.Parse(qqAuthorizationURL)
	if err != nil {
		return "", err
	}
	query := authURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", provider.ClientID)
	query.Set("redirect_uri", callbackURL)
	query.Set("state", state)
	query.Set("scope", "get_user_info")
	authURL.RawQuery = query.Encode()
	return authURL.String(), nil
}

// Callback validates the provider response and redirects back to the frontend
// with a one-time ticket.
func (s *OIDCCallbackService) Callback(c *gin.Context) (string, error) {
	dep := dependency.FromContext(c)
	frontendBase := strings.TrimSuffix(dep.SettingProvider().SiteURL(c).String(), "/")

	if s.Error != "" {
		return oidcFrontendError(frontendBase, s.Error, s.ErrorDescription), nil
	}
	if s.Code == "" || s.State == "" {
		return oidcFrontendError(frontendBase, "invalid_request", "Missing code or state"), nil
	}

	rawState, ok := dep.KV().Get(externalStatePrefix + s.State)
	if !ok {
		return oidcFrontendError(frontendBase, "invalid_state", "The sign-in session has expired"), nil
	}
	_ = dep.KV().Delete(externalStatePrefix, s.State)

	session := &oidcState{}
	if err := json.Unmarshal([]byte(rawState.(string)), session); err != nil {
		return oidcFrontendError(frontendBase, "invalid_state", "Malformed sign-in session"), nil
	}

	provider, err := resolveSessionProvider(c, session)
	if err != nil {
		return oidcFrontendError(frontendBase, "provider_unavailable", err.Error()), nil
	}
	expectedUser, err := completeExternalFlow(c, provider, dep, s.Code, session)
	if err != nil {
		code, description := oidcFlowErrorParts(err)
		return oidcFrontendError(frontendBase, code, description), nil
	}

	if session.Link {
		queries := url.Values{}
		queries.Set("linked", "1")
		if session.Redirect != "" {
			queries.Set("redirect", session.Redirect)
		}
		return frontendBase + "/session/sso/callback?" + queries.Encode(), nil
	}

	ticket := randomToken(32)
	payload, _ := json.Marshal(&oidcTicket{UserID: expectedUser.ID, Redirect: session.Redirect})
	if err := dep.KV().Set(externalTicketPrefix+ticket, string(payload), oidcTicketTTL); err != nil {
		return oidcFrontendError(frontendBase, "ticket_failed", "Failed to create sign-in ticket"), nil
	}

	queries := url.Values{}
	queries.Set("ticket", ticket)
	if session.Redirect != "" {
		queries.Set("redirect", session.Redirect)
	}
	return frontendBase + "/session/sso/callback?" + queries.Encode(), nil
}

// completeOIDCFlowForProvider is the provider-aware entry used by the shared
// dispatcher.
func completeOIDCFlowForProvider(c *gin.Context, provider *setting.ExternalLoginProvider,
	dep dependency.Dep, code string, session *oidcState) (*ent.User, error) {
	cfg := providerSetting(provider)
	discovery, err := loadDiscovery(c, dep, cfg)
	if err != nil {
		return nil, newOIDCFlowError("discovery_failed", err.Error(), err)
	}
	tokens, err := exchangeCode(c, cfg, discovery, code, session.Verifier, session.CallbackURL)
	if err != nil {
		return nil, newOIDCFlowError("token_exchange_failed", err.Error(), err)
	}
	claims := &oidcIDTokenClaims{}
	claimsRaw := make(map[string]any)
	if tokens.IDToken != "" {
		if err := verifyIDToken(c, dep, discovery, cfg.ClientID, tokens.IDToken, session.Nonce, claims); err != nil {
			return nil, newOIDCFlowError("invalid_id_token", err.Error(), err)
		}
		claimsRaw = decodeJWTPayload(tokens.IDToken)
	} else if tokens.AccessToken != "" {
		info, err := fetchUserInfo(c, dep, discovery, tokens.AccessToken)
		if err != nil {
			return nil, newOIDCFlowError("userinfo_failed", err.Error(), err)
		}
		claimsRaw = info.Raw
		claims.Subject, claims.Email = info.Sub, info.Email
		claims.EmailVerified, claims.Name = info.EmailVerified, info.Name
		claims.PreferredUsername, claims.Nickname = info.PreferredUsername, info.Nickname
		claims.Picture = info.Picture
	}
	return completeExternalIdentity(c, dep, provider, session, claims, claimsRaw)
}

// Exchange trades a one-time ticket for a token pair.
func (s *OIDCExchangeService) Exchange(c *gin.Context) (*OIDCExchangeResponse, error) {
	dep := dependency.FromContext(c)
	raw, ok := dep.KV().Get(externalTicketPrefix + s.Ticket)
	if !ok {
		return nil, serializer.NewError(serializer.CodeCredentialInvalid, "Sign-in ticket is invalid or expired", nil)
	}
	_ = dep.KV().Delete(externalTicketPrefix, s.Ticket)

	ticket := &oidcTicket{}
	if err := json.Unmarshal([]byte(raw.(string)), ticket); err != nil {
		return nil, serializer.NewError(serializer.CodeCredentialInvalid, "Malformed sign-in ticket", err)
	}

	return exchangeTicket(c, ticket.UserID)
}

func exchangeTicket(c *gin.Context, userID int) (*OIDCExchangeResponse, error) {
	dep := dependency.FromContext(c)

	// BuildUser needs the user's group to be hydrated, otherwise the issued
	// session payload would be missing its group information.
	ctx := context.WithValue(c, inventory.LoadUserGroup{}, true)
	expectedUser, err := dep.UserClient().GetActiveByID(ctx, userID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeUserNotFound, "User not found", err)
	}

	token, err := dep.TokenAuth().Issue(c, &auth.IssueTokenArgs{User: expectedUser})
	if err != nil {
		audit.Record(c, audit.EventUserLoginFailed, &audit.LogEntry{
			Failed:    true,
			Error:     err.Error(),
			Name:      expectedUser.Email,
			LoginWith: "openid",
		}, audit.WithUser(expectedUser.ID))
		return nil, serializer.NewError(serializer.CodeEncryptError, "Failed to issue token pair", err)
	}

	audit.Record(c, audit.EventUserLogin, &audit.LogEntry{
		Name:      expectedUser.Email,
		LoginWith: "openid",
	}, audit.WithUser(expectedUser.ID))
	return &OIDCExchangeResponse{
		User:  usersvc.BuildUser(expectedUser, dep.HashIDEncoder()),
		Token: *token,
	}, nil
}

// Status reports whether OIDC is available and optionally whether the current
// user already linked an account.
func (s *OIDCLinkService) Status(c *gin.Context) (*OIDCStatusResponse, error) {
	dep := dependency.FromContext(c)
	provider := firstConfiguredProviderOfType(c, ProviderOIDC)
	if provider == nil {
		provider = firstConfiguredProviderOfType(c, ProviderLogto)
	}
	cfg := dep.SettingProvider().OIDC(c)
	res := &OIDCStatusResponse{
		Enabled:   provider != nil && provider.Enabled && provider.ClientID != "",
		Name:      firstNonEmpty(cfg.Name, defaultOIDCName),
		Provider:  ProviderOIDC,
		DirectSSO: cfg.DirectSSO,
	}
	if provider != nil {
		res.Name = providerDisplayName(provider)
		res.Provider = provider.Type
		res.DirectSSO = provider.DirectSSO
		res.ProviderID = provider.ID
	}

	loginUser := inventory.UserFromContext(c)
	if loginUser == nil || loginUser.ID <= 0 {
		return res, nil
	}

	identities, err := dep.ExternalIdentityClient().ListByUser(c, loginUser.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to load linked identities", err)
	}

	if identity, ok := lo.Find(identities, func(item *inventory.ExternalIdentity) bool {
		providerType, _ := setting.ParseExternalIdentityProviderKey(item.Provider)
		return provider != nil && providerType == provider.Type
	}); ok {
		res.Linked = true
		res.Email = identity.Email
		res.Avatar = identity.Avatar
	}

	return res, nil
}

// Link starts an OIDC flow that binds the identity to the current user.
func (s *OIDCLinkService) Link(c *gin.Context) (string, error) {
	provider := firstConfiguredProviderOfType(c, ProviderOIDC)
	if provider == nil {
		provider = firstConfiguredProviderOfType(c, ProviderLogto)
	}
	if provider == nil {
		return "", serializer.NewError(serializer.CodeFeatureNotEnabled,
			"OIDC sign-in is not enabled", nil)
	}
	return startExternalFlowWithCallback(c, provider, s.Redirect, true, legacyOIDCCallbackURL(c))
}

// Unlink removes the OIDC/Logto binding for the current user.
func (s *OIDCLinkService) Unlink(c *gin.Context) error {
	dep := dependency.FromContext(c)
	loginUser := inventory.UserFromContext(c)
	if loginUser == nil || loginUser.ID <= 0 {
		return serializer.NewError(serializer.CodeCheckLogin, "Please sign in first", nil)
	}

	providerType := ProviderOIDC
	provider := firstConfiguredProviderOfType(c, ProviderOIDC)
	if provider == nil {
		provider = firstConfiguredProviderOfType(c, ProviderLogto)
		if provider != nil {
			providerType = ProviderLogto
		}
	} else {
		providerType = provider.Type
	}
	providerID := openIDProviderNumber(providerType, 0)
	if provider != nil {
		providerID = providerOpenIDNumber(provider)
	}

	if err := unlinkIdentity(c, dep, loginUser.ID, providerType, providerID); err != nil {
		audit.Record(c, audit.EventUnlinkAccount, &audit.LogEntry{
			Failed:         true,
			Error:          err.Error(),
			OpenIDProvider: providerID,
		}, audit.WithUser(loginUser.ID))
		return serializer.NewError(serializer.CodeDBError, "Failed to unlink identity", err)
	}

	audit.Record(c, audit.EventUnlinkAccount, &audit.LogEntry{
		OpenIDProvider: providerID,
	}, audit.WithUser(loginUser.ID))
	return nil
}

func applyOIDCGroup(c *gin.Context, dep dependency.Dep, cfg *setting.OIDCSetting, raw map[string]any,
	expectedUser *ent.User) error {
	groupField := strings.TrimSpace(cfg.GroupField)
	if groupField == "" || len(raw) == 0 {
		return nil
	}

	value := lookupOIDCClaim(raw, groupField)
	if value == "" {
		return nil
	}

	var mapping map[string]int
	if err := json.Unmarshal([]byte(cfg.GroupMapping), &mapping); err != nil {
		return nil
	}
	groupID, ok := mapping[value]
	if !ok || groupID <= 0 || groupID == expectedUser.GroupUsers {
		return nil
	}

	group, err := dep.GroupClient().GetByID(c, groupID)
	if err != nil {
		return fmt.Errorf("mapped group %d is unavailable: %w", groupID, err)
	}
	if group.ID != groupID {
		return fmt.Errorf("mapped group %d is unavailable", groupID)
	}

	previousGroup := expectedUser.GroupUsers
	updated, err := dep.UserClient().SetGroup(c, expectedUser.ID, groupID)
	if err != nil {
		return fmt.Errorf("failed to apply mapped group: %w", err)
	}
	*expectedUser = *updated
	audit.Record(c, audit.EventGroupChanged, &audit.LogEntry{
		GroupID:     groupID,
		GroupIDFrom: previousGroup,
	}, audit.WithUser(expectedUser.ID))
	return nil
}

func applyOIDCFieldMapping(cfg *setting.OIDCSetting, claims *oidcIDTokenClaims, raw map[string]any) {
	mapping := make(map[string]string)
	if cfg.FieldMapping != "" {
		_ = json.Unmarshal([]byte(cfg.FieldMapping), &mapping)
	}

	fieldValue := func(configured, legacyKey string) string {
		claimPath := firstNonEmpty(configured, mapping[legacyKey])
		if claimPath == "" {
			return ""
		}
		return lookupOIDCClaim(raw, claimPath)
	}

	if subject := fieldValue(cfg.SubField, "sub"); subject != "" {
		claims.Subject = subject
	}
	if email := fieldValue(cfg.EmailField, "email"); email != "" {
		claims.Email = email
		verifiedPath := "email_verified"
		if field := strings.TrimSpace(cfg.EmailField); field != "" {
			verifiedPath = field + "_verified"
		}
		if verified, ok := lookupOIDCBool(raw, verifiedPath); ok {
			claims.EmailVerified = verified
		} else if verifiedPath != "email_verified" {
			if verified, ok := lookupOIDCBool(raw, "email_verified"); ok {
				claims.EmailVerified = verified
			}
		}
	}
	if name := firstNonEmpty(fieldValue(cfg.NameField, "name"), fieldValue(cfg.NameField, "nickname"),
		fieldValue(cfg.NameField, "preferred_username")); name != "" {
		claims.Name = name
	}
	if avatar := fieldValue(cfg.AvatarField, "picture"); avatar != "" {
		claims.Picture = avatar
	}
}

func lookupOIDCBool(raw map[string]any, path string) (bool, bool) {
	current := any(raw)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return false, false
		}
		current, ok = object[part]
		if !ok {
			return false, false
		}
	}

	switch value := current.(type) {
	case bool:
		return value, true
	case string:
		parsed, err := strconv.ParseBool(value)
		return parsed, err == nil
	default:
		return false, false
	}
}

func lookupOIDCClaim(raw map[string]any, path string) string {
	path = strings.TrimSpace(strings.TrimPrefix(path, "."))
	if path == "" {
		return ""
	}

	var current any = raw
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = object[segment]
		if !ok {
			return ""
		}
	}

	switch value := current.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		if len(value) == 0 {
			return ""
		}
		return lookupOIDCClaim(map[string]any{"value": value[0]}, "value")
	default:
		return ""
	}
}

func decodeJWTPayload(rawToken string) map[string]any {
	parts := strings.Split(rawToken, ".")
	if len(parts) < 2 {
		return map[string]any{}
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return map[string]any{}
	}

	res := make(map[string]any)
	if err := json.Unmarshal(payload, &res); err != nil {
		return map[string]any{}
	}
	return res
}

func loadDiscovery(ctx context.Context, dep dependency.Dep, cfg *setting.OIDCSetting) (*OIDCDiscovery, error) {
	if cfg.WellKnownJSON != "" {
		discovery := &OIDCDiscovery{}
		if err := json.Unmarshal([]byte(cfg.WellKnownJSON), discovery); err == nil {
			if discovery.AuthorizationEndpoint != "" && discovery.TokenEndpoint != "" {
				return discovery, nil
			}

			// Older admin UIs stored either an issuer or a well-known URL in a
			// partial document. Accept that shape and fetch the real discovery
			// document instead of failing on the partial JSON.
			var partial struct {
				Issuer    string `json:"issuer"`
				WellKnown string `json:"well_known"`
			}
			_ = json.Unmarshal([]byte(cfg.WellKnownJSON), &partial)
			wellKnown := strings.TrimSpace(partial.WellKnown)
			if wellKnown == "" {
				issuer := strings.TrimSuffix(firstNonEmpty(partial.Issuer, discovery.Issuer), "/")
				if issuer != "" {
					wellKnown = issuer + "/.well-known/openid-configuration"
				}
			}
			if wellKnown != "" {
				if err := getJSON(ctx, dep, wellKnown, "", discovery); err != nil {
					return nil, err
				}
				if discovery.AuthorizationEndpoint == "" || discovery.TokenEndpoint == "" {
					return nil, errors.New("discovery document is missing required endpoints")
				}
				return discovery, nil
			}
		}
	}

	wellKnown := strings.TrimSpace(cfg.WellKnown)
	if wellKnown == "" {
		issuer := strings.TrimSuffix(strings.TrimSpace(cfg.Issuer), "/")
		if issuer == "" {
			return nil, errors.New("OIDC issuer or well-known URL is required")
		}
		wellKnown = issuer + "/.well-known/openid-configuration"
	}

	discovery := &OIDCDiscovery{}
	if err := getJSON(ctx, dep, wellKnown, "", discovery); err != nil {
		return nil, err
	}
	if discovery.AuthorizationEndpoint == "" || discovery.TokenEndpoint == "" {
		return nil, errors.New("discovery document is missing required endpoints")
	}

	return discovery, nil
}

func exchangeCode(ctx context.Context, cfg *setting.OIDCSetting, discovery *OIDCDiscovery,
	code, verifier, callbackURL string) (*oidcTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", cfg.ClientID)
	form.Set("redirect_uri", callbackURL)
	form.Set("code_verifier", verifier)
	if cfg.ClientSecret != "" {
		form.Set("client_secret", cfg.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discovery.TokenEndpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result := &oidcTokenResponse{}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	if resp.StatusCode >= 400 || result.Error != "" {
		return nil, fmt.Errorf("token endpoint error: %s %s", result.Error, result.Description)
	}
	if result.IDToken == "" && result.AccessToken == "" {
		return nil, errors.New("token endpoint returned no usable token")
	}

	return result, nil
}

func verifyIDToken(ctx context.Context, dep dependency.Dep, discovery *OIDCDiscovery, clientID, rawToken,
	expectedNonce string, claims *oidcIDTokenClaims) error {
	if discovery.JWKSURI == "" {
		return errors.New("discovery document is missing jwks_uri")
	}

	jwks, err := loadJWKS(ctx, dep, discovery.JWKSURI)
	if err != nil {
		return err
	}

	parse := func(keys *oidcJWKS) error {
		keyFunc := func(token *jwt.Token) (any, error) {
			kid, _ := token.Header["kid"].(string)
			for _, candidate := range keys.Keys {
				if candidate.Kty != "RSA" {
					continue
				}
				if kid != "" && candidate.Kid != kid {
					continue
				}
				if candidate.Use != "" && candidate.Use != "sig" {
					continue
				}
				return rsaPublicKey(candidate)
			}
			return nil, errors.New("no matching signing key found")
		}

		parsed, err := jwt.ParseWithClaims(rawToken, claims, keyFunc,
			jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}),
			jwt.WithIssuer(strings.TrimSuffix(discovery.Issuer, "/")),
			jwt.WithAudience(clientID),
			jwt.WithExpirationRequired(),
		)
		if err != nil {
			return err
		}
		if !parsed.Valid {
			return errors.New("ID token signature is invalid")
		}
		return nil
	}

	if err := parse(jwks); err != nil {
		// Cached keys may predate a provider key rotation. Invalidate the
		// cache once and retry with a fresh JWKS before failing the sign-in.
		_ = dep.KV().Delete(oidcJWKSPrefix, discovery.JWKSURI)
		fresh, refreshErr := loadJWKS(ctx, dep, discovery.JWKSURI)
		if refreshErr != nil {
			return err
		}
		if retryErr := parse(fresh); retryErr != nil {
			return retryErr
		}
	}
	if expectedNonce != "" && subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(expectedNonce)) != 1 {
		return errors.New("ID token nonce does not match")
	}
	if claims.Subject == "" {
		return errors.New("ID token is missing subject")
	}

	return nil
}

func loadJWKS(ctx context.Context, dep dependency.Dep, uri string) (*oidcJWKS, error) {
	jwks := &oidcJWKS{}
	if raw, ok := dep.KV().Get(oidcJWKSPrefix + uri); ok {
		_ = json.Unmarshal([]byte(raw.(string)), jwks)
	}
	if len(jwks.Keys) > 0 {
		return jwks, nil
	}

	loaded := &oidcJWKS{}
	if err := getJSON(ctx, dep, uri, "", loaded); err != nil {
		return nil, fmt.Errorf("failed to load JWKS: %w", err)
	}
	if payload, err := json.Marshal(loaded); err == nil {
		_ = dep.KV().Set(oidcJWKSPrefix+uri, string(payload), 300)
	}
	if len(loaded.Keys) == 0 {
		return nil, errors.New("JWKS contains no signing keys")
	}
	return loaded, nil
}

func fetchUserInfo(ctx context.Context, dep dependency.Dep, discovery *OIDCDiscovery, accessToken string) (*oidcUserInfo, error) {
	if discovery.UserinfoEndpoint == "" {
		return nil, errors.New("discovery document is missing userinfo_endpoint")
	}

	raw := make(map[string]any)
	if err := getJSON(ctx, dep, discovery.UserinfoEndpoint, accessToken, &raw); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	info := &oidcUserInfo{Raw: raw}
	if err := json.Unmarshal(encoded, info); err != nil {
		return nil, err
	}

	return info, nil
}

func getJSON(ctx context.Context, dep dependency.Dep, target, bearer string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("request to %s failed with status %d", target, resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}

func rsaPublicKey(key oidcJWK) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, fmt.Errorf("invalid modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, fmt.Errorf("invalid exponent: %w", err)
	}

	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e == 0 {
		return nil, errors.New("invalid exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}, nil
}

func oidcScopes(extra string) string {
	scopes := []string{"openid", "email", "profile"}
	for _, item := range strings.FieldsFunc(extra, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		item = strings.TrimSpace(item)
		if item == "" || lo.Contains(scopes, item) {
			continue
		}
		scopes = append(scopes, item)
	}

	return strings.Join(scopes, " ")
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func sanitizeRedirect(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || strings.HasPrefix(raw, "//") {
		return ""
	}
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func oidcFrontendError(base, code, description string) string {
	queries := url.Values{}
	queries.Set("error", code)
	if description != "" {
		queries.Set("error_description", description)
	}
	return base + "/session/sso/callback?" + queries.Encode()
}
