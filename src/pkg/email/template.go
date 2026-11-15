package email

import (
	"context"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strings"
	"text/template"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
)

type CommonContext struct {
	SiteBasic *setting.SiteBasic
	Logo      *setting.Logo
	SiteUrl   string
}

// ResetContext used for variables in reset email
type ResetContext struct {
	*CommonContext
	User *ent.User
	Url  string
}

// NewResetEmail generates reset email from template
func NewResetEmail(ctx context.Context, settings setting.Provider, user *ent.User, url string) (string, string, error) {
	templates := settings.ResetEmailTemplate(ctx)
	if len(templates) == 0 {
		return "", "", fmt.Errorf("reset email template not configured")
	}

	selected := selectTemplate(templates, user)
	resetCtx := ResetContext{
		CommonContext: commonContext(ctx, settings),
		User:          user,
		Url:           url,
	}

	tmplTitle, err := htmltemplate.New("resetTitle").Parse(selected.Title)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse email title: %w", err)
	}

	var resTitle strings.Builder
	err = tmplTitle.Execute(&resTitle, resetCtx.templateData())
	if err != nil {
		return "", "", fmt.Errorf("failed to execute email title: %w", err)
	}

	tmplBody, err := htmltemplate.New("resetBody").Parse(selected.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse email template: %w", err)
	}

	var resBody strings.Builder
	err = tmplBody.Execute(&resBody, resetCtx.templateData())
	if err != nil {
		return "", "", fmt.Errorf("failed to execute email template: %w", err)
	}

	return resTitle.String(), resBody.String(), nil
}

// ActivationContext used for variables in activation email
type ActivationContext struct {
	*CommonContext
	User *ent.User
	Url  string
}

// ReceiptContext contains the variables available to payment receipt emails.
type ReceiptContext struct {
	*CommonContext
	User    *ent.User
	Payment map[string]any
	Url     string
}

// QuotaExceedContext contains the variables available to quota exceeded emails.
type QuotaExceedContext struct {
	*CommonContext
	User *ent.User
	Days int
	Url  string
}

// NewActivationEmail generates activation email from template
func NewActivationEmail(ctx context.Context, settings setting.Provider, user *ent.User, url string) (string, string, error) {
	templates := settings.ActivationEmailTemplate(ctx)
	if len(templates) == 0 {
		return "", "", fmt.Errorf("activation email template not configured")
	}

	selected := selectTemplate(templates, user)
	activationCtx := ActivationContext{
		CommonContext: commonContext(ctx, settings),
		User:          user,
		Url:           url,
	}

	tmplTitle, err := htmltemplate.New("activationTitle").Parse(selected.Title)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse email title: %w", err)
	}

	var resTitle strings.Builder
	err = tmplTitle.Execute(&resTitle, activationCtx.templateData())
	if err != nil {
		return "", "", fmt.Errorf("failed to execute email title: %w", err)
	}

	tmplBody, err := htmltemplate.New("activationBody").Parse(selected.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse email template: %w", err)
	}

	var resBody strings.Builder
	err = tmplBody.Execute(&resBody, activationCtx.templateData())
	if err != nil {
		return "", "", fmt.Errorf("failed to execute email template: %w", err)
	}

	return resTitle.String(), resBody.String(), nil
}

// ReceiptPayment is the data exposed to receipt email templates.
type ReceiptPayment struct {
	ID           int
	TradeNo      string
	Name         string
	PriceOneUnit int64
	PriceMark    string
	PriceID      string
	PriceUnit    int64
	Qyt          int64
}

// NewReceiptEmail generates a payment receipt email from the configured template.
func NewReceiptEmail(ctx context.Context, settings setting.Provider, user *ent.User,
	payment any) (string, string, error) {
	templates := settings.ReceiptEmailTemplate(ctx)
	if len(templates) == 0 {
		return "", "", fmt.Errorf("receipt email template not configured")
	}

	selected := selectTemplate(templates, user)
	receiptCtx := ReceiptContext{
		CommonContext: commonContext(ctx, settings),
		User:          user,
		Payment:       receiptPaymentData(payment),
		Url:           "",
	}
	return renderEmailTemplates("receipt", selected, receiptCtx.templateData())
}

func receiptPaymentData(payment any) map[string]any {
	switch value := payment.(type) {
	case ReceiptPayment:
		return value.templateData()
	case *ReceiptPayment:
		if value == nil {
			return ReceiptPayment{}.templateData()
		}
		return value.templateData()
	case map[string]any:
		return value
	case *inventory.Payment:
		return ReceiptPaymentFromInventory(value).templateData()
	default:
		return ReceiptPayment{}.templateData()
	}
}

// ReceiptPaymentFromInventory converts a stored payment into receipt template data.
func ReceiptPaymentFromInventory(payment *inventory.Payment) ReceiptPayment {
	if payment == nil {
		return ReceiptPayment{}
	}

	return ReceiptPayment{
		ID:           payment.ID,
		TradeNo:      payment.TradeNo,
		Name:         payment.Name,
		PriceOneUnit: payment.PriceOneUnit,
		PriceMark:    payment.PriceMark,
		PriceID:      payment.PriceID,
		PriceUnit:    payment.PriceUnit,
		Qyt:          payment.Qyt,
	}
}

// NewQuotaExceedEmail generates a storage quota exceeded notification email.
func NewQuotaExceedEmail(ctx context.Context, settings setting.Provider, user *ent.User,
	days int) (string, string, error) {
	templates := settings.QuotaExceedEmailTemplate(ctx)
	if len(templates) == 0 {
		return "", "", fmt.Errorf("quota exceed email template not configured")
	}

	selected := selectTemplate(templates, user)
	quotaCtx := QuotaExceedContext{
		CommonContext: commonContext(ctx, settings),
		User:          user,
		Days:          days,
		Url:           "",
	}
	return renderEmailTemplates("quota", selected, quotaCtx.templateData())
}

func renderEmailTemplates(name string, selected setting.EmailTemplate, data map[string]any) (string, string, error) {
	tmplTitle, err := template.New(name + "Title").Parse(selected.Title)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse email title: %w", err)
	}

	var resTitle strings.Builder
	if err := tmplTitle.Execute(&resTitle, data); err != nil {
		return "", "", fmt.Errorf("failed to execute email title: %w", err)
	}

	tmplBody, err := template.New(name + "Body").Parse(selected.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse email template: %w", err)
	}

	var resBody strings.Builder
	if err := tmplBody.Execute(&resBody, data); err != nil {
		return "", "", fmt.Errorf("failed to execute email template: %w", err)
	}

	return resTitle.String(), resBody.String(), nil
}

func (p ReceiptPayment) templateData() map[string]any {
	return map[string]any{
		"ID":           p.ID,
		"TradeNo":      p.TradeNo,
		"Name":         p.Name,
		"PriceOneUnit": p.PriceOneUnit,
		"PriceMark":    p.PriceMark,
		"PriceID":      p.PriceID,
		"PriceUnit":    p.PriceUnit,
		"Qyt":          p.Qyt,
	}
}

func commonContext(ctx context.Context, settings setting.Provider) *CommonContext {
	logo := settings.Logo(ctx)
	siteUrl := settings.SiteURL(ctx)
	resolvedLogo := *logo

	// Add site url if logo is not an url
	if !strings.HasPrefix(logo.Light, "http") {
		logoPath, _ := url.Parse(logo.Light)
		resolvedLogo.Light = siteUrl.ResolveReference(logoPath).String()
	}

	if !strings.HasPrefix(logo.Normal, "http") {
		logoPath, _ := url.Parse(logo.Normal)
		resolvedLogo.Normal = siteUrl.ResolveReference(logoPath).String()
	}

	return &CommonContext{
		SiteBasic: settings.SiteBasic(ctx),
		Logo:      &resolvedLogo,
		SiteUrl:   siteUrl.String(),
	}
}

func (c *CommonContext) templateData() map[string]any {
	return map[string]any{
		"SiteBasic": map[string]string{
			"Name":        c.SiteBasic.Name,
			"Title":       c.SiteBasic.Title,
			"ID":          c.SiteBasic.ID,
			"Description": c.SiteBasic.Description,
			"Script":      c.SiteBasic.Script,
		},
		"Logo": map[string]string{
			"Normal": c.Logo.Normal,
			"Light":  c.Logo.Light,
		},
		"SiteUrl": c.SiteUrl,
	}
}

func (c *CommonContext) TemplateData() map[string]any {
	return c.templateData()
}

func (c ResetContext) templateData() map[string]any {
	return map[string]any{
		"CommonContext": c.CommonContext.templateData(),
		"User":          c.User,
		"Url":           c.Url,
	}
}

func (c ResetContext) TemplateData() map[string]any {
	return c.templateData()
}

func (c ActivationContext) templateData() map[string]any {
	return map[string]any{
		"CommonContext": c.CommonContext.templateData(),
		"User":          c.User,
		"Url":           c.Url,
	}
}

func (c ActivationContext) TemplateData() map[string]any {
	return c.templateData()
}

func (c ReceiptContext) templateData() map[string]any {
	return map[string]any{
		"CommonContext": c.CommonContext.templateData(),
		"User":          c.User,
		"Payment":       c.Payment,
		"Url":           c.Url,
	}
}

func (c ReceiptContext) TemplateData() map[string]any {
	return c.templateData()
}

func (c QuotaExceedContext) templateData() map[string]any {
	return map[string]any{
		"CommonContext": c.CommonContext.templateData(),
		"User":          c.User,
		"Days":          c.Days,
		"Url":           c.Url,
	}
}

func (c QuotaExceedContext) TemplateData() map[string]any {
	return c.templateData()
}

// SelectTemplate picks the template matching the user's preferred language.
func SelectTemplate(templates []setting.EmailTemplate, user *ent.User) setting.EmailTemplate {
	return selectTemplate(templates, user)
}

// BuildCommonContext constructs the common template data available to all emails.
func BuildCommonContext(ctx context.Context, settings setting.Provider) *CommonContext {
	return commonContext(ctx, settings)
}

func selectTemplate(templates []setting.EmailTemplate, u *ent.User) setting.EmailTemplate {
	selected := templates[0]
	if u != nil && u.Settings != nil {
		for _, t := range templates {
			if strings.EqualFold(t.Language, u.Settings.Language) {
				selected = t
				break
			}
		}
	}

	return selected
}
