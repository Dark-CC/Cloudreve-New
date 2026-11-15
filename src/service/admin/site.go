package admin

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/constants"
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/pkg/thumb"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func init() {
	gob.Register(map[string]interface{}{})
	gob.Register(map[string]string{})
}

// NoParamService 无需参数的服务
type NoParamService struct {
}

// BatchSettingChangeService 设定批量更改服务
type BatchSettingChangeService struct {
	Options []SettingChangeService `json:"options"`
}

// SettingChangeService  设定更改服务
type SettingChangeService struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}

// Change 批量更改站点设定
func (service *BatchSettingChangeService) Change() serializer.Response {
	//cacheClean := make([]string, 0, len(service.Options))
	//tx := model.DB.Begin()
	//
	//for _, setting := range service.Options {
	//
	//	if err := tx.Model(&model.Setting{}).Where("name = ?", setting.Key).Update("value", setting.Value).Error; err != nil {
	//		cache.Deletes(cacheClean, "setting_")
	//		tx.Rollback()
	//		return serializer.ErrDeprecated(serializer.CodeUpdateSetting, "Setting "+setting.Key+" failed to update", err)
	//	}
	//
	//	cacheClean = append(cacheClean, setting.Key)
	//}
	//
	//if err := tx.Commit().Error; err != nil {
	//	return serializer.DBErrDeprecated("Failed to update setting", err)
	//}
	//
	//cache.Deletes(cacheClean, "setting_")

	return serializer.Response{}
}

const (
	SummaryRangeDays = 12
	MetricCacheKey   = "admin_summary"
	metricErrMsg     = "Failed to generate metrics summary"
)

type (
	SummaryService struct {
		Generate bool `form:"generate"`
	}
	SummaryParamCtx struct{}
)

// Summary 获取站点统计概况
func (s *SummaryService) Summary(c *gin.Context) (*HomepageSummary, error) {
	dep := dependency.FromContext(c)
	kv := dep.KV()
	siteURLs := dep.SettingProvider().AllSiteURLs(c)
	license := licenseFromSetting(dep.SettingProvider().License(c), licenseDomains(siteURLs))
	if strings.TrimSpace(dep.SettingProvider().License(c)) == "" {
		if err := saveLicense(c, license); err != nil {
			return nil, err
		}
	}
	res := &HomepageSummary{
		Version: &Version{
			Version: constants.BackendVersion,
			Pro:     constants.IsProBool,
			Commit:  constants.LastCommit,
		},
		SiteURls: lo.Map(siteURLs, func(item *url.URL, index int) string {
			return item.String()
		}),
		License: license,
	}

	if summary, ok := kv.Get(MetricCacheKey); ok {
		summaryCasted := summary.(MetricsSummary)
		res.MetricsSummary = &summaryCasted
		return res, nil
	}

	if !s.Generate {
		return res, nil
	}

	summary := &MetricsSummary{
		Files:       make([]int, SummaryRangeDays),
		Users:       make([]int, SummaryRangeDays),
		Shares:      make([]int, SummaryRangeDays),
		Dates:       make([]time.Time, SummaryRangeDays),
		GeneratedAt: time.Now(),
	}

	fileClient := dep.FileClient()
	userClient := dep.UserClient()
	shareClient := dep.ShareClient()

	toRound := time.Now()
	timeBase := time.Date(toRound.Year(), toRound.Month(), toRound.Day()+1, 0, 0, 0, 0, toRound.Location())
	for day := range summary.Files {
		start := timeBase.Add(-time.Duration(SummaryRangeDays-day) * time.Hour * 24)
		end := timeBase.Add(-time.Duration(SummaryRangeDays-day-1) * time.Hour * 24)
		summary.Dates[day] = start
		fileTotal, err := fileClient.CountByTimeRange(c, &start, &end)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, metricErrMsg, nil)
		}
		userTotal, err := userClient.CountByTimeRange(c, &start, &end)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, metricErrMsg, nil)
		}
		shareTotal, err := shareClient.CountByTimeRange(c, &start, &end)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, metricErrMsg, nil)
		}
		summary.Files[day] = fileTotal
		summary.Users[day] = userTotal
		summary.Shares[day] = shareTotal
	}

	var err error
	summary.FileTotal, err = fileClient.CountByTimeRange(c, nil, nil)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, metricErrMsg, nil)
	}
	summary.UserTotal, err = userClient.CountByTimeRange(c, nil, nil)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, metricErrMsg, nil)
	}
	summary.ShareTotal, err = shareClient.CountByTimeRange(c, nil, nil)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, metricErrMsg, nil)
	}
	summary.EntitiesTotal, err = fileClient.CountEntityByTimeRange(c, nil, nil)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, metricErrMsg, nil)
	}

	_ = kv.Set(MetricCacheKey, *summary, 86400)
	res.MetricsSummary = summary

	return res, nil
}

// ThumbGeneratorTestService 缩略图生成测试服务
type (
	ThumbGeneratorTestService struct {
		Name       string `json:"name" binding:"required"`
		Executable string `json:"executable" binding:"required"`
	}
	ThumbGeneratorTestParamCtx struct{}
)

// Test 通过获取生成器版本来测试
func (s *ThumbGeneratorTestService) Test(c *gin.Context) (string, error) {
	version, err := thumb.TestGenerator(c, s.Name, s.Executable)
	if err != nil {
		return "", serializer.NewError(serializer.CodeParamErr, "Failed to invoke generator: "+err.Error(), err)
	}

	return version, nil
}

type (
	GetSettingService struct {
		Keys []string `json:"keys" binding:"required"`
	}
	GetSettingParamCtx struct{}
)

func (s *GetSettingService) GetSetting(c *gin.Context) (map[string]string, error) {
	dep := dependency.FromContext(c)
	res, err := dep.SettingClient().Gets(c, lo.Filter(s.Keys, func(item string, index int) bool {
		_, ok := inventory.RedactedSettings[strings.ToLower(item)]
		return !ok
	}))
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get settings", err)
	}
	if value, ok := res["external_login_providers"]; ok {
		res["external_login_providers"] = redactExternalLoginProviders(value)
	}

	return res, nil
}

type (
	SetSettingService struct {
		Settings map[string]string `json:"settings" binding:"required"`
	}
	SetSettingParamCtx   struct{}
	SettingPreProcessor  func(ctx context.Context, settings map[string]string) error
	SettingPostProcessor func(ctx context.Context, settings map[string]string) error
)

var (
	preprocessors = map[string]SettingPreProcessor{
		"siteURL":                  siteUrlPreProcessor,
		"mime_mapping":             mimeMappingPreProcessor,
		"secret_key":               secretKeyPreProcessor,
		"external_login_providers": externalLoginProvidersPreProcessor,
	}
	postprocessors = map[string]SettingPostProcessor{
		"mime_mapping":                               mimeMappingPostProcessor,
		"media_meta_exif":                            mediaMetaPostProcessor,
		"media_meta_music":                           mediaMetaPostProcessor,
		"media_meta_ffprobe":                         mediaMetaPostProcessor,
		"smtpUser":                                   emailPostProcessor,
		"smtpPass":                                   emailPostProcessor,
		"smtpHost":                                   emailPostProcessor,
		"smtpPort":                                   emailPostProcessor,
		"smtpEncryption":                             emailPostProcessor,
		"smtpFrom":                                   emailPostProcessor,
		"replyTo":                                    emailPostProcessor,
		"fromName":                                   emailPostProcessor,
		"fromAdress":                                 emailPostProcessor,
		"queue_media_meta_worker_num":                mediaMetaQueuePostProcessor,
		"queue_media_meta_max_execution":             mediaMetaQueuePostProcessor,
		"queue_media_meta_backoff_factor":            mediaMetaQueuePostProcessor,
		"queue_media_meta_backoff_max_duration":      mediaMetaQueuePostProcessor,
		"queue_media_meta_max_retry":                 mediaMetaQueuePostProcessor,
		"queue_media_meta_retry_delay":               mediaMetaQueuePostProcessor,
		"queue_thumb_worker_num":                     thumbQueuePostProcessor,
		"queue_thumb_max_execution":                  thumbQueuePostProcessor,
		"queue_thumb_backoff_factor":                 thumbQueuePostProcessor,
		"queue_thumb_backoff_max_duration":           thumbQueuePostProcessor,
		"queue_thumb_max_retry":                      thumbQueuePostProcessor,
		"queue_thumb_retry_delay":                    thumbQueuePostProcessor,
		"queue_recycle_worker_num":                   entityRecycleQueuePostProcessor,
		"queue_recycle_max_execution":                entityRecycleQueuePostProcessor,
		"queue_recycle_backoff_factor":               entityRecycleQueuePostProcessor,
		"queue_recycle_backoff_max_duration":         entityRecycleQueuePostProcessor,
		"queue_recycle_max_retry":                    entityRecycleQueuePostProcessor,
		"queue_recycle_retry_delay":                  entityRecycleQueuePostProcessor,
		"queue_io_intense_worker_num":                ioIntenseQueuePostProcessor,
		"queue_io_intense_max_execution":             ioIntenseQueuePostProcessor,
		"queue_io_intense_backoff_factor":            ioIntenseQueuePostProcessor,
		"queue_io_intense_backoff_max_duration":      ioIntenseQueuePostProcessor,
		"queue_io_intense_max_retry":                 ioIntenseQueuePostProcessor,
		"queue_io_intense_retry_delay":               ioIntenseQueuePostProcessor,
		"queue_remote_download_worker_num":           remoteDownloadQueuePostProcessor,
		"queue_remote_download_max_execution":        remoteDownloadQueuePostProcessor,
		"queue_remote_download_backoff_factor":       remoteDownloadQueuePostProcessor,
		"queue_remote_download_backoff_max_duration": remoteDownloadQueuePostProcessor,
		"queue_remote_download_max_retry":            remoteDownloadQueuePostProcessor,
		"queue_remote_download_retry_delay":          remoteDownloadQueuePostProcessor,
		"secret_key":                                 secretKeyPostProcessor,
		"fts_meilisearch_embed_config":               meilisearchPostProcessor,
		"fts_meilisearch_endpoint":                   meilisearchPostProcessor,
		"fts_meilisearch_api_key":                    meilisearchPostProcessor,
		"fts_meilisearch_embed_enabled":              meilisearchPostProcessor,
		"fts_meilisearch_page_size":                  meilisearchPostProcessor,
		"fts_tika_endpoint":                          tikaPostProcessor,
		"fts_tika_exts":                              tikaPostProcessor,
		"fts_tika_max_file_size":                     tikaPostProcessor,
	}
)

func (s *SetSettingService) SetSetting(c *gin.Context) (map[string]string, error) {
	dep := dependency.FromContext(c)
	kv := dep.KV()
	settingClient := dep.SettingClient()

	// Preprocess settings
	allPreprocessors := make(map[string]SettingPreProcessor)
	allPostprocessors := make(map[string]SettingPostProcessor)
	for k, _ := range s.Settings {
		if preprocessor, ok := preprocessors[k]; ok {
			fnName := reflect.TypeOf(preprocessor).Name()
			if _, ok := allPreprocessors[fnName]; !ok {
				allPreprocessors[fnName] = preprocessor
			}
		}

		if postprocessor, ok := postprocessors[k]; ok {
			fnName := reflect.TypeOf(postprocessor).Name()
			if _, ok := allPostprocessors[fnName]; !ok {
				allPostprocessors[fnName] = postprocessor
			}
		}
	}

	// Execute all preprocessors
	for _, preprocessor := range allPreprocessors {
		if err := preprocessor(c, s.Settings); err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Failed to validate settings", err)
		}
	}

	// Save to db
	sc, tx, ctx, err := inventory.WithTx(c, settingClient)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create transaction", err)
	}

	if err := sc.Set(ctx, s.Settings); err != nil {
		_ = inventory.Rollback(tx)
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to save settings", err)
	}

	if err := inventory.Commit(tx); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to commit transaction", err)
	}

	// Clean cache
	if err := kv.Delete(setting.KvSettingPrefix, lo.Keys(s.Settings)...); err != nil {
		return nil, serializer.NewError(serializer.CodeInternalSetting, "Failed to clear cache", err)
	}

	// Execute post preprocessors
	for _, postprocessor := range allPostprocessors {
		if err := postprocessor(ctx, s.Settings); err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Failed to post process settings", err)
		}
	}

	return s.Settings, nil
}

func siteUrlPreProcessor(ctx context.Context, settings map[string]string) error {
	siteURL := settings["siteURL"]
	urls := strings.Split(siteURL, ",")
	for index, u := range urls {
		urlParsed, err := url.Parse(u)
		if err != nil {
			return fmt.Errorf("Failed to parse siteURL %q: %w", u, err)
		}

		urls[index] = urlParsed.String()
	}
	settings["siteURL"] = strings.Join(urls, ",")
	return nil
}

func secretKeyPreProcessor(ctx context.Context, settings map[string]string) error {
	settings["secret_key"] = util.RandStringRunesCrypto(256)
	return nil
}

func mimeMappingPreProcessor(ctx context.Context, settings map[string]string) error {
	var mapping map[string]string
	if err := json.Unmarshal([]byte(settings["mime_mapping"]), &mapping); err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid mime mapping", err)
	}

	return nil
}

func externalLoginProvidersPreProcessor(ctx context.Context, settings map[string]string) error {
	encoded, ok := settings["external_login_providers"]
	if !ok {
		return nil
	}

	incoming, err := decodeExternalLoginProviders(encoded)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid external login providers", err)
	}

	dep := dependency.FromContext(ctx)
	currentRaw, err := dep.SettingClient().Get(ctx, "external_login_providers")
	if err != nil {
		currentRaw = "{}"
	}
	current, err := decodeExternalLoginProviders(currentRaw)
	if err != nil {
		current = nil
	}

	for index := range incoming {
		incoming[index].Config = mergeProviderSecret(incoming[index], current)
	}

	normalized, err := mergeExternalLoginProviders(encoded, incoming)
	if err != nil {
		return fmt.Errorf("failed to encode external login providers: %w", err)
	}
	settings["external_login_providers"] = normalized
	return nil
}

const redactedSecret = "***"

type externalLoginProvider struct {
	ID        int                        `json:"id,omitempty"`
	Type      string                     `json:"type"`
	Enabled   bool                       `json:"enabled,omitempty"`
	Config    map[string]json.RawMessage `json:"config,omitempty"`
	WellKnown map[string]json.RawMessage `json:"wellknown,omitempty"`
	extra     map[string]json.RawMessage
	hasID     bool
}

func (p *externalLoginProvider) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.ID = 0
	p.Type = ""
	p.Enabled = false
	p.Config = nil
	p.WellKnown = nil
	p.hasID = false

	if value, ok := raw["id"]; ok {
		if err := json.Unmarshal(value, &p.ID); err != nil {
			return err
		}
		p.hasID = true
	}
	if value, ok := raw["type"]; ok {
		if err := json.Unmarshal(value, &p.Type); err != nil {
			return err
		}
	}
	if value, ok := raw["enabled"]; ok {
		if err := json.Unmarshal(value, &p.Enabled); err != nil {
			return err
		}
	}
	if value, ok := raw["config"]; ok {
		if err := json.Unmarshal(value, &p.Config); err != nil {
			return err
		}
	}
	if value, ok := raw["wellknown"]; ok {
		if err := json.Unmarshal(value, &p.WellKnown); err != nil {
			return err
		}
	}

	for _, key := range []string{"id", "type", "enabled", "config", "wellknown"} {
		delete(raw, key)
	}
	p.extra = raw

	return nil
}

func (p externalLoginProvider) MarshalJSON() ([]byte, error) {
	raw := make(map[string]json.RawMessage, len(p.extra)+5)
	for key, value := range p.extra {
		raw[key] = value
	}
	raw["id"] = mustMarshalJSON(p.ID)
	raw["type"] = mustMarshalJSON(p.Type)
	raw["enabled"] = mustMarshalJSON(p.Enabled)
	if p.Config != nil {
		raw["config"] = mustMarshalJSON(p.Config)
	}
	if p.WellKnown != nil {
		raw["wellknown"] = mustMarshalJSON(p.WellKnown)
	}
	return json.Marshal(raw)
}

func mustMarshalJSON(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func decodeExternalLoginProviders(raw string) ([]externalLoginProvider, error) {
	if strings.TrimSpace(raw) == "" {
		return []externalLoginProvider{}, nil
	}

	var payload struct {
		Providers []externalLoginProvider `json:"providers"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	if payload.Providers == nil {
		payload.Providers = []externalLoginProvider{}
	}
	return payload.Providers, nil
}

func mergeExternalLoginProviders(raw string, providers []externalLoginProvider) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload["providers"] = providers

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func redactExternalLoginProviders(raw string) string {
	providers, err := decodeExternalLoginProviders(raw)
	if err != nil {
		return raw
	}

	for index := range providers {
		if providers[index].Config == nil {
			continue
		}
		if _, ok := providers[index].Config["client_secret"]; ok {
			providers[index].Config["client_secret"] = json.RawMessage(strconv.Quote(redactedSecret))
		}
		if _, ok := providers[index].Config["clientSecret"]; ok {
			providers[index].Config["clientSecret"] = json.RawMessage(strconv.Quote(redactedSecret))
		}
		if _, ok := providers[index].Config["app_secret"]; ok {
			providers[index].Config["app_secret"] = json.RawMessage(strconv.Quote(redactedSecret))
		}
	}

	encoded, err := mergeExternalLoginProviders(raw, providers)
	if err != nil {
		return raw
	}
	return encoded
}

func RedactExternalLoginProviders(raw string) string {
	return redactExternalLoginProviders(raw)
}

func mergeProviderSecret(incoming externalLoginProvider, current []externalLoginProvider) map[string]json.RawMessage {
	config := incoming.Config
	if config == nil {
		config = map[string]json.RawMessage{}
	}

	for _, key := range []string{"client_secret", "clientSecret", "app_secret"} {
		value, ok := config[key]
		if !ok {
			continue
		}
		var secret string
		if err := json.Unmarshal(value, &secret); err != nil || secret != redactedSecret {
			continue
		}

		restored := false
		if previous := findProviderForSecret(current, incoming); previous != nil {
			for _, oldKey := range []string{key, "client_secret", "clientSecret", "app_secret"} {
				if old, exists := previous.Config[oldKey]; exists {
					config[key] = old
					restored = true
					break
				}
			}
		}
		if !restored {
			delete(config, key)
		}
	}

	return config
}

func findProviderForSecret(providers []externalLoginProvider, incoming externalLoginProvider) *externalLoginProvider {
	// Logto legitimately uses provider id 0, so presence of the id field is
	// what distinguishes it from a legacy payload that predates ids.
	if incoming.hasID {
		for index := range providers {
			if providers[index].hasID && providers[index].ID == incoming.ID {
				return &providers[index]
			}
		}
		return nil
	}
	for index := range providers {
		if providers[index].Type == incoming.Type {
			return &providers[index]
		}
	}
	return nil
}

func mimeMappingPostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.MimeDetector(context.WithValue(ctx, dependency.ReloadCtx{}, true))

	return nil
}

func mediaMetaPostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.MediaMetaExtractor(context.WithValue(ctx, dependency.ReloadCtx{}, true))
	return nil
}

func emailPostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.EmailClient(context.WithValue(ctx, dependency.ReloadCtx{}, true))
	return nil
}

func mediaMetaQueuePostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.MediaMetaQueue(context.WithValue(ctx, dependency.ReloadCtx{}, true)).Start()
	return nil
}

func ioIntenseQueuePostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.IoIntenseQueue(context.WithValue(ctx, dependency.ReloadCtx{}, true)).Start()
	return nil
}

func remoteDownloadQueuePostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.RemoteDownloadQueue(context.WithValue(ctx, dependency.ReloadCtx{}, true)).Start()
	return nil
}

func entityRecycleQueuePostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.EntityRecycleQueue(context.WithValue(ctx, dependency.ReloadCtx{}, true)).Start()
	return nil
}

func thumbQueuePostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.ThumbQueue(context.WithValue(ctx, dependency.ReloadCtx{}, true)).Start()
	return nil
}

func secretKeyPostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.KV().Delete(manager.EntityUrlCacheKeyPrefix)
	settings["secret_key"] = ""
	return nil
}

func meilisearchPostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.SearchIndexer(context.WithValue(ctx, dependency.ReloadCtx{}, true))
	return nil
}

func tikaPostProcessor(ctx context.Context, settings map[string]string) error {
	dep := dependency.FromContext(ctx)
	dep.TextExtractor(context.WithValue(ctx, dependency.ReloadCtx{}, true))
	return nil
}
