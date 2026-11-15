package vas

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
)

func TestCustomPaymentCreateAndQuery(t *testing.T) {
	secret := "test-secret"
	var created bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if !constantTimeEqual(hmacDigest(secret, body), r.Header.Get("X-Cloudreve-Signature")) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			created = true
			_, _ = w.Write([]byte(`{"payment_url":"https://pay.example/checkout/1","provider_reference_id":"ref-1"}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"paid":true,"amount":1999,"currency":"CNY","provider_reference_id":"ref-1"}`))
		}
	}))
	defer server.Close()

	provider := setting.PaymentProviderConfig{Type: "custom", Endpoint: server.URL, SecretKey: secret}
	order := &inventory.Payment{TradeNo: "trade-1", Name: "Storage", PriceUnit: 1999, PriceMark: "CNY"}
	adapter := customPaymentAdapter{}
	session, err := adapter.Create(t.Context(), provider, order, "CNY",
		"https://site.example/api/v4/vas/payment/callback/trade-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !created || session.PaymentURL != "https://pay.example/checkout/1" || session.ProviderReferenceID != "ref-1" {
		t.Fatalf("unexpected session: %#v", session)
	}
	result, err := adapter.Query(t.Context(), provider, order, "CNY")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !result.Paid || result.Amount != 1999 || result.Currency != "CNY" {
		t.Fatalf("unexpected query result: %#v", result)
	}
}

func TestCustomPaymentCallbackRejectsBadSignature(t *testing.T) {
	provider := setting.PaymentProviderConfig{Type: "custom", SecretKey: "secret"}
	body := []byte(`{"trade_no":"trade-1","paid":true,"amount":100}`)
	header := http.Header{}
	header.Set("X-Cloudreve-Signature", "bad")
	if _, err := parseCustomCallback(provider, header, body, &inventory.Payment{}); err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}
	header.Set("X-Cloudreve-Signature", hmacDigest(provider.SecretKey, body))
	if _, err := parseCustomCallback(provider, header, body, &inventory.Payment{}); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestStripeSignatureRejectsTamperingAndReplayAge(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"type":"checkout.session.completed"}`)
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join([]string{strconv.FormatInt(timestamp, 10), string(body)}, ".")))
	header := http.Header{}
	header.Set("Stripe-Signature", "t="+strconv.FormatInt(timestamp, 10)+",v1="+hex.EncodeToString(mac.Sum(nil)))
	if err := verifyStripeSignature(secret, header, body); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := verifyStripeSignature(secret, header, []byte(`{"type":"tampered"}`)); err == nil {
		t.Fatal("tampered body accepted")
	}
	header.Set("Stripe-Signature", "t=1,v1="+hex.EncodeToString(mac.Sum(nil)))
	if err := verifyStripeSignature(secret, header, body); err == nil {
		t.Fatal("stale signature accepted")
	}
}

func TestAlipayNotifySignature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{}
	values.Set("out_trade_no", "trade-1")
	values.Set("trade_status", "TRADE_SUCCESS")
	values.Set("total_amount", "19.99")
	values.Set("sign_type", "RSA2")
	signature, err := signAlipayParams(flatValues(values), key)
	if err != nil {
		t.Fatal(err)
	}
	values.Set("sign", signature)
	if err := verifyAlipayParams(values, &key.PublicKey); err != nil {
		t.Fatalf("valid alipay signature rejected: %v", err)
	}
	values.Set("total_amount", "0.01")
	if err := verifyAlipayParams(values, &key.PublicKey); err == nil {
		t.Fatal("tampered alipay notification accepted")
	}
}

func TestWeixinResourceDecryption(t *testing.T) {
	plaintext := []byte(`{"out_trade_no":"trade-1","trade_state":"SUCCESS","amount":{"total":100,"currency":"CNY"}}`)
	key := []byte("01234567890123456789012345678901")
	nonce := []byte("123456789012")
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, []byte("associated"))
	got, err := decryptWeixinResource(string(key), base64.StdEncoding.EncodeToString(ciphertext), string(nonce), "associated")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("decrypted payload mismatch: %s", got)
	}
}

func TestWeixinPlatformCertificateResolution(t *testing.T) {
	tests := []struct {
		name     string
		provider setting.PaymentProviderConfig
		want     string
	}{
		{
			name:     "top level certificate",
			provider: setting.PaymentProviderConfig{PlatformCertificate: "top-level"},
			want:     "top-level",
		},
		{
			name:     "config certificate fallback",
			provider: setting.PaymentProviderConfig{Config: json.RawMessage(`{"platform_certificate":"config"}`)},
			want:     "config",
		},
		{
			name: "top level certificate takes precedence",
			provider: setting.PaymentProviderConfig{
				PlatformCertificate: "top-level",
				Config:              json.RawMessage(`{"platform_certificate":"config"}`),
			},
			want: "top-level",
		},
		{
			name: "empty top level certificate falls back",
			provider: setting.PaymentProviderConfig{
				PlatformCertificate: "",
				Config:              json.RawMessage(`{"platform_certificate":"config"}`),
			},
			want: "config",
		},
		{
			name:     "missing certificate",
			provider: setting.PaymentProviderConfig{},
			want:     "",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := weixinPlatformCertificate(testCase.provider); got != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}

func TestPaymentCurrencyUsesConfiguredISOCode(t *testing.T) {
	payment := &setting.PaymentSetting{CurrencyCode: "usd"}
	if got := paymentCurrency(payment, "CNY"); got != "USD" {
		t.Fatalf("expected USD, got %q", got)
	}
	if got := paymentCurrency(nil, "eur"); got != "EUR" {
		t.Fatalf("expected EUR fallback, got %q", got)
	}
	if got := paymentCurrency(nil, "¥"); got != "CNY" {
		t.Fatalf("display symbol must not be used as a currency code, got %q", got)
	}
}

func TestValidatePaymentConfirmation(t *testing.T) {
	order := &inventory.Payment{
		TradeNo:   "trade-1",
		PriceUnit: 1999,
	}
	valid := &PaymentQueryResult{
		Paid:     true,
		TradeNo:  "trade-1",
		Amount:   1999,
		Currency: "CNY",
	}
	tests := []struct {
		name     string
		result   *PaymentQueryResult
		currency string
		wantErr  bool
	}{
		{name: "valid", result: valid, currency: "CNY"},
		{name: "unpaid is ignored", result: &PaymentQueryResult{Paid: false}, currency: "CNY"},
		{name: "wrong amount", result: &PaymentQueryResult{Paid: true, TradeNo: "trade-1", Amount: 1, Currency: "CNY"}, currency: "CNY", wantErr: true},
		{name: "zero amount is rejected", result: &PaymentQueryResult{Paid: true, TradeNo: "trade-1", Amount: 0, Currency: "CNY"}, currency: "CNY", wantErr: true},
		{name: "negative amount is rejected", result: &PaymentQueryResult{Paid: true, TradeNo: "trade-1", Amount: -1999, Currency: "CNY"}, currency: "CNY", wantErr: true},
		{name: "wrong currency", result: &PaymentQueryResult{Paid: true, TradeNo: "trade-1", Amount: 1999, Currency: "USD"}, currency: "CNY", wantErr: true},
		{name: "missing currency", result: &PaymentQueryResult{Paid: true, TradeNo: "trade-1", Amount: 1999}, currency: "CNY", wantErr: true},
		{name: "wrong trade number", result: &PaymentQueryResult{Paid: true, TradeNo: "trade-2", Amount: 1999, Currency: "CNY"}, currency: "CNY", wantErr: true},
		{name: "missing trade number", result: &PaymentQueryResult{Paid: true, Amount: 1999, Currency: "CNY"}, currency: "CNY", wantErr: true},
		{name: "currency is case insensitive", result: &PaymentQueryResult{Paid: true, TradeNo: "trade-1", Amount: 1999, Currency: "cny"}, currency: "CNY"},
		{name: "nil result", result: nil, currency: "CNY"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := validatePaymentConfirmation(order, testCase.result, testCase.currency)
			if testCase.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
	if err := validatePaymentConfirmation(nil, valid, "CNY"); err == nil {
		t.Fatal("expected missing local payment to be rejected")
	}
}

func hmacDigest(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func flatValues(values url.Values) map[string]string {
	res := make(map[string]string, len(values))
	for key, value := range values {
		res[key] = value[0]
	}
	return res
}
