import {
  Alert,
  Box,
  FormControl,
  IconButton,
  ListItemText,
  Menu,
  MenuItem,
  Stack,
  Typography,
} from "@mui/material";
import { bindMenu, bindTrigger, usePopupState } from "material-ui-popup-state/hooks";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { DenseFilledTextField, SecondaryButton } from "../../../Common/StyledComponents";
import Add from "../../../Icons/Add";
import Delete from "../../../Icons/Delete";
import SettingForm from "../../../Pages/Setting/SettingForm";
import { NoMarginHelperText } from "../Settings";

export type PaymentProviderType = "stripe" | "weixin" | "alipay" | "custom";

export interface PaymentProviderConfig {
  id: string;
  type: PaymentProviderType;
  name: string;
  secret_key?: string;
  endpoint?: string;
  app_id?: string;
  public_key?: string;
  merchant_id?: string;
  certificate_serial?: string;
  api_private_key?: string;
}

export interface PaymentProvidersProps {
  value: string;
  onChange: (value: string) => void;
}

const providerTypes: PaymentProviderType[] = ["stripe", "weixin", "alipay", "custom"];

const newID = () => {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
};

const parseValue = (value: string): PaymentProviderConfig[] => {
  try {
    const parsed = JSON.parse(value || "[]");
    if (Array.isArray(parsed)) {
      return parsed;
    }
  } catch {
    // Invalid JSON is treated as an empty configuration.
  }
  return [];
};

const PaymentProviders = ({ value, onChange }: PaymentProvidersProps) => {
  const { t } = useTranslation("dashboard");
  const [providers, setProviders] = useState<PaymentProviderConfig[]>([]);
  const popupState = usePopupState({ variant: "popover", popupId: "paymentProviderSelector" });

  useEffect(() => {
    setProviders(parseValue(value));
  }, [value]);

  const save = useCallback(
    (next: PaymentProviderConfig[]) => {
      setProviders(next);
      onChange(JSON.stringify(next));
    },
    [onChange],
  );

  const update = useCallback(
    (id: string, patch: Partial<PaymentProviderConfig>) => {
      save(providers.map((provider) => (provider.id === id ? { ...provider, ...patch } : provider)));
    },
    [providers, save],
  );

  const remove = useCallback(
    (id: string) => {
      save(providers.filter((provider) => provider.id !== id));
    },
    [providers, save],
  );

  const add = useCallback(
    (type: PaymentProviderType) => {
      save([...providers, { id: newID(), type, name: "", secret_key: "" }]);
      popupState.close();
    },
    [providers, save, popupState],
  );

  const { onClose, ...menuProps } = bindMenu(popupState);

  const providerLabel = useMemo(
    () => ({
      stripe: t("settings.stripeProvider"),
      weixin: t("settings.weixinProvider"),
      alipay: t("settings.alipayProvider"),
      custom: t("settings.customProvider"),
    }),
    [t],
  );

  return (
    <Stack spacing={2}>
      {providers.map((provider) => (
        <Box key={provider.id} sx={{ p: 2, border: 1, borderColor: "divider", borderRadius: 1 }}>
          <Stack spacing={2}>
            <Stack direction="row" spacing={2} alignItems="flex-start" justifyContent="space-between">
              <Typography fontWeight={600} color="text.secondary">
                {providerLabel[provider.type] ?? provider.type}
              </Typography>
              <IconButton size="small" onClick={() => remove(provider.id)}>
                <Delete fontSize="small" />
              </IconButton>
            </Stack>

            {provider.type === "custom" && (
              <Alert severity="info">
                <Trans
                  i18nKey="settings.customProviderDes"
                  ns="dashboard"
                  components={[
                    <a href="https://docs.cloudreve.org/payment/custom" target="_blank" rel="noopener noreferrer" />,
                  ]}
                />
              </Alert>
            )}

            <SettingForm title={t("settings.displayName")} lgWidth={12}>
              <DenseFilledTextField
                value={provider.name}
                onChange={(e) => update(provider.id, { name: e.target.value })}
                required
                fullWidth
              />
              <NoMarginHelperText>{t("settings.providerName")}</NoMarginHelperText>
            </SettingForm>

            {provider.type === "custom" && (
              <>
                <SettingForm title={t("vas.customPaymentEndpoint")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.endpoint ?? ""}
                    onChange={(e) => update(provider.id, { endpoint: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>{t("vas.customPaymentEndpointDes")}</NoMarginHelperText>
                </SettingForm>
                <SettingForm title={t("vas.communicationSecret")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.secret_key ?? ""}
                    onChange={(e) => update(provider.id, { secret_key: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>{t("vas.customPaymentSecretDes")}</NoMarginHelperText>
                </SettingForm>
              </>
            )}

            {provider.type === "alipay" && (
              <>
                <SettingForm title={t("vas.appID")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.app_id ?? ""}
                    onChange={(e) => update(provider.id, { app_id: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>{t("vas.appIDDes")}</NoMarginHelperText>
                </SettingForm>
                <SettingForm title={t("vas.rsaPrivate")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.secret_key ?? ""}
                    multiline
                    rows={4}
                    onChange={(e) => update(provider.id, { secret_key: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>
                    <Trans
                      i18nKey="vas.rsaPrivateDes"
                      ns="dashboard"
                      components={[
                        <a href="https://opendocs.alipay.com/common/02kipl" target="_blank" rel="noopener noreferrer" />,
                      ]}
                    />
                  </NoMarginHelperText>
                </SettingForm>
                <SettingForm title={t("vas.alipayPublicKey")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.public_key ?? ""}
                    multiline
                    rows={4}
                    onChange={(e) => update(provider.id, { public_key: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>{t("vas.alipayPublicKeyDes")}</NoMarginHelperText>
                </SettingForm>
              </>
            )}

            {provider.type === "weixin" && (
              <>
                <SettingForm title={t("vas.applicationID")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.app_id ?? ""}
                    onChange={(e) => update(provider.id, { app_id: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>{t("vas.applicationIDDes")}</NoMarginHelperText>
                </SettingForm>
                <SettingForm title={t("vas.merchantID")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.merchant_id ?? ""}
                    onChange={(e) => update(provider.id, { merchant_id: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>{t("vas.merchantIDDes")}</NoMarginHelperText>
                </SettingForm>
                <SettingForm title={t("vas.apiV3Secret")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.secret_key ?? ""}
                    onChange={(e) => update(provider.id, { secret_key: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>{t("vas.apiV3SecretDes")}</NoMarginHelperText>
                </SettingForm>
                <SettingForm title={t("vas.mcCertificateSerial")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.certificate_serial ?? ""}
                    onChange={(e) => update(provider.id, { certificate_serial: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>{t("vas.mcCertificateSerialDes")}</NoMarginHelperText>
                </SettingForm>
                <SettingForm title={t("vas.mcAPISecret")} lgWidth={12}>
                  <DenseFilledTextField
                    value={provider.api_private_key ?? ""}
                    multiline
                    rows={4}
                    onChange={(e) => update(provider.id, { api_private_key: e.target.value })}
                    required
                    fullWidth
                  />
                  <NoMarginHelperText>{t("vas.mcAPISecretDes")}</NoMarginHelperText>
                </SettingForm>
              </>
            )}

            {provider.type === "stripe" && (
              <SettingForm title={t("settings.providerKey")} lgWidth={12}>
                <DenseFilledTextField
                  value={provider.secret_key ?? ""}
                  onChange={(e) => update(provider.id, { secret_key: e.target.value })}
                  required
                  fullWidth
                />
                <NoMarginHelperText>{t("settings.providerKeyDes")}</NoMarginHelperText>
              </SettingForm>
            )}
          </Stack>
        </Box>
      ))}

      <Box>
        <SecondaryButton variant="contained" startIcon={<Add />} {...bindTrigger(popupState)}>
          {t("settings.addPaymentProvider")}
        </SecondaryButton>
      </Box>

      <Menu
        {...menuProps}
        onClose={onClose}
        anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
        transformOrigin={{ vertical: "top", horizontal: "left" }}
      >
        {providerTypes.map((type) => (
          <MenuItem key={type} onClick={() => add(type)}>
            <ListItemText slotProps={{ primary: { variant: "body2" } }}>{providerLabel[type]}</ListItemText>
          </MenuItem>
        ))}
      </Menu>
    </Stack>
  );
};

export default PaymentProviders;
