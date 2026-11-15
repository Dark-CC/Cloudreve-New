package sso

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/stretchr/testify/assert"
)

func TestParseQQJSONP(t *testing.T) {
	payload := parseQQJSONP([]byte(`callback( {"client_id":"10001","openid":"ABCDEF"} );`))
	assert.Equal(t, "10001", stringValue(payload["client_id"]))
	assert.Equal(t, "ABCDEF", stringValue(payload["openid"]))

	plain := parseQQJSONP([]byte(`{"access_token":"token-1","expires_in":7776000}`))
	assert.Equal(t, "token-1", stringValue(plain["access_token"]))
}

func TestQQPlaceholderEmailIsDeterministicAndNonRoutable(t *testing.T) {
	assert.Equal(t, "qq_abcdef@qq-connect.invalid", qqPlaceholderEmail("ABCDEF"))
	assert.Equal(t, qqPlaceholderEmail("ABCDEF"), qqPlaceholderEmail("ABCDEF"))
	assert.Equal(t, "qq_a_b@qq-connect.invalid", qqPlaceholderEmail("A/B"))
	assert.Equal(t, "qq_unknown@qq-connect.invalid", qqPlaceholderEmail(""))
	assert.NotContains(t, qqPlaceholderEmail("ABC"), "@qq.com")
}

func TestOpenIDProviderNumber(t *testing.T) {
	assert.Equal(t, 0, openIDProviderNumber(ProviderLogto, 7))
	assert.Equal(t, 1, openIDProviderNumber(ProviderQQ, 9))
	assert.Equal(t, 3, openIDProviderNumber("oidc", 3))
	assert.Equal(t, 2, openIDProviderNumber("oidc", 0))
}

func TestExternalIdentityProviderKeyRoundTrip(t *testing.T) {
	key := setting.ExternalIdentityProviderKey("oidc", 7)
	assert.Equal(t, "oidc#7", key)

	providerType, id := setting.ParseExternalIdentityProviderKey(key)
	assert.Equal(t, "oidc", providerType)
	assert.Equal(t, 7, id)

	// Legacy rows have no separator.
	providerType, id = setting.ParseExternalIdentityProviderKey("logto")
	assert.Equal(t, "logto", providerType)
	assert.Equal(t, -1, id)

	// Malformed suffixes must not be treated as configured ids.
	providerType, id = setting.ParseExternalIdentityProviderKey("oidc#abc")
	assert.Equal(t, "oidc#abc", providerType)
	assert.Equal(t, -1, id)
}

func TestProviderDirectSignInAllowed(t *testing.T) {
	assert.False(t, providerDirectSignInAllowed(&setting.ExternalLoginProvider{Type: ProviderQQ}, true))
	assert.True(t, providerDirectSignInAllowed(&setting.ExternalLoginProvider{Type: ProviderLogto}, true))
	assert.False(t, providerDirectSignInAllowed(&setting.ExternalLoginProvider{Type: ProviderLogto}, false))
	assert.True(t, providerDirectSignInAllowed(
		&setting.ExternalLoginProvider{Type: ProviderQQ, DirectSignIn: "true"}, false))
	assert.False(t, providerDirectSignInAllowed(
		&setting.ExternalLoginProvider{Type: ProviderLogto, DirectSignIn: "not-a-bool"}, true))
}

