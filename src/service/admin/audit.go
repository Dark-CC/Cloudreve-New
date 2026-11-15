package admin

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
	"github.com/samber/lo"
)

const (
	auditTypeCondition           = "audit_type"
	auditTypeConditionPro        = "audit_log_category"
	auditUserCondition           = "audit_user_id"
	auditUserConditionPro        = "audit_log_user"
	auditFileCondition           = "audit_file_id"
	auditFileConditionPro        = "audit_log_file"
	auditIPCondition             = "audit_ip"
	auditIPConditionPro          = "audit_log_ip"
	auditCorrelationCondition    = "audit_correlation_id"
	auditCorrelationConditionPro = "audit_log_correlation_id"
	maxAuditTypeSplitNum         = 64
)

func (s *AdminListService) AuditLogs(c *gin.Context) (*ListAuditLogResponse, error) {
	dep := dependency.FromContext(c)
	client := dep.AuditLogClient()
	hasher := dep.HashIDEncoder()

	var (
		err    error
		userID int
		fileID int
		types  []int
		ip     string
	)

	if v := s.Conditions[auditTypeConditionPro]; v != "" {
		if types, err = parseAuditQueryTypes(v); err != nil {
			return nil, err
		}
	} else if v := s.Conditions[auditTypeCondition]; v != "" {
		if types, err = parseAuditQueryTypes(v); err != nil {
			return nil, err
		}
	}

	if v := firstNonEmpty(s.Conditions[auditUserConditionPro], s.Conditions[auditUserCondition]); v != "" {
		if userID, err = strconv.Atoi(v); err != nil {
			userID, err = hasher.Decode(v, hashid.UserID)
			if err != nil {
				return nil, serializer.NewError(serializer.CodeParamErr, "Invalid audit user ID", err)
			}
		}
	}

	if v := firstNonEmpty(s.Conditions[auditFileConditionPro], s.Conditions[auditFileCondition]); v != "" {
		if fileID, err = strconv.Atoi(v); err != nil {
			fileID, err = hasher.Decode(v, hashid.FileID)
			if err != nil {
				return nil, serializer.NewError(serializer.CodeParamErr, "Invalid audit file ID", err)
			}
		}
	}

	ip = firstNonEmpty(s.Conditions[auditIPConditionPro], s.Conditions[auditIPCondition])

	var correlationID *uuid.UUID
	if v := firstNonEmpty(s.Conditions[auditCorrelationConditionPro], s.Conditions[auditCorrelationCondition]); v != "" {
		cid, err := uuid.FromString(v)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid audit correlation ID", err)
		}
		correlationID = &cid
	}

	ctx := context.WithValue(c, inventory.LoadAuditLogRelation{}, true)
	res, err := client.List(ctx, &inventory.ListAuditLogArgs{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     s.Page - 1,
			PageSize: s.PageSize,
			Order:    inventory.OrderDirection(s.OrderDirection),
		},
		Types:         types,
		UserID:        userID,
		FileID:        fileID,
		IP:            ip,
		CorrelationID: correlationID,
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list audit logs", err)
	}

	return &ListAuditLogResponse{
		Pagination: res.PaginationResults,
		Logs: lo.Map(res.Logs, func(log *ent.AuditLog, _ int) GetAuditLogResponse {
			return buildAuditLogResponse(log, hasher)
		}),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseAuditQueryTypes(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	if len(parts) > maxAuditTypeSplitNum {
		return nil, serializer.NewError(serializer.CodeParamErr, "Too many audit types", nil)
	}
	types := make([]int, 0, len(parts))
	for _, part := range parts {
		t, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid audit type", err)
		}
		types = append(types, t)
	}
	return types, nil
}

type (
	SingleAuditLogService struct {
		ID int `uri:"id" binding:"required"`
	}
	SingleAuditLogParamCtx struct{}
)

func (s *SingleAuditLogService) Get(c *gin.Context) (*GetAuditLogResponse, error) {
	dep := dependency.FromContext(c)
	ctx := context.WithValue(c, inventory.LoadAuditLogRelation{}, true)
	log, err := dep.AuditLogClient().GetByID(ctx, s.ID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, serializer.NewError(serializer.CodeNotFound, "Audit log not found", err)
		}
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get audit log", err)
	}

	res := buildAuditLogResponse(log, dep.HashIDEncoder())
	return &res, nil
}

func buildAuditLogResponse(log *ent.AuditLog, hasher hashid.Encoder) GetAuditLogResponse {
	res := GetAuditLogResponse{AuditLog: log}
	if log.Edges.User != nil {
		res.UserHashID = hashid.EncodeUserID(hasher, log.Edges.User.ID)
	}
	return res
}
