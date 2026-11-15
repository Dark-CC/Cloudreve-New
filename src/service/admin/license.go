package admin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/gin-gonic/gin"
)

const (
	defaultOfflineLicenseValidity = 30 * 24 * time.Hour
	licenseSettingKey             = "license"
)

type (
	// LicenseRefreshService refreshes the local offline license information.
	LicenseRefreshService  struct{}
	LicenseRefreshParamCtx struct{}

	// ManualLicenseRefreshService imports an offline license supplied by an
	// administrator. The community build accepts the same JSON shape as its
	// local license response instead of validating a commercial signature.
	ManualLicenseRefreshService struct {
		License string `json:"license" binding:"required"`
	}
	ManualLicenseRefreshParamCtx struct{}
)

type storedLicense struct {
	ExpiredAt  time.Time `json:"expired_at"`
	Domains    []string  `json:"domains"`
	VolDomains []string  `json:"vol_domains"`
}

// RefreshAuto refreshes the domain list and extends the local offline license.
func (s *LicenseRefreshService) RefreshAuto(c *gin.Context) (*License, error) {
	dep := dependency.FromContext(c)
	domains := licenseDomains(dep.SettingProvider().AllSiteURLs(c))
	license := &License{
		ExpiredAt:  time.Now().Add(defaultOfflineLicenseValidity).UTC(),
		Domains:    domains,
		VolDomains: domains,
	}
	if err := saveLicense(c, license); err != nil {
		return nil, err
	}
	return license, nil
}

// RefreshManual imports a locally issued offline license.
func (s *ManualLicenseRefreshService) RefreshManual(c *gin.Context) (*License, error) {
	var parsed storedLicense
	if err := json.Unmarshal([]byte(strings.TrimSpace(s.License)), &parsed); err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid license format", err)
	}
	if parsed.ExpiredAt.IsZero() {
		return nil, serializer.NewError(serializer.CodeParamErr, "License expiration time is required", nil)
	}

	if len(parsed.Domains) == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "License domains cannot be empty", nil)
	}
	parsed.Domains = normalizeLicenseDomains(parsed.Domains)
	parsed.VolDomains = normalizeLicenseDomains(parsed.VolDomains)
	if len(parsed.Domains) == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "License domains cannot be empty", nil)
	}
	if parsed.ExpiredAt.Before(time.Now()) {
		return nil, serializer.NewError(serializer.CodeParamErr, "License has expired", nil)
	}
	if err := validateLicenseDomains(&License{Domains: parsed.Domains}, c); err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, err.Error(), err)
	}

	license := &License{
		ExpiredAt:  parsed.ExpiredAt.UTC(),
		Domains:    parsed.Domains,
		VolDomains: parsed.VolDomains,
	}
	if err := saveLicense(c, license); err != nil {
		return nil, err
	}
	return license, nil
}

func licenseFromSetting(raw string, defaultDomains []string) *License {
	license := &License{
		ExpiredAt:  time.Now().Add(defaultOfflineLicenseValidity).UTC(),
		Domains:    defaultDomains,
		VolDomains: defaultDomains,
	}
	if strings.TrimSpace(raw) == "" {
		return license
	}

	var stored storedLicense
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return license
	}
	if !stored.ExpiredAt.IsZero() {
		license.ExpiredAt = stored.ExpiredAt.UTC()
	}
	if domains := normalizeLicenseDomains(stored.Domains); len(domains) > 0 {
		license.Domains = domains
	}
	if volDomains := normalizeLicenseDomains(stored.VolDomains); len(volDomains) > 0 {
		license.VolDomains = volDomains
	}
	return license
}

func saveLicense(c *gin.Context, license *License) error {
	dep := dependency.FromContext(c)
	raw, err := json.Marshal(storedLicense{
		ExpiredAt:  license.ExpiredAt,
		Domains:    license.Domains,
		VolDomains: license.VolDomains,
	})
	if err != nil {
		return serializer.NewError(serializer.CodeInternalSetting, "Failed to encode license", err)
	}

	client := dep.SettingClient()
	sc, tx, ctx, err := inventory.WithTx(c, client)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to create transaction", err)
	}
	if err := sc.Set(ctx, map[string]string{licenseSettingKey: string(raw)}); err != nil {
		_ = inventory.Rollback(tx)
		return serializer.NewError(serializer.CodeDBError, "Failed to save license", err)
	}
	if err := inventory.Commit(tx); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to commit license", err)
	}
	if err := dep.KV().Delete(setting.KvSettingPrefix, licenseSettingKey); err != nil {
		return serializer.NewError(serializer.CodeInternalSetting, "Failed to clear license cache", err)
	}
	return nil
}

func licenseDomains(urls []*url.URL) []string {
	domains := make([]string, 0, len(urls))
	for _, u := range urls {
		domains = append(domains, u.Hostname())
	}
	return normalizeLicenseDomains(domains)
}

func normalizeLicenseDomains(domains []string) []string {
	seen := make(map[string]struct{}, len(domains))
	result := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.TrimSpace(strings.ToLower(domain))
		if domain == "" {
			continue
		}
		if parsed, err := url.Parse(domain); err == nil && parsed.Hostname() != "" {
			domain = strings.ToLower(parsed.Hostname())
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	sort.Strings(result)
	return result
}

func validateLicenseDomains(license *License, c *gin.Context) error {
	siteURLs := dependency.FromContext(c).SettingProvider().AllSiteURLs(c)
	return validateLicenseDomainsAgainstURLs(license, siteURLs)
}

func validateLicenseDomainsAgainstURLs(license *License, siteURLs []*url.URL) error {
	allowed := make(map[string]struct{})
	for _, domain := range licenseDomains(siteURLs) {
		allowed[domain] = struct{}{}
	}
	for _, domain := range license.Domains {
		if _, ok := allowed[domain]; !ok {
			return fmt.Errorf("domain %q is not configured as a site URL", domain)
		}
	}
	return nil
}
