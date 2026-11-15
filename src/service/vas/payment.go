package vas

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
)

const (
	paymentProviderStripe  = "stripe"
	paymentProviderAlipay  = "alipay"
	paymentProviderWeixin  = "weixin"
	paymentProviderCustom  = "custom"
	paymentHTTPTimeout     = 20 * time.Second
	maxPaymentResponseSize = 2 << 20
)

// PaymentSession is a provider independent description of how a buyer should
// complete payment. Secrets and provider credentials never appear here.
type PaymentSession struct {
	ProviderReferenceID string `json:"provider_reference_id"`
	PaymentURL          string `json:"payment_url,omitempty"`
	QRCode              string `json:"qr_code,omitempty"`
	Signature           string `json:"payment_sign,omitempty"`
}

// PaymentQueryResult reports whether a gateway has confirmed payment. Amount
// and currency are checked by the caller against the local order before any
// state transition.
type PaymentQueryResult struct {
	Paid                bool
	ProviderReferenceID string
	Amount              int64
	Currency            string
	TradeNo             string
}

type paymentAdapter interface {
	Create(ctx context.Context, provider setting.PaymentProviderConfig, order *inventory.Payment, currency string,
		callbackURL string) (*PaymentSession, error)
	Query(ctx context.Context, provider setting.PaymentProviderConfig, order *inventory.Payment, currency string) (*PaymentQueryResult, error)
}

func paymentAdapterFor(provider setting.PaymentProviderConfig) (paymentAdapter, error) {
	switch strings.ToLower(provider.Type) {
	case paymentProviderCustom:
		return customPaymentAdapter{}, nil
	case paymentProviderStripe:
		return stripePaymentAdapter{}, nil
	case paymentProviderAlipay:
		return alipayPaymentAdapter{}, nil
	case paymentProviderWeixin:
		return weixinPaymentAdapter{}, nil
	default:
		return nil, fmt.Errorf("unsupported payment provider type %q", provider.Type)
	}
}

func findPaymentProvider(cfg *setting.PaymentSetting, id string) (setting.PaymentProviderConfig, error) {
	if cfg == nil {
		return setting.PaymentProviderConfig{}, errors.New("payment is not configured")
	}
	for _, provider := range cfg.Providers {
		if provider.ID != id {
			continue
		}
		if provider.Enabled == false && provider.SecretKey == "" && provider.Endpoint == "" &&
			provider.AppID == "" && provider.MerchantID == "" {
			return setting.PaymentProviderConfig{}, errors.New("payment provider is disabled")
		}
		if !provider.Enabled && (provider.SecretKey != "" || provider.Endpoint != "" ||
			provider.AppID != "" || provider.MerchantID != "") {
			// Existing configurations predate the enabled flag and omit it.
			// Treat a configured provider as enabled for compatibility.
			provider.Enabled = true
		}
		return provider, nil
	}
	return setting.PaymentProviderConfig{}, errors.New("payment provider not found")
}

func paymentCallbackURL(base *url.URL, tradeNo string) string {
	callback := *base
	callback.Path = strings.TrimSuffix(callback.Path, "/") + "/api/v4/vas/payment/callback/" + tradeNo
	callback.RawQuery = ""
	callback.Fragment = ""
	return callback.String()
}

func paymentReturnURL(base *url.URL, tradeNo string) string {
	returnURL := *base
	returnURL.Path = strings.TrimSuffix(returnURL.Path, "/") + "/settings"
	query := returnURL.Query()
	query.Set("tab", "finance")
	query.Set("trade_no", tradeNo)
	returnURL.RawQuery = query.Encode()
	returnURL.Fragment = ""
	return returnURL.String()
}

func paymentHTTPClient() *http.Client {
	return &http.Client{Timeout: paymentHTTPTimeout}
}

func readLimitedBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxPaymentResponseSize))
}

// customPaymentAdapter implements the documented custom gateway protocol:
// the site POSTs a JSON order with an HMAC-SHA256 signature, and the gateway
// returns a payment URL and/or QR code plus its reference ID.
type customPaymentAdapter struct{}

type customPaymentRequest struct {
	TradeNo     string `json:"trade_no"`
	Name        string `json:"name"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	CallbackURL string `json:"callback_url"`
	ReturnURL   string `json:"return_url,omitempty"`
}

type customPaymentResponse struct {
	PaymentURL          string `json:"payment_url"`
	QRCode              string `json:"qr_code"`
	ProviderReferenceID string `json:"provider_reference_id"`
	ReferenceID         string `json:"reference_id"`
	Paid                bool   `json:"paid"`
	Amount              int64  `json:"amount"`
	Currency            string `json:"currency"`
}

func (customPaymentAdapter) Create(ctx context.Context, provider setting.PaymentProviderConfig,
	order *inventory.Payment, currency string, callbackURL string) (*PaymentSession, error) {
	if provider.Endpoint == "" || provider.SecretKey == "" {
		return nil, errors.New("custom payment endpoint or secret is missing")
	}
	if err := validateHTTPURL(provider.Endpoint); err != nil {
		return nil, fmt.Errorf("invalid custom payment endpoint: %w", err)
	}

	payload := customPaymentRequest{
		TradeNo:     order.TradeNo,
		Name:        order.Name,
		Amount:      order.PriceUnit,
		Currency:    currency,
		CallbackURL: callbackURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	signature := hmacHex(provider.SecretKey, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Cloudreve-Signature", signature)

	resp, err := paymentHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("custom payment request failed: %w", err)
	}
	respBody, err := readLimitedBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("custom payment returned status %d", resp.StatusCode)
	}

	var result customPaymentResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("invalid custom payment response: %w", err)
	}
	reference := result.ProviderReferenceID
	if reference == "" {
		reference = result.ReferenceID
	}
	if reference == "" {
		reference = order.TradeNo
	}
	if result.PaymentURL == "" && result.QRCode == "" {
		return nil, errors.New("custom payment returned no payment URL or QR code")
	}

	return &PaymentSession{
		ProviderReferenceID: reference,
		PaymentURL:          result.PaymentURL,
		QRCode:              result.QRCode,
	}, nil
}

func (customPaymentAdapter) Query(ctx context.Context, provider setting.PaymentProviderConfig,
	order *inventory.Payment, currency string) (*PaymentQueryResult, error) {
	if provider.Endpoint == "" || provider.SecretKey == "" {
		return nil, errors.New("custom payment endpoint or secret is missing")
	}

	queryURL := strings.TrimRight(provider.Endpoint, "/") + "/" + url.PathEscape(order.TradeNo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("X-Cloudreve-Timestamp", timestamp)
	req.Header.Set("X-Cloudreve-Signature", hmacHex(provider.SecretKey,
		[]byte(order.TradeNo+"\n"+timestamp)))

	resp, err := paymentHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("custom payment query failed: %w", err)
	}
	body, err := readLimitedBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("custom payment query returned status %d", resp.StatusCode)
	}

	var result customPaymentResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid custom payment query response: %w", err)
	}
	reference := result.ProviderReferenceID
	if reference == "" {
		reference = result.ReferenceID
	}
	if reference == "" {
		reference = order.ProviderReferenceID
	}
	return &PaymentQueryResult{
		Paid:                result.Paid,
		ProviderReferenceID: reference,
		Amount:              result.Amount,
		Currency:            result.Currency,
		TradeNo:             order.TradeNo,
	}, nil
}

// stripePaymentAdapter creates a Stripe Checkout Session using form encoded
// requests, avoiding an SDK dependency.
type stripePaymentAdapter struct{}

func (stripePaymentAdapter) Create(ctx context.Context, provider setting.PaymentProviderConfig,
	order *inventory.Payment, currency string, callbackURL string) (*PaymentSession, error) {
	if provider.SecretKey == "" {
		return nil, errors.New("stripe secret key is missing")
	}

	form := url.Values{}
	form.Set("mode", "payment")
	returnURL := strings.Replace(callbackURL, "/api/v4/vas/payment/callback/", "/settings?tab=finance&trade_no=", 1)
	if strings.Contains(returnURL, "?") {
		returnURL += "&payment=success"
	}
	cancelURL := strings.Replace(returnURL, "payment=success", "payment=cancel", 1)
	form.Set("success_url", returnURL)
	form.Set("cancel_url", cancelURL)
	form.Set("client_reference_id", order.TradeNo)
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(currency))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(order.PriceUnit, 10))
	form.Set("line_items[0][price_data][product_data][name]", order.Name)
	form.Set("metadata[trade_no]", order.TradeNo)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.stripe.com/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(provider.SecretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := paymentHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe request failed: %w", err)
	}
	body, err := readLimitedBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stripe returned status %d", resp.StatusCode)
	}
	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid stripe response: %w", err)
	}
	if result.ID == "" || result.URL == "" {
		return nil, errors.New("stripe returned no checkout session")
	}
	return &PaymentSession{ProviderReferenceID: result.ID, PaymentURL: result.URL}, nil
}

func (stripePaymentAdapter) Query(ctx context.Context, provider setting.PaymentProviderConfig,
	order *inventory.Payment, currency string) (*PaymentQueryResult, error) {
	if provider.SecretKey == "" {
		return nil, errors.New("stripe secret key is missing")
	}
	reference := order.ProviderReferenceID
	if reference == "" {
		return nil, errors.New("stripe reference is missing")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.stripe.com/v1/checkout/sessions/"+url.PathEscape(reference), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(provider.SecretKey, "")
	resp, err := paymentHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe query failed: %w", err)
	}
	body, err := readLimitedBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stripe query returned status %d", resp.StatusCode)
	}
	var result struct {
		ID            string `json:"id"`
		PaymentStatus string `json:"payment_status"`
		AmountTotal   int64  `json:"amount_total"`
		Currency      string `json:"currency"`
		ClientRef     string `json:"client_reference_id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid stripe query response: %w", err)
	}
	return &PaymentQueryResult{
		Paid:                result.PaymentStatus == "paid",
		ProviderReferenceID: result.ID,
		Amount:              result.AmountTotal,
		Currency:            strings.ToUpper(result.Currency),
		TradeNo:             result.ClientRef,
	}, nil
}

// alipayPaymentAdapter uses Alipay's precreate API to obtain a QR code.
type alipayPaymentAdapter struct{}

func (alipayPaymentAdapter) Create(ctx context.Context, provider setting.PaymentProviderConfig,
	order *inventory.Payment, currency string, callbackURL string) (*PaymentSession, error) {
	if provider.AppID == "" || provider.SecretKey == "" || provider.PublicKey == "" {
		return nil, errors.New("alipay credentials are incomplete")
	}
	key, err := parseRSAPrivateKey(provider.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("invalid alipay private key: %w", err)
	}
	publicKey, err := parseRSAPublicKey(provider.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid alipay public key: %w", err)
	}

	bizContent, _ := json.Marshal(map[string]string{
		"out_trade_no": order.TradeNo,
		"total_amount": formatMinorAmount(order.PriceUnit, 2),
		"subject":      order.Name,
	})
	params := map[string]string{
		"app_id":      provider.AppID,
		"method":      "alipay.trade.precreate",
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"notify_url":  callbackURL,
		"biz_content": string(bizContent),
	}
	signature, err := signAlipayParams(params, key)
	if err != nil {
		return nil, err
	}
	params["sign"] = signature

	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://openapi.alipay.com/gateway.do", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := paymentHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("alipay request failed: %w", err)
	}
	body, err := readLimitedBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alipay returned status %d", resp.StatusCode)
	}
	var result struct {
		Response struct {
			Code       string `json:"code"`
			Msg        string `json:"msg"`
			QRCode     string `json:"qr_code"`
			OutTradeNo string `json:"out_trade_no"`
		} `json:"alipay_trade_precreate_response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid alipay response: %w", err)
	}
	if err := verifyAlipayJSONResponse(body, publicKey, "alipay_trade_precreate_response"); err != nil {
		return nil, err
	}
	if result.Response.Code != "10000" || result.Response.QRCode == "" {
		return nil, fmt.Errorf("alipay error: %s %s", result.Response.Code, result.Response.Msg)
	}
	return &PaymentSession{
		ProviderReferenceID: order.TradeNo,
		QRCode:              result.Response.QRCode,
	}, nil
}

func (alipayPaymentAdapter) Query(ctx context.Context, provider setting.PaymentProviderConfig,
	order *inventory.Payment, currency string) (*PaymentQueryResult, error) {
	if provider.AppID == "" || provider.SecretKey == "" || provider.PublicKey == "" {
		return nil, errors.New("alipay credentials are incomplete")
	}
	privateKey, err := parseRSAPrivateKey(provider.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("invalid alipay private key: %w", err)
	}
	publicKey, err := parseRSAPublicKey(provider.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid alipay public key: %w", err)
	}

	bizContent, _ := json.Marshal(map[string]string{"out_trade_no": order.TradeNo})
	params := map[string]string{
		"app_id":      provider.AppID,
		"method":      "alipay.trade.query",
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"biz_content": string(bizContent),
	}
	signature, err := signAlipayParams(params, privateKey)
	if err != nil {
		return nil, err
	}
	params["sign"] = signature
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://openapi.alipay.com/gateway.do", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := paymentHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("alipay query failed: %w", err)
	}
	body, err := readLimitedBody(resp)
	if err != nil {
		return nil, err
	}
	var result struct {
		Response struct {
			Code        string `json:"code"`
			Msg         string `json:"msg"`
			TradeStatus string `json:"trade_status"`
			TotalAmount string `json:"total_amount"`
			OutTradeNo  string `json:"out_trade_no"`
		} `json:"alipay_trade_query_response"`
		Sign string `json:"sign"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid alipay query response: %w", err)
	}
	if err := verifyAlipayJSONResponse(body, publicKey, "alipay_trade_query_response"); err != nil {
		return nil, err
	}
	if result.Response.Code != "10000" {
		return &PaymentQueryResult{}, nil
	}
	amount := parseMajorAmount(result.Response.TotalAmount, 2)
	return &PaymentQueryResult{
		Paid:                result.Response.TradeStatus == "TRADE_SUCCESS" || result.Response.TradeStatus == "TRADE_FINISHED",
		ProviderReferenceID: order.TradeNo,
		Amount:              amount,
		Currency:            "CNY",
		TradeNo:             result.Response.OutTradeNo,
	}, nil
}

// weixinPaymentAdapter creates a Native (QR code) transaction via API v3.
type weixinPaymentAdapter struct{}

type weixinNativeResponse struct {
	CodeURL string `json:"code_url"`
}

func (weixinPaymentAdapter) Create(ctx context.Context, provider setting.PaymentProviderConfig,
	order *inventory.Payment, currency string, callbackURL string) (*PaymentSession, error) {
	if provider.AppID == "" || provider.MerchantID == "" || provider.SecretKey == "" ||
		provider.CertificateSerial == "" || provider.APIPrivateKey == "" {
		return nil, errors.New("weixin payment credentials are incomplete")
	}
	key, err := parseRSAPrivateKey(provider.APIPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid weixin API private key: %w", err)
	}

	payload := map[string]any{
		"appid":        provider.AppID,
		"mchid":        provider.MerchantID,
		"description":  order.Name,
		"out_trade_no": order.TradeNo,
		"notify_url":   callbackURL,
		"amount": map[string]any{
			"total":    order.PriceUnit,
			"currency": currency,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	target := "https://api.mch.weixin.qq.com/v3/pay/transactions/native"
	authorization, err := weixinAuthorization(provider, key, http.MethodPost, target, body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", authorization)

	resp, err := paymentHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("weixin request failed: %w", err)
	}
	respBody, err := readLimitedBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("weixin returned status %d", resp.StatusCode)
	}
	var result weixinNativeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("invalid weixin response: %w", err)
	}
	if result.CodeURL == "" {
		return nil, errors.New("weixin returned no code_url")
	}
	return &PaymentSession{ProviderReferenceID: order.TradeNo, QRCode: result.CodeURL}, nil
}

func (weixinPaymentAdapter) Query(ctx context.Context, provider setting.PaymentProviderConfig,
	order *inventory.Payment, currency string) (*PaymentQueryResult, error) {
	if provider.AppID == "" || provider.MerchantID == "" || provider.SecretKey == "" ||
		provider.CertificateSerial == "" || provider.APIPrivateKey == "" {
		return nil, errors.New("weixin payment credentials are incomplete")
	}
	key, err := parseRSAPrivateKey(provider.APIPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid weixin API private key: %w", err)
	}
	target := "https://api.mch.weixin.qq.com/v3/pay/transactions/out-trade-no/" +
		url.PathEscape(order.TradeNo) + "?mchid=" + url.QueryEscape(provider.MerchantID)
	authorization, err := weixinAuthorization(provider, key, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", authorization)
	resp, err := paymentHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("weixin query failed: %w", err)
	}
	body, err := readLimitedBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("weixin query returned status %d", resp.StatusCode)
	}
	var result struct {
		TradeState string `json:"trade_state"`
		OutTradeNo string `json:"out_trade_no"`
		Amount     struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid weixin query response: %w", err)
	}
	return &PaymentQueryResult{
		Paid:                result.TradeState == "SUCCESS",
		ProviderReferenceID: order.TradeNo,
		Amount:              result.Amount.Total,
		Currency:            result.Amount.Currency,
		TradeNo:             result.OutTradeNo,
	}, nil
}

func weixinAuthorization(provider setting.PaymentProviderConfig, key *rsa.PrivateKey,
	method, target string, body []byte) (string, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomHex(16)
	message := method + "\n" + target + "\n" + timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",timestamp="%s",serial_no="%s",signature="%s"`,
		provider.MerchantID, nonce, timestamp, provider.CertificateSerial,
		base64.StdEncoding.EncodeToString(signature)), nil
}

func signAlipayParams(params map[string]string, key *rsa.PrivateKey) (string, error) {
	keys := make([]string, 0, len(params))
	for name, value := range params {
		if name == "sign" || name == "sign_type" || value == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var content strings.Builder
	for index, name := range keys {
		if index > 0 {
			content.WriteByte('&')
		}
		content.WriteString(name)
		content.WriteByte('=')
		content.WriteString(params[name])
	}
	digest := sha256.Sum256([]byte(content.String()))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// verifyAlipayParams verifies an asynchronous Alipay notification. It uses
// RSA2 when sign_type says so and RSA (SHA1) otherwise, matching Alipay.
func verifyAlipayParams(params url.Values, publicKey *rsa.PublicKey) error {
	signatureText := params.Get("sign")
	signType := params.Get("sign_type")
	if signatureText == "" {
		return errors.New("missing alipay signature")
	}
	signature, err := base64.StdEncoding.DecodeString(signatureText)
	if err != nil {
		return errors.New("invalid alipay signature encoding")
	}

	keys := make([]string, 0, len(params))
	for name := range params {
		if name == "sign" || name == "sign_type" || len(params[name]) == 0 {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var content strings.Builder
	for index, name := range keys {
		if index > 0 {
			content.WriteByte('&')
		}
		content.WriteString(name)
		content.WriteByte('=')
		content.WriteString(params.Get(name))
	}

	var hashType crypto.Hash
	var digest []byte
	if signType == "RSA" {
		sum := sha1.Sum([]byte(content.String()))
		digest = sum[:]
		hashType = crypto.SHA1
	} else {
		sum := sha256.Sum256([]byte(content.String()))
		digest = sum[:]
		hashType = crypto.SHA256
	}
	return rsa.VerifyPKCS1v15(publicKey, hashType, digest, signature)
}

// verifyAlipayJSONResponse verifies the response node signature. Alipay signs
// the exact JSON object value of the response node, not a re-serialized map.
func verifyAlipayJSONResponse(body []byte, publicKey *rsa.PublicKey, node string) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("invalid alipay JSON response: %w", err)
	}
	signatureText := ""
	if raw, ok := envelope["sign"]; ok {
		_ = json.Unmarshal(raw, &signatureText)
	}
	if signatureText == "" {
		return errors.New("alipay response is not signed")
	}
	content, ok := envelope[node]
	if !ok {
		return fmt.Errorf("alipay response node %q is missing", node)
	}
	signature, err := base64.StdEncoding.DecodeString(signatureText)
	if err != nil {
		return errors.New("invalid alipay response signature encoding")
	}
	digest := sha256.Sum256(content)
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return errors.New("invalid alipay response signature")
	}
	return nil
}

func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil {
		block = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte(strings.TrimSpace(raw))}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not RSA")
	}
	return rsaKey, nil
}

func parseRSAPublicKey(raw string) (*rsa.PublicKey, error) {
	cleaned := strings.TrimSpace(raw)
	block, _ := pem.Decode([]byte(cleaned))
	if block != nil {
		cleaned = string(block.Bytes)
	}
	if key, err := x509.ParsePKIXPublicKey([]byte(cleaned)); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PublicKey([]byte(cleaned)); err == nil {
		return key, nil
	}

	// Alipay often displays public keys as bare base64 DER. Handle both
	// standard and URL-safe alphabets.
	normalized := strings.NewReplacer("\n", "", "\r", "", " ", "").Replace(cleaned)
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		der, err := encoding.DecodeString(normalized)
		if err != nil {
			continue
		}
		if key, err := x509.ParsePKIXPublicKey(der); err == nil {
			if rsaKey, ok := key.(*rsa.PublicKey); ok {
				return rsaKey, nil
			}
		}
		if key, err := x509.ParsePKCS1PublicKey(der); err == nil {
			return key, nil
		}
	}
	return nil, errors.New("unable to parse RSA public key")
}

func hmacHex(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

func formatMinorAmount(amount int64, digits int) string {
	divisor := int64(1)
	for index := 0; index < digits; index++ {
		divisor *= 10
	}
	return fmt.Sprintf("%d.%0*d", amount/divisor, digits, amount%divisor)
}

func parseMajorAmount(raw string, digits int) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	parts := strings.SplitN(raw, ".", 2)
	major, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	for len(fraction) < digits {
		fraction += "0"
	}
	minor, _ := strconv.ParseInt(fraction[:digits], 10, 64)
	return major*int64(mathPow10(digits)) + minor
}

func mathPow10(exp int) int {
	result := 1
	for index := 0; index < exp; index++ {
		result *= 10
	}
	return result
}

func validateHTTPURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("only http and https URLs are supported")
	}
	if parsed.Host == "" {
		return errors.New("host is required")
	}
	return nil
}

func weixinPlatformCertificate(provider setting.PaymentProviderConfig) string {
	if provider.PlatformCertificate != "" {
		return provider.PlatformCertificate
	}
	var config struct {
		PlatformCertificate string `json:"platform_certificate"`
	}
	_ = json.Unmarshal(provider.Config, &config)
	return config.PlatformCertificate
}

// verifyWeixinSignature validates API v3 callback signatures with the platform
// certificate configured at the provider level or in provider.Config.
func verifyWeixinSignature(provider setting.PaymentProviderConfig, header http.Header, body []byte) error {
	platformCertificate := weixinPlatformCertificate(provider)
	if platformCertificate == "" {
		return errors.New("weixin platform certificate is not configured")
	}

	timestamp := header.Get("Wechatpay-Timestamp")
	nonce := header.Get("Wechatpay-Nonce")
	signatureText := header.Get("Wechatpay-Signature")
	if timestamp == "" || nonce == "" || signatureText == "" {
		return errors.New("missing weixin signature headers")
	}
	if ts, err := strconv.ParseInt(timestamp, 10, 64); err != nil ||
		time.Since(time.Unix(ts, 0)) > 5*time.Minute || time.Until(time.Unix(ts, 0)) > 5*time.Minute {
		return errors.New("stale weixin callback timestamp")
	}

	block, _ := pem.Decode([]byte(platformCertificate))
	if block == nil {
		return errors.New("invalid weixin platform certificate")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	publicKey, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("weixin platform certificate is not RSA")
	}
	signature, err := base64.StdEncoding.DecodeString(signatureText)
	if err != nil {
		return errors.New("invalid weixin signature encoding")
	}
	message := timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	digest := sha256.Sum256([]byte(message))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return errors.New("invalid weixin signature")
	}

	serial := header.Get("Wechatpay-Serial")
	if serial != "" {
		serialNumber := new(big.Int).SetBytes(certificate.SerialNumber.Bytes())
		if strings.ToUpper(serial) != strings.ToUpper(serialNumber.Text(16)) {
			return errors.New("weixin certificate serial mismatch")
		}
	}
	return nil
}

func verifyStripeSignature(secret string, header http.Header, body []byte) error {
	signatureHeader := header.Get("Stripe-Signature")
	if signatureHeader == "" {
		return errors.New("missing stripe signature")
	}

	var timestamp string
	var signatures []string
	for _, part := range strings.Split(signatureHeader, ",") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 {
			continue
		}
		switch pair[0] {
		case "t":
			timestamp = pair[1]
		case "v1":
			signatures = append(signatures, pair[1])
		}
	}
	if timestamp == "" || len(signatures) == 0 {
		return errors.New("invalid stripe signature header")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(ts, 0)) > 5*time.Minute || time.Until(time.Unix(ts, 0)) > 5*time.Minute {
		return errors.New("stale stripe signature")
	}
	expected := hmacHex(secret, append([]byte(timestamp+"."), body...))
	for _, signature := range signatures {
		if constantTimeEqual(expected, signature) {
			return nil
		}
	}
	return errors.New("invalid stripe signature")
}

// verifyAlipayNotify is used by the public callback handler.
func verifyAlipayNotify(provider setting.PaymentProviderConfig, values url.Values) error {
	if provider.PublicKey == "" {
		return errors.New("alipay public key is missing")
	}
	publicKey, err := parseRSAPublicKey(provider.PublicKey)
	if err != nil {
		return err
	}
	return verifyAlipayParams(values, publicKey)
}
