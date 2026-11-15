import { Box, Button, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Typography } from "@mui/material";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getCredit, getCreditChanges, getPayments } from "../../../api/api";
import { CreditChange, Payment, PaymentStatus, ProductType } from "../../../api/dashboard";
import { UserSettings } from "../../../api/user";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks";
import { loadSiteConfig } from "../../../redux/thunks/site";
import { formatPrice } from "../../../util";
import TimeBadge from "../../Common/TimeBadge";
import CoinStack from "../../Icons/CoinStack";
import Savings from "../../Icons/Savings";
import PurchaseDialog from "../Shop/PurchaseDialog";
import SettingForm from "./SettingForm";

const creditReasons: Record<string, string> = {
  pay: "vas.creditReasonSharePay",
  recharge: "vas.creditReasonRecharge",
  share_purchased: "vas.creditReasonShareGain",
  adjust: "vas.creditReasonAdjust",
  gift_code: "vas.creditReasonGiftCode",
};

const statusLabels: Record<string, string> = {
  [PaymentStatus.canceled]: "application:setting.canceled",
  [PaymentStatus.created]: "application:vas.unpaid",
  [PaymentStatus.paid]: "application:vas.paid",
  fulfilling: "application:vas.fulfilling",
  [PaymentStatus.fulfilled]: "application:setting.finished",
  [PaymentStatus.fulfill_failed]: "application:vas.fulfillFailedStatus",
};

const statusColors: Record<string, string> = {
  [PaymentStatus.canceled]: "action.disabled",
  [PaymentStatus.created]: "text.primary",
  [PaymentStatus.paid]: "success.light",
  fulfilling: "success.light",
  [PaymentStatus.fulfilled]: "success.main",
  [PaymentStatus.fulfill_failed]: "error.main",
};

const productLabels: Record<number, string> = {
  [ProductType.share_link]: "application:modals.shareLink",
  [ProductType.group]: "application:setting.group",
  [ProductType.storage]: "application:vas.extendStorage",
  [ProductType.points]: "application:vas.points",
};

interface FinanceSettingProps {
  setting: UserSettings;
}

const CreditChangeTable = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const [changes, setChanges] = useState<CreditChange[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);

  const load = useCallback(
    (nextPage: number, append: boolean) => {
      setLoading(true);
      dispatch(getCreditChanges(nextPage, 10))
        .then((res) => {
          const items = res.changes ?? res.credit_changes ?? [];
          setChanges((prev) => (append ? [...prev, ...items] : items));
          setTotal(res.pagination?.total_items ?? 0);
          setPage(nextPage);
        })
        .finally(() => setLoading(false));
    },
    [dispatch],
  );

  useEffect(() => {
    load(1, false);
  }, [load]);

  return (
    <TableContainer>
      <Table sx={{ width: "100%", tableLayout: "fixed" }} size="small">
        <TableHead>
          <TableRow>
            <TableCell width={100}>{t("setting.change")}</TableCell>
            <TableCell width={200}>{t("setting.time")}</TableCell>
            <TableCell width={200}>{t("setting.reason")}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {changes.map((change) => {
            const diff = change.diff ?? change.change;
            return (
              <TableRow key={change.id} hover>
                <TableCell
                  component="th"
                  scope="row"
                  sx={{ color: (theme) => (diff > 0 ? theme.palette.success.dark : theme.palette.warning.dark) }}
                >
                  {`${diff > 0 ? "+" : "−"}${Math.abs(diff)}`}
                </TableCell>
                <TableCell component="th" scope="row">
                  <TimeBadge datetime={change.changed_at ?? change.created_at} variant="inherit" />
                </TableCell>
                <TableCell component="th" scope="row">
                  {change.reason ? t(creditReasons[change.reason] ?? `vas.${change.reason}`) : ""}
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
      {changes.length < total && (
        <Box sx={{ p: 1, width: "100%", textAlign: "center" }}>
          <Button onClick={() => load(page + 1, true)} disabled={loading} fullWidth>
            {t("download.loadMore")}
          </Button>
        </Box>
      )}
      {!loading && changes.length === 0 && (
        <Box sx={{ p: 1, width: "100%", textAlign: "center" }}>
          <Typography variant="caption" color="text.secondary">
            {t("application:setting.listEmpty")}
          </Typography>
        </Box>
      )}
    </TableContainer>
  );
};

const PaymentTable = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const [payments, setPayments] = useState<Payment[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);

  const load = useCallback(
    (nextPage: number, append: boolean) => {
      setLoading(true);
      dispatch(getPayments(nextPage, 10))
        .then((res) => {
          setPayments((prev) => (append ? [...prev, ...(res.payments ?? [])] : res.payments ?? []));
          setTotal(res.pagination?.total_items ?? 0);
          setPage(nextPage);
        })
        .finally(() => setLoading(false));
    },
    [dispatch],
  );

  useEffect(() => {
    load(1, false);
  }, [load]);

  return (
    <TableContainer>
      <Table sx={{ width: "100%", tableLayout: "fixed" }} size="small">
        <TableHead>
          <TableRow>
            <TableCell width={150}>{t("vas.tradeNo")}</TableCell>
            <TableCell width={150}>{t("fileManager.name")}</TableCell>
            <TableCell width={100}>{t("vas.amount")}</TableCell>
            <TableCell width={100}>{t("share.price")}</TableCell>
            <TableCell width={100}>{t("setting.taskStatus")}</TableCell>
            <TableCell width={100}>{t("vas.productType")}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {payments.map((payment) => (
            <TableRow key={payment.id} hover>
              <TableCell component="th" scope="row">
                {payment.trade_no}
              </TableCell>
              <TableCell>{payment.name}</TableCell>
              <TableCell>
                {payment.price_mark && payment.price_one_unit
                  ? formatPrice(
                      payment.price_mark,
                      (payment.price_unit ?? 0) * payment.qyt,
                      payment.price_one_unit,
                    )
                  : t("vas.creditsTotalNum", { num: payment.qyt * (payment.price_unit ?? 0) })}
              </TableCell>
              <TableCell>
                {`${
                  payment.price_mark && payment.price_one_unit
                    ? formatPrice(payment.price_mark, payment.price_unit ?? 0, payment.price_one_unit)
                    : t("vas.creditsTotalNum", { num: payment.price_unit ?? 0 })
                } x ${payment.qyt}`}
              </TableCell>
              <TableCell sx={{ color: statusColors[payment.status] }}>
                {statusLabels[payment.status] ? t(statusLabels[payment.status]) : ""}
              </TableCell>
              <TableCell>{productLabels[payment.product_type] ? t(productLabels[payment.product_type]) : ""}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {payments.length < total && (
        <Box sx={{ p: 1, width: "100%", textAlign: "center" }}>
          <Button onClick={() => load(page + 1, true)} disabled={loading} fullWidth>
            {t("download.loadMore")}
          </Button>
        </Box>
      )}
      {!loading && payments.length === 0 && (
        <Box sx={{ p: 1, width: "100%", textAlign: "center" }}>
          <Typography variant="caption" color="text.secondary">
            {t("application:setting.listEmpty")}
          </Typography>
        </Box>
      )}
    </TableContainer>
  );
};

const FinanceSetting = ({ setting }: FinanceSettingProps) => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const [credit, setCredit] = useState(setting.credit ?? 0);
  const [rechargeOpen, setRechargeOpen] = useState(false);
  const pointPrice = useAppSelector((state) => state.siteConfig.vas?.config?.point_price) ?? 0;

  useEffect(() => {
    dispatch(loadSiteConfig("vas"));
  }, [dispatch]);

  useEffect(() => {
    setCredit(setting.credit ?? 0);
  }, [setting.credit]);

  const refreshCredit = useCallback(() => {
    dispatch(getCredit())
      .then((res) => setCredit(res.credit))
      .catch(() => undefined);
  }, [dispatch]);

  return (
    <Stack spacing={3}>
      <PurchaseDialog
        type={ProductType.points}
        name={t("vas.credits")}
        priceCashUnit={pointPrice}
        pricePoints={0}
        duration={0}
        open={rechargeOpen}
        onClose={() => setRechargeOpen(false)}
        onPaymentCompleted={refreshCredit}
      />
      <SettingForm title={t("vas.credits")}>
        <Box sx={{ display: "flex", flexDirection: "row", alignItems: "center", gap: 2 }}>
          <Typography
            variant="h4"
            color="primary"
            fontWeight={600}
            sx={{ display: "flex", alignItems: "center" }}
          >
            <CoinStack />
            {credit}
          </Typography>
          <Button onClick={() => setRechargeOpen(true)} variant="contained" startIcon={<Savings />}>
            {t("vas.recharge")}
          </Button>
        </Box>
      </SettingForm>
      <SettingForm title={t("vas.creditChanges")} lgWidth={12}>
        <CreditChangeTable />
      </SettingForm>
      <SettingForm title={t("vas.payments")} lgWidth={12}>
        <PaymentTable />
      </SettingForm>
    </Stack>
  );
};

export default FinanceSetting;
