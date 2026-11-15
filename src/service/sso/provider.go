package sso

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/gin-gonic/gin"
)

const (
	ProviderQQ    = "qq"
	ProviderLogto = "logto"

	externalStatePrefix  = "sso_external_state_"
	externalTicketPrefix = "sso_external_ticket_"

	qqAuthorizationEndpoint = "https://graph.qq.com/oauth2.0/authorize"
	qqTokenEndpoint         = "https://graph.qq.com/oauth2.0/token"
	qqOpenIDEndpoint        = "https://graph.qq.com/oauth2.0/me"
	qqUserInfoEndpoint      = "https://graph.qq.com/user/get_user_info"
)

// Endpoint variables are package-level so tests can point them at a local
// fixture server without changing the public OAuth routes.
var (
	qqAuthorizationURL = qqAuthorizationEndpoint
	qqTokenURL         = qqTokenEndpoint
	qqOpenIDURL        = qqOpenIDEndpoint
	qqUserInfoURL      = qqUserInfoEndpoint
)

// openIDProviderNumber maps a provider type to the open_ids.provider value.
// Pro writes logto=0, qq=1 and oidc>=2.
func openIDProviderNumber(providerType string, configuredID int) int {
	switch providerType {
	case ProviderLogto:
		return 0
	case ProviderQQ:
		return 1
	default:
		if configuredID > 0 {
			return configuredID
		}
		return 2
	}
}

func resolveExternalProvider(c *gin.Context, providerID int) (*setting.ExternalLoginProvider, error) {
	dep := dependency.FromContext(c)
	providerType := ""
	if hint := c.Param("provider"); hint != "" {
		switch hint {
		case "oidc":
			providerType = ProviderOIDC
		case "logto":
			providerType = ProviderLogto
		case "qq":
			providerType = ProviderQQ
		}
	}
	provider := dep.SettingProvider().ExternalLoginProvider(c, providerID)
	if provider == nil && providerType != "" {
		provider = firstConfiguredProviderOfType(c, providerType)
	}
	if provider != nil && (!provider.Enabled || provider.ClientID == "" || provider.ClientSecret == "") {
		return nil, serializer.NewError(serializer.CodeFeatureNotEnabled,
			"This sign-in provider is not enabled", nil)
	}
	if provider == nil {
		return nil, serializer.NewError(serializer.CodeFeatureNotEnabled,
			"This sign-in provider is not configured", nil)
	}
	return provider, nil
}

func firstConfiguredProvider(c *gin.Context) *setting.ExternalLoginProvider {
	providers := dependency.FromContext(c).SettingProvider().ExternalLoginProviders(c)
	if len(providers) == 0 {
		return nil
	}
	return &providers[0]
}

func firstConfiguredProviderOfType(c *gin.Context, providerType string) *setting.ExternalLoginProvider {
	for _, provider := range dependency.FromContext(c).SettingProvider().ExternalLoginProviders(c) {
		if provider.Type == providerType {
			selected := provider
			return &selected
		}
	}
	return nil
}

// resolveSessionProvider restores the provider from the exact id stored at
// authorization time. A callback path hint may disambiguate legacy sessions
// that predate ProviderID.
func resolveSessionProvider(c *gin.Context, session *oidcState) (*setting.ExternalLoginProvider, error) {
	provider, err := resolveExternalProvider(c, session.ProviderID)
	if err == nil {
		return provider, nil
	}
	if providerType := c.Param("provider"); providerType == ProviderQQ ||
		providerType == ProviderLogto || providerType == ProviderOIDC {
		if hinted := firstConfiguredProviderOfType(c, providerType); hinted != nil &&
			hinted.Enabled && hinted.ClientID != "" && hinted.ClientSecret != "" {
			return hinted, nil
		}
	}
	return nil, err
}

// providerCallbackURL returns the exact redirect URI shown in the admin UI.
func providerCallbackURL(c *gin.Context, dep dependency.Dep, provider *setting.ExternalLoginProvider) string {
	base := strings.TrimSuffix(dep.SettingProvider().SiteURL(c).String(), "/")
	if provider.Type == ProviderQQ && provider.ID == 1 {
		return base + "/login/qq"
	}
	return fmt.Sprintf("%s/callback/openid/%d", base, provider.ID)
}

// startExternalFlow dispatches the authorization code flow by provider type.
func startExternalFlow(c *gin.Context, provider *setting.ExternalLoginProvider, redirect string,
	link bool) (string, error) {
	dep := dependency.FromContext(c)
	callbackURL := providerCallbackURL(c, dep, provider)
	return startExternalFlowWithCallback(c, provider, redirect, link, callbackURL)
}

// startExternalFlowWithCallback lets the legacy OIDC endpoints keep their
// original redirect URI while the Pro-compatible endpoints use the new path.
func startExternalFlowWithCallback(c *gin.Context, provider *setting.ExternalLoginProvider,
	redirect string, link bool, callbackURL string) (string, error) {
	switch provider.Type {
	case ProviderQQ:
		return startQQFlow(c, provider, redirect, link, callbackURL)
	case ProviderOIDC, ProviderLogto:
		return startOIDCFlowForProvider(c, provider, redirect, link, callbackURL)
	default:
		return "", serializer.NewError(serializer.CodeFeatureNotEnabled,
			"Unsupported sign-in provider type", nil)
	}
}

// completeExternalFlow dispatches callback completion by provider type.
func completeExternalFlow(c *gin.Context, provider *setting.ExternalLoginProvider, dep dependency.Dep,
	code string, session *oidcState) (*ent.User, error) {
	switch provider.Type {
	case ProviderQQ:
		return completeQQFlow(c, provider, dep, code, session)
	case ProviderOIDC, ProviderLogto:
		return completeOIDCFlowForProvider(c, provider, dep, code, session)
	default:
		return nil, newOIDCFlowError("unsupported_provider", "Unsupported sign-in provider", nil)
	}
}

// completeQQFlow exchanges a QQ authorization code, fetches the OpenID and
// profile, then applies the same account linking policy as OIDC.
func completeQQFlow(c *gin.Context, provider *setting.ExternalLoginProvider, dep dependency.Dep,
	code string, session *oidcState) (*ent.User, error) {
	accessToken, err := exchangeQQCode(c, provider, code, session.CallbackURL)
	if err != nil {
		return nil, newOIDCFlowError("token_exchange_failed", err.Error(), err)
	}
	identity, err := fetchQQIdentity(c, dep, provider, accessToken)
	if err != nil {
		return nil, newOIDCFlowError("userinfo_failed", err.Error(), err)
	}
	claims := &oidcIDTokenClaims{
		Email:             identity.Email,
		EmailVerified:     identity.Email != "",
		Name:              identity.Nickname,
		PreferredUsername: identity.Nickname,
		Picture:           identity.Avatar,
	}
	claims.Subject = identity.OpenID
	claimsRaw := map[string]any{
		"sub":      identity.OpenID,
		"openid":   identity.OpenID,
		"name":     identity.Nickname,
		"nickname": identity.Nickname,
		"picture":  identity.Avatar,
	}
	if identity.Email != "" {
		claimsRaw["email"] = identity.Email
		claimsRaw["email_verified"] = true
	}
	return completeExternalIdentity(c, dep, provider, session, claims, claimsRaw)
}

func exchangeQQCode(ctx context.Context, provider *setting.ExternalLoginProvider,
	code, callbackURL string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", provider.ClientID)
	form.Set("client_secret", provider.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", callbackURL)
	form.Set("fmt", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, qqTokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("QQ token endpoint returned status %d", resp.StatusCode)
	}
	payload := parseQQJSONP(body)
	accessToken := stringValue(payload["access_token"])
	if accessToken == "" {
		return "", fmt.Errorf("QQ token endpoint did not return an access token: %s",
			firstNonEmpty(stringValue(payload["error_description"]), stringValue(payload["error"])))
	}
	return accessToken, nil
}

func providerOpenIDNumber(provider *setting.ExternalLoginProvider) int {
	return openIDProviderNumber(provider.Type, provider.ID)
}

// identityProviderKey namespaces the stored binding by configured provider id
// so two providers of the same type never collide. Logto keeps its bare type
// only for rows written before ids were recorded.
func identityProviderKey(provider *setting.ExternalLoginProvider) string {
	return setting.ExternalIdentityProviderKey(provider.Type, provider.ID)
}

// findExternalIdentity resolves a binding by its provider-scoped key and
// migrates rows written by older builds, which stored just the provider
// type (for example "logto") without the configured provider id.
func findExternalIdentity(ctx context.Context, client inventory.ExternalIdentityClient,
	provider *setting.ExternalLoginProvider, subject string) (*inventory.ExternalIdentity, error) {
	key := identityProviderKey(provider)
	identity, err := client.GetBySubject(ctx, key, subject)
	if err == nil {
		return identity, nil
	}
	if !errors.Is(err, inventory.ErrExternalIdentityNotFound) || key == provider.Type {
		return nil, err
	}

	identity, err = client.GetBySubject(ctx, provider.Type, subject)
	if err != nil {
		return nil, err
	}
	if err := client.UpdateProvider(ctx, identity.ID, key); err != nil {
		return nil, err
	}
	identity.Provider = key
	return identity, nil
}

// deleteIdentityForReplacement removes both the current provider-scoped row
// and any legacy bare-type row owned by the same user.
func deleteIdentityForReplacement(ctx context.Context, client inventory.ExternalIdentityClient,
	userID int, provider *setting.ExternalLoginProvider) error {
	key := identityProviderKey(provider)
	if err := client.DeleteByUserAndProvider(ctx, userID, key); err != nil {
		return err
	}
	if key != provider.Type {
		return client.DeleteByUserAndProvider(ctx, userID, provider.Type)
	}
	return nil
}

func providerDisplayName(provider *setting.ExternalLoginProvider) string {
	if strings.TrimSpace(provider.Name) != "" {
		return provider.Name
	}
	switch provider.Type {
	case ProviderQQ:
		return "QQ"
	case ProviderLogto:
		return "Logto"
	default:
		return "SSO"
	}
}

// providerDirectSignInAllowed mirrors the direct_sign_in switch. QQ defaults
// to requiring an existing local account, Logto/OIDC default to the site
// registration setting.
func providerDirectSignInAllowed(provider *setting.ExternalLoginProvider, registerEnabled bool) bool {
	raw := strings.TrimSpace(provider.DirectSignIn)
	if raw == "" {
		if provider.Type == ProviderQQ {
			return false
		}
		return registerEnabled
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return enabled
}

// ensureOpenIDLinkWithClient mirrors a linked identity into the Pro-compatible
// open_ids table. It never replaces a binding owned by another user.
func ensureOpenIDLinkWithClient(ctx context.Context, client *ent.Client,
	provider *setting.ExternalLoginProvider, subject string, userID int) error {
	providerID := providerOpenIDNumber(provider)
	existing, err := queryOpenIDUserByProviderWithClient(ctx, client, providerID, subject)
	if err != nil {
		return err
	}
	if existing == userID {
		return nil
	}
	if existing > 0 {
		return errors.New("this identity is already linked to another user")
	}

	now := time.Now()
	query := `INSERT INTO open_ids (created_at, updated_at, deleted_at, provider, sub, user_id) VALUES (?, ?, NULL, ?, ?, ?)`
	args := []any{now, now, providerID, subject, userID}
	if client.Driver().Dialect() == dialect.Postgres {
		query = `INSERT INTO open_ids (created_at, updated_at, deleted_at, provider, sub, user_id) VALUES ($1, $2, NULL, $3, $4, $5)`
	}

	var result sql.Result
	if err := client.Driver().Exec(ctx, query, args, &result); err != nil {
		return err
	}
	return nil
}

func queryOpenIDUserByProviderWithClient(ctx context.Context, client *ent.Client, providerID int, subject string) (int, error) {
	query := `SELECT user_id FROM open_ids WHERE provider = ? AND sub = ? AND deleted_at IS NULL LIMIT 1`
	args := []any{providerID, subject}
	if client.Driver().Dialect() == dialect.Postgres {
		query = `SELECT user_id FROM open_ids WHERE provider = $1 AND sub = $2 AND deleted_at IS NULL LIMIT 1`
	}

	rows := new(entsql.Rows)
	if err := client.Driver().Query(ctx, query, args, rows); err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, nil
	}

	var userID int
	if err := rows.Scan(&userID); err != nil {
		return 0, err
	}
	return userID, nil
}

func deleteOpenIDLinkByProviderWithClient(ctx context.Context, client *ent.Client, userID int,
	providerID int) error {
	now := time.Now()
	query := `UPDATE open_ids SET deleted_at = ?, updated_at = ? WHERE user_id = ? AND provider = ? AND deleted_at IS NULL`
	args := []any{now, now, userID, providerID}
	if client.Driver().Dialect() == dialect.Postgres {
		query = `UPDATE open_ids SET deleted_at = $1, updated_at = $2 WHERE user_id = $3 AND provider = $4 AND deleted_at IS NULL`
	}

	var result sql.Result
	return client.Driver().Exec(ctx, query, args, &result)
}

// unlinkIdentity atomically removes both the profile record and the Pro
// compatible open_ids row so a failed unlink cannot leave a half-binding.
func unlinkIdentity(ctx context.Context, dep dependency.Dep, userID int, providerType string,
	providerID int) error {
	tx, err := dep.DBClient().Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	txClient := tx.Client()
	if err := inventory.NewExternalIdentityClient(txClient).DeleteByUserAndProvider(ctx, userID, providerType); err != nil {
		return err
	}
	if err := deleteOpenIDLinkByProviderWithClient(ctx, txClient, userID, providerID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// UnlinkIdentityByID removes the linked identity named in the DELETE
// /session/openid/:id endpoint. The id belongs to external_identities.
func UnlinkIdentityByID(c *gin.Context, identityID int) error {
	dep := dependency.FromContext(c)
	loginUser := inventory.UserFromContext(c)
	if loginUser == nil || loginUser.ID <= 0 {
		return serializer.NewError(serializer.CodeCheckLogin, "Please sign in first", nil)
	}
	identities, err := dep.ExternalIdentityClient().ListByUser(c, loginUser.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to load linked identities", err)
	}
	for _, identity := range identities {
		if identity.ID != identityID {
			continue
		}
		providerType, storedID := setting.ParseExternalIdentityProviderKey(identity.Provider)
		providerID := openIDProviderNumber(providerType, storedID)
		if err := unlinkIdentity(c, dep, loginUser.ID, identity.Provider, providerID); err != nil {
			audit.Record(c, audit.EventUnlinkAccount, &audit.LogEntry{
				Failed: true, Error: err.Error(), OpenIDProvider: providerID,
			}, audit.WithUser(loginUser.ID))
			return serializer.NewError(serializer.CodeDBError, "Failed to unlink identity", err)
		}
		audit.Record(c, audit.EventUnlinkAccount, &audit.LogEntry{
			OpenIDProvider: providerID,
		}, audit.WithUser(loginUser.ID))
		return nil
	}
	return serializer.NewError(serializer.CodeNotFound, "Linked identity not found", nil)
}

// normalizeProviderEmail normalizes the identity email without fabricating one.
func normalizeProviderEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// completeExternalIdentity applies the shared account resolution and linking
// policy for all providers.
func completeExternalIdentity(c *gin.Context, dep dependency.Dep,
	provider *setting.ExternalLoginProvider, session *oidcState, claims *oidcIDTokenClaims,
	claimsRaw map[string]any) (*ent.User, error) {
	applyOIDCFieldMapping(providerSetting(provider), claims, claimsRaw)
	if claims.Subject == "" {
		return nil, newOIDCFlowError("invalid_subject", "Provider did not return a subject", nil)
	}

	email := normalizeProviderEmail(claims.Email)
	displayName := firstNonEmpty(claims.Name, claims.Nickname, claims.PreferredUsername)
	userClient := dep.UserClient()
	identityClient := dep.ExternalIdentityClient()

	var (
		expectedUser *ent.User
		identity     *inventory.ExternalIdentity
		err          error
	)

	if session.Link {
		if session.UserID <= 0 {
			return nil, newOIDCFlowError("invalid_session", "Link session is missing the user", nil)
		}
		expectedUser, err = userClient.GetActiveByID(c, session.UserID)
		if err != nil {
			return nil, newOIDCFlowError("user_not_found", "The linking user is unavailable", err)
		}
	} else {
		identity, err = findExternalIdentity(c, identityClient, provider, claims.Subject)
		switch {
		case err == nil:
			expectedUser, err = userClient.GetActiveByID(c, identity.UserID)
			if err != nil {
				return nil, newOIDCFlowError("user_not_found", "The linked user is unavailable", err)
			}
		case errors.Is(err, inventory.ErrExternalIdentityNotFound):
			if !providerDirectSignInAllowed(provider, dep.SettingProvider().RegisterEnabled(c)) {
				return nil, newOIDCFlowError("account_not_linked",
					"No linked account was found. Sign in locally and link this identity from security settings.", nil)
			}
			if email == "" {
				if provider.Type != ProviderQQ {
					return nil, newOIDCFlowError("email_required", "The identity provider did not return an email", nil)
				}
				// QQ does not expose an email address. Pro creates a local
				// account on first direct sign-in, so derive a stable,
				// non-registrable placeholder from the OpenID instead of
				// failing the flow or trusting an unverified address.
				email = qqPlaceholderEmail(claims.Subject)
			}
			if !claims.EmailVerified && provider.Type != ProviderQQ {
				return nil, newOIDCFlowError("email_not_verified",
					"The identity provider did not verify this email address. Sign in first and link the account from security settings.", nil)
			}
			expectedUser, err = userClient.GetByEmail(c, email)
			if err != nil && !ent.IsNotFound(err) {
				return nil, newOIDCFlowError("user_lookup_failed", err.Error(), err)
			}
			if expectedUser == nil {
				expectedUser, err = userClient.Create(c, &inventory.NewUserArgs{
					Email:   email,
					Nick:    firstNonEmpty(displayName, email),
					Status:  user.StatusActive,
					Avatar:  claims.Picture,
					GroupID: dep.SettingProvider().DefaultGroup(c),
				})
			}
			if err != nil {
				return nil, newOIDCFlowError("user_create_failed", err.Error(), err)
			}
		default:
			return nil, newOIDCFlowError("identity_lookup_failed", err.Error(), err)
		}
	}

	if expectedUser == nil {
		return nil, newOIDCFlowError("user_not_found", "User not found", nil)
	}
	if expectedUser.Status != user.StatusActive {
		return nil, newOIDCFlowError("user_inactive", "This account is not active", nil)
	}
	if err := applyOIDCGroup(c, dep, providerSetting(provider), claimsRaw, expectedUser); err != nil {
		return nil, newOIDCFlowError("group_mapping_failed", err.Error(), err)
	}

	tx, err := dep.DBClient().Tx(c)
	if err != nil {
		return nil, newOIDCFlowError("link_failed", err.Error(), err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	txClient := tx.Client()
	identityClient = inventory.NewExternalIdentityClient(txClient)

	if session.Link {
		if current, err := findExternalIdentity(c, identityClient, provider, claims.Subject); err == nil {
			if current.UserID != expectedUser.ID {
				return nil, newOIDCFlowError("identity_already_linked", "This identity is already linked to another user", nil)
			}
			identity = current
		} else if !errors.Is(err, inventory.ErrExternalIdentityNotFound) {
			return nil, newOIDCFlowError("identity_lookup_failed", err.Error(), err)
		}
		if identity == nil {
			if err := deleteIdentityForReplacement(c, identityClient, expectedUser.ID, provider); err != nil {
				return nil, newOIDCFlowError("link_failed", err.Error(), err)
			}
		}
	}

	if identity == nil {
		if current, err := findExternalIdentity(c, identityClient, provider, claims.Subject); err == nil {
			if current.UserID != expectedUser.ID {
				return nil, newOIDCFlowError("identity_already_linked", "This identity is already linked to another user", nil)
			}
			identity = current
		} else if errors.Is(err, inventory.ErrExternalIdentityNotFound) {
			identity, err = identityClient.Create(c, &inventory.ExternalIdentity{
				Provider:    identityProviderKey(provider),
				Subject:     claims.Subject,
				Email:       email,
				DisplayName: displayName,
				Avatar:      claims.Picture,
				UserID:      expectedUser.ID,
			})
			if err != nil {
				return nil, newOIDCFlowError("link_failed", err.Error(), err)
			}
		} else {
			return nil, newOIDCFlowError("identity_lookup_failed", err.Error(), err)
		}
	}

	if err := ensureOpenIDLinkWithClient(c, txClient, provider, claims.Subject, expectedUser.ID); err != nil {
		return nil, newOIDCFlowError("link_failed", err.Error(), err)
	}
	if err := tx.Commit(); err != nil {
		return nil, newOIDCFlowError("link_failed", err.Error(), err)
	}
	committed = true

	if session.Link {
		audit.Record(c, audit.EventLinkAccount, &audit.LogEntry{
			OpenIDProvider: providerOpenIDNumber(provider),
			Sub:            claims.Subject,
			Name:           displayName,
		}, audit.WithUser(expectedUser.ID))
	}

	return expectedUser, nil
}

// qqPlaceholderEmail returns a deterministic RFC 2606 reserved-domain address
// for QQ identities that do not expose an email. The domain cannot be used for
// registration, so it can never shadow a real local account.
func qqPlaceholderEmail(subject string) string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, subject)
	if sanitized == "" {
		sanitized = "unknown"
	}
	return strings.ToLower("qq_" + sanitized + "@qq-connect.invalid")
}

func providerSetting(provider *setting.ExternalLoginProvider) *setting.OIDCSetting {
	return &setting.OIDCSetting{
		Enabled:       provider.Enabled,
		ProviderID:    provider.ID,
		Name:          providerDisplayName(provider),
		ClientID:      provider.ClientID,
		ClientSecret:  provider.ClientSecret,
		Issuer:        provider.Issuer,
		WellKnown:     provider.WellKnown,
		WellKnownJSON: provider.WellKnownJSON,
		Scopes:        provider.Scopes,
		DirectSSO:     provider.DirectSSO,
		DisplayName:   providerDisplayName(provider),
		Icon:          provider.Icon,
		DirectSignIn:  provider.DirectSignIn,
		SubField:      provider.SubField,
		EmailField:    provider.EmailField,
		AvatarField:   provider.AvatarField,
		NameField:     provider.NameField,
		GroupField:    provider.GroupField,
		GroupMapping:  provider.GroupMapping,
		FieldMapping:  provider.FieldMapping,
	}
}

// qqIdentity is the normalized result of QQ's OAuth and OpenID endpoints.
type qqIdentity struct {
	OpenID   string
	Nickname string
	Avatar   string
	Email    string
}

func fetchQQIdentity(ctx context.Context, dep dependency.Dep, provider *setting.ExternalLoginProvider,
	accessToken string) (*qqIdentity, error) {
	openIDRaw, err := qqGetWithToken(ctx, qqOpenIDURL, accessToken, nil)
	if err != nil {
		return nil, err
	}
	openIDPayload := parseQQJSONP(openIDRaw)
	openID := firstNonEmpty(stringValue(openIDPayload["openid"]))
	if openID == "" {
		return nil, errors.New("QQ did not return an openid")
	}

	infoRaw, err := qqGetWithToken(ctx, qqUserInfoURL, accessToken, url.Values{
		"oauth_consumer_key": {provider.ClientID},
		"openid":             {openID},
	})
	if err != nil {
		return nil, err
	}
	info := make(map[string]any)
	if err := json.Unmarshal(infoRaw, &info); err != nil {
		return nil, fmt.Errorf("failed to parse QQ user info: %w", err)
	}
	if code, _ := info["ret"].(float64); code != 0 {
		return nil, fmt.Errorf("QQ user info error: %s", stringValue(info["msg"]))
	}

	return &qqIdentity{
		OpenID:   openID,
		Nickname: firstNonEmpty(stringValue(info["nickname"])),
		Avatar: firstNonEmpty(stringValue(info["figureurl_qq_2"]),
			stringValue(info["figureurl_qq_1"]), stringValue(info["figureurl_2"]),
			stringValue(info["figureurl_1"])),
	}, nil
}

func qqGetWithToken(ctx context.Context, endpoint, accessToken string, extra url.Values) ([]byte, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("access_token", accessToken)
	for key, values := range extra {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("QQ endpoint returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func parseQQJSONP(raw []byte) map[string]any {
	text := strings.TrimSpace(string(raw))
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		text = text[start : end+1]
	}
	res := make(map[string]any)
	_ = json.Unmarshal([]byte(text), &res)
	return res
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}
