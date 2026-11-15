package audit

// EventType enumerates audit log event types. The values are part of the public
// API contract and must match the frontend AuditLogType enum.
type EventType int

const (
	EventServerStart         EventType = 0
	EventUserSignup          EventType = 1
	EventEmailSent           EventType = 2
	EventUserActivated       EventType = 3
	EventUserLoginFailed     EventType = 4
	EventUserLogin           EventType = 5
	EventUserTokenRefresh    EventType = 6
	EventFileCreate          EventType = 7
	EventFileRename          EventType = 8
	EventSetFilePermission   EventType = 9
	EventEntityUploaded      EventType = 10
	EventEntityDownloaded    EventType = 11
	EventCopyFrom            EventType = 12
	EventCopyTo              EventType = 13
	EventMoveTo              EventType = 14
	EventDeleteFile          EventType = 15
	EventMoveToTrash         EventType = 16
	EventShare               EventType = 17
	EventShareLinkViewed     EventType = 18
	EventSetCurrentVersion   EventType = 19
	EventDeleteVersion       EventType = 20
	EventThumbGenerated      EventType = 21
	EventLivePhotoUploaded   EventType = 22
	EventUpdateMetadata      EventType = 23
	EventEditShare           EventType = 24
	EventDeleteShare         EventType = 25
	EventMount               EventType = 26
	EventRelocate            EventType = 27
	EventCreateArchive       EventType = 28
	EventExtractArchive      EventType = 29
	EventWebdavLoginFailed   EventType = 30
	EventWebdavAccountCreate EventType = 31
	EventWebdavAccountUpdate EventType = 32
	EventWebdavAccountDelete EventType = 33
	EventPaymentCreated      EventType = 34
	EventPointsChange        EventType = 35
	EventPaymentPaid         EventType = 36
	EventPaymentFulfilled    EventType = 37
	EventPaymentFulfillFail  EventType = 38
	EventStorageAdded        EventType = 39
	EventGroupChanged        EventType = 40
	EventUserExceedQuota     EventType = 41
	EventUserChanged         EventType = 42
	EventGetDirectLink       EventType = 43
	EventLinkAccount         EventType = 44
	EventUnlinkAccount       EventType = 45
	EventChangeNick          EventType = 46
	EventChangeAvatar        EventType = 47
	EventMembershipUnsub     EventType = 48
	EventChangePassword      EventType = 49
	EventEnable2FA           EventType = 50
	EventDisable2FA          EventType = 51
	EventAddPasskey          EventType = 52
	EventRemovePasskey       EventType = 53
	EventRedeemGiftCode      EventType = 54
	EventFileImported        EventType = 55
	EventUpdateView          EventType = 56
	EventDeleteDirectLink    EventType = 57
	EventReportAbuse         EventType = 58
	EventOAuthGrantCreate    EventType = 59
	EventOAuthTokenExchange  EventType = 60
	EventOAuthGrantRevoke    EventType = 61
)

// Category groups event types for settings toggles and filtering.
type Category int

const (
	CategorySystem Category = iota
	CategoryUser
	CategoryFile
	CategoryShare
	CategoryVersion
	CategoryMedia
	CategoryFilesystem
	CategoryWebdav
	CategoryPayment
	CategoryEmail
)

// categoryOf maps an event type to its category.
func categoryOf(t EventType) Category {
	switch t {
	case EventServerStart:
		return CategorySystem
	case EventUserSignup, EventUserActivated, EventUserLogin, EventUserLoginFailed,
		EventUserTokenRefresh, EventUserChanged, EventUserExceedQuota, EventChangeNick,
		EventChangeAvatar, EventChangePassword, EventEnable2FA, EventDisable2FA,
		EventAddPasskey, EventRemovePasskey, EventLinkAccount, EventUnlinkAccount,
		EventReportAbuse, EventOAuthGrantCreate, EventOAuthTokenExchange, EventOAuthGrantRevoke:
		return CategoryUser
	case EventFileCreate, EventFileImported, EventFileRename, EventSetFilePermission,
		EventEntityUploaded, EventEntityDownloaded, EventCopyFrom, EventCopyTo, EventMoveTo,
		EventDeleteFile, EventMoveToTrash, EventUpdateMetadata, EventGetDirectLink,
		EventDeleteDirectLink, EventUpdateView:
		return CategoryFile
	case EventShare, EventShareLinkViewed, EventEditShare, EventDeleteShare:
		return CategoryShare
	case EventSetCurrentVersion, EventDeleteVersion:
		return CategoryVersion
	case EventThumbGenerated, EventLivePhotoUploaded:
		return CategoryMedia
	case EventMount, EventRelocate, EventCreateArchive, EventExtractArchive:
		return CategoryFilesystem
	case EventWebdavLoginFailed, EventWebdavAccountCreate, EventWebdavAccountUpdate, EventWebdavAccountDelete:
		return CategoryWebdav
	case EventPaymentCreated, EventPointsChange, EventPaymentPaid, EventPaymentFulfilled,
		EventPaymentFulfillFail, EventStorageAdded, EventGroupChanged, EventMembershipUnsub,
		EventRedeemGiftCode:
		return CategoryPayment
	case EventEmailSent:
		return CategoryEmail
	default:
		return CategorySystem
	}
}

// LogEntry is the structured payload stored in the content column.
type LogEntry struct {
	Category          int               `json:"category"`
	Failed            bool              `json:"failed,omitempty"`
	Error             string            `json:"error,omitempty"`
	UserAgent         string            `json:"user_agent,omitempty"`
	IsSystem          bool              `json:"is_system,omitempty"`
	Reason            string            `json:"reason,omitempty"`
	EmailTo           string            `json:"email_to,omitempty"`
	EmailTitle        string            `json:"email_title,omitempty"`
	OriginalName      string            `json:"original_name,omitempty"`
	NewName           string            `json:"new_name,omitempty"`
	From              string            `json:"from,omitempty"`
	To                string            `json:"to,omitempty"`
	EntityCreateTime  *string           `json:"entity_create_time,omitempty"`
	StoragePolicyID   string            `json:"storage_policy_id,omitempty"`
	StoragePolicyName string            `json:"storage_policy_name,omitempty"`
	AccountID         int               `json:"account_id,omitempty"`
	Account           string            `json:"account,omitempty"`
	AccountURI        string            `json:"account_uri,omitempty"`
	PaymentID         int               `json:"payment_id,omitempty"`
	PointsChange      int               `json:"points_change,omitempty"`
	SKU               string            `json:"sku,omitempty"`
	StorageSize       int64             `json:"storage_size,omitempty"`
	Expire            string            `json:"expire,omitempty"`
	GroupID           int               `json:"group_id,omitempty"`
	GroupIDFrom       int               `json:"group_id_from,omitempty"`
	DirectLinkID      string            `json:"direct_link_id,omitempty"`
	OpenIDProvider    int               `json:"openid_provider,omitempty"`
	Sub               string            `json:"sub,omitempty"`
	Name              string            `json:"name,omitempty"`
	PasskeyID         int               `json:"passkey_id,omitempty"`
	LoginWith         string            `json:"login_with,omitempty"`
	Passkey           string            `json:"passkey,omitempty"`
	Exts              map[string]string `json:"exts,omitempty"`
}
