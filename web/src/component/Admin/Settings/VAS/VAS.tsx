import {
  Box,
  Button,
  FormControl,
  FormControlLabel,
  InputAdornment,
  Link,
  Stack,
  Switch,
  Typography,
} from "@mui/material";
import { bindTrigger, usePopupState } from "material-ui-popup-state/hooks";
import { useContext, useMemo } from "react";
import { Trans, useTranslation } from "react-i18next";
import { isTrueVal } from "../../../../session/utils";
import { DenseFilledTextField } from "../../../Common/StyledComponents";
import SettingForm from "../../../Pages/Setting/SettingForm";
import { NoMarginHelperText, SettingSection, SettingSectionContent } from "../Settings";
import { SettingContext } from "../SettingWrapper";
import GiftCodes from "./GiftCodes";
import GroupProducts from "./GroupProducts";
import PaymentProviders from "./PaymentProviders";
import StorageProducts from "./StorageProducts";

interface CurrencyOption {
  code: string;
  symbol: string;
  unit: number;
  label: string;
}

const currencies: CurrencyOption[] = [
  { code: "CNY", symbol: "¥", unit: 100, label: "人民币 (CNY)" },
  { code: "USD", symbol: "$", unit: 100, label: "美元 (USD)" },
  { code: "EUR", symbol: "€", unit: 100, label: "欧元 (EUR)" },
  { code: "JPY", symbol: "¥", unit: 1, label: "日元 (JPY)" },
  { code: "GBP", symbol: "£", unit: 100, label: "英镑 (GBP)" },
];

const CurrencySelector = ({
  onSelect,
}: {
  onSelect: (option: CurrencyOption) => void;
}) => {
  const { t } = useTranslation("dashboard");
  const popupState = usePopupState({ variant: "popover", popupId: "currencySelector" });

  return (
    <>
      <Button {...bindTrigger(popupState)}>{t("settings.selectCurrency")}</Button>
      <Box
        sx={{
          position: "absolute",
          right: 0,
          top: 40,
          zIndex: 10,
          backgroundColor: "background.paper",
          boxShadow: 3,
          borderRadius: 1,
          display: popupState.isOpen ? "block" : "none",
          minWidth: 180,
        }}
      >
        {currencies.map((currency) => (
          <Button
            key={currency.code}
            fullWidth
            sx={{ justifyContent: "flex-start" }}
            onClick={() => {
              onSelect(currency);
              popupState.close();
            }}
          >
            {currency.label}
          </Button>
        ))}
      </Box>
    </>
  );
};

const VAS = () => {
  const { t } = useTranslation("dashboard");
  const { formRef, setSettings, values } = useContext(SettingContext);
  const storageProducts = useMemo(
    () => values.storage_products || values.vas_storage_products || "[]",
    [values.storage_products, values.vas_storage_products],
  );
  const groupSellData = useMemo(
    () => values.group_sell_data || values.vas_group_products || "[]",
    [values.group_sell_data, values.vas_group_products],
  );

  const payment = useMemo(() => {
    const fallback = {
      currency_code: values.vas_currency_code || "CNY",
      currency_mark: values.vas_currency_symbol || "¥",
      currency_unit: parseInt(values.vas_currency_unit || "100") || 100,
      providers: [],
      local_enabled: isTrueVal(values.vas_local_payment_enabled),
    };
    try {
      const parsed = JSON.parse(values.payment || "{}");
      return {
        ...fallback,
        ...parsed,
        providers: Array.isArray(parsed.providers) ? parsed.providers : fallback.providers,
      };
    } catch {
      return fallback;
    }
  }, [
    values.payment,
    values.vas_currency_code,
    values.vas_currency_symbol,
    values.vas_currency_unit,
    values.vas_local_payment_enabled,
  ]);
  const updatePayment = (patch: Record<string, unknown>) => {
    setSettings({ payment: JSON.stringify({ ...payment, ...patch }) });
  };

  const currencySymbol = payment.currency_mark ?? "¥";
  const currencyUnit = payment.currency_unit || 100;

  return (
    <Box component={"form"} ref={formRef} onSubmit={(e) => e.preventDefault()}>
      <Stack spacing={5}>
        <SettingSection>
          <Typography variant="h6" gutterBottom>
            {t("settings.creditAndVAS")}
          </Typography>
          <SettingSectionContent>
            <SettingForm lgWidth={5}>
              <FormControl fullWidth>
                <FormControlLabel
                  control={
                    <Switch
                      checked={isTrueVal(values.score_enabled)}
                      onChange={(e) => setSettings({ score_enabled: e.target.checked ? "1" : "0" })}
                    />
                  }
                  label={t("settings.enableCredit")}
                />
                <NoMarginHelperText>{t("settings.enableCreditDes")}</NoMarginHelperText>
              </FormControl>
            </SettingForm>

            <Stack spacing={2}>
              <SettingForm title={t("settings.creditPrice")} lgWidth={5}>
                <FormControl fullWidth>
                  <DenseFilledTextField
                    type="number"
                    value={values.score_price ?? "1"}
                    onChange={(e) => setSettings({ score_price: e.target.value })}
                  />
                  <NoMarginHelperText>{t("settings.creditPriceDes")}</NoMarginHelperText>
                </FormControl>
              </SettingForm>

              <SettingForm title={t("settings.shareScoreRate")} lgWidth={5}>
                <FormControl fullWidth>
                  <DenseFilledTextField
                    type="number"
                    value={values.share_score_rate ?? "80"}
                    onChange={(e) => setSettings({ share_score_rate: e.target.value })}
                  />
                  <NoMarginHelperText>{t("settings.shareScoreRateDes")}</NoMarginHelperText>
                </FormControl>
              </SettingForm>
            </Stack>

            <SettingForm title={t("vas.banBufferPeriod")} lgWidth={5}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  type="number"
                  value={values.ban_time ?? "604800"}
                  onChange={(e) => setSettings({ ban_time: e.target.value })}
                />
                <NoMarginHelperText>{t("vas.banBufferPeriodDes")}</NoMarginHelperText>
              </FormControl>
            </SettingForm>

            <SettingForm title={t("settings.cronNotifyUser")} lgWidth={5}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  value={values.cron_notify_user ?? "@every 1h"}
                  onChange={(e) => setSettings({ cron_notify_user: e.target.value })}
                />
                <NoMarginHelperText>
                  <Trans
                    i18nKey="settings.cronDes"
                    values={{ des: t("settings.cronNotifyUserDes") }}
                    ns={"dashboard"}
                    components={[<Link href="https://crontab.guru/" target="_blank" rel="noopener noreferrer" />]}
                  />
                </NoMarginHelperText>
              </FormControl>
            </SettingForm>

            <SettingForm title={t("settings.cronBanUser")} lgWidth={5}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  value={values.cron_ban_user ?? "@every 1h"}
                  onChange={(e) => setSettings({ cron_ban_user: e.target.value })}
                />
                <NoMarginHelperText>
                  <Trans
                    i18nKey="settings.cronDes"
                    values={{ des: t("settings.cronBanUserDes") }}
                    ns={"dashboard"}
                    components={[<Link href="https://crontab.guru/" target="_blank" rel="noopener noreferrer" />]}
                  />
                </NoMarginHelperText>
              </FormControl>
            </SettingForm>

            <SettingForm lgWidth={5}>
              <FormControl fullWidth>
                <FormControlLabel
                  control={
                    <Switch
                      checked={isTrueVal(values.anonymous_purchase)}
                      onChange={(e) => setSettings({ anonymous_purchase: e.target.checked ? "1" : "0" })}
                    />
                  }
                  label={t("settings.anonymousPurchase")}
                />
                <NoMarginHelperText>{t("settings.anonymousPurchaseDes")}</NoMarginHelperText>
              </FormControl>
            </SettingForm>

            <SettingForm lgWidth={5}>
              <FormControl fullWidth>
                <FormControlLabel
                  control={
                    <Switch
                      checked={isTrueVal(values.shop_nav_enabled)}
                      onChange={(e) => setSettings({ shop_nav_enabled: e.target.checked ? "1" : "0" })}
                    />
                  }
                  label={t("settings.shopNavEnabled")}
                />
                <NoMarginHelperText>{t("settings.shopNavEnabledDes")}</NoMarginHelperText>
              </FormControl>
            </SettingForm>

          </SettingSectionContent>
        </SettingSection>

        <SettingSection>
          <Typography variant="h6" gutterBottom>
            {t("settings.paymentSettings")}
          </Typography>
          <SettingSectionContent>
            <SettingForm title={t("settings.currencyCode")} lgWidth={5}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  value={payment.currency_code ?? "CNY"}
                  onChange={(e) => updatePayment({ currency_code: e.target.value })}
                  slotProps={{
                    input: {
                      endAdornment: (
                        <InputAdornment position="end">
                          <Box sx={{ position: "relative" }}>
                            <CurrencySelector
                              onSelect={(currency) =>
                                updatePayment({
                                  currency_code: currency.code,
                                  currency_mark: currency.symbol,
                                  currency_unit: currency.unit,
                                })
                              }
                            />
                          </Box>
                        </InputAdornment>
                      ),
                    },
                  }}
                />
                <NoMarginHelperText>{t("settings.currencyCodeDes")}</NoMarginHelperText>
              </FormControl>
            </SettingForm>

            <SettingForm title={t("settings.currencySymbol")} lgWidth={5}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  value={payment.currency_mark ?? "¥"}
                  onChange={(e) => updatePayment({ currency_mark: e.target.value })}
                />
                <NoMarginHelperText>{t("settings.currencySymbolDes")}</NoMarginHelperText>
              </FormControl>
            </SettingForm>

            <SettingForm title={t("settings.currencyUnit")} lgWidth={5}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  type="number"
                  value={payment.currency_unit ?? 100}
                  onChange={(e) => updatePayment({ currency_unit: parseInt(e.target.value) || 0 })}
                />
                <NoMarginHelperText>{t("settings.currencyUnitDes")}</NoMarginHelperText>
              </FormControl>
            </SettingForm>

            <Box sx={{ mt: 2 }}>
              <Typography variant="subtitle1" gutterBottom>
                {t("settings.paymentProviders")}
              </Typography>
              <SettingForm lgWidth={6}>
                <PaymentProviders
                  value={JSON.stringify(payment.providers ?? [])}
                  onChange={(value) => updatePayment({ providers: JSON.parse(value || "[]") })}
                />
              </SettingForm>
            </Box>
          </SettingSectionContent>
        </SettingSection>

        <SettingSection>
          <Typography variant="h6" gutterBottom>
            {t("settings.storageProductSettings")}
          </Typography>
          <SettingSectionContent>
            <SettingForm lgWidth={12}>
              <FormControl fullWidth>
                <StorageProducts
                  value={storageProducts}
                  onChange={(value) => setSettings({ storage_products: value })}
                  currencySymbol={currencySymbol}
                  currencyUnit={currencyUnit}
                />
                <NoMarginHelperText>{t("settings.storageProductsDes")}</NoMarginHelperText>
              </FormControl>
            </SettingForm>
          </SettingSectionContent>
        </SettingSection>

        <SettingSection>
          <Typography variant="h6" gutterBottom>
            {t("settings.groupProductSettings")}
          </Typography>
          <SettingSectionContent>
            <SettingForm lgWidth={12}>
              <FormControl fullWidth>
                <GroupProducts
                  value={groupSellData}
                  onChange={(value) => setSettings({ group_sell_data: value })}
                  currencySymbol={currencySymbol}
                  currencyUnit={currencyUnit}
                />
                <NoMarginHelperText>{t("settings.groupProductsDes")}</NoMarginHelperText>
              </FormControl>
            </SettingForm>
          </SettingSectionContent>
        </SettingSection>

        <SettingSection>
          <Typography variant="h6" gutterBottom>
            {t("giftCodes.giftCodesSettings")}
          </Typography>
          <SettingSectionContent>
            <GiftCodes storageProductsConfig={storageProducts} groupProductsConfig={groupSellData} />
          </SettingSectionContent>
        </SettingSection>
      </Stack>
    </Box>
  );
};

export default VAS;
