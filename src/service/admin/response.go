package admin

import (
	"encoding/gob"
	"encoding/json"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/queue"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
)

type ListShareResponse struct {
	Pagination *inventory.PaginationResults `json:"pagination"`
	Shares     []GetShareResponse           `json:"shares"`
}

type GetShareResponse struct {
	*ent.Share
	UserHashID string `json:"user_hash_id,omitempty"`
	ShareLink  string `json:"share_link,omitempty"`
}

type ListTaskResponse struct {
	Pagination *inventory.PaginationResults `json:"pagination"`
	Tasks      []GetTaskResponse            `json:"tasks"`
}

type GetTaskResponse struct {
	*ent.Task
	UserHashID string         `json:"user_hash_id,omitempty"`
	TaskHashID string         `json:"task_hash_id,omitempty"`
	Summary    *queue.Summary `json:"summary,omitempty"`
	Node       *ent.Node      `json:"node,omitempty"`
}

type ListEntityResponse struct {
	Pagination *inventory.PaginationResults `json:"pagination"`
	Entities   []GetEntityResponse          `json:"entities"`
}

type GetEntityResponse struct {
	*ent.Entity
	UserHashID    string         `json:"user_hash_id,omitempty"`
	UserHashIDMap map[int]string `json:"user_hash_id_map,omitempty"`
}

type ListFileResponse struct {
	Pagination *inventory.PaginationResults `json:"pagination"`
	Files      []GetFileResponse            `json:"files"`
}

type GetFileResponse struct {
	*ent.File
	UserHashID    string         `json:"user_hash_id,omitempty"`
	FileHashID    string         `json:"file_hash_id,omitempty"`
	DirectLinkMap map[int]string `json:"direct_link_map,omitempty"`
}

type ListUserResponse struct {
	Pagination *inventory.PaginationResults `json:"pagination"`
	Users      []GetUserResponse            `json:"users"`
}

type GetUserResponse struct {
	*ent.User
	HashID        string          `json:"hash_id,omitempty"`
	TwoFAEnabled  bool            `json:"two_fa_enabled,omitempty"`
	Capacity      *fs.Capacity    `json:"capacity,omitempty"`
	Credit        int64           `json:"credit"`
	GroupExpires  *time.Time      `json:"group_expires,omitempty"`
	PreviousGroup *int            `json:"previous_group,omitempty"`
	Edges         *AdminUserEdges `json:"edges,omitempty"`
}

// MarshalJSON merges the ent user edges with the Pro-only identity edges.
// The outer Edges field shadows ent.User.Edges during normal JSON encoding,
// which would otherwise drop edges.group from admin user responses.
func (r GetUserResponse) MarshalJSON() ([]byte, error) {
	if r.User == nil {
		return []byte("null"), nil
	}

	base, err := json.Marshal(struct {
		*ent.User
		HashID        string       `json:"hash_id,omitempty"`
		TwoFAEnabled  bool         `json:"two_fa_enabled,omitempty"`
		Capacity      *fs.Capacity `json:"capacity,omitempty"`
		Credit        int64        `json:"credit"`
		GroupExpires  *time.Time   `json:"group_expires,omitempty"`
		PreviousGroup *int         `json:"previous_group,omitempty"`
	}{
		User:          r.User,
		HashID:        r.HashID,
		TwoFAEnabled:  r.TwoFAEnabled,
		Capacity:      r.Capacity,
		Credit:        r.Credit,
		GroupExpires:  r.GroupExpires,
		PreviousGroup: r.PreviousGroup,
	})
	if err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal(base, &payload); err != nil {
		return nil, err
	}

	edges, _ := payload["edges"].(map[string]any)
	if edges == nil {
		edges = make(map[string]any)
	}
	if r.Edges != nil && len(r.Edges.OpenID) > 0 {
		edges["openid"] = r.Edges.OpenID
	}
	if len(edges) > 0 {
		payload["edges"] = edges
	}

	return json.Marshal(payload)
}

// AdminUserEdges carries Pro-only relations that are not part of the ent user
// model in the community build.
type AdminUserEdges struct {
	OpenID []OpenIDLink `json:"openid,omitempty"`
}

type OpenIDLink struct {
	ID       int       `json:"id,omitempty"`
	Provider int       `json:"provider"`
	Sub      string    `json:"sub,omitempty"`
	Email    string    `json:"email,omitempty"`
	Name     string    `json:"name,omitempty"`
	Avatar   string    `json:"avatar,omitempty"`
	LinkedAt time.Time `json:"linked_at"`
}

type GetNodeResponse struct {
	*ent.Node
}

type GetGroupResponse struct {
	*ent.Group
	TotalUsers       int                  `json:"total_users"`
	StoragePolicyIDs []int                `json:"storage_policy_ids,omitempty"`
	StoragePolicies  []*GroupStoragePolicy `json:"-"`
}

// GroupStoragePolicy is the redacted storage policy shape embedded in group
// responses. The admin group page only needs identification and display
// fields; returning ent.StoragePolicy directly leaked S3 credentials to any
// admin-role user who can read a group.
type GroupStoragePolicy struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type,omitempty"`
	MaxSize   int64     `json:"max_size,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// MarshalJSON keeps the Pro-compatible edges.storage_policies array while
// retaining the legacy storage_policy_ids field used by the community frontend.
// The ent Group edge is a single relation, so its default JSON shape cannot be
// used directly when a group has multiple policies selected.
func (r GetGroupResponse) MarshalJSON() ([]byte, error) {
	if r.Group == nil {
		return []byte("null"), nil
	}

	base, err := json.Marshal(struct {
		*ent.Group
		TotalUsers       int   `json:"total_users"`
		StoragePolicyIDs []int `json:"storage_policy_ids,omitempty"`
	}{
		Group:            r.Group,
		TotalUsers:       r.TotalUsers,
		StoragePolicyIDs: r.StoragePolicyIDs,
	})
	if err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal(base, &payload); err != nil {
		return nil, err
	}

	edges, _ := payload["edges"].(map[string]any)
	if edges == nil {
		edges = make(map[string]any)
	}
	policies := r.StoragePolicies
	if policies == nil {
		policies = []*GroupStoragePolicy{}
	}
	edges["storage_policies"] = policies
	payload["edges"] = edges
	return json.Marshal(payload)
}

type OauthCredentialStatus struct {
	Valid           bool       `json:"valid"`
	LastRefreshTime *time.Time `json:"last_refresh_time"`
}

type GetStoragePolicyResponse struct {
	*ent.StoragePolicy
	EntitiesCount int `json:"entities_count,omitempty"`
	EntitiesSize  int `json:"entities_size,omitempty"`
}

type ListNodeResponse struct {
	Pagination *inventory.PaginationResults `json:"pagination"`
	Nodes      []*ent.Node                  `json:"nodes"`
}

type ListPolicyResponse struct {
	Pagination *inventory.PaginationResults `json:"pagination"`
	Policies   []*ent.StoragePolicy         `json:"policies"`
}

type ListAbuseReportResponse struct {
	Pagination *inventory.PaginationResults `json:"pagination"`
	Reports    []GetAbuseReportResponse     `json:"reports"`
}

type GetAbuseReportResponse struct {
	*ent.AbuseReport
	UserHashID         string `json:"user_hash_id,omitempty"`
	ReporterHashID     string `json:"reporter_hash_id,omitempty"`
	ReportedUserHashID string `json:"reported_user_hash_id,omitempty"`
	ShareHashID        string `json:"share_hash_id,omitempty"`
}

type ListAuditLogResponse struct {
	Pagination *inventory.PaginationResults `json:"pagination"`
	Logs       []GetAuditLogResponse        `json:"logs"`
}

type GetAuditLogResponse struct {
	*ent.AuditLog
	UserHashID string `json:"user_hash_id,omitempty"`
}

// MarshalJSON always emits the audit log type. The ent model declares
// `type,omitempty`, so type 0 (server_start) would disappear from the response
// and the admin UI would render it as an unknown event.
func (r GetAuditLogResponse) MarshalJSON() ([]byte, error) {
	if r.AuditLog == nil {
		return []byte("null"), nil
	}

	type auditLogAlias ent.AuditLog
	return json.Marshal(struct {
		*auditLogAlias
		Type       int    `json:"type"`
		UserHashID string `json:"user_hash_id,omitempty"`
	}{
		auditLogAlias: (*auditLogAlias)(r.AuditLog),
		Type:          r.AuditLog.Type,
		UserHashID:    r.UserHashID,
	})
}

type QueueMetric struct {
	Name            setting.QueueType `json:"name"`
	BusyWorkers     int               `json:"busy_workers"`
	SuccessTasks    int               `json:"success_tasks"`
	FailureTasks    int               `json:"failure_tasks"`
	SubmittedTasks  int               `json:"submitted_tasks"`
	SuspendingTasks int               `json:"suspending_tasks"`
}

type ListGroupResponse struct {
	Groups     []GetGroupResponse           `json:"groups"`
	Pagination *inventory.PaginationResults `json:"pagination"`
}

type ListOAuthClientResponse struct {
	Clients    []GetOAuthClientResponse     `json:"clients"`
	Pagination *inventory.PaginationResults `json:"pagination"`
}

type GetOAuthClientResponse struct {
	*ent.OAuthClient
	IsSystem    bool `json:"is_system"`
	TotalGrants int  `json:"total_grants,omitempty"`
}

type HomepageSummary struct {
	MetricsSummary *MetricsSummary `json:"metrics_summary"`
	SiteURls       []string        `json:"site_urls"`
	Version        *Version        `json:"version"`
	License        *License        `json:"license,omitempty"`
}

// License describes the locally managed license information shown on the
// admin dashboard. It mirrors the upstream response shape so the dashboard
// layout and interactions stay compatible.
type License struct {
	ExpiredAt  time.Time `json:"expired_at"`
	Domains    []string  `json:"domains"`
	VolDomains []string  `json:"vol_domains"`
}

type MetricsSummary struct {
	Dates         []time.Time `json:"dates"`
	Files         []int       `json:"files"`
	Users         []int       `json:"users"`
	Shares        []int       `json:"shares"`
	FileTotal     int         `json:"file_total"`
	UserTotal     int         `json:"user_total"`
	ShareTotal    int         `json:"share_total"`
	EntitiesTotal int         `json:"entities_total"`
	GeneratedAt   time.Time   `json:"generated_at"`
}

type Version struct {
	Version string `json:"version"`
	Pro     bool   `json:"pro"`
	Commit  string `json:"commit"`
}

func init() {
	gob.Register(MetricsSummary{})
}
