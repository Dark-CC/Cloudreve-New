package vas

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/gin-gonic/gin"
)

// PaymentCallbackService handles asynchronous gateway notifications. It is
// deliberately unauthenticated: every provider payload is verified with its
// own signature before the referenced local order can change state.
type PaymentCallbackService struct {
	TradeNo string `uri:"trade_no" binding:"required"`
}

type PaymentCallbackParamCtx struct{}

const maxPaymentCallbackBody = 1 << 20

// Callback processes Stripe, custom gateway, Alipay and WeChat Pay callbacks.
func (s *PaymentCallbackService) Callback(c *gin.Context) error {
	dep := dependency.FromContext(c)
	payment, err := dep.VASClient().GetPaymentByTradeNo(c, s.TradeNo)
	if err != nil {
		if errors.Is(err, inventory.ErrPaymentNotFound) {
			return serializer.NewError(serializer.CodeNotFound, "Payment not found", err)
		}
		return serializer.NewError(serializer.CodeDBError, "Failed to get payment", err)
	}
	if payment.ProviderID == nil {
		return serializer.NewError(serializer.CodeParamErr, "Payment has no provider", nil)
	}

	provider, err := findPaymentProvider(dep.SettingProvider().VAS(c).Payment, *payment.ProviderID)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, err.Error(), err)
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxPaymentCallbackBody))
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Failed to read callback body", err)
	}

	var result *PaymentQueryResult
	switch strings.ToLower(provider.Type) {
	case paymentProviderCustom:
		result, err = parseCustomCallback(provider, c.Request.Header, body, payment)
	case paymentProviderStripe:
		result, err = parseStripeCallback(provider, c.Request.Header, body, payment)
	case paymentProviderAlipay:
		result, err = parseAlipayCallback(provider, c.Request, payment)
	case paymentProviderWeixin:
		result, err = parseWeixinCallback(provider, c.Request.Header, body, payment)
	default:
		err = errors.New("unsupported payment provider")
	}
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid payment callback", err)
	}
	if result == nil || !result.Paid {
		return nil
	}

	vasSetting := dep.SettingProvider().VAS(c)
	currency := paymentCurrency(vasSetting.Payment, vasSetting.CurrencyCode)
	if err := confirmPaymentPaid(c, dep, payment, result, currency); err != nil {
		return err
	}
	claimed, err := dep.VASClient().MarkPaymentFulfilling(c, payment.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to claim payment", err)
	}
	if !claimed {
		return nil
	}
	if err := fulfillPayment(c, payment); err != nil {
		_ = dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfillFailed)
		return err
	}
	if err := dep.VASClient().UpdatePaymentStatus(c, payment.ID, PaymentStatusFulfilled); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to update payment", err)
	}
	sendPaymentReceipt(c, dep, payment)
	return nil
}

func parseCustomCallback(provider setting.PaymentProviderConfig, header http.Header,
	body []byte, payment *inventory.Payment) (*PaymentQueryResult, error) {
	signature := strings.TrimSpace(header.Get("X-Cloudreve-Signature"))
	if signature == "" {
		signature = strings.TrimSpace(header.Get("X-Signature"))
	}
	if signature == "" || !constantTimeEqual(hmacHex(provider.SecretKey, body), strings.ToLower(signature)) {
		return nil, errors.New("invalid custom payment callback signature")
	}

	var callback struct {
		TradeNo             string `json:"trade_no"`
		ProviderReferenceID string `json:"provider_reference_id"`
		Amount              int64  `json:"amount"`
		Currency            string `json:"currency"`
		Paid                bool   `json:"paid"`
	}
	if err := json.Unmarshal(body, &callback); err != nil {
		return nil, err
	}
	return &PaymentQueryResult{
		Paid:                callback.Paid,
		ProviderReferenceID: callback.ProviderReferenceID,
		Amount:              callback.Amount,
		Currency:            callback.Currency,
		TradeNo:             callback.TradeNo,
	}, nil
}

func parseStripeCallback(provider setting.PaymentProviderConfig, header http.Header,
	body []byte, payment *inventory.Payment) (*PaymentQueryResult, error) {
	if err := verifyStripeSignature(provider.SecretKey, header, body); err != nil {
		return nil, err
	}
	var event struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID            string `json:"id"`
				PaymentStatus string `json:"payment_status"`
				AmountTotal   int64  `json:"amount_total"`
				Currency      string `json:"currency"`
				ClientRef     string `json:"client_reference_id"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	object := event.Data.Object
	return &PaymentQueryResult{
		Paid:                object.PaymentStatus == "paid",
		ProviderReferenceID: object.ID,
		Amount:              object.AmountTotal,
		Currency:            strings.ToUpper(object.Currency),
		TradeNo:             object.ClientRef,
	}, nil
}

func parseAlipayCallback(provider setting.PaymentProviderConfig, request *http.Request,
	payment *inventory.Payment) (*PaymentQueryResult, error) {
	if err := request.ParseForm(); err != nil {
		return nil, err
	}
	if err := verifyAlipayNotify(provider, request.PostForm); err != nil {
		return nil, err
	}
	return &PaymentQueryResult{
		Paid: request.PostForm.Get("trade_status") == "TRADE_SUCCESS" ||
			request.PostForm.Get("trade_status") == "TRADE_FINISHED",
		ProviderReferenceID: request.PostForm.Get("trade_no"),
		Amount:              parseMajorAmount(request.PostForm.Get("total_amount"), 2),
		Currency:            "CNY",
		TradeNo:             request.PostForm.Get("out_trade_no"),
	}, nil
}

func parseWeixinCallback(provider setting.PaymentProviderConfig, header http.Header,
	body []byte, payment *inventory.Payment) (*PaymentQueryResult, error) {
	if err := verifyWeixinSignature(provider, header, body); err != nil {
		return nil, err
	}
	var notification struct {
		Resource struct {
			Algorithm      string `json:"algorithm"`
			Ciphertext     string `json:"ciphertext"`
			AssociatedData string `json:"associated_data"`
			Nonce          string `json:"nonce"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(body, &notification); err != nil {
		return nil, err
	}
	if notification.Resource.Algorithm != "AEAD_AES_256_GCM" {
		return nil, errors.New("unsupported weixin resource algorithm")
	}
	plaintext, err := decryptWeixinResource(provider.SecretKey, notification.Resource.Ciphertext,
		notification.Resource.Nonce, notification.Resource.AssociatedData)
	if err != nil {
		return nil, err
	}
	var transaction struct {
		OutTradeNo string `json:"out_trade_no"`
		TradeState string `json:"trade_state"`
		Amount     struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
		TransactionID string `json:"transaction_id"`
	}
	if err := json.Unmarshal(plaintext, &transaction); err != nil {
		return nil, err
	}
	return &PaymentQueryResult{
		Paid:                transaction.TradeState == "SUCCESS",
		ProviderReferenceID: transaction.TransactionID,
		Amount:              transaction.Amount.Total,
		Currency:            transaction.Amount.Currency,
		TradeNo:             transaction.OutTradeNo,
	}, nil
}

func decryptWeixinResource(apiV3Key, ciphertext, nonce, associatedData string) ([]byte, error) {
	if len(apiV3Key) != 32 {
		return nil, errors.New("weixin API v3 key must be 32 bytes")
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, []byte(nonce), raw, []byte(associatedData))
}
