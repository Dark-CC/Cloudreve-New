package vas

import (
	"context"
	"strconv"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/pkg/crontab"
	"github.com/cloudreve/Cloudreve/v4/pkg/email"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
)

const quotaScanPageSize = 200
const quotaGraceKeyPrefix = "quota_exceed_grace_"
const quotaNotifiedKeyPrefix = "quota_exceed_notified_"

func init() {
	crontab.Register(setting.CronTypeNotifyUser, NotifyQuotaExceeded)
}

// NotifyQuotaExceeded emails active users whose storage usage is over quota and
// suspends users that remain over quota longer than the configured grace time.
func NotifyQuotaExceeded(ctx context.Context) {
	dep := dependency.FromContext(ctx)
	settings := dep.SettingProvider()
	grace := time.Duration(settings.BanTime(ctx)) * time.Second
	if grace <= 0 {
		grace = 7 * 24 * time.Hour
	}

	lastID := 0
	for {
		users, err := dep.UserClient().ListActiveWithGroup(ctx, quotaScanPageSize, lastID)
		if err != nil {
			dep.Logger().Warning("Failed to scan users for quota notifications: %s", err)
			return
		}
		for _, userModel := range users {
			lastID = userModel.ID
			processQuotaNotification(ctx, dep, userModel, grace)
		}
		if len(users) < quotaScanPageSize {
			return
		}
	}
}

func processQuotaNotification(ctx context.Context, dep dependency.Dep, userModel *ent.User, grace time.Duration) {
	group, err := userModel.Edges.GroupOrErr()
	if err != nil || group.MaxStorage <= 0 {
		return
	}

	packSize, err := dep.VASClient().TotalStoragePackSize(ctx, userModel.ID, time.Now())
	if err != nil {
		dep.Logger().Warning("Failed to load storage packs for user %d: %s", userModel.ID, err)
		return
	}
	if userModel.Storage <= group.MaxStorage+packSize {
		_ = dep.KV().Delete("", quotaGraceKeyPrefix+strconv.Itoa(userModel.ID))
		_ = dep.KV().Delete("", quotaNotifiedKeyPrefix+strconv.Itoa(userModel.ID))
		return
	}

	key := quotaGraceKeyPrefix + strconv.Itoa(userModel.ID)
	raw, ok := dep.KV().Get(key)
	firstSeen := time.Now()
	if ok {
		if stored, ok := raw.(time.Time); ok {
			firstSeen = stored
		}
	} else {
		if err := dep.KV().Set(key, firstSeen, 0); err != nil {
			dep.Logger().Warning("Failed to persist quota grace period for user %d: %s", userModel.ID, err)
		}
		return
	}

	overdue := time.Since(firstSeen)
	if overdue < grace/2 {
		return
	}

	if overdue < grace {
		notifyQuotaExceeded(ctx, dep, userModel, grace)
		return
	}

	notifyQuotaExceeded(ctx, dep, userModel, grace)
	if _, err := dep.UserClient().SetStatus(ctx, userModel, user.StatusSysBanned); err != nil {
		dep.Logger().Warning("Failed to suspend over-quota user %d: %s", userModel.ID, err)
	}
}

func notifyQuotaExceeded(ctx context.Context, dep dependency.Dep, userModel *ent.User, grace time.Duration) {
	notifiedKey := quotaNotifiedKeyPrefix + strconv.Itoa(userModel.ID)
	if _, notified := dep.KV().Get(notifiedKey); notified {
		return
	}

	sendQuotaEmail(ctx, dep, userModel, grace)
	audit.Record(ctx, audit.EventUserExceedQuota, &audit.LogEntry{
		StorageSize: userModel.Storage,
		IsSystem:    true,
	}, audit.WithUser(userModel.ID))
	_ = dep.KV().Set(notifiedKey, true, 0)
}

func sendQuotaEmail(ctx context.Context, dep dependency.Dep, userModel *ent.User, grace time.Duration) {
	title, body, err := email.NewQuotaExceedEmail(ctx, dep.SettingProvider(), userModel, int(grace.Hours()/24))
	if err == nil {
		if sendErr := dep.EmailClient(ctx).Send(ctx, userModel.Email, title, body); sendErr != nil {
			audit.Record(ctx, audit.EventEmailSent, &audit.LogEntry{
				Failed:     true,
				Error:      sendErr.Error(),
				EmailTo:    userModel.Email,
				EmailTitle: title,
				IsSystem:   true,
			}, audit.WithUser(userModel.ID))
			dep.Logger().Warning("Failed to send quota email to user %d: %s", userModel.ID, sendErr)
		} else {
			audit.Record(ctx, audit.EventEmailSent, &audit.LogEntry{
				EmailTo:    userModel.Email,
				EmailTitle: title,
				IsSystem:   true,
			}, audit.WithUser(userModel.ID))
		}
	}
}
