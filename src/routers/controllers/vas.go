package controllers

import (
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/vas"
	"github.com/gin-gonic/gin"
)

// UserGetCredit returns the current user's credit balance.
func UserGetCredit(c *gin.Context) {
	service := ParametersFromContext[*vas.CreditService](c, vas.CreditParamCtx{})
	res, err := service.Get(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// UserListCreditChanges lists the current user's credit changes.
func UserListCreditChanges(c *gin.Context) {
	service := ParametersFromContext[*vas.ListCreditChangesService](c, vas.ListCreditChangesParamCtx{})
	res, err := service.List(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// GetGiftCode previews a gift code.
func GetGiftCode(c *gin.Context) {
	service := ParametersFromContext[*vas.GetGiftCodeService](c, vas.GetGiftCodeParamCtx{})
	res, err := service.Get(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// RedeemGiftCode redeems a gift code.
func RedeemGiftCode(c *gin.Context) {
	service := ParametersFromContext[*vas.RedeemGiftCodeService](c, vas.RedeemGiftCodeParamCtx{})
	res, err := service.Redeem(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// AdminCreateGiftCodes generates gift codes.
func AdminCreateGiftCodes(c *gin.Context) {
	service := ParametersFromContext[*vas.CreateGiftCodesService](c, vas.CreateGiftCodesParamCtx{})
	res, err := service.Create(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// AdminUpdateGiftCode updates an unused gift code.
func AdminUpdateGiftCode(c *gin.Context) {
	service := ParametersFromContext[*vas.UpdateGiftCodeService](c, vas.UpdateGiftCodeParamCtx{})
	res, err := service.Update(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// AdminListGiftCodes lists gift codes.
func AdminListGiftCodes(c *gin.Context) {
	service := ParametersFromContext[*vas.ListGiftCodesService](c, vas.ListGiftCodesParamCtx{})
	res, err := service.List(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// AdminDeleteGiftCode deletes a gift code.
func AdminDeleteGiftCode(c *gin.Context) {
	service := ParametersFromContext[*vas.DeleteGiftCodeService](c, vas.DeleteGiftCodeParamCtx{})
	if err := service.Delete(c); err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{})
}

// UserListStoragePacks lists the current user's storage packs.
func UserListStoragePacks(c *gin.Context) {
	service := ParametersFromContext[*vas.ListStoragePacksService](c, vas.ListStoragePacksParamCtx{})
	res, err := service.List(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// UserCreatePayment creates an order.
func UserCreatePayment(c *gin.Context) {
	service := ParametersFromContext[*vas.CreatePaymentService](c, vas.CreatePaymentParamCtx{})
	res, err := service.Create(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// UserListPayments lists the current user's orders.
func UserListPayments(c *gin.Context) {
	service := ParametersFromContext[*vas.ListPaymentsService](c, vas.ListPaymentsParamCtx{})
	res, err := service.List(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// PaymentStatus polls the status of an order.
func PaymentStatus(c *gin.Context) {
	service := ParametersFromContext[*vas.PaymentStatusService](c, vas.PaymentStatusParamCtx{})
	res, err := service.Status(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// PaymentCallback receives asynchronous gateway notifications.
func PaymentCallback(c *gin.Context) {
	service := ParametersFromContext[*vas.PaymentCallbackService](c, vas.PaymentCallbackParamCtx{})
	if err := service.Callback(c); err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{})
}

// AdminListPayments lists all orders.
func AdminListPayments(c *gin.Context) {
	service := ParametersFromContext[*vas.AdminListPaymentsService](c, vas.AdminListPaymentsParamCtx{})
	res, err := service.AdminList(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// AdminFulfillPayment manually fulfills an order.
func AdminFulfillPayment(c *gin.Context) {
	service := ParametersFromContext[*vas.AdminFulfillPaymentService](c, vas.AdminFulfillPaymentParamCtx{})
	res, err := service.Fulfill(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}

// AdminDeletePayments deletes orders in batch.
func AdminDeletePayments(c *gin.Context) {
	service := ParametersFromContext[*vas.AdminDeletePaymentsService](c, vas.AdminDeletePaymentsParamCtx{})
	if err := service.AdminDelete(c); err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{})
}

// PurchaseShare buys access to a paid share with credits.
func PurchaseShare(c *gin.Context) {
	service := ParametersFromContext[*vas.PurchaseShareService](c, vas.PurchaseShareParamCtx{})
	res, err := service.Purchase(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}
	c.JSON(200, serializer.Response{Data: res})
}
