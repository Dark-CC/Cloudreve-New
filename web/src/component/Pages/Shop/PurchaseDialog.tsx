import { LoadingButton } from "@mui/lab";
import {
  Alert,
  Box,
  Button,
  DialogContent,
  FormControl,
  InputAdornment,
  InputLabel,
  Link,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import i18next from "i18next";
import { QRCodeSVG } from "qrcode.react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { createPayment, getPaymentStatus } from "../../../api/api";
import { Payment, PaymentStatus, ProductType } from "../../../api/dashboard";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks";
import SessionManager from "../../../session";
import { formatPrice } from "../../../util";
import DraggableDialog from "../../Dialogs/DraggableDialog";
import CheckCircleFilled from "../../Icons/CheckCircleFilled";
import CoinStack from "../../Icons/CoinStack";

export interface PurchaseDialogProps {
  open: boolean;
  type: ProductType;
  name: string;
  productId?: string;
  priceCashUnit: number;
  pricePoints: number;
  duration: number;
  onClose: () => void;
  onPaymentCompleted?: (payment: Payment) => void;
}

interface PaymentOption {
  id: string;
  name: string;
  points?: number;
}

const PurchaseDialog = ({
  open,
  type,
  name,
  productId,
  priceCashUnit,
  pricePoints,
  duration,
  onClose,
  onPaymentCompleted,
}: PurchaseDialogProps) => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const paymentSetting = useAppSelector((state) => state.siteConfig.vas?.config?.payment);
  const pointsEnabled = useAppSelector((state) => state.siteConfig.vas?.config?.point_enabled);
  const [step, setStep] = useState(0);
  const [quantity, setQuantity] = useState(1);
  const [providerID, setProviderID] = useState("");
  const [email, setEmail] = useState("");
  const [payment, setPayment] = useState<Payment>();
  const [error, setError] = useState("");
  const [polling, setPolling] = useState(false);
  const pollTimer = useRef<ReturnType<typeof setInterval>>();
  const paymentRef = useRef<Payment>();
  const paymentStarted = useRef(false);
  const loggedIn = useMemo(() => !!SessionManager.currentLoginOrNull(), []);

  const paymentOptions = useMemo<PaymentOption[]>(() => {
    let options: PaymentOption[] = [...(paymentSetting?.providers ?? [])].map((provider) => ({
      id: provider.id,
      name: provider.name || provider.id,
    }));
    if (type === ProductType.share_link) {
      options = [];
    }
    if (pricePoints > 0 && pointsEnabled) {
      options.push({
        id: "points",
        name: t("application:vas.payWithPoints"),
        points: pricePoints,
      });
    }
    return options;
  }, [paymentSetting, pointsEnabled, pricePoints, t, type]);

  const clearPolling = useCallback(() => {
    if (pollTimer.current) {
      clearInterval(pollTimer.current);
      pollTimer.current = undefined;
    }
  }, []);

  const reset = useCallback(() => {
    clearPolling();
    paymentRef.current = undefined;
    paymentStarted.current = false;
    setStep(0);
    setQuantity(1);
    setProviderID("");
    setEmail("");
    setPayment(undefined);
    setError("");
    setPolling(false);
  }, [clearPolling]);

  const finishPayment = useCallback(
    (result: Payment) => {
      clearPolling();
      setPayment(result);
      setPolling(false);
      if (result.status === PaymentStatus.fulfilled) {
        setStep(3);
        window.setTimeout(() => onPaymentCompleted?.(result), 2000);
        return;
      }
      if (result.status === PaymentStatus.canceled || result.status === PaymentStatus.fulfill_failed) {
        setStep(4);
        setError(t("application:vas.fulfillFailed"));
      }
    },
    [clearPolling, onPaymentCompleted, t],
  );

  const pollPayment = useCallback(() => {
    const current = paymentRef.current;
    if (
      !current ||
      !current.trade_no ||
      !current.payment_sign ||
      current.status === PaymentStatus.fulfilled
    ) {
      return;
    }
    setPolling(true);
    dispatch(getPaymentStatus(current.trade_no, current.payment_sign))
      .then((result) => {
        paymentRef.current = result;
        finishPayment(result);
      })
      .catch(() => undefined)
      .finally(() => setPolling(false));
  }, [dispatch, finishPayment]);

  const startPayment = useCallback(
    (selectedProvider?: string) => {
      if (!paymentSetting) {
        return;
      }
      const provider = (selectedProvider || providerID || paymentOptions[0]?.id || "").replace(/\s/g, "");
      if (!provider) {
        return;
      }
      setPolling(true);
      setError("");
      dispatch(
        createPayment({
          product: {
            type,
            sku_id: productId,
          },
          type,
          sku_id: productId,
          quantity,
          email,
          provider_id: provider,
          use_points: provider === "points",
          payment_method: provider,
          language: i18next.language,
        }),
      )
        .then((result) => {
          paymentRef.current = result;
          setPayment(result);
          paymentStarted.current = true;
          clearPolling();
          if (!result.request?.payment_needed || result.status === PaymentStatus.fulfilled) {
            finishPayment(result);
            return;
          }
          setStep(2);
          if (!result.request.qr_code_preferred && result.request.url) {
            window.open(result.request.url, "_blank");
          }
          pollTimer.current = setInterval(pollPayment, 10000);
        })
        .catch((e: Error) => {
          setStep(4);
          setError(e.message);
        })
        .finally(() => setPolling(false));
    },
    [
      clearPolling,
      dispatch,
      email,
      finishPayment,
      paymentOptions,
      paymentSetting,
      pollPayment,
      productId,
      providerID,
      quantity,
      t,
      type,
    ],
  );

  useEffect(() => {
    if (!open) {
      reset();
      return;
    }
    if (!paymentSetting) {
      setStep(0);
      return;
    }
    if (paymentOptions.length === 1 && loggedIn) {
      setProviderID(paymentOptions[0].id);
      return;
    }
    if (paymentOptions.length === 0) {
      setStep(4);
      setError(t("application:vas.noAvailableSharePurchaseMethod"));
      return;
    }
    setEmail("");
    setProviderID(paymentOptions[0]?.id ?? "");
    setStep(1);
  }, [loggedIn, open, paymentOptions, paymentSetting, reset, t]);

  useEffect(() => {
    if (step === 1 && paymentOptions.length === 1 && loggedIn && !paymentStarted.current) {
      paymentStarted.current = true;
      startPayment(paymentOptions[0].id);
    }
  }, [loggedIn, paymentOptions, startPayment, step]);

  useEffect(() => clearPolling, [clearPolling]);

  const close = () => {
    if (!polling) {
      onClose();
    }
  };

  const cashPrice = formatPrice(
    paymentSetting?.currency_mark ?? "¥",
    priceCashUnit * (Number.isNaN(quantity) ? 1 : quantity),
    paymentSetting?.currency_unit ?? 100,
  );

  return (
    <DraggableDialog
      title={t("application:vas.purchaseSomething", { name })}
      hideOk
      dialogProps={{ open, onClose: close, fullWidth: true, maxWidth: "xs" }}
    >
      <DialogContent sx={{ px: 3, pb: 2 }}>
        {step === 0 && (
          <Stack spacing={2} sx={{ pt: 1 }}>
            {duration > 0 && (
              <TextField
                label={t("application:vas.purchaseDuration")}
                type="number"
                variant="filled"
                value={quantity}
                onChange={(event) => setQuantity(parseInt(event.target.value) || 0)}
                slotProps={{
                  htmlInput: { step: 1, min: 1, max: Math.pow(2, 31) - 1 },
                  input: {
                    endAdornment: (
                      <InputAdornment position="end">
                        <Typography sx={{ pt: "10px" }}>
                          x{" "}
                          {t("application:vas.validDurationDays", {
                            num: Math.ceil(duration / 86400),
                          })}
                        </Typography>
                      </InputAdornment>
                    ),
                  },
                }}
              />
            )}
            {type === ProductType.points && (
              <TextField
                label={t("application:vas.creditsNum")}
                type="number"
                variant="filled"
                value={quantity}
                onChange={(event) => setQuantity(parseInt(event.target.value) || 0)}
                slotProps={{
                  htmlInput: { step: 1, min: 1, max: Math.pow(2, 31) - 1 },
                }}
              />
            )}
            {pricePoints > 0 && pointsEnabled && (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: "flex", alignItems: "center", gap: 0.5 }}
              >
                <Trans
                  i18nKey="application:vas.pointsPayAvailable"
                  components={[
                    <Box key="points" component="span" sx={{ display: "inline-flex", alignItems: "center", gap: 0.5 }}>
                      <CoinStack sx={{ fontSize: 16 }} />
                      {pricePoints * (Number.isNaN(quantity) ? 1 : quantity)}
                    </Box>,
                  ]}
                />
              </Typography>
            )}
            <Button
              variant="contained"
              disabled={!quantity || quantity <= 0}
              onClick={() => setStep(1)}
            >
              {t("application:vas.payAmount", { price: cashPrice })}
            </Button>
          </Stack>
        )}

        {step === 1 && (
          <Stack spacing={3} sx={{ pt: 1 }}>
            {paymentOptions.length > 1 && (
              <FormControl variant="outlined" fullWidth>
                <InputLabel>{t("application:vas.paymentMethod")}</InputLabel>
                <Select
                  variant="outlined"
                  label={t("application:vas.paymentMethod")}
                  value={providerID}
                  onChange={(event) => setProviderID(event.target.value)}
                >
                  {paymentOptions.map((option) => (
                    <MenuItem dense key={option.id} value={option.id}>
                      {option.points ? (
                        <Box component="span" sx={{ display: "inline-flex", alignItems: "center", gap: 0.5 }}>
                          <CoinStack sx={{ fontSize: 18 }} />
                          {quantity * option.points}
                        </Box>
                      ) : (
                        option.name
                      )}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}
            {!loggedIn && (
              <TextField
                variant="outlined"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                label={t("application:login.email")}
                helperText={t("application:vas.paymentEmailHelper")}
                fullWidth
              />
            )}
            <Button
              onClick={() => startPayment()}
              variant="contained"
              disabled={(!loggedIn && !email) || !providerID || polling}
            >
              {t("application:login.continue")}
            </Button>
          </Stack>
        )}

        {step === 2 && payment?.request && (
          <Stack spacing={2} sx={{ pt: 1 }}>
            {payment.request.url && !payment.request.qr_code_preferred && (
              <Typography variant="body2">
                <Trans
                  i18nKey="application:vas.payInNewWindow"
                  components={[
                    <Link key="link" target="_blank" underline="hover" variant="inherit" href={payment.request.url} />,
                  ]}
                />
              </Typography>
            )}
            {payment.request.qr_code_preferred && (
              <Box>
                <Typography variant="body2">
                  <Trans
                    i18nKey="vas.qrcodeCustom"
                    ns="application"
                    components={[
                      <Link key="link" target="_blank" href={payment.request.url ?? ""} />,
                    ]}
                  />
                </Typography>
                <Box sx={{ mt: 1, display: "flex", justifyContent: "center" }}>
                  <QRCodeSVG value={payment.request.url ?? ""} />
                </Box>
              </Box>
            )}
            <LoadingButton onClick={pollPayment} variant="contained" loading={polling}>
              {t("application:vas.paidButton")}
            </LoadingButton>
          </Stack>
        )}

        {step === 3 && (
          <Stack sx={{ display: "flex", alignItems: "center", pt: 7, pb: 9 }}>
            <CheckCircleFilled fontSize="large" color="success" />
            <Typography variant="h6" color="success.main" sx={{ mt: 1 }}>
              {t("application:vas.paymentSuccess")}
            </Typography>
          </Stack>
        )}

        {step === 4 && (
          <Alert severity="error" sx={{ mt: 2 }}>
            <Typography variant="inherit" fontWeight={600}>
              {t("application:vas.paymentFailedTitle")}
            </Typography>
            {error}
          </Alert>
        )}
      </DialogContent>
    </DraggableDialog>
  );
};

export default PurchaseDialog;
