package vas

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/email"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ProductType values mirror Pro's payment product types. Orders and gift
// codes share the same wire values so data migrated from Pro stays valid.
const (
	ProductTypeShareLink inventory.ProductType = 1
	ProductTypeGroup     inventory.ProductType = 2
	ProductTypeStorage   inventory.ProductType = 3
	ProductTypePoints    inventory.ProductType = 4

	// Legacy aliases used by the credit wallet code.
	ProductTypeCredit = ProductTypePoints

	GiftCodeProductTypeShare   = ProductTypeShareLink
	GiftCodeProductTypeGroup   = ProductTypeGroup
	GiftCodeProductTypeStorage = ProductTypeStorage
	GiftCodeProductTypePoints  = ProductTypePoints
)

// PaymentStatus values.
const (
	PaymentStatusCreated       = "created"
	PaymentStatusPaid          = "paid"
	PaymentStatusFulfilling    = "fulfilling"
	PaymentStatusFulfilled     = "fulfilled"
	PaymentStatusFulfillFailed = "fulfill_failed"
	PaymentStatusCanceled      = "canceled"
)

type (
	// CreditResponse describes the credit balance of a user.
	CreditResponse struct {
		Credit        int64 `json:"credit"`
		CreditEnabled bool  `json:"credit_enabled"`
		PerUnit       int64 `json:"per_unit"`
	}

	// CreditChange is one credit ledger style entry derived from payments and
	// gift code redemptions.
	CreditChange struct {
		ID        int       `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		ChangedAt time.Time `json:"changed_at"`
		Change    int64     `json:"change"`
		Diff      int64     `json:"diff"`
		Reason    string    `json:"reason"`
		Detail    string    `json:"detail,omitempty"`
	}

	// ListCreditChangesResponse is the paginated credit change list.
	ListCreditChangesResponse struct {
		Pagination *inventory.PaginationResults `json:"pagination"`
		Changes    []CreditChange               `json:"changes"`
		// Legacy field kept for older community clients.
		LegacyChanges []CreditChange `json:"credit_changes,omitempty"`
	}

	// CreditService returns the current user's credit info.
	CreditService  struct{}
	CreditParamCtx struct{}

	// ListCreditChangesService lists credit changes for the current user.
	ListCreditChangesService struct {
		Page     int `form:"page" binding:"required,min=1"`
		PageSize int `form:"page_size" binding:"required,min=10,max=100"`
	}
	ListCreditChangesParamCtx struct{}

	// GiftCodeProduct describes what a gift code grants.
	GiftCodeProduct struct {
		Type   inventory.ProductType `json:"type"`
		SKUID  string                `json:"sku_id,omitempty"`
		Amount int64                 `json:"amount,omitempty"`
		Days   int                   `json:"days,omitempty"`
		Group  int                   `json:"group,omitempty"`
		Name   string                `json:"name,omitempty"`
	}

	// GiftCodeResponse is the public gift code view.
	GiftCodeResponse struct {
		ID      int             `json:"id"`
		Code    string          `json:"code"`
		Used    bool            `json:"used"`
		Qyt     int64           `json:"qyt"`
		Product GiftCodeProduct `json:"product"`
		// Flattened product fields keep the response compatible with Pro,
		// whose redemption page reads name/duration/amount/group directly.
		Type         inventory.ProductType `json:"type,omitempty"`
		SKUID        string                `json:"sku_id,omitempty"`
		Name         string                `json:"name,omitempty"`
		Duration     int                   `json:"duration,omitempty"`
		Amount       int64                 `json:"amount,omitempty"`
		Group        int                   `json:"group,omitempty"`
		ProductProps json.RawMessage       `json:"product_props,omitempty"`
		UsedBy       int                   `json:"used_by,omitempty"`
		UserHashID   string                `json:"user_hash_id,omitempty"`
		Edges        *GiftCodeEdges        `json:"edges,omitempty"`
		CreatedAt    time.Time             `json:"created_at"`
	}
	GiftCodeEdges struct {
		User *GiftCodeUser `json:"user,omitempty"`
	}
	// GiftCodeUser mirrors the compact user shape returned by Pro gift codes.
	GiftCodeUser struct {
		ID        string    `json:"id"`
		Nick      string    `json:"nick"`
		CreatedAt time.Time `json:"created_at"`
	}

	// GetGiftCodeService previews a gift code before redeeming.
	GetGiftCodeService struct {
		Code string `uri:"code" binding:"required"`
	}
	GetGiftCodeParamCtx struct{}

	// RedeemGiftCodeService redeems a gift code for the current user.
	RedeemGiftCodeService struct {
		Code string `uri:"code" binding:"required"`
	}
	RedeemGiftCodeParamCtx struct{}

	// CreateGiftCodesService generates gift codes in the admin panel.
	CreateGiftCodesService struct {
		Qyt     int64           `json:"qyt" binding:"required,min=1"`
		Num     int             `json:"num" binding:"min=0,max=1000"`
		Count   int             `json:"count" binding:"min=0,max=1000"`
		Product GiftCodeProduct `json:"product"`
	}
	CreateGiftCodesParamCtx struct{}

	// UpdateGiftCodeService updates an unused gift code in the admin panel.
	UpdateGiftCodeService struct {
		ID      int             `json:"id" binding:"required,min=1"`
		Qyt     int64           `json:"qyt" binding:"required,min=1"`
		Product GiftCodeProduct `json:"product"`
	}
	UpdateGiftCodeParamCtx struct{}

	// ListGiftCodesService lists gift codes in the admin panel.
	ListGiftCodesService struct {
		Page           int               `json:"page" form:"page" binding:"min=1"`
		PageSize       int               `json:"page_size" form:"page_size" binding:"required,min=10,max=100"`
		OrderBy        string            `json:"order_by" form:"order_by"`
		OrderDirection string            `json:"order_direction" form:"order_direction"`
		Conditions     map[string]string `json:"conditions" form:"conditions"`
		Searches       map[string]string `json:"searches" form:"searches"`
	}
	ListGiftCodesParamCtx struct{}

	// DeleteGiftCodeService deletes one gift code.
	DeleteGiftCodeService struct {
		ID int `uri:"id" binding:"required"`
	}
	DeleteGiftCodeParamCtx struct{}

	// StoragePackProduct is one purchasable storage expansion.
	StoragePackProduct struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		Price    int64  `json:"price"`
		Duration int    `json:"duration"`
		Time     int    `json:"time"`
		Points   int64  `json:"points"`
		Chip     string `json:"chip"`
	}

	// StoragePackResponse is the purchased pack view.
	StoragePackResponse struct {
		ID          int        `json:"id"`
		Name        string     `json:"name"`
		Size        int64      `json:"size"`
		ActiveSince *time.Time `json:"active_since,omitempty"`
		ExpireAt    *time.Time `json:"expire_at,omitempty"`
	}

	// ListStoragePacksResponse lists a user's storage packs.
	ListStoragePacksResponse struct {
		Packs []StoragePackResponse `json:"packs"`
		Total int64                 `json:"total"`
	}

	// ListStoragePacksService lists the current user's packs.
	ListStoragePacksService  struct{}
	ListStoragePacksParamCtx struct{}

	// CreatePaymentService creates a new order.
	CreatePaymentService struct {
		ProductType      inventory.ProductType `json:"product_type"`
		ProductProps     json.RawMessage       `json:"product_props"`
		ProviderID       string                `json:"provider_id"`
		ProductTypeAlias inventory.ProductType `json:"type"`
		SKUID            string                `json:"sku_id"`
		Name             string                `json:"name"`
		PriceUnit        int64                 `json:"price_unit"`
		PriceMark        string                `json:"price_mark"`
		PriceID          string                `json:"price_id"`
		Qyt              int64                 `json:"qyt"`
		AnonymousEmail   string                `json:"anonymous_email"`
		Email            string                `json:"email"`
		Language         string                `json:"language"`
		Quantity         int64                 `json:"quantity"`
		UsePoints        bool                  `json:"use_points"`
		PaymentMethod    string                `json:"payment_method"`
		Product          *CreatePaymentProduct `json:"product"`
	}
	CreatePaymentProduct struct {
		Type  inventory.ProductType `json:"type"`
		SKUID string                `json:"sku_id"`
	}
	CreatePaymentParamCtx struct{}

	// PaymentResponse is the order view returned to clients.
	PaymentResponse struct {
		ID                  int                   `json:"id"`
		AliasID             int                   `json:"payment_id,omitempty"`
		CreatedAt           time.Time             `json:"created_at"`
		TradeNo             string                `json:"trade_no"`
		ProductType         inventory.ProductType `json:"product_type"`
		ProductProps        json.RawMessage       `json:"product_props"`
		ProviderID          string                `json:"provider_id,omitempty"`
		Name                string                `json:"name"`
		PriceUnit           int64                 `json:"price_unit"`
		PriceMark           string                `json:"price_mark,omitempty"`
		PriceID             string                `json:"price_id"`
		PriceOneUnit        int64                 `json:"price_one_unit"`
		Qyt                 int64                 `json:"qyt"`
		Status              string                `json:"status"`
		AnonymousEmail      string                `json:"anonymous_email,omitempty"`
		CreatorHashID       string                `json:"creator_hash_id,omitempty"`
		UserHashID          string                `json:"user_hash_id,omitempty"`
		Edges               *PaymentEdgesResponse `json:"edges,omitempty"`
		PaymentURL          string                `json:"payment_url,omitempty"`
		QRCode              string                `json:"qr_code,omitempty"`
		PaymentSign         string                `json:"payment_sign,omitempty"`
		ProviderReferenceID string                `json:"provider_reference_id,omitempty"`
		Request             *PaymentRequestView   `json:"request,omitempty"`
		Order               *PaymentView          `json:"payment,omitempty"`
		QRCodePreferred     bool                  `json:"qr_code_preferred"`
	}

	// PaymentRequestView mirrors the checkout information exposed by Pro.
	PaymentRequestView struct {
		PaymentNeeded   bool   `json:"payment_needed"`
		URL             string `json:"url,omitempty"`
		QRCodePreferred bool   `json:"qr_code_preferred"`
	}

	// PaymentView is the nested order identifier used by Pro's status polls.
	PaymentView struct {
		ID      int    `json:"id"`
		TradeNo string `json:"trade_no"`
		Status  string `json:"status"`
	}

	// PaymentEdgesResponse mirrors Pro's eager loaded order relations.
	PaymentEdgesResponse struct {
		User *PaymentUserResponse `json:"user,omitempty"`
	}

	// PaymentUserResponse is the order owner shown in the admin list.
	PaymentUserResponse struct {
		ID        int       `json:"id"`
		Nick      string    `json:"nick"`
		CreatedAt time.Time `json:"created_at"`
	}

	// ListPaymentsResponse is the paginated order list.
	ListPaymentsResponse struct {
		Pagination    *inventory.PaginationResults `json:"pagination,omitempty"`
		Payments      []PaymentResponse            `json:"payments"`
		NextPageToken string                       `json:"next_page_token,omitempty"`
	}

	// PaymentStatusService polls the status of an order.
	PaymentStatusService struct {
		TradeNo   string `uri:"trade_no" binding:"required"`
		Signature string `uri:"sign" binding:"required"`
	}
	PaymentStatusParamCtx struct{}

	// ListPaymentsService lists orders of the current user.
	ListPaymentsService struct {
		Page           int    `form:"page" binding:"min=0"`
		PageSize       int    `form:"page_size" binding:"min=1,max=100"`
		NextPageToken  string `form:"next_page_token"`
		OrderDirection string `form:"order_direction" binding:"omitempty,eq=asc|eq=desc"`
	}
	ListPaymentsParamCtx struct{}

	// AdminListPaymentsService lists all orders.
	AdminListPaymentsService struct {
		Page           int               `json:"page" binding:"min=1"`
		PageSize       int               `json:"page_size" binding:"required,min=1,max=100"`
		NextPageToken  string            `json:"next_page_token"`
		OrderBy        string            `json:"order_by"`
		OrderDirection string            `json:"order_direction"`
		Conditions     map[string]string `json:"conditions"`
	}
	AdminListPaymentsParamCtx struct{}

	// AdminFulfillPaymentService marks an order as fulfilled.
	AdminFulfillPaymentService struct {
		ID int `uri:"id" binding:"required"`
	}
	AdminFulfillPaymentParamCtx struct{}

	// AdminDeletePaymentsService deletes orders.
	AdminDeletePaymentsService struct {
		IDs []int `json:"ids" binding:"required"`
	}
	AdminDeletePaymentsParamCtx struct{}

	// PurchaseShareService buys access to a paid share using credits.
	PurchaseShareService struct {
		ID string `uri:"id" binding:"required"`
	}
	PurchaseShareParamCtx struct{}

	// PurchaseShareResponse holds the purchase result and access ticket.
	PurchaseShareResponse struct {
		Ticket  string `json:"ticket"`
		Credits int64  `json:"credits"`
	}

	resolvedProduct struct {
		ProductType inventory.ProductType
		Name        string
		Price       int64
		Points      int64
		Props       json.RawMessage
	}
)

type storageProductConfig struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Price  int64  `json:"price"`
	Time   int    `json:"time"`
	Points int64  `json:"points"`
	Chip   string `json:"chip"`
}

type groupProductConfig struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	GroupID int      `json:"group_id"`
	Time    int      `json:"time"`
	Price   int64    `json:"price"`
	Points  int64    `json:"points"`
	Chip    string   `json:"chip"`
	Des     []string `json:"des"`
}

// GetCredit returns the current user's credit balance.
func (s *CreditService) Get(c *gin.Context) (*CreditResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	cfg := dep.SettingProvider().VAS(c)

	credit, err := dep.VASClient().Credit(c, user.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get credit", err)
	}

	return &CreditResponse{Credit: credit, CreditEnabled: cfg.CreditEnabled, PerUnit: cfg.CreditPerUnit}, nil
}

// List returns the credit ledger for the current user.
func (s *ListCreditChangesService) List(c *gin.Context) (*ListCreditChangesResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)

	changes, pagination, err := dep.VASClient().ListCreditChanges(c, user.ID, &inventory.PaginationArgs{
		Page: s.Page - 1, PageSize: s.PageSize,
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list credit changes", err)
	}

	res := make([]CreditChange, 0, len(changes))
	for _, change := range changes {
		res = append(res, CreditChange{
			ID:        change.ID,
			CreatedAt: change.CreatedAt,
			ChangedAt: change.CreatedAt,
			Change:    change.Change,
			Diff:      change.Change,
			Reason:    change.Reason,
			Detail:    change.Detail,
		})
	}

	return &ListCreditChangesResponse{Pagination: pagination, Changes: res, LegacyChanges: res}, nil
}

// Get previews a gift code.
func (s *GetGiftCodeService) Get(c *gin.Context) (*GiftCodeResponse, error) {
	dep := dependency.FromContext(c)
	code, err := dep.VASClient().GetGiftCode(c, s.Code)
	if err != nil {
		if errors.Is(err, inventory.ErrGiftCodeNotFound) {
			return nil, serializer.NewError(serializer.CodeNotFound, "Gift code not found", err)
		}
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get gift code", err)
	}

	return buildGiftCodeResponse(code), nil
}

// Redeem redeems a gift code for the current user.
func (s *RedeemGiftCodeService) Redeem(c *gin.Context) (*GiftCodeResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)

	code, err := dep.VASClient().RedeemGiftCode(c, s.Code, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, inventory.ErrGiftCodeNotFound):
			return nil, serializer.NewError(serializer.CodeNotFound, "Gift code not found", err)
		case errors.Is(err, inventory.ErrGiftCodeUsed):
			return nil, serializer.NewError(serializer.CodeParamErr, "Gift code already used", err)
		default:
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to redeem gift code", err)
		}
	}

	res := buildGiftCodeResponse(code)
	product := res.Product
	codeQty := res.Qyt
	if codeQty <= 0 {
		codeQty = 1
	}
	if err := applyGiftCodeProduct(c, user.ID, product, codeQty); err != nil {
		// A claim is made before fulfillment to prevent concurrent redemptions.
		// Release it when the product cannot be granted, otherwise the user is
		// left with a used code and no benefit.
		_ = dep.VASClient().ReleaseGiftCode(c, code.Code, user.ID)
		return nil, err
	}

	audit.Record(c, audit.EventRedeemGiftCode, &audit.LogEntry{
		Account:      code.Code,
		Reason:       "gift_code",
		StorageSize:  product.Amount,
		PointsChange: int(product.Amount),
		GroupID:      product.Group,
		Expire:       strconv.Itoa(product.Days),
	})

	return res, nil
}

// Create generates gift codes in the admin panel.
func (s *CreateGiftCodesService) Create(c *gin.Context) ([]GiftCodeResponse, error) {
	dep := dependency.FromContext(c)
	product, err := normalizeGiftCodeProduct(c, s.Product, s.Qyt)
	if err != nil {
		return nil, err
	}
	props := mustMarshalGiftCodeProps(s.Qyt, product)

	count := s.Num
	if count <= 0 {
		count = s.Count
	}
	if count <= 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Gift code quantity is required", nil)
	}

	codes, err := dep.VASClient().CreateGiftCodes(c, &inventory.GiftCodeArgs{
		Qyt:          s.Qyt,
		ProductProps: props,
	}, count)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create gift codes", err)
	}

	res := make([]GiftCodeResponse, 0, len(codes))
	for _, code := range codes {
		res = append(res, *buildGiftCodeResponse(code))
	}
	return res, nil
}

// Update updates an unused gift code in the admin panel.
func (s *UpdateGiftCodeService) Update(c *gin.Context) (*GiftCodeResponse, error) {
	dep := dependency.FromContext(c)
	product, err := normalizeGiftCodeProduct(c, s.Product, s.Qyt)
	if err != nil {
		return nil, err
	}

	code, err := dep.VASClient().UpdateGiftCode(c, s.ID, s.Qyt, mustMarshalGiftCodeProps(s.Qyt, product))
	if err != nil {
		switch {
		case errors.Is(err, inventory.ErrGiftCodeNotFound):
			return nil, serializer.NewError(serializer.CodeNotFound, "Gift code not found", err)
		case errors.Is(err, inventory.ErrGiftCodeUsed):
			return nil, serializer.NewError(serializer.CodeParamErr, "Gift code is used or unavailable", err)
		default:
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to update gift code", err)
		}
	}

	return buildGiftCodeResponse(code), nil
}

// List lists gift codes in the admin panel.
func (s *ListGiftCodesService) List(c *gin.Context) (map[string]any, error) {
	dep := dependency.FromContext(c)
	args, err := buildGiftCodeListArgs(s, dep.HashIDEncoder())
	if err != nil {
		return nil, err
	}

	codes, pagination, err := dep.VASClient().ListGiftCodes(c, args)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list gift codes", err)
	}

	hasher := dep.HashIDEncoder()
	userIDs := make([]int, 0, len(codes))
	for _, code := range codes {
		if code.UsedBy != nil {
			userIDs = append(userIDs, *code.UsedBy)
		}
	}
	users := map[int]*GiftCodeUser{}
	if len(userIDs) > 0 {
		for _, uid := range userIDs {
			model, err := dep.UserClient().GetByID(c, uid)
			if err != nil {
				continue
			}
			users[uid] = &GiftCodeUser{
				ID:        hashid.EncodeUserID(hasher, model.ID),
				Nick:      model.Nick,
				CreatedAt: model.CreatedAt,
			}
		}
	}

	res := make([]GiftCodeResponse, 0, len(codes))
	for _, code := range codes {
		item := buildGiftCodeResponse(code)
		if code.UsedBy != nil {
			item.UserHashID = hashid.EncodeUserID(hasher, *code.UsedBy)
			if model := users[*code.UsedBy]; model != nil {
				item.Edges = &GiftCodeEdges{User: model}
			}
		}
		res = append(res, *item)
	}

	return map[string]any{"pagination": pagination, "gift_codes": res}, nil
}

func buildGiftCodeListArgs(s *ListGiftCodesService, hasher hashid.Encoder) (*inventory.GiftCodeListArgs, error) {
	args := &inventory.GiftCodeListArgs{
		PaginationArgs: &inventory.PaginationArgs{},
	}
	if s == nil {
		return args, nil
	}

	args.Page = max(s.Page-1, 0)
	args.PageSize = s.PageSize
	args.OrderBy = s.OrderBy
	args.Order = inventory.OrderDirection(s.OrderDirection)
	args.Code = s.Conditions["gift_code_code"]
	args.CodeContains = s.Searches["gift_code_code"]

	if v := s.Conditions["gift_code_used"]; v != "" {
		used, err := strconv.ParseBool(v)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid gift code used condition", err)
		}
		args.Used = &used
	}

	if v := s.Conditions["gift_code_user_id"]; v != "" {
		userID, err := hasher.Decode(v, hashid.UserID)
		if err != nil {
			userID, err = strconv.Atoi(v)
		}
		if err != nil || userID <= 0 {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid user ID", err)
		}
		args.UserID = userID
	}

	return args, nil
}

// Delete soft deletes one gift code.
func (s *DeleteGiftCodeService) Delete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	if err := dep.VASClient().DeleteGiftCode(c, s.ID); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete gift code", err)
	}
	return nil
}

// List lists the current user's storage packs.
func (s *ListStoragePacksService) List(c *gin.Context) (*ListStoragePacksResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)

	packs, err := dep.VASClient().ListStoragePacks(c, user.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list storage packs", err)
	}

	res := &ListStoragePacksResponse{Packs: make([]StoragePackResponse, 0, len(packs))}
	for _, pack := range packs {
		expireAt := pack.ExpireAt
		if expireAt != nil && expireAt.IsZero() {
			expireAt = nil
		}
		res.Packs = append(res.Packs, StoragePackResponse{
			ID:          pack.ID,
			Name:        pack.Name,
			Size:        pack.Size,
			ActiveSince: pack.ActiveSince,
			ExpireAt:    expireAt,
		})
		res.Total += pack.Size
	}

	return res, nil
}

// Create creates a new order for the current user. Prices and product
// properties are always resolved from the server-side SKU configuration.
func (s *CreatePaymentService) Create(c *gin.Context) (*PaymentResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	cfg := dep.SettingProvider().VAS(c)

	if !cfg.CreditEnabled {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Value added service is disabled", nil)
	}

	anonymousEmail := s.AnonymousEmail
	if anonymousEmail == "" {
		anonymousEmail = s.Email
	}
	if anonymousEmail != "" && !cfg.AnonymousPurchase {
		return nil, serializer.NewError(serializer.CodeNoPermissionErr, "Anonymous purchase is disabled", nil)
	}

	productType := s.ProductType
	skuID := s.SKUID
	if s.Product != nil {
		if s.Product.Type != 0 {
			productType = s.Product.Type
		}
		if s.Product.SKUID != "" {
			skuID = s.Product.SKUID
		}
	}
	// New clients send "type"; older clients send "product_type". Some
	// clients send both, so the explicit type must always win.
	if s.ProductTypeAlias != 0 {
		productType = s.ProductTypeAlias
	}
	quantity := s.Qyt
	if quantity < 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid quantity", nil)
	}
	if quantity <= 0 {
		quantity = s.Quantity
	}
	if quantity < 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid quantity", nil)
	}
	if quantity <= 0 {
		quantity = 1
	}
	if quantity > 100000 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Quantity is too large", nil)
	}

	product, err := resolveProduct(c, productType, skuID)
	if err != nil {
		return nil, err
	}
	if product == nil {
		switch productType {
		case ProductTypeCredit:
			// Credit top-ups are a fixed server-side product. The client may
			// choose the quantity, but never the price.
		case ProductTypeStorage, ProductTypeGroup, ProductTypeShareLink:
			// These products grant privileges and must always come from the
			// server-side SKU configuration.
			return nil, serializer.NewError(serializer.CodeParamErr, "Product not found", nil)
		default:
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid product", nil)
		}
	}

	name := s.Name
	priceOneUnit := s.PriceUnit
	props := s.ProductProps
	priceMark := s.PriceMark
	priceID := s.PriceID
	pointsPrice := int64(0)
	if product != nil {
		name = product.Name
		priceOneUnit = product.Price
		props = product.Props
		priceMark = cfg.CurrencySymbol
		priceID = skuID
		pointsPrice = product.Points
	} else if productType == ProductTypeCredit {
		if cfg.CreditPerUnit <= 0 {
			return nil, serializer.NewError(serializer.CodeParamErr, "Credit recharge is disabled", nil)
		}
		if cfg.CreditPerUnit > math.MaxInt64/quantity {
			return nil, serializer.NewError(serializer.CodeParamErr, "Quantity is too large", nil)
		}
		name = "Credits"
		priceOneUnit = cfg.CreditPerUnit
		props = json.RawMessage(`{}`)
		priceMark = cfg.CurrencySymbol
		priceID = "credit"
	}

	usePoints := s.UsePoints || s.PaymentMethod == "points" ||
		(s.ProviderID == "" && pointsPrice > 0 && priceOneUnit <= 0)
	if s.ProviderID == "points" {
		usePoints = true
	}
	if usePoints {
		if pointsPrice > math.MaxInt64/quantity {
			return nil, serializer.NewError(serializer.CodeParamErr, "Quantity is too large", nil)
		}
		return createPointPayment(c, dep, user, productType, name, priceID, props, pointsPrice, quantity)
	}
	if priceOneUnit <= 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Product does not accept cash payment", nil)
	}
	if priceOneUnit > math.MaxInt64/quantity {
		return nil, serializer.NewError(serializer.CodeParamErr, "Quantity is too large", nil)
	}
	priceUnit := priceOneUnit * quantity
	if s.ProviderID == "local" && !cfg.LocalPayment {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Local payment is disabled", nil)
	}
	var provider setting.PaymentProviderConfig
	providerID := s.ProviderID
	if providerID == "" {
		providerID = "local"
	}
	if providerID != "local" {
		var err error
		provider, err = findPaymentProvider(cfg.Payment, providerID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, err.Error(), err)
		}
	}

	var creator *int
	if user.ID > 0 {
		id := user.ID
		creator = &id
	}

	var providerPtr *string
	if providerID != "" {
		providerPtr = &providerID
	}

	tradeNo := strings.ReplaceAll(uuid.NewString(), "-", "")
	payment, err := dep.VASClient().CreatePayment(c, &inventory.PaymentCreateArgs{
		TradeNo:        tradeNo,
		ProductType:    productType,
		ProductProps:   props,
		ProviderID:     providerPtr,
		Name:           name,
		PriceUnit:      priceUnit,
		PriceMark:      priceMark,
		PriceID:        priceID,
		PriceOneUnit:   priceOneUnit,
		Qyt:            quantity,
		Status:         PaymentStatusCreated,
		Creator:        creator,
		AnonymousEmail: anonymousEmail,
		Language:       s.Language,
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create payment", err)
	}

	audit.Record(c, audit.EventPaymentCreated, &audit.LogEntry{
		PaymentID: payment.ID,
		SKU:       priceID,
		Name:      name,
	})

	var session *PaymentSession
	if providerID != "local" {
		adapter, err := paymentAdapterFor(provider)
		if err != nil {
			_ = dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusCanceled)
			return nil, serializer.NewError(serializer.CodeParamErr, err.Error(), err)
		}
		session, err = adapter.Create(c, provider, payment, paymentCurrency(cfg.Payment, cfg.CurrencyCode),
			paymentCallbackURL(dep.SettingProvider().SiteURL(c), tradeNo))
		if err != nil {
			_ = dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusCanceled)
			return nil, serializer.NewError(serializer.CodeParamErr, "Failed to create payment session", err)
		}
		if session.ProviderReferenceID != "" {
			if err := dep.VASClient().SetPaymentProviderReference(c, payment.ID, session.ProviderReferenceID); err != nil {
				_ = dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusCanceled)
				return nil, serializer.NewError(serializer.CodeDBError, "Failed to save provider reference", err)
			}
			payment.ProviderReferenceID = session.ProviderReferenceID
		}
	}

	// The built-in "local" provider is a sandbox/test channel. It confirms the
	// order immediately so the shop can be exercised end-to-end without an
	// external payment gateway. Real providers keep the created status until
	// their signed callback/poll confirms payment.
	if providerID == "local" {
		if updated, err := dep.VASClient().MarkPaymentPaid(c, payment.ID); err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to mark payment paid", err)
		} else if updated {
			payment.Status = PaymentStatusPaid
			audit.Record(c, audit.EventPaymentPaid, &audit.LogEntry{
				PaymentID: payment.ID,
				SKU:       priceID,
				Name:      name,
			})
		}

		claimed, err := dep.VASClient().MarkPaymentFulfilling(c, payment.ID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to claim payment", err)
		}
		if !claimed {
			payment, err = dep.VASClient().GetPaymentByID(c, payment.ID)
			if err != nil {
				return nil, serializer.NewError(serializer.CodeDBError, "Failed to get payment", err)
			}
			res := buildPaymentResponse(payment, dep)
			return &res, nil
		}
		if err := fulfillPayment(c, payment); err != nil {
			_ = dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfillFailed)
			audit.Record(c, audit.EventPaymentFulfillFail, &audit.LogEntry{
				PaymentID: payment.ID,
				SKU:       priceID,
				Name:      name,
				Failed:    true,
			})
			return nil, err
		}
		if err := dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfilled); err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to update payment status", err)
		}
		payment.Status = PaymentStatusFulfilled
		sendPaymentReceipt(c, dep, payment)
		audit.Record(c, audit.EventPaymentFulfilled, &audit.LogEntry{
			PaymentID: payment.ID,
			SKU:       priceID,
			Name:      name,
		})
	}

	res := buildPaymentResponse(payment, dep)
	if session != nil {
		res.PaymentURL = session.PaymentURL
		res.QRCode = session.QRCode
		res.PaymentSign = signPaymentStatus(dep, payment.TradeNo)
	}
	attachPaymentCompatibility(&res)
	return &res, nil
}

// attachPaymentCompatibility keeps the flat response layout while adding the
// nested checkout fields consumed by the payment dialog.
func attachPaymentCompatibility(res *PaymentResponse) {
	if res == nil {
		return
	}
	res.AliasID = res.ID
	res.Order = &PaymentView{
		ID:      res.ID,
		TradeNo: res.TradeNo,
		Status:  res.Status,
	}
	qrPreferred := res.QRCode != ""
	res.QRCodePreferred = qrPreferred
	res.Request = &PaymentRequestView{
		PaymentNeeded:   res.Status != PaymentStatusFulfilled && res.Status != PaymentStatusPaid,
		URL:             firstNonEmpty(res.PaymentURL, res.QRCode),
		QRCodePreferred: qrPreferred,
	}
}

// List lists orders of the current user.
func (s *ListPaymentsService) List(c *gin.Context) (*ListPaymentsResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)

	page := s.Page
	if page > 0 {
		page--
	}
	pageSize := s.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	if s.NextPageToken != "" {
		if decoded, err := strconv.Atoi(s.NextPageToken); err == nil && decoded > 0 {
			page = decoded
		}
	}
	res, err := dep.VASClient().ListPayments(c, &inventory.PaymentListArgs{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     page,
			PageSize: pageSize,
			Order:    inventory.OrderDirection(s.OrderDirection),
		},
		Creator: user.ID,
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list payments", err)
	}

	payments := make([]PaymentResponse, 0, len(res.Payments))
	for _, payment := range res.Payments {
		payments = append(payments, buildPaymentResponse(payment, dep))
	}
	enrichPaymentUsers(c, dep, payments)

	nextPageToken := ""
	if res.PaginationResults != nil && (page+1)*pageSize < int(res.PaginationResults.TotalItems) {
		nextPageToken = strconv.Itoa(page + 1)
	}
	return &ListPaymentsResponse{
		Pagination:    res.PaginationResults,
		Payments:      payments,
		NextPageToken: nextPageToken,
	}, nil
}

// Status polls an order status by trade number and signature.
func (s *PaymentStatusService) Status(c *gin.Context) (*PaymentResponse, error) {
	dep := dependency.FromContext(c)
	cfg := dep.SettingProvider().VAS(c)
	if !validPaymentSignature(c, dep, s.TradeNo, s.Signature) {
		return nil, serializer.NewError(serializer.CodeNoPermissionErr, "Invalid payment signature", nil)
	}

	payment, err := dep.VASClient().GetPaymentByTradeNo(c, s.TradeNo)
	if err != nil {
		if errors.Is(err, inventory.ErrPaymentNotFound) {
			return nil, serializer.NewError(serializer.CodeNotFound, "Payment not found", err)
		}
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get payment", err)
	}

	if payment.Status == PaymentStatusCreated && payment.ProviderID != nil && *payment.ProviderID != "local" {
		provider, err := findPaymentProvider(cfg.Payment, *payment.ProviderID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, err.Error(), err)
		}
		adapter, err := paymentAdapterFor(provider)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, err.Error(), err)
		}
		result, err := adapter.Query(c, provider, payment, paymentCurrency(cfg.Payment, cfg.CurrencyCode))
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Failed to query payment", err)
		}
		if result.Paid {
			if err := confirmPaymentPaid(c, dep, payment, result, paymentCurrency(cfg.Payment, cfg.CurrencyCode)); err != nil {
				return nil, err
			}
		}
	}

	// A local payment provider confirms the order via a signed status poll.
	// Fulfillment remains idempotent for repeated polls.
	if payment.ProviderID != nil && *payment.ProviderID == "local" &&
		(payment.Status == PaymentStatusCreated || payment.Status == PaymentStatusPaid) {
		if updated, err := dep.VASClient().MarkPaymentPaid(c, payment.ID); err == nil && updated {
			payment.Status = PaymentStatusPaid
			audit.Record(c, audit.EventPaymentPaid, &audit.LogEntry{
				PaymentID: payment.ID,
				SKU:       payment.PriceID,
				Name:      payment.Name,
			})
		} else if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to mark payment paid", err)
		}
	}

	if payment.Status == PaymentStatusPaid {
		claimed, err := dep.VASClient().MarkPaymentFulfilling(c, payment.ID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to claim payment", err)
		}
		if !claimed {
			payment, err = dep.VASClient().GetPaymentByID(c, payment.ID)
			if err != nil {
				return nil, serializer.NewError(serializer.CodeDBError, "Failed to get payment", err)
			}
			res := buildPaymentResponse(payment, dep)
			return &res, nil
		}
		if err := fulfillPayment(c, payment); err != nil {
			_ = dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfillFailed)
			return nil, err
		}
		if err := dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfilled); err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to fulfill payment", err)
		}
		payment.Status = PaymentStatusFulfilled
		sendPaymentReceipt(c, dep, payment)
		audit.Record(c, audit.EventPaymentFulfilled, &audit.LogEntry{
			PaymentID: payment.ID,
			SKU:       payment.PriceID,
			Name:      payment.Name,
		})
	}

	res := buildPaymentResponse(payment, dep)
	return &res, nil
}

// AdminList lists all orders.
func (s *AdminListPaymentsService) AdminList(c *gin.Context) (*ListPaymentsResponse, error) {
	dep := dependency.FromContext(c)

	// A worker that dies between claiming and finalizing an order leaves it in
	// fulfilling forever; recovery makes such orders retryable again.
	if _, err := dep.VASClient().RecoverStaleFulfillingPayments(c, time.Now().Add(-10*time.Minute)); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to recover stale payments", err)
	}

	page := s.Page
	if page <= 0 {
		page = 1
	}
	if s.NextPageToken != "" {
		if decoded, err := strconv.Atoi(s.NextPageToken); err == nil && decoded >= 0 {
			page = decoded + 1
		}
	}

	args := &inventory.PaymentListArgs{
		PaginationArgs: &inventory.PaginationArgs{Page: page - 1, PageSize: s.PageSize, Order: inventory.OrderDirection(s.OrderDirection)},
		Status:         s.Conditions["payment_status"],
		ProviderID:     s.Conditions["payment_provider_id"],
		TradeNo:        s.Conditions["payment_trade_no"],
	}

	if v := s.Conditions["payment_product_type"]; v != "" {
		productType, err := strconv.Atoi(v)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid product type", err)
		}
		args.ProductType = inventory.ProductType(productType)
		args.HasProductType = true
	}

	if v := s.Conditions["payment_user_id"]; v != "" {
		userID, err := dep.HashIDEncoder().Decode(v, hashid.UserID)
		if err != nil {
			userID, err = strconv.Atoi(v)
		}
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid user ID", err)
		}
		args.Creator = userID
	}

	res, err := dep.VASClient().ListPayments(c, args)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list payments", err)
	}

	payments := make([]PaymentResponse, 0, len(res.Payments))
	for _, payment := range res.Payments {
		payments = append(payments, buildPaymentResponse(payment, dep))
	}
	enrichPaymentUsers(c, dep, payments)

	nextPageToken := ""
	if res.PaginationResults != nil && page*s.PageSize < int(res.PaginationResults.TotalItems) {
		nextPageToken = strconv.Itoa(page)
	}
	return &ListPaymentsResponse{
		Pagination:    res.PaginationResults,
		Payments:      payments,
		NextPageToken: nextPageToken,
	}, nil
}

// Fulfill marks an order as fulfilled and applies its product.
func (s *AdminFulfillPaymentService) Fulfill(c *gin.Context) (*PaymentResponse, error) {
	dep := dependency.FromContext(c)
	payment, err := dep.VASClient().GetPaymentByID(c, s.ID)
	if err != nil {
		if errors.Is(err, inventory.ErrPaymentNotFound) {
			return nil, serializer.NewError(serializer.CodeNotFound, "Payment not found", err)
		}
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get payment", err)
	}

	if payment.Status == PaymentStatusFulfilled {
		res := buildPaymentResponse(payment, dep)
		return &res, nil
	}

	claimed, err := dep.VASClient().MarkPaymentFulfilling(c, payment.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to claim payment", err)
	}
	if !claimed {
		payment, err = dep.VASClient().GetPaymentByID(c, payment.ID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to get payment", err)
		}
		res := buildPaymentResponse(payment, dep)
		return &res, nil
	}

	if err := fulfillPayment(c, payment); err != nil {
		_ = dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfillFailed)
		audit.Record(c, audit.EventPaymentFulfillFail, &audit.LogEntry{
			PaymentID: payment.ID,
			SKU:       payment.PriceID,
			Name:      payment.Name,
			Failed:    true,
		})
		return nil, err
	}

	if err := dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfilled); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update payment status", err)
	}
	payment.Status = PaymentStatusFulfilled
	sendPaymentReceipt(c, dep, payment)
	audit.Record(c, audit.EventPaymentFulfilled, &audit.LogEntry{
		PaymentID: payment.ID,
		SKU:       payment.PriceID,
		Name:      payment.Name,
	})

	res := buildPaymentResponse(payment, dep)
	return &res, nil
}

// AdminDelete deletes orders in batch.
func (s *AdminDeletePaymentsService) AdminDelete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	if err := dep.VASClient().DeletePayments(c, s.IDs...); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete payments", err)
	}
	return nil
}

// Purchase buys access to a paid share using credits.
func (s *PurchaseShareService) Purchase(c *gin.Context) (*PurchaseShareResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)

	shareID, err := dep.HashIDEncoder().Decode(s.ID, hashid.ShareID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid share ID", err)
	}

	// Load the owner edge so crediting the share owner does not need a second
	// query on the transaction's single connection.
	shareCtx := context.WithValue(c, inventory.LoadShareUser{}, true)
	shareCtx = context.WithValue(shareCtx, inventory.LoadShareFile{}, true)
	share, err := dep.ShareClient().GetByID(shareCtx, shareID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeNotFound, "Share not found", err)
	}

	price, err := dep.ShareClient().GetPrice(c, share.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get share price", err)
	}

	if price <= 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Share is free", nil)
	}

	client, tx, txCtx, err := inventory.WithTx(c, dep.VASClient())
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to start purchase transaction", err)
	}
	defer func() {
		_ = inventory.Rollback(tx)
	}()

	owned, err := client.HasSharePurchase(txCtx, shareID, user.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to check share purchase", err)
	}
	if owned {
		purchase, err := client.CreateSharePurchase(txCtx, &inventory.SharePurchase{
			TicketID: uuid.NewString(),
			ShareID:  &shareID,
			UserID:   &user.ID,
		})
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to create share purchase ticket", err)
		}
		balance, _ := client.Credit(txCtx, user.ID)
		if err := inventory.Commit(tx); err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to commit purchase ticket", err)
		}
		return &PurchaseShareResponse{Ticket: purchase.TicketID, Credits: balance}, nil
	}

	change, err := client.ChangeCredit(txCtx, &inventory.CreditChangeArgs{
		UserID: user.ID,
		Change: -price,
		Reason: "share_purchase",
		Detail: fmt.Sprintf("share#%d", share.ID),
	})
	if err != nil {
		if errors.Is(err, inventory.ErrInsufficientCredit) {
			return nil, serializer.NewError(serializer.CodeInsufficientCredit, "Insufficient credit", err)
		}
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to deduct credit", err)
	}

	ticket := uuid.NewString()
	uid := user.ID
	if _, err := client.CreateSharePurchase(txCtx, &inventory.SharePurchase{
		TicketID: ticket,
		ShareID:  &shareID,
		UserID:   &uid,
	}); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create share purchase", err)
	}

	if err := creditShareOwnerWithClient(txCtx, c, client, share, price); err != nil {
		return nil, err
	}
	if err := inventory.Commit(tx); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to commit share purchase", err)
	}

	audit.Record(c, audit.EventPointsChange, &audit.LogEntry{
		PointsChange: int(-price),
		Reason:       "share_purchase",
		Name:         fmt.Sprintf("share#%d", share.ID),
	}, audit.WithUser(user.ID), audit.WithShare(share.ID))

	return &PurchaseShareResponse{Ticket: ticket, Credits: change.Balance}, nil
}

func buildGiftCodeResponse(code *inventory.GiftCode) *GiftCodeResponse {
	res := &GiftCodeResponse{
		ID:           code.ID,
		Code:         code.Code,
		Used:         code.Used,
		Qyt:          code.Qyt,
		ProductProps: code.ProductProps,
		CreatedAt:    code.CreatedAt,
	}
	if code.UsedBy != nil {
		res.UsedBy = *code.UsedBy
	}

	var raw struct {
		Qyt     int64           `json:"qyt"`
		Product GiftCodeProduct `json:"product"`
	}
	if len(code.ProductProps) > 0 {
		_ = json.Unmarshal(code.ProductProps, &raw)
	}
	res.Product = raw.Product
	if res.Qyt == 0 {
		res.Qyt = raw.Qyt
	}
	if raw.Product.Type != 0 {
		if productProps, err := json.Marshal(raw.Product); err == nil {
			res.ProductProps = productProps
		}
	} else if len(code.ProductProps) > 0 {
		// Pro stores product_props as the product object itself. Accept that
		// shape as well so databases migrated from Pro keep working.
		var direct GiftCodeProduct
		if err := json.Unmarshal(code.ProductProps, &direct); err == nil && direct.Type != 0 {
			res.Product = direct
			res.ProductProps = code.ProductProps
		}
	}

	res.Type = res.Product.Type
	res.SKUID = res.Product.SKUID
	res.Name = res.Product.Name
	res.Duration = res.Product.Days
	res.Amount = res.Product.Amount
	res.Group = res.Product.Group

	return res
}

func resolveProduct(c *gin.Context, productType inventory.ProductType, skuID string) (*resolvedProduct, error) {
	if skuID == "" {
		return nil, nil
	}

	cfg := dependency.FromContext(c).SettingProvider().VAS(c)
	switch productType {
	case ProductTypeStorage:
		var products []storageProductConfig
		if err := json.Unmarshal([]byte(cfg.StorageProducts), &products); err != nil {
			return nil, serializer.NewError(serializer.CodeInternalSetting, "Invalid storage product config", err)
		}
		for _, product := range products {
			if product.ID != skuID {
				continue
			}
			props, _ := json.Marshal(map[string]any{
				"id":       product.ID,
				"name":     product.Name,
				"size":     product.Size,
				"duration": durationDays(product.Time),
				"time":     product.Time,
			})
			return &resolvedProduct{
				ProductType: ProductTypeStorage,
				Name:        product.Name,
				Price:       product.Price,
				Points:      product.Points,
				Props:       props,
			}, nil
		}
	case ProductTypeGroup:
		var products []groupProductConfig
		if err := json.Unmarshal([]byte(cfg.GroupProducts), &products); err != nil {
			return nil, serializer.NewError(serializer.CodeInternalSetting, "Invalid group product config", err)
		}
		for _, product := range products {
			if product.ID != skuID {
				continue
			}
			props, _ := json.Marshal(map[string]any{
				"id":       product.ID,
				"name":     product.Name,
				"group_id": product.GroupID,
				"duration": durationDays(product.Time),
				"time":     product.Time,
			})
			return &resolvedProduct{
				ProductType: ProductTypeGroup,
				Name:        product.Name,
				Price:       product.Price,
				Points:      product.Points,
				Props:       props,
			}, nil
		}
	}

	return nil, serializer.NewError(serializer.CodeParamErr, "Product not found", nil)
}

// durationDays converts the SKU's second-based validity to the whole days
// stored in order properties. Zero means a permanent benefit.
func durationDays(seconds int) int {
	if seconds <= 0 {
		return 0
	}
	return int(math.Ceil(float64(seconds) / 86400))
}

func paymentProviderExists(raw, id string) bool {
	var providers []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &providers); err != nil {
		return false
	}
	for _, provider := range providers {
		if provider.ID == id {
			return true
		}
	}
	return false
}

func createPointPayment(c *gin.Context, dep dependency.Dep, user *ent.User, productType inventory.ProductType,
	name, priceID string, props json.RawMessage, pointsPrice, quantity int64) (*PaymentResponse, error) {
	if user == nil || user.ID <= 0 {
		return nil, serializer.NewError(serializer.CodeNoPermissionErr, "Login required", nil)
	}
	if productType != ProductTypeStorage && productType != ProductTypeGroup && productType != ProductTypeCredit {
		return nil, serializer.NewError(serializer.CodeParamErr, "Unsupported product", nil)
	}
	if pointsPrice <= 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Product does not accept credits", nil)
	}

	totalPoints := pointsPrice * quantity
	creator := user.ID
	tradeNo := strings.ReplaceAll(uuid.NewString(), "-", "")
	payment, err := dep.VASClient().CreatePayment(c, &inventory.PaymentCreateArgs{
		TradeNo:      tradeNo,
		ProductType:  productType,
		ProductProps: props,
		Name:         name,
		PriceMark:    "",
		PriceID:      priceID,
		PriceOneUnit: pointsPrice,
		PriceUnit:    totalPoints,
		Qyt:          quantity,
		Status:       PaymentStatusPaid,
		Creator:      &creator,
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create payment", err)
	}

	_, err = dep.VASClient().ChangeCredit(c, &inventory.CreditChangeArgs{
		UserID:    user.ID,
		Change:    -totalPoints,
		Reason:    "purchase",
		Detail:    name,
		PaymentID: &payment.ID,
	})
	if err != nil {
		_ = dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusCanceled)
		if errors.Is(err, inventory.ErrInsufficientCredit) {
			return nil, serializer.NewError(serializer.CodeInsufficientCredit, "Insufficient credit", err)
		}
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to deduct credit", err)
	}

	audit.Record(c, audit.EventPointsChange, &audit.LogEntry{
		PaymentID:    payment.ID,
		PointsChange: int(-totalPoints),
		Reason:       "purchase",
		Name:         name,
		SKU:          priceID,
	}, audit.WithUser(user.ID))

	audit.Record(c, audit.EventPaymentCreated, &audit.LogEntry{
		PaymentID:    payment.ID,
		SKU:          priceID,
		Name:         name,
		PointsChange: int(-totalPoints),
	})

	if err := fulfillPayment(c, payment); err != nil {
		_ = dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfillFailed)
		_, _ = dep.VASClient().ChangeCredit(c, &inventory.CreditChangeArgs{
			UserID:    user.ID,
			Change:    totalPoints,
			Reason:    "refund",
			Detail:    name,
			PaymentID: &payment.ID,
		})
		audit.Record(c, audit.EventPointsChange, &audit.LogEntry{
			PaymentID:    payment.ID,
			PointsChange: int(totalPoints),
			Reason:       "refund",
			Name:         name,
			SKU:          priceID,
		}, audit.WithUser(user.ID))
		audit.Record(c, audit.EventPaymentFulfillFail, &audit.LogEntry{
			PaymentID: payment.ID,
			SKU:       priceID,
			Name:      name,
			Failed:    true,
		})
		return nil, err
	}

	if err := dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfilled); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update payment", err)
	}
	payment.Status = PaymentStatusFulfilled
	sendPaymentReceipt(c, dep, payment)
	audit.Record(c, audit.EventPaymentFulfilled, &audit.LogEntry{
		PaymentID: payment.ID,
		SKU:       priceID,
		Name:      name,
	})

	res := buildPaymentResponse(payment, dep)
	return &res, nil
}

func buildPaymentResponse(payment *inventory.Payment, dep dependency.Dep) PaymentResponse {
	res := PaymentResponse{
		ID:                  payment.ID,
		CreatedAt:           payment.CreatedAt,
		TradeNo:             payment.TradeNo,
		ProductType:         payment.ProductType,
		ProductProps:        payment.ProductProps,
		Name:                payment.Name,
		PriceUnit:           payment.PriceUnit,
		PriceMark:           payment.PriceMark,
		PriceID:             payment.PriceID,
		PriceOneUnit:        payment.PriceOneUnit,
		Qyt:                 payment.Qyt,
		Status:              payment.Status,
		AnonymousEmail:      payment.AnonymousEmail,
		ProviderReferenceID: payment.ProviderReferenceID,
	}
	if payment.ProviderID != nil {
		res.ProviderID = *payment.ProviderID
	}
	if payment.Creator != nil {
		res.CreatorHashID = hashid.EncodeUserID(dep.HashIDEncoder(), *payment.Creator)
		res.UserHashID = res.CreatorHashID
	}
	attachPaymentCompatibility(&res)
	return res
}

func enrichPaymentUsers(c *gin.Context, dep dependency.Dep, payments []PaymentResponse) {
	for i := range payments {
		if payments[i].CreatorHashID == "" {
			continue
		}
		userID, err := dep.HashIDEncoder().Decode(payments[i].CreatorHashID, hashid.UserID)
		if err != nil {
			continue
		}
		user, err := dep.UserClient().GetByID(c, userID)
		if err != nil {
			continue
		}
		payments[i].Edges = &PaymentEdgesResponse{
			User: &PaymentUserResponse{
				ID:        user.ID,
				Nick:      user.Nick,
				CreatedAt: user.CreatedAt,
			},
		}
	}
}

// applyGiftCodeProduct grants the product described by the gift code.
func applyGiftCodeProduct(c *gin.Context, userID int, product GiftCodeProduct, codeQty int64) error {
	dep := dependency.FromContext(c)
	client := dep.VASClient()

	switch product.Type {
	case ProductTypePoints:
		amount := product.Amount
		if amount <= 0 {
			return nil
		}
		if _, err := client.ChangeCredit(c, &inventory.CreditChangeArgs{
			UserID: userID,
			Change: amount,
			Reason: "gift_code",
			Detail: product.Name,
		}); err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to apply credit", err)
		}
		audit.Record(c, audit.EventPointsChange, &audit.LogEntry{
			PointsChange: int(amount),
			Reason:       "gift_code",
			Name:         product.Name,
		}, audit.WithUser(userID))
	case GiftCodeProductTypeStorage:
		amount := product.Amount
		if amount <= 0 {
			return nil
		}
		var expire time.Time
		if product.Days > 0 {
			expire = time.Now().AddDate(0, 0, product.Days*int(codeQty))
		}
		if _, err := client.CreateStoragePack(c, &inventory.CreateStoragePackArgs{
			UserID:   userID,
			Name:     product.Name,
			Size:     amount,
			ExpireAt: expire,
		}); err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to apply storage pack", err)
		}
		entry := &audit.LogEntry{
			StorageSize: amount,
			Reason:      "gift_code",
			Name:        product.Name,
		}
		if !expire.IsZero() {
			entry.Expire = expire.Format(time.RFC3339)
		}
		audit.Record(c, audit.EventStorageAdded, entry, audit.WithUser(userID))
	case GiftCodeProductTypeGroup:
		if product.Group <= 0 {
			return nil
		}
		days := product.Days
		if codeQty > 0 {
			days *= int(codeQty)
		}
		if err := applyGroupProduct(c, userID, product.Group, days); err != nil {
			return err
		}
	}

	return nil
}

func normalizeGiftCodeProduct(c *gin.Context, product GiftCodeProduct, qyt int64) (GiftCodeProduct, error) {
	switch product.Type {
	case ProductTypePoints:
		if qyt <= 0 {
			return product, serializer.NewError(serializer.CodeParamErr, "Invalid points amount", nil)
		}
		product.Type = GiftCodeProductTypePoints
		product.Amount = qyt
		product.Days = 0
		product.Group = 0
		return product, nil
	case GiftCodeProductTypeStorage, GiftCodeProductTypeGroup:
		if product.SKUID == "" {
			return product, serializer.NewError(serializer.CodeParamErr, "Product SKU is required", nil)
		}

		productType := ProductTypeStorage
		if product.Type == GiftCodeProductTypeGroup {
			productType = ProductTypeGroup
		}
		resolved, err := resolveProduct(c, productType, product.SKUID)
		if err != nil {
			return product, err
		}
		if resolved == nil || len(resolved.Props) == 0 {
			return product, serializer.NewError(serializer.CodeParamErr, "Product not found", nil)
		}

		var props struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Size     int64  `json:"size"`
			GroupID  int    `json:"group_id"`
			Duration int    `json:"duration"`
		}
		if err := json.Unmarshal(resolved.Props, &props); err != nil {
			return product, serializer.NewError(serializer.CodeInternalSetting, "Invalid product config", err)
		}

		product.SKUID = firstNonEmpty(props.ID, product.SKUID)
		product.Name = props.Name
		product.Amount = props.Size
		product.Days = props.Duration
		product.Group = props.GroupID
		if product.Type == GiftCodeProductTypeGroup {
			if _, err := dependency.FromContext(c).GroupClient().GetByID(c, product.Group); err != nil {
				return product, serializer.NewError(serializer.CodeGroupNotFound, "Product group not found", err)
			}
		}
		return product, nil
	default:
		return product, serializer.NewError(serializer.CodeParamErr, "Unsupported gift code product type", nil)
	}
}

func mustMarshalGiftCodeProps(qyt int64, product GiftCodeProduct) json.RawMessage {
	props, _ := json.Marshal(map[string]any{
		"qyt":     qyt,
		"product": product,
	})
	return props
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func fulfillPayment(c *gin.Context, payment *inventory.Payment) error {
	dep := dependency.FromContext(c)

	switch payment.ProductType {
	case ProductTypeStorage:
		var props struct {
			Size     int64  `json:"size"`
			Duration int    `json:"duration"`
			Name     string `json:"name"`
		}
		if len(payment.ProductProps) > 0 {
			_ = json.Unmarshal(payment.ProductProps, &props)
		}

		if props.Size <= 0 {
			return serializer.NewError(serializer.CodeParamErr, "Invalid storage product", nil)
		}
		if payment.Qyt <= 0 || props.Size > math.MaxInt64/payment.Qyt {
			return serializer.NewError(serializer.CodeParamErr, "Invalid storage quantity", nil)
		}
		size := props.Size * payment.Qyt
		if size <= 0 {
			return serializer.NewError(serializer.CodeParamErr, "Invalid storage product", nil)
		}

		var expire time.Time
		if props.Duration > 0 {
			expire = time.Now().AddDate(0, 0, props.Duration*int(payment.Qyt))
		}

		var userID int
		if payment.Creator != nil {
			userID = *payment.Creator
		}
		if userID == 0 {
			return serializer.NewError(serializer.CodeParamErr, "Storage product requires a user", nil)
		}

		_, err := dep.VASClient().CreateStoragePack(c, &inventory.CreateStoragePackArgs{
			UserID:   userID,
			Name:     props.Name,
			Size:     size,
			ExpireAt: expire,
		})
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to fulfill storage product", err)
		}
		entry := &audit.LogEntry{
			PaymentID:   payment.ID,
			StorageSize: size,
			Reason:      "purchase",
			Name:        props.Name,
			SKU:         payment.PriceID,
		}
		if !expire.IsZero() {
			entry.Expire = expire.Format(time.RFC3339)
		}
		audit.Record(c, audit.EventStorageAdded, entry, audit.WithUser(userID))
	case ProductTypeCredit:
		if payment.Creator == nil || payment.Qyt <= 0 {
			return nil
		}
		if _, err := dep.VASClient().ChangeCredit(c, &inventory.CreditChangeArgs{
			UserID:    *payment.Creator,
			Change:    payment.Qyt,
			Reason:    "recharge",
			Detail:    payment.Name,
			PaymentID: &payment.ID,
		}); err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to fulfill credit product", err)
		}
		audit.Record(c, audit.EventPointsChange, &audit.LogEntry{
			PaymentID:    payment.ID,
			PointsChange: int(payment.Qyt),
			Reason:       "recharge",
			Name:         payment.Name,
			SKU:          payment.PriceID,
		}, audit.WithUser(*payment.Creator))
	case ProductTypeGroup:
		if payment.Creator == nil {
			return nil
		}

		var props struct {
			GroupID  int `json:"group_id"`
			Duration int `json:"duration"`
		}
		if len(payment.ProductProps) > 0 {
			_ = json.Unmarshal(payment.ProductProps, &props)
		}
		if payment.Qyt <= 0 || int64(props.Duration) > int64(math.MaxInt)/payment.Qyt {
			return serializer.NewError(serializer.CodeParamErr, "Invalid membership quantity", nil)
		}
		if err := applyGroupProduct(c, *payment.Creator, props.GroupID, props.Duration*int(payment.Qyt)); err != nil {
			return err
		}
	case ProductTypeShareLink:
		if payment.Creator == nil {
			return nil
		}

		var props struct {
			ShareID int `json:"share_id"`
		}
		if len(payment.ProductProps) > 0 {
			_ = json.Unmarshal(payment.ProductProps, &props)
		}
		if props.ShareID <= 0 {
			return serializer.NewError(serializer.CodeParamErr, "Invalid share product", nil)
		}

		ticket := uuid.NewString()
		uid := *payment.Creator
		paymentID := payment.ID
		if _, err := dep.VASClient().CreateSharePurchase(c, &inventory.SharePurchase{
			TicketID:  ticket,
			PaymentID: &paymentID,
			ShareID:   &props.ShareID,
			UserID:    &uid,
		}); err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to fulfill share product", err)
		}
	}

	return nil
}

// sendPaymentReceipt sends a receipt once per order. Delivery failures are
// logged but never roll back an already fulfilled product.
func sendPaymentReceipt(c *gin.Context, dep dependency.Dep, payment *inventory.Payment) {
	if payment == nil || payment.Creator == nil || *payment.Creator <= 0 {
		return
	}

	user, err := dep.UserClient().GetByIDWithGroup(c, *payment.Creator)
	if err != nil {
		dep.Logger().Warning("Failed to load receipt recipient for payment %d: %s", payment.ID, err)
		return
	}
	if user == nil || user.Email == "" {
		return
	}

	if payment.Language != "" {
		if user.Settings == nil {
			user.Settings = &types.UserSetting{}
		}
		user.Settings.Language = payment.Language
	}

	title, body, err := email.NewReceiptEmail(c, dep.SettingProvider(), user,
		email.ReceiptPaymentFromInventory(payment))
	if err != nil {
		if err.Error() != "receipt email template not configured" {
			dep.Logger().Warning("Failed to render receipt email for payment %d: %s", payment.ID, err)
		}
		return
	}
	if err := dep.EmailClient(c).Send(c, user.Email, title, body); err != nil {
		audit.Record(c, audit.EventEmailSent, &audit.LogEntry{
			Failed:     true,
			Error:      err.Error(),
			EmailTo:    user.Email,
			EmailTitle: title,
		}, audit.WithUser(user.ID))
		dep.Logger().Warning("Failed to send receipt email for payment %d: %s", payment.ID, err)
		return
	}

	audit.Record(c, audit.EventEmailSent, &audit.LogEntry{
		EmailTo:    user.Email,
		EmailTitle: title,
	}, audit.WithUser(user.ID))
}

func creditShareOwner(c *gin.Context, share *ent.Share, price int64) error {
	dep := dependency.FromContext(c)
	return creditShareOwnerWithClient(c, c, dep.VASClient(), share, price)
}

func creditShareOwnerWithClient(ctx context.Context, ginCtx *gin.Context, client inventory.VASClient,
	share *ent.Share, price int64) error {
	dep := dependency.FromContext(ginCtx)
	cfg := dep.SettingProvider().VAS(ginCtx)
	if cfg.ShareScoreRate <= 0 || price <= 0 {
		return nil
	}

	commission := price * int64(cfg.ShareScoreRate) / 100
	if commission <= 0 {
		return nil
	}
	// Read the owner from the already-loaded share. Issuing another query here
	// would leave an open cursor on the transaction's single connection, and
	// PostgreSQL then rejects the following UPDATE.
	ownerID := 0
	if share.Edges.User != nil {
		ownerID = share.Edges.User.ID
	}
	if ownerID <= 0 {
		return serializer.NewError(serializer.CodeNotFound, "Share owner not found", nil)
	}
	if _, err := client.ChangeCredit(ctx, &inventory.CreditChangeArgs{
		UserID: ownerID,
		Change: commission,
		Reason: "share_income",
		Detail: fmt.Sprintf("share#%d", share.ID),
	}); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to credit share owner", err)
	}
	audit.Record(ginCtx, audit.EventPointsChange, &audit.LogEntry{
		PointsChange: int(commission),
		Reason:       "share_income",
		Name:         fmt.Sprintf("share#%d", share.ID),
	}, audit.WithUser(ownerID), audit.WithShare(share.ID))
	return nil
}

// applyGroupProduct moves the user into a group for a number of days. Renewals
// of the same, still-active group extend the existing expiry instead of
// replacing it.
func applyGroupProduct(c *gin.Context, userID, groupID, days int) error {
	if groupID <= 0 {
		return serializer.NewError(serializer.CodeParamErr, "Invalid group product", nil)
	}

	client := dependency.FromContext(c).DBClient()
	drv := client.Driver()
	if drv == nil {
		return serializer.NewError(serializer.CodeDBError, "Database driver is unavailable", nil)
	}

	now := time.Now()
	query, args := rebindQuery(drv.Dialect(),
		`SELECT group_users, group_expires FROM users WHERE id = ? LIMIT 1`, userID)
	rows := new(entsql.Rows)
	if err := drv.Query(c, query, args, rows); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to get current group", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return serializer.NewError(serializer.CodeNotFound, "User not found", nil)
	}

	var (
		currentGroup  int
		currentExpire sql.NullTime
	)
	if err := rows.Scan(&currentGroup, &currentExpire); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to scan current group", err)
	}
	rows.Close()

	if days <= 0 {
		query, args = rebindQuery(drv.Dialect(),
			`UPDATE users SET group_users = ?, group_expires = NULL WHERE id = ?`,
			groupID, userID)
	} else {
		expire := now
		if currentGroup == groupID && currentExpire.Valid && currentExpire.Time.After(now) {
			expire = currentExpire.Time
		}
		expire = expire.AddDate(0, 0, days)

		if currentGroup == groupID {
			query, args = rebindQuery(drv.Dialect(),
				`UPDATE users SET group_expires = ? WHERE id = ?`, expire, userID)
		} else {
			query, args = rebindQuery(drv.Dialect(),
				`UPDATE users SET group_users = ?, previous_group = ?, group_expires = ? WHERE id = ?`,
				groupID, currentGroup, expire, userID)
		}
	}

	var res sql.Result
	if err := drv.Exec(c, query, args, &res); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to apply group product", err)
	}

	return nil
}

func rebindPostgres(query string, args []any) (string, []any) {
	var (
		builder strings.Builder
		idx     int
	)
	for _, r := range query {
		if r == '?' {
			idx++
			builder.WriteString("$" + strconv.Itoa(idx))
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String(), args
}

func rebindQuery(dialect, query string, args ...any) (string, []any) {
	if dialect == "postgres" {
		return rebindPostgres(query, args)
	}
	return query, args
}

func validPaymentSignature(ctx context.Context, dep dependency.Dep, tradeNo, signature string) bool {
	if signature == "" {
		return false
	}

	secret := dep.SettingProvider().SecretKey(ctx)
	if secret == "" {
		return false
	}

	sum := sha256.Sum256([]byte(tradeNo + dep.SettingProvider().SecretKey(ctx)))
	return constantTimeEqual(fmt.Sprintf("%x", sum), signature)
}

// paymentCurrency returns the canonical ISO-4217 code configured by the site.
// PriceMark is only a display symbol and must never be sent to gateways.
func paymentCurrency(payment *setting.PaymentSetting, fallback string) string {
	if payment != nil {
		if code := strings.TrimSpace(payment.CurrencyCode); len(code) == 3 {
			return strings.ToUpper(code)
		}
	}
	if code := strings.TrimSpace(fallback); len(code) == 3 {
		return strings.ToUpper(code)
	}
	return "CNY"
}

func signPaymentStatus(dep dependency.Dep, tradeNo string) string {
	sum := sha256.Sum256([]byte(tradeNo + dep.SettingProvider().SecretKey(context.Background())))
	return fmt.Sprintf("%x", sum)
}

// confirmPaymentPaid validates a provider's confirmed transaction against the
// local order, then atomically marks it paid. Any mismatch is rejected before
// state changes so a callback cannot underpay or target another order.
func confirmPaymentPaid(c *gin.Context, dep dependency.Dep, payment *inventory.Payment,
	result *PaymentQueryResult, currency string) error {
	if result == nil || !result.Paid {
		return nil
	}
	if err := validatePaymentConfirmation(payment, result, currency); err != nil {
		return serializer.NewError(serializer.CodeParamErr, err.Error(), err)
	}

	updated, err := dep.VASClient().MarkPaymentPaid(c, payment.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to mark payment paid", err)
	}
	if updated {
		payment.Status = PaymentStatusPaid
		audit.Record(c, audit.EventPaymentPaid, &audit.LogEntry{
			PaymentID: payment.ID,
			SKU:       payment.PriceID,
			Name:      payment.Name,
		})
	}
	return nil
}

// validatePaymentConfirmation rejects incomplete or mismatched provider
// confirmations. All identity and monetary fields are mandatory so a gateway
// cannot confirm a different order, a different currency, or an underpayment
// by omitting fields from its response.
func validatePaymentConfirmation(payment *inventory.Payment,
	result *PaymentQueryResult, currency string) error {
	if payment == nil {
		return errors.New("payment is missing")
	}
	if result == nil || !result.Paid {
		return nil
	}
	if strings.TrimSpace(result.TradeNo) == "" || result.TradeNo != payment.TradeNo {
		return errors.New("Payment trade number mismatch")
	}
	if result.Amount != payment.PriceUnit {
		return errors.New("Payment amount mismatch")
	}
	if strings.TrimSpace(result.Currency) == "" ||
		!strings.EqualFold(result.Currency, currency) {
		return errors.New("Payment currency mismatch")
	}
	return nil
}
