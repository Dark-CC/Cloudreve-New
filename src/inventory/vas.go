package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/google/uuid"
)

var (
	ErrGiftCodeNotFound      = errors.New("gift code not found")
	ErrGiftCodeUsed          = errors.New("gift code already used")
	ErrPaymentNotFound       = errors.New("payment not found")
	ErrSharePurchaseNotFound = errors.New("share purchase not found")
	ErrInsufficientCredit    = errors.New("insufficient credit")
)

type (
	// ProductType mirrors Pro's payment product types.
	ProductType int

	// GiftCode is an redeemable gift code record.
	GiftCode struct {
		ID           int
		CreatedAt    time.Time
		UpdatedAt    time.Time
		Code         string
		Used         bool
		Qyt          int64
		ProductProps json.RawMessage
		UsedBy       *int
	}

	// GiftCodeArgs describes a gift code to create.
	GiftCodeArgs struct {
		Qyt          int64
		ProductProps json.RawMessage
		Code         string
	}

	// StoragePack is a purchased quota expansion with an optional expiry.
	StoragePack struct {
		ID          int
		Name        string
		Size        int64
		ActiveSince *time.Time
		ExpireAt    *time.Time
		UserID      *int
	}

	// Payment records an order created for a product.
	Payment struct {
		ID                  int
		CreatedAt           time.Time
		UpdatedAt           time.Time
		TradeNo             string
		ProductType         ProductType
		ProductProps        json.RawMessage
		ProviderID          *string
		Name                string
		PriceUnit           int64
		PriceMark           string
		PriceID             string
		ProviderReferenceID string
		PriceOneUnit        int64
		Qyt                 int64
		Status              string
		AnonymousEmail      string
		Language            string
		Creator             *int
	}

	// PaymentCreateArgs describes a new order.
	PaymentCreateArgs struct {
		TradeNo             string
		ProductType         ProductType
		ProductProps        json.RawMessage
		ProviderID          *string
		ProviderReferenceID string
		Name                string
		PriceUnit           int64
		PriceMark           string
		PriceID             string
		PriceOneUnit        int64
		Qyt                 int64
		Status              string
		Creator             *int
		AnonymousEmail      string
		Language            string
	}

	// PaymentListArgs filters payments.
	PaymentListArgs struct {
		*PaginationArgs
		Status         string
		ProductType    ProductType
		HasProductType bool
		ProviderID     string
		Creator        int
		TradeNo        string
		AnonymousEmail string
	}

	// GiftCodeListArgs filters gift codes.
	GiftCodeListArgs struct {
		*PaginationArgs
		Used         *bool
		Code         string
		CodeContains string
		UserID       int
	}

	PaymentListResult struct {
		*PaginationResults
		Payments []*Payment
	}

	// SharePurchase grants a user access to a paid share.
	SharePurchase struct {
		ID        int
		CreatedAt time.Time
		UpdatedAt time.Time
		TicketID  string
		PaymentID *int
		ShareID   *int
		UserID    *int
	}

	// CreditChange records one credit ledger entry.
	CreditChange struct {
		ID        int
		CreatedAt time.Time
		UserID    int
		Change    int64
		Balance   int64
		Reason    string
		Detail    string
		PaymentID *int
	}

	CreditChangeArgs struct {
		UserID    int
		Change    int64
		Reason    string
		Detail    string
		PaymentID *int
	}

	// VASClient stores credits, gift codes, storage packs, payments and share
	// purchases.
	VASClient interface {
		TxOperator
		// AddCredit atomically adds delta to the user's credit balance and
		// returns the new balance.
		AddCredit(ctx context.Context, userID int, delta int64) (int64, error)
		// ChangeCredit atomically changes credit and appends a ledger entry.
		// It rejects a negative balance by default.
		ChangeCredit(ctx context.Context, args *CreditChangeArgs) (*CreditChange, error)
		// Credit returns the user's current credit balance.
		Credit(ctx context.Context, userID int) (int64, error)
		// ListCreditChanges lists ledger entries for a user.
		ListCreditChanges(ctx context.Context, userID int, args *PaginationArgs) ([]*CreditChange, *PaginationResults, error)
		// SetCredit overwrites the user's credit balance.
		SetCredit(ctx context.Context, userID int, value int64) error

		// CreateGiftCodes inserts count new codes.
		CreateGiftCodes(ctx context.Context, args *GiftCodeArgs, count int) ([]*GiftCode, error)
		// ListGiftCodes lists gift codes.
		ListGiftCodes(ctx context.Context, args *GiftCodeListArgs) ([]*GiftCode, *PaginationResults, error)
		// GetGiftCode looks up a gift code by its UUID.
		GetGiftCode(ctx context.Context, code string) (*GiftCode, error)
		// RedeemGiftCode marks the code as used by the user.
		RedeemGiftCode(ctx context.Context, code string, userID int) (*GiftCode, error)
		// ReleaseGiftCode reverts a claim when fulfillment fails.
		ReleaseGiftCode(ctx context.Context, code string, userID int) error
		// DeleteGiftCode removes a gift code.
		DeleteGiftCode(ctx context.Context, id int) error
		// UpdateGiftCode updates an unused gift code.
		UpdateGiftCode(ctx context.Context, id int, qyt int64, productProps json.RawMessage) (*GiftCode, error)

		// CreateStoragePack grants a storage pack to a user.
		CreateStoragePack(ctx context.Context, args *CreateStoragePackArgs) (*StoragePack, error)
		// ListStoragePacks lists storage packs, optionally for one user.
		ListStoragePacks(ctx context.Context, userID int) ([]*StoragePack, error)
		// TotalStoragePackSize returns the sum of non-expired pack sizes.
		TotalStoragePackSize(ctx context.Context, userID int, now time.Time) (int64, error)
		// ExpireStoragePacks marks expired packs as expired for the user.
		ExpireStoragePacks(ctx context.Context, now time.Time) (int64, error)

		// CreatePayment inserts an order.
		CreatePayment(ctx context.Context, args *PaymentCreateArgs) (*Payment, error)
		// GetPaymentByTradeNo looks up an order by trade number.
		GetPaymentByTradeNo(ctx context.Context, tradeNo string) (*Payment, error)
		// GetPaymentByID looks up an order by ID.
		GetPaymentByID(ctx context.Context, id int) (*Payment, error)
		// ListPayments lists orders.
		ListPayments(ctx context.Context, args *PaymentListArgs) (*PaymentListResult, error)
		// UpdatePaymentStatus sets the order status.
		UpdatePaymentStatus(ctx context.Context, id int, status string) error
		// MarkPaymentPaid atomically transitions created -> paid.
		MarkPaymentPaid(ctx context.Context, id int) (bool, error)
		// MarkPaymentFulfilling atomically claims an order for fulfillment.
		MarkPaymentFulfilling(ctx context.Context, id int) (bool, error)
		// RecoverStaleFulfillingPayments releases orders abandoned mid-fulfillment.
		RecoverStaleFulfillingPayments(ctx context.Context, olderThan time.Time) (int64, error)
		// SetPaymentProviderReference stores the gateway's payment identifier.
		SetPaymentProviderReference(ctx context.Context, id int, reference string) error
		// DeletePayments batch deletes orders by ID.
		DeletePayments(ctx context.Context, ids ...int) error

		// CreateSharePurchase records a share purchase.
		CreateSharePurchase(ctx context.Context, purchase *SharePurchase) (*SharePurchase, error)
		// GetSharePurchaseByTicket looks up a purchase by ticket.
		GetSharePurchaseByTicket(ctx context.Context, ticket string) (*SharePurchase, error)
		// HasSharePurchase reports whether the user already bought the share.
		HasSharePurchase(ctx context.Context, shareID, userID int) (bool, error)
		// DeleteSharePurchasesByShare removes purchases for a deleted share.
		DeleteSharePurchasesByShare(ctx context.Context, shareID int) error
	}
)

// NewVASClient creates a SQL backed VAS client.
func NewVASClient(client *ent.Client) VASClient {
	return &vasClient{client: client}
}

type vasClient struct {
	client *ent.Client
}

func (c *vasClient) SetClient(newClient *ent.Client) TxOperator {
	return &vasClient{client: newClient}
}

func (c *vasClient) GetClient() *ent.Client {
	return c.client
}

func (c *vasClient) AddCredit(ctx context.Context, userID int, delta int64) (int64, error) {
	query, args := c.rebind(`UPDATE users SET credit = credit + ? WHERE id = ?`, delta, userID)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return 0, fmt.Errorf("failed to update credit: %w", err)
	}

	return c.Credit(ctx, userID)
}

func (c *vasClient) ChangeCredit(ctx context.Context, args *CreditChangeArgs) (*CreditChange, error) {
	if args.UserID <= 0 {
		return nil, fmt.Errorf("invalid user")
	}
	args.Reason = normalizeCreditReason(args.Reason)

	now := time.Now()
	query, queryArgs := c.rebind(
		`UPDATE users SET credit = credit + ? WHERE id = ? AND credit + ? >= 0`,
		args.Change, args.UserID, args.Change,
	)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, queryArgs, &res); err != nil {
		return nil, fmt.Errorf("failed to update credit: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check credit update: %w", err)
	}
	if affected == 0 {
		if _, err := c.Credit(ctx, args.UserID); err != nil {
			return nil, err
		}
		return nil, ErrInsufficientCredit
	}

	balance, err := c.Credit(ctx, args.UserID)
	if err != nil {
		return nil, err
	}

	entry := &CreditChange{
		CreatedAt: now,
		UserID:    args.UserID,
		Change:    args.Change,
		Balance:   balance,
		Reason:    args.Reason,
		Detail:    args.Detail,
		PaymentID: args.PaymentID,
	}
	insert, insertArgs := c.rebind(
		`INSERT INTO credit_changes (created_at, user_id, change, balance, reason, detail, payment_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		now, args.UserID, args.Change, balance, args.Reason, args.Detail, args.PaymentID,
	)
	lastID, err := c.insertReturningID(ctx, insert, insertArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to record credit change: %w", err)
	}
	entry.ID = int(lastID)

	return entry, nil
}

// normalizeCreditReason maps internal ledger reason identifiers to the values
// rendered by the finance page.
func normalizeCreditReason(reason string) string {
	switch reason {
	case "purchase":
		return "pay"
	case "share_income":
		return "share_purchased"
	case "share_purchase":
		return "pay"
	}
	return reason
}

func (c *vasClient) ListCreditChanges(ctx context.Context, userID int, args *PaginationArgs) ([]*CreditChange, *PaginationResults, error) {
	pageSize := args.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}

	total, err := c.count(ctx, `SELECT COUNT(*) FROM credit_changes WHERE user_id = ?`, userID)
	if err != nil {
		return nil, nil, err
	}

	query, queryArgs := c.rebind(
		`SELECT id, created_at, user_id, change, balance, reason, detail, payment_id
FROM credit_changes WHERE user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		userID, pageSize, args.Page*pageSize,
	)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, queryArgs, rows); err != nil {
		return nil, nil, fmt.Errorf("failed to list credit changes: %w", err)
	}
	defer rows.Close()

	res := make([]*CreditChange, 0)
	for rows.Next() {
		entry := &CreditChange{}
		var (
			detail    sql.NullString
			paymentID sql.NullInt64
		)
		if err := rows.Scan(&entry.ID, &entry.CreatedAt, &entry.UserID, &entry.Change, &entry.Balance, &entry.Reason, &detail, &paymentID); err != nil {
			return nil, nil, fmt.Errorf("failed to scan credit change: %w", err)
		}
		entry.Detail = detail.String
		if paymentID.Valid {
			id := int(paymentID.Int64)
			entry.PaymentID = &id
		}
		res = append(res, entry)
	}

	return res, &PaginationResults{TotalItems: total, Page: args.Page, PageSize: pageSize}, nil
}

func (c *vasClient) Credit(ctx context.Context, userID int) (int64, error) {
	query, args := c.rebind(`SELECT credit FROM users WHERE id = ?`, userID)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return 0, fmt.Errorf("failed to query credit: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, fmt.Errorf("user %d not found", userID)
	}

	var credit sql.NullInt64
	if err := rows.Scan(&credit); err != nil {
		return 0, fmt.Errorf("failed to scan credit: %w", err)
	}

	return credit.Int64, nil
}

func (c *vasClient) SetCredit(ctx context.Context, userID int, value int64) error {
	query, args := c.rebind(`UPDATE users SET credit = ? WHERE id = ?`, value, userID)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to set credit: %w", err)
	}
	return nil
}

func (c *vasClient) CreateGiftCodes(ctx context.Context, args *GiftCodeArgs, count int) ([]*GiftCode, error) {
	if count <= 0 {
		return nil, nil
	}

	props := args.ProductProps
	if len(props) == 0 {
		props = json.RawMessage("{}")
	}

	res := make([]*GiftCode, 0, count)
	now := time.Now()
	for i := 0; i < count; i++ {
		code := args.Code
		if code == "" {
			code = uuid.NewString()
		} else if count > 1 {
			code = uuid.NewString()
		}

		query, queryArgs := c.rebind(
			`INSERT INTO gift_codes (created_at, updated_at, code, used, qyt, product_props) VALUES (?, ?, ?, ?, ?, ?)`,
			now, now, code, false, args.Qyt, string(props),
		)
	lastID, err := c.insertReturningID(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gift code: %w", err)
	}

		res = append(res, &GiftCode{
			ID:           int(lastID),
			CreatedAt:    now,
			UpdatedAt:    now,
			Code:         code,
			Used:         false,
			Qyt:          args.Qyt,
			ProductProps: props,
		})
	}

	return res, nil
}

func (c *vasClient) ListGiftCodes(ctx context.Context, args *GiftCodeListArgs) ([]*GiftCode, *PaginationResults, error) {
	whereSQL, queryArgs := giftCodeListWhere(args)
	countQuery, countArgs := c.rebind(`SELECT COUNT(*) FROM gift_codes WHERE `+whereSQL, queryArgs...)
	total, err := c.count(ctx, countQuery, countArgs...)
	if err != nil {
		return nil, nil, err
	}

	var pagination *PaginationArgs
	if args != nil {
		pagination = args.PaginationArgs
	}
	if pagination == nil {
		pagination = &PaginationArgs{}
	}

	pageSize := pagination.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}

	orderBy, orderDirection := giftCodeListOrder(args)
	query, queryArgs := c.rebind(
		`SELECT id, created_at, updated_at, code, used, qyt, product_props, used_by
FROM gift_codes WHERE `+whereSQL+` ORDER BY `+orderBy+` `+orderDirection+` LIMIT ? OFFSET ?`,
		append(queryArgs, pageSize, pagination.Page*pageSize)...,
	)

	codes, err := c.queryGiftCodes(ctx, query, queryArgs)
	if err != nil {
		return nil, nil, err
	}

	return codes, &PaginationResults{TotalItems: total, Page: pagination.Page, PageSize: pageSize}, nil
}

func giftCodeListWhere(args *GiftCodeListArgs) (string, []any) {
	where := []string{"deleted_at IS NULL"}
	queryArgs := make([]any, 0, 4)
	if args == nil {
		return strings.Join(where, " AND "), queryArgs
	}

	if args.Used != nil {
		where = append(where, "used = ?")
		queryArgs = append(queryArgs, *args.Used)
	}
	if args.Code != "" {
		where = append(where, "code = ?")
		queryArgs = append(queryArgs, args.Code)
	}
	if args.UserID > 0 {
		where = append(where, "used_by = ?")
		queryArgs = append(queryArgs, args.UserID)
	}
	if args.CodeContains != "" {
		where = append(where, "LOWER(code) LIKE LOWER(?)")
		queryArgs = append(queryArgs, "%"+args.CodeContains+"%")
	}

	return strings.Join(where, " AND "), queryArgs
}

func giftCodeListOrder(args *GiftCodeListArgs) (string, string) {
	orderBy := "id"
	orderDirection := "DESC"
	if args == nil {
		return orderBy, orderDirection
	}
	if args.PaginationArgs != nil {
		switch args.OrderBy {
		case "id", "created_at", "used", "qyt":
			orderBy = args.OrderBy
		}
		if args.Order == OrderDirectionAsc {
			orderDirection = "ASC"
		}
	}
	return orderBy, orderDirection
}

func (c *vasClient) GetGiftCode(ctx context.Context, code string) (*GiftCode, error) {
	query, args := c.rebind(
		`SELECT id, created_at, updated_at, code, used, qyt, product_props, used_by
FROM gift_codes WHERE code = ? AND deleted_at IS NULL LIMIT 1`, code,
	)

	codes, err := c.queryGiftCodes(ctx, query, args)
	if err != nil {
		return nil, err
	}
	if len(codes) == 0 {
		return nil, ErrGiftCodeNotFound
	}

	return codes[0], nil
}

func (c *vasClient) RedeemGiftCode(ctx context.Context, code string, userID int) (*GiftCode, error) {
	gift, err := c.GetGiftCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if gift.Used {
		return nil, ErrGiftCodeUsed
	}

	query, args := c.rebind(
		`UPDATE gift_codes SET used = ?, used_by = ?, updated_at = ? WHERE id = ? AND used = ?`,
		true, userID, time.Now(), gift.ID, false,
	)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return nil, fmt.Errorf("failed to redeem gift code: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check redeem result: %w", err)
	}
	if affected == 0 {
		return nil, ErrGiftCodeUsed
	}

	usedBy := userID
	gift.Used = true
	gift.UsedBy = &usedBy
	return gift, nil
}

func (c *vasClient) ReleaseGiftCode(ctx context.Context, code string, userID int) error {
	query, args := c.rebind(
		`UPDATE gift_codes SET used = ?, used_by = NULL, updated_at = ? WHERE code = ? AND used = ? AND used_by = ?`,
		false, time.Now(), code, true, userID,
	)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to release gift code: %w", err)
	}
	return nil
}

func (c *vasClient) DeleteGiftCode(ctx context.Context, id int) error {
	query, args := c.rebind(`UPDATE gift_codes SET deleted_at = ? WHERE id = ?`, time.Now(), id)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to delete gift code: %w", err)
	}
	return nil
}

func (c *vasClient) UpdateGiftCode(ctx context.Context, id int, qyt int64, productProps json.RawMessage) (*GiftCode, error) {
	if len(productProps) == 0 {
		productProps = json.RawMessage("{}")
	}

	query, args := c.rebind(
		`UPDATE gift_codes SET qyt = ?, product_props = ?, updated_at = ? WHERE id = ? AND used = ? AND deleted_at IS NULL`,
		qyt, string(productProps), time.Now(), id, false,
	)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return nil, fmt.Errorf("failed to update gift code: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check gift code update: %w", err)
	}
	if affected == 0 {
		return nil, ErrGiftCodeUsed
	}

	query, args = c.rebind(
		`SELECT id, created_at, updated_at, code, used, qyt, product_props, used_by
FROM gift_codes WHERE id = ? AND deleted_at IS NULL LIMIT 1`, id,
	)
	codes, err := c.queryGiftCodes(ctx, query, args)
	if err != nil {
		return nil, err
	}
	if len(codes) == 0 {
		return nil, ErrGiftCodeNotFound
	}
	return codes[0], nil
}

func (c *vasClient) CreateStoragePack(ctx context.Context, args *CreateStoragePackArgs) (*StoragePack, error) {
	if args.Size <= 0 {
		return nil, fmt.Errorf("invalid storage pack size")
	}
	if args.Size > (1<<63-1)/2 {
		return nil, fmt.Errorf("storage pack size is too large")
	}
	now := time.Now()
	var expire any
	if !args.ExpireAt.IsZero() {
		expire = args.ExpireAt
	}
	query, queryArgs := c.rebind(
		`INSERT INTO storage_packs (name, active_since, expire_at, size, user_storage_packs) VALUES (?, ?, ?, ?, ?)`,
		args.Name, now, expire, args.Size, args.UserID,
	)
	lastID, err := c.insertReturningID(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage pack: %w", err)
	}

	var expireAt *time.Time
	if !args.ExpireAt.IsZero() {
		expireAt = &args.ExpireAt
	}

	userID := args.UserID
	return &StoragePack{
		ID:          int(lastID),
		Name:        args.Name,
		Size:        args.Size,
		ActiveSince: &now,
		ExpireAt:    expireAt,
		UserID:      &userID,
	}, nil
}

func (c *vasClient) ListStoragePacks(ctx context.Context, userID int) ([]*StoragePack, error) {
	query := `SELECT id, name, active_since, expire_at, size, user_storage_packs FROM storage_packs`
	var args []any
	if userID > 0 {
		query += ` WHERE user_storage_packs = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY id DESC`
	query, args = c.rebind(query, args...)

	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to list storage packs: %w", err)
	}
	defer rows.Close()

	var res []*StoragePack
	for rows.Next() {
		pack := &StoragePack{}
		var (
			activeSince, expireAt sql.NullTime
			owner                 sql.NullInt64
		)
		if err := rows.Scan(&pack.ID, &pack.Name, &activeSince, &expireAt, &pack.Size, &owner); err != nil {
			return nil, fmt.Errorf("failed to scan storage pack: %w", err)
		}
		if activeSince.Valid {
			pack.ActiveSince = &activeSince.Time
		}
		if expireAt.Valid {
			pack.ExpireAt = &expireAt.Time
		}
		if owner.Valid {
			id := int(owner.Int64)
			pack.UserID = &id
		}
		res = append(res, pack)
	}

	return res, nil
}

func (c *vasClient) TotalStoragePackSize(ctx context.Context, userID int, now time.Time) (int64, error) {
	query, args := c.rebind(
		`SELECT COALESCE(SUM(size), 0) FROM storage_packs WHERE user_storage_packs = ? AND (expire_at IS NULL OR expire_at > ?)`,
		userID, now,
	)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return 0, fmt.Errorf("failed to sum storage packs: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, nil
	}

	var total sql.NullInt64
	if err := rows.Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to scan storage pack total: %w", err)
	}

	return total.Int64, nil
}

func (c *vasClient) ExpireStoragePacks(ctx context.Context, now time.Time) (int64, error) {
	query, args := c.rebind(
		`DELETE FROM storage_packs WHERE expire_at IS NOT NULL AND expire_at <= ?`, now,
	)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return 0, fmt.Errorf("failed to expire storage packs: %w", err)
	}

	return res.RowsAffected()
}

func (c *vasClient) CreatePayment(ctx context.Context, args *PaymentCreateArgs) (*Payment, error) {
	props := args.ProductProps
	if len(props) == 0 {
		props = json.RawMessage("{}")
	}

	status := args.Status
	if status == "" {
		status = "created"
	}

	now := time.Now()
	query, queryArgs := c.rebind(
		`INSERT INTO payments (created_at, updated_at, trade_no, product_type, product_props, provider_id, name,
price_unit, price_mark, price_id, provider_reference_id, price_one_unit, qyt, status, anonymous_email, language, creator)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		now, now, args.TradeNo, int(args.ProductType), string(props), args.ProviderID, args.Name,
		args.PriceUnit, args.PriceMark, args.PriceID, args.ProviderReferenceID, args.PriceOneUnit, args.Qyt, status,
		args.AnonymousEmail, args.Language, args.Creator,
	)
	lastID, err := c.insertReturningID(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	return &Payment{
		ID:                  int(lastID),
		CreatedAt:           now,
		UpdatedAt:           now,
		TradeNo:             args.TradeNo,
		ProductType:         args.ProductType,
		ProductProps:        props,
		ProviderID:          args.ProviderID,
		Name:                args.Name,
		PriceUnit:           args.PriceUnit,
		PriceMark:           args.PriceMark,
		PriceID:             args.PriceID,
		ProviderReferenceID: args.ProviderReferenceID,
		PriceOneUnit:        args.PriceOneUnit,
		Qyt:                 args.Qyt,
		Status:              status,
		AnonymousEmail:      args.AnonymousEmail,
		Language:            args.Language,
		Creator:             args.Creator,
	}, nil
}

func (c *vasClient) GetPaymentByTradeNo(ctx context.Context, tradeNo string) (*Payment, error) {
	query, args := c.rebind(
		`SELECT id, created_at, updated_at, trade_no, product_type, product_props, provider_id, name, price_unit,
price_mark, price_id, provider_reference_id, price_one_unit, qyt, status, anonymous_email, language, creator
FROM payments WHERE trade_no = ? AND deleted_at IS NULL LIMIT 1`, tradeNo,
	)
	payments, err := c.queryPayments(ctx, query, args)
	if err != nil {
		return nil, err
	}
	if len(payments) == 0 {
		return nil, ErrPaymentNotFound
	}
	return payments[0], nil
}

func (c *vasClient) GetPaymentByID(ctx context.Context, id int) (*Payment, error) {
	query, args := c.rebind(
		`SELECT id, created_at, updated_at, trade_no, product_type, product_props, provider_id, name, price_unit,
price_mark, price_id, provider_reference_id, price_one_unit, qyt, status, anonymous_email, language, creator
FROM payments WHERE id = ? AND deleted_at IS NULL LIMIT 1`, id,
	)
	payments, err := c.queryPayments(ctx, query, args)
	if err != nil {
		return nil, err
	}
	if len(payments) == 0 {
		return nil, ErrPaymentNotFound
	}
	return payments[0], nil
}

func (c *vasClient) ListPayments(ctx context.Context, args *PaymentListArgs) (*PaymentListResult, error) {
	var (
		where     []string
		queryArgs []any
	)
	where = append(where, "deleted_at IS NULL")

	if args.Status != "" {
		where = append(where, "status = ?")
		queryArgs = append(queryArgs, args.Status)
	}
	if args.HasProductType {
		where = append(where, "product_type = ?")
		queryArgs = append(queryArgs, int(args.ProductType))
	}
	if args.ProviderID != "" {
		where = append(where, "provider_id = ?")
		queryArgs = append(queryArgs, args.ProviderID)
	}
	if args.Creator > 0 {
		where = append(where, "creator = ?")
		queryArgs = append(queryArgs, args.Creator)
	}
	if args.TradeNo != "" {
		where = append(where, "trade_no = ?")
		queryArgs = append(queryArgs, args.TradeNo)
	}
	if args.AnonymousEmail != "" {
		where = append(where, "anonymous_email = ?")
		queryArgs = append(queryArgs, args.AnonymousEmail)
	}

	whereSQL := strings.Join(where, " AND ")
	countQuery, countArgs := c.rebind(`SELECT COUNT(*) FROM payments WHERE `+whereSQL, queryArgs...)
	total, err := c.count(ctx, countQuery, countArgs...)
	if err != nil {
		return nil, err
	}

	pageSize := args.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}

	order := "DESC"
	if strings.EqualFold(string(args.Order), "asc") {
		order = "ASC"
	}

	query, finalArgs := c.rebind(
		`SELECT id, created_at, updated_at, trade_no, product_type, product_props, provider_id, name, price_unit,
price_mark, price_id, provider_reference_id, price_one_unit, qyt, status, anonymous_email, language, creator
FROM payments WHERE `+whereSQL+` ORDER BY id `+order+` LIMIT ? OFFSET ?`,
		append(queryArgs, pageSize, args.Page*pageSize)...,
	)

	payments, err := c.queryPayments(ctx, query, finalArgs)
	if err != nil {
		return nil, err
	}

	return &PaymentListResult{
		Payments: payments,
		PaginationResults: &PaginationResults{
			TotalItems: total,
			Page:       args.Page,
			PageSize:   pageSize,
		},
	}, nil
}

func (c *vasClient) UpdatePaymentStatus(ctx context.Context, id int, status string) error {
	query, args := c.rebind(`UPDATE payments SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now(), id)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}
	return nil
}

// MarkPaymentPaid atomically moves an order from created to paid. It returns
// false when another request already changed the status, which keeps payment
// fulfillment idempotent.
func (c *vasClient) MarkPaymentPaid(ctx context.Context, id int) (bool, error) {
	query, args := c.rebind(
		`UPDATE payments SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		"paid", time.Now(), id, "created",
	)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return false, fmt.Errorf("failed to mark payment paid: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to check payment update: %w", err)
	}
	return affected > 0, nil
}

// MarkPaymentFulfilling atomically claims an order so only one worker grants
// its benefits. Failed orders may be retried by an administrator.
func (c *vasClient) MarkPaymentFulfilling(ctx context.Context, id int) (bool, error) {
	query, args := c.rebind(
		`UPDATE payments SET status = ?, updated_at = ? WHERE id = ? AND status IN (?, ?, ?)`,
		"fulfilling", time.Now(), id, "created", "paid", "fulfill_failed",
	)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return false, fmt.Errorf("failed to claim payment fulfillment: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to check payment claim: %w", err)
	}
	return affected > 0, nil
}

// RecoverStaleFulfillingPayments moves orders left in fulfilling by a crashed
// worker back to fulfill_failed so they can be retried by an administrator.
// Fulfillment itself is each caller's responsibility to make idempotent.
func (c *vasClient) RecoverStaleFulfillingPayments(ctx context.Context, olderThan time.Time) (int64, error) {
	query, args := c.rebind(
		`UPDATE payments SET status = ?, updated_at = ? WHERE status = ? AND updated_at < ?`,
		"fulfill_failed", time.Now(), "fulfilling", olderThan,
	)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return 0, fmt.Errorf("failed to recover stale fulfilling payments: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to check recovered payments: %w", err)
	}
	return affected, nil
}

// SetPaymentProviderReference stores the external gateway identifier used to
// reconcile callbacks without trusting client supplied order data.
func (c *vasClient) SetPaymentProviderReference(ctx context.Context, id int, reference string) error {
	query, args := c.rebind(
		`UPDATE payments SET provider_reference_id = ?, updated_at = ? WHERE id = ?`,
		reference, time.Now(), id,
	)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to update payment provider reference: %w", err)
	}
	return nil
}

func (c *vasClient) DeletePayments(ctx context.Context, ids ...int) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, time.Now())
	for _, id := range ids {
		args = append(args, id)
	}

	query, args := c.rebind(`UPDATE payments SET deleted_at = ? WHERE id IN (`+placeholders+`)`, args...)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to delete payments: %w", err)
	}
	return nil
}

func (c *vasClient) CreateSharePurchase(ctx context.Context, purchase *SharePurchase) (*SharePurchase, error) {
	now := time.Now()
	query, args := c.rebind(
		`INSERT INTO share_purchases (created_at, updated_at, ticket_id, payment_id, share_id, user_id)
VALUES (?, ?, ?, ?, ?, ?)`,
		now, now, purchase.TicketID, purchase.PaymentID, purchase.ShareID, purchase.UserID,
	)
	lastID, err := c.insertReturningID(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to create share purchase: %w", err)
	}

	purchase.ID = int(lastID)
	purchase.CreatedAt = now
	purchase.UpdatedAt = now
	return purchase, nil
}

func (c *vasClient) GetSharePurchaseByTicket(ctx context.Context, ticket string) (*SharePurchase, error) {
	if ticket == "" {
		return nil, ErrSharePurchaseNotFound
	}

	query, args := c.rebind(
		`SELECT id, created_at, updated_at, ticket_id, payment_id, share_id, user_id
FROM share_purchases WHERE ticket_id = ? AND deleted_at IS NULL LIMIT 1`, ticket,
	)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to query share purchase: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, ErrSharePurchaseNotFound
	}

	return scanSharePurchase(rows.ColumnScanner)
}

func (c *vasClient) HasSharePurchase(ctx context.Context, shareID, userID int) (bool, error) {
	query, args := c.rebind(
		`SELECT COUNT(*) FROM share_purchases WHERE share_id = ? AND user_id = ? AND deleted_at IS NULL`,
		shareID, userID,
	)
	count, err := c.count(ctx, query, args...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (c *vasClient) DeleteSharePurchasesByShare(ctx context.Context, shareID int) error {
	query, args := c.rebind(`UPDATE share_purchases SET deleted_at = ? WHERE share_id = ?`, time.Now(), shareID)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to delete share purchases: %w", err)
	}
	return nil
}

func (c *vasClient) queryGiftCodes(ctx context.Context, query string, args []any) ([]*GiftCode, error) {
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to query gift codes: %w", err)
	}
	defer rows.Close()

	var res []*GiftCode
	for rows.Next() {
		code := &GiftCode{}
		var (
			props  sql.NullString
			usedBy sql.NullInt64
		)
		if err := rows.Scan(&code.ID, &code.CreatedAt, &code.UpdatedAt, &code.Code, &code.Used, &code.Qyt, &props, &usedBy); err != nil {
			return nil, fmt.Errorf("failed to scan gift code: %w", err)
		}
		if props.Valid {
			code.ProductProps = json.RawMessage(props.String)
		}
		if usedBy.Valid {
			id := int(usedBy.Int64)
			code.UsedBy = &id
		}
		res = append(res, code)
	}

	return res, nil
}

func (c *vasClient) queryPayments(ctx context.Context, query string, args []any) ([]*Payment, error) {
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to query payments: %w", err)
	}
	defer rows.Close()

	var res []*Payment
	for rows.Next() {
		payment, err := scanPayment(rows.ColumnScanner)
		if err != nil {
			return nil, err
		}
		res = append(res, payment)
	}

	return res, nil
}

func (c *vasClient) count(ctx context.Context, query string, args ...any) (int, error) {
	// Placeholder syntax differs per dialect (SQLite/MySQL use ?, Postgres uses
	// $1..$n). Every caller passes positional placeholders, so rebind here to
	// keep counted queries working on Postgres.
	query, args = c.rebind(query, args...)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return 0, fmt.Errorf("failed to count rows: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, nil
	}

	var total int
	if err := rows.Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to scan count: %w", err)
	}
	return total, nil
}

// insertReturningID executes an INSERT and returns the generated id.
//
// SQLite and MySQL report the id through sql.Result.LastInsertId, but the
// Postgres driver returns an error for that call. Postgres therefore needs an
// explicit RETURNING clause, which the other dialects do not support.
func (c *vasClient) insertReturningID(ctx context.Context, query string, args ...any) (int64, error) {
	if c.client.Driver().Dialect() == dialect.Postgres {
		query, args = c.rebind(query+" RETURNING id", args...)
		rows := new(entsql.Rows)
		if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
			return 0, err
		}
		defer rows.Close()

		if !rows.Next() {
			return 0, sql.ErrNoRows
		}

		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}

	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (c *vasClient) rebind(query string, args ...any) (string, []any) {
	if c.client.Driver().Dialect() != dialect.Postgres {
		return query, args
	}

	var (
		builder strings.Builder
		argIdx  int
	)
	for _, r := range query {
		if r == '?' {
			argIdx++
			builder.WriteString(fmt.Sprintf("$%d", argIdx))
			continue
		}
		builder.WriteRune(r)
	}

	return builder.String(), args
}

func scanPayment(rows entsql.ColumnScanner) (*Payment, error) {
	payment := &Payment{}
	var (
		props       sql.NullString
		providerID  sql.NullString
		priceMark   sql.NullString
		providerRef sql.NullString
		anonymEmail sql.NullString
		language    sql.NullString
		creator     sql.NullInt64
	)
	if err := rows.Scan(
		&payment.ID,
		&payment.CreatedAt,
		&payment.UpdatedAt,
		&payment.TradeNo,
		&payment.ProductType,
		&props,
		&providerID,
		&payment.Name,
		&payment.PriceUnit,
		&priceMark,
		&payment.PriceID,
		&providerRef,
		&payment.PriceOneUnit,
		&payment.Qyt,
		&payment.Status,
		&anonymEmail,
		&language,
		&creator,
	); err != nil {
		return nil, fmt.Errorf("failed to scan payment: %w", err)
	}

	if props.Valid {
		payment.ProductProps = json.RawMessage(props.String)
	}
	if providerID.Valid {
		id := providerID.String
		payment.ProviderID = &id
	}
	payment.PriceMark = priceMark.String
	payment.ProviderReferenceID = providerRef.String
	payment.AnonymousEmail = anonymEmail.String
	payment.Language = language.String
	if creator.Valid {
		id := int(creator.Int64)
		payment.Creator = &id
	}

	return payment, nil
}

func scanSharePurchase(rows entsql.ColumnScanner) (*SharePurchase, error) {
	purchase := &SharePurchase{}
	var (
		ticket    sql.NullString
		paymentID sql.NullInt64
		shareID   sql.NullInt64
		userID    sql.NullInt64
	)
	if err := rows.Scan(&purchase.ID, &purchase.CreatedAt, &purchase.UpdatedAt, &ticket, &paymentID, &shareID, &userID); err != nil {
		return nil, fmt.Errorf("failed to scan share purchase: %w", err)
	}

	purchase.TicketID = ticket.String
	if paymentID.Valid {
		id := int(paymentID.Int64)
		purchase.PaymentID = &id
	}
	if shareID.Valid {
		id := int(shareID.Int64)
		purchase.ShareID = &id
	}
	if userID.Valid {
		id := int(userID.Int64)
		purchase.UserID = &id
	}

	return purchase, nil
}
