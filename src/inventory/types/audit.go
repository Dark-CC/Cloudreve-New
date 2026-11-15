package types

import (
	"encoding/json"
)

// AuditLogEntry is the structured payload stored in the audit_logs content column.
type AuditLogEntry struct {
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

// UnmarshalJSON tolerates legacy Pro payloads where exts values are not
// strings (numbers, booleans, arrays or objects). Those values are kept as
// their JSON representation so existing records stay readable.
func (e *AuditLogEntry) UnmarshalJSON(data []byte) error {
	type auditLogEntry AuditLogEntry
	aux := struct {
		*auditLogEntry
		Exts map[string]json.RawMessage `json:"exts,omitempty"`
	}{
		auditLogEntry: (*auditLogEntry)(e),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.Exts) == 0 {
		e.Exts = nil
		return nil
	}

	e.Exts = make(map[string]string, len(aux.Exts))
	for key, raw := range aux.Exts {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			e.Exts[key] = str
			continue
		}

		if len(raw) == 0 || string(raw) == "null" {
			continue
		}

		e.Exts[key] = string(raw)
	}

	return nil
}
