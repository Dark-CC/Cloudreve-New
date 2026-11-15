package basic

import (
	"testing"

	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
)

func TestPublicPaymentSettingStripsCredentials(t *testing.T) {
	src := &setting.PaymentSetting{
		CurrencyCode: "CNY",
		CurrencyMark: "¥",
		CurrencyUnit: 100,
		Providers: []setting.PaymentProviderConfig{
			{
				ID:                "stripe-live",
				Name:              "Stripe",
				Type:              "stripe",
				Enabled:           true,
				SecretKey:         "sk_live_secret",
				Endpoint:          "https://gateway.example/pay",
				AppID:             "app-id",
				PublicKey:         "public-key",
				MerchantID:        "merchant-id",
				CertificateSerial: "serial",
				APIPrivateKey:     "api-private-key",
			},
		},
	}

	got := publicPaymentSetting(src)
	if got == nil || len(got.Providers) != 1 {
		t.Fatalf("expected one public provider, got %#v", got)
	}
	provider := got.Providers[0]
	if provider.ID != "stripe-live" || provider.Name != "Stripe" || provider.Type != "stripe" {
		t.Fatalf("public fields were not preserved: %#v", provider)
	}
	if provider.SecretKey != "" || provider.Endpoint != "" || provider.AppID != "" ||
		provider.PublicKey != "" || provider.MerchantID != "" ||
		provider.CertificateSerial != "" || provider.APIPrivateKey != "" {
		t.Fatalf("provider credentials leaked: %#v", provider)
	}
	if src.Providers[0].SecretKey != "sk_live_secret" {
		t.Fatal("public projection mutated the source configuration")
	}
}

func TestPublicPaymentSettingExposesLocalProvider(t *testing.T) {
	got := publicPaymentSetting(&setting.PaymentSetting{
		CurrencyCode: "CNY",
		CurrencyMark: "¥",
		CurrencyUnit: 100,
		LocalEnabled: true,
		Providers:    []setting.PaymentProviderConfig{},
	})
	if got == nil || len(got.Providers) != 1 {
		t.Fatalf("expected local provider, got %#v", got)
	}
	if got.Providers[0].ID != "local" || got.Providers[0].Type != "local" {
		t.Fatalf("unexpected local provider: %#v", got.Providers[0])
	}
}
