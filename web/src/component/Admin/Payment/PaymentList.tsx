import {
  Badge,
  Box,
  Button,
  Checkbox,
  Container,
  Divider,
  IconButton,
  Link,
  ListItemText,
  Popover,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TableSortLabel,
  Tooltip,
  Typography,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import { bindPopover, bindTrigger, usePopupState } from "material-ui-popup-state/hooks";
import { useQueryState } from "nuqs";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { adminBatchDeletePayments, adminFulfillPayment, adminListPayments } from "../../../api/api";
import { AdminListService, Payment, PaymentStatus, ProductType } from "../../../api/dashboard";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks";
import { confirmOperation } from "../../../redux/thunks/dialog";
import { loadSiteConfig } from "../../../redux/thunks/site";
import { formatPrice } from "../../../util";
import { DenseFilledTextField, DenseSelect, SecondaryButton, StyledTableContainerPaper } from "../../Common/StyledComponents";
import TimeBadge from "../../Common/TimeBadge";
import UserAvatar from "../../Common/User/UserAvatar";
import { SquareMenuItem } from "../../FileManager/ContextMenu/ContextMenu";
import ArrowSync from "../../Icons/ArrowSync";
import CheckCircleFilled from "../../Icons/CheckCircleFilled";
import Delete from "../../Icons/Delete";
import Filter from "../../Icons/Filter";
import PageContainer from "../../Pages/PageContainer";
import PageHeader from "../../Pages/PageHeader";
import SettingForm from "../../Pages/Setting/SettingForm";
import TablePagination from "../Common/TablePagination";
import { OrderByQuery, OrderDirectionQuery, PageQuery, PageSizeQuery } from "../StoragePolicy/StoragePolicySetting";
import UserDialog from "../User/UserDialog/UserDialog";

export const PaymentTradeNoQuery = "trade_no";
export const PaymentProductTypeQuery = "product_type";
export const PaymentProviderQuery = "provider_id";
export const PaymentStatusQuery = "status";
export const PaymentUserQuery = "user";

const statusColor = (status?: PaymentStatus): string => {
  switch (status) {
    case PaymentStatus.canceled:
      return "text.disabled";
    case PaymentStatus.fulfilled:
      return "success.main";
    case PaymentStatus.paid:
      return "success.light";
    case PaymentStatus.fulfill_failed:
      return "error.main";
    default:
      return "text.primary";
  }
};

const usePaymentLabels = () => {
  const { t } = useTranslation("dashboard");
  return useMemo(
    () => ({
      product: {
        [ProductType.share_link]: t("application:modals.shareLink"),
        [ProductType.group]: t("application:setting.group"),
        [ProductType.storage]: t("application:vas.extendStorage"),
        [ProductType.points]: t("application:vas.points"),
      } as Record<number, string>,
      status: {
        [PaymentStatus.created]: t("application:vas.unpaid"),
        [PaymentStatus.paid]: t("application:vas.paid"),
        fulfilling: t("application:vas.fulfilling"),
        [PaymentStatus.fulfilled]: t("application:setting.finished"),
        [PaymentStatus.fulfill_failed]: t("application:vas.fulfillFailedStatus"),
        [PaymentStatus.canceled]: t("application:setting.canceled"),
      } as Record<string, string>,
    }),
    [t],
  );
};

const PaymentFilterPopover = ({
  user,
  setUser,
  tradeNo,
  setTradeNo,
  productType,
  setProductType,
  providerID,
  setProviderID,
  status,
  setStatus,
  clearFilters,
  onClose,
  open,
  ...rest
}: {
  user: string;
  setUser: (value: string) => void;
  tradeNo: string;
  setTradeNo: (value: string) => void;
  productType: string;
  setProductType: (value: string) => void;
  providerID: string;
  setProviderID: (value: string) => void;
  status: string;
  setStatus: (value: string) => void;
  clearFilters: () => void;
  onClose: () => void;
  open: boolean;
}) => {
  const { t } = useTranslation("dashboard");
  const labels = usePaymentLabels();
  const providers = useAppSelector((state) => state.siteConfig.vas.config?.payment?.providers ?? []);
  const [localUser, setLocalUser] = useState(user);
  const [localTradeNo, setLocalTradeNo] = useState(tradeNo);
  const [localProductType, setLocalProductType] = useState(productType);
  const [localProviderID, setLocalProviderID] = useState(providerID);
  const [localStatus, setLocalStatus] = useState(status);

  useEffect(() => {
    if (open) {
      setLocalUser(user);
      setLocalTradeNo(tradeNo);
      setLocalProductType(productType);
      setLocalProviderID(providerID);
      setLocalStatus(status);
    }
  }, [open, user, tradeNo, productType, providerID, status]);

  const apply = () => {
    setUser(localUser);
    setTradeNo(localTradeNo);
    setProductType(localProductType);
    setProviderID(localProviderID === " " ? "" : localProviderID);
    setStatus(localStatus === " " ? "" : localStatus);
    onClose();
  };

  const reset = () => {
    setLocalUser("");
    setLocalTradeNo("");
    setLocalProductType(" ");
    setLocalProviderID(" ");
    setLocalStatus(" ");
    clearFilters();
    onClose();
  };

  return (
    <Popover
      anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
      transformOrigin={{ vertical: "top", horizontal: "left" }}
      slotProps={{ paper: { sx: { p: 2, width: 300, maxWidth: "100%" } } }}
      onClose={onClose}
      open={open}
      {...rest}
    >
      <Stack spacing={2}>
        <SettingForm title={t("event.userID")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localUser}
            onChange={(e) => setLocalUser(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>
        <SettingForm title={t("payment.tradeNo")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localTradeNo}
            onChange={(e) => setLocalTradeNo(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>
        <SettingForm title={t("payment.status")} noContainer lgWidth={12}>
          <DenseSelect
            fullWidth
            value={localStatus || " "}
            onChange={(e) => setLocalStatus(e.target.value === " " ? "" : `${e.target.value}`)}
          >
            {Object.values(PaymentStatus).map((value) => (
              <SquareMenuItem key={value} value={value}>
                <ListItemText slotProps={{ primary: { variant: "body2" } }} primary={labels.status[value]} />
              </SquareMenuItem>
            ))}
            <SquareMenuItem value=" ">
              <ListItemText slotProps={{ primary: { variant: "body2" } }} primary={<em>{t("user.all")}</em>} />
            </SquareMenuItem>
          </DenseSelect>
        </SettingForm>
        <SettingForm title={t("payment.productType")} noContainer lgWidth={12}>
          <DenseSelect
            fullWidth
            value={localProductType || " "}
            onChange={(e) => setLocalProductType(e.target.value === " " ? "" : `${e.target.value}`)}
          >
            {Object.entries(labels.product).map(([value, label]) => (
              <SquareMenuItem key={value} value={value}>
                <ListItemText slotProps={{ primary: { variant: "body2" } }} primary={label} />
              </SquareMenuItem>
            ))}
            <SquareMenuItem value=" ">
              <ListItemText slotProps={{ primary: { variant: "body2" } }} primary={<em>{t("user.all")}</em>} />
            </SquareMenuItem>
          </DenseSelect>
        </SettingForm>
        <SettingForm title={t("payment.providerID")} noContainer lgWidth={12}>
          <DenseSelect
            fullWidth
            value={localProviderID || " "}
            onChange={(e) => setLocalProviderID(e.target.value === " " ? "" : `${e.target.value}`)}
          >
            {providers.map((provider) => (
              <SquareMenuItem key={provider.id} value={provider.id}>
                <ListItemText slotProps={{ primary: { variant: "body2" } }} primary={provider.name || provider.id} />
              </SquareMenuItem>
            ))}
            <SquareMenuItem value=" ">
              <ListItemText slotProps={{ primary: { variant: "body2" } }} primary={<em>{t("user.all")}</em>} />
            </SquareMenuItem>
          </DenseSelect>
        </SettingForm>
        <Box display="flex" justifyContent="space-between">
          <Button variant="outlined" size="small" onClick={reset}>
            {t("user.reset")}
          </Button>
          <Button variant="contained" size="small" onClick={apply}>
            {t("user.apply")}
          </Button>
        </Box>
      </Stack>
    </Popover>
  );
};

const PaymentRow = ({
  payment,
  loading,
  deleting,
  selected,
  onDelete,
  onFulfill,
  onSelect,
  openUserDialog,
}: {
  payment?: Payment;
  loading?: boolean;
  deleting?: boolean;
  selected?: boolean;
  onDelete?: () => void;
  onFulfill?: () => void;
  onSelect?: (id: number) => void;
  openUserDialog?: (id?: number) => void;
}) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const labels = usePaymentLabels();
  const providers = useAppSelector((state) => state.siteConfig.vas.config?.payment?.providers ?? []);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [fulfillLoading, setFulfillLoading] = useState(false);
  const provider = useMemo(
    () => providers.find((item) => item.id === payment?.provider_id),
    [providers, payment?.provider_id],
  );

  if (loading) {
    return (
      <TableRow sx={{ height: "43px" }}>
        <TableCell>
          <Skeleton variant="circular" width={20} height={20} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={30} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={200} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={50} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={50} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={50} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={50} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={50} />
        </TableCell>
        <TableCell>
          <Stack direction="row" alignItems="center" gap={1}>
            <Skeleton variant="circular" width={24} height={24} />
            <Skeleton variant="text" width={50} />
          </Stack>
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={70} />
        </TableCell>
        <TableCell>
          <Skeleton variant="circular" width={24} height={24} />
        </TableCell>
      </TableRow>
    );
  }

  const handleDelete = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    e.preventDefault();
    if (!payment?.id) {
      return;
    }
    dispatch(confirmOperation(t("vas.confirmDelete"))).then(() => {
      setDeleteLoading(true);
      dispatch(adminBatchDeletePayments([payment.id]))
        .then(() => onDelete?.())
        .finally(() => setDeleteLoading(false));
    });
  };

  const handleFulfill = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    e.preventDefault();
    if (!payment?.id) {
      return;
    }
    dispatch(confirmOperation(t("vas.confirmManualFulfill", { tradeNo: payment.trade_no }))).then(() => {
      setFulfillLoading(true);
      dispatch(adminFulfillPayment(payment.id))
        .finally(() => {
          setFulfillLoading(false);
          onFulfill?.();
        });
    });
  };

  const handleSelect = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    e.preventDefault();
    if (payment?.id) {
      onSelect?.(payment.id);
    }
  };

  const handleUserClick = (e: React.MouseEvent<HTMLAnchorElement>) => {
    e.stopPropagation();
    e.preventDefault();
    openUserDialog?.(payment?.edges?.user?.id);
  };

  const owner = payment?.edges?.user;

  return (
    <TableRow hover selected={selected}>
      <TableCell padding="checkbox">
        <Checkbox
          size="small"
          disableRipple
          color="primary"
          checked={!!selected}
          onClick={handleSelect}
          disabled={deleting || deleteLoading}
        />
      </TableCell>
      <TableCell>
        <Typography variant="inherit">{payment?.id}</Typography>
      </TableCell>
      <TableCell>
        <Typography variant="inherit">{payment?.name}</Typography>
      </TableCell>
      <TableCell>
        <Typography variant="inherit">
          {labels.product[payment?.product_type ?? ProductType.points]}
        </Typography>
      </TableCell>
      <TableCell>
        <Typography variant="inherit">{payment?.trade_no}</Typography>
      </TableCell>
      <TableCell>
        <Typography variant="inherit">
          {payment?.price_mark && payment.price_one_unit
            ? formatPrice(payment.price_mark, payment.price_unit ?? 0, payment.price_one_unit)
            : t("application:vas.creditsTotalNum", { num: payment?.price_unit ?? 0 })}
          {` x ${payment?.qyt}`}
        </Typography>
      </TableCell>
      <TableCell sx={{ color: statusColor(payment?.status) }}>
        <Typography variant="inherit">{labels.status[payment?.status ?? PaymentStatus.created]}</Typography>
      </TableCell>
      <TableCell>
        <Typography variant="inherit">{provider?.name}</Typography>
      </TableCell>
      <TableCell>
        {owner?.nick && (
          <Stack direction="row" alignItems="center" gap={1}>
            <UserAvatar
              sx={{ width: 24, height: 24 }}
              overwriteTextSize
              user={{
                id: payment?.user_hash_id ?? "",
                nickname: owner.nick,
                created_at: owner.created_at,
              }}
            />
            <Typography variant="inherit">
              <Link
                sx={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}
                onClick={handleUserClick}
                underline="hover"
                href="#/"
              >
                {owner.nick}
              </Link>
            </Typography>
          </Stack>
        )}
      </TableCell>
      <TableCell>
        <Typography variant="inherit">
          <TimeBadge datetime={payment?.created_at ?? ""} variant="inherit" timeAgoThreshold={0} />
        </Typography>
      </TableCell>
      <TableCell>
        <Stack direction="row" alignItems="center" gap={1}>
          {payment?.status === PaymentStatus.created && payment.provider_id && (
            <Tooltip title={t("vas.manualFulfill")}>
              <span>
                <IconButton
                  size="small"
                  color="success"
                  onClick={handleFulfill}
                  disabled={fulfillLoading || deleteLoading || deleting}
                >
                  <CheckCircleFilled fontSize="small" />
                </IconButton>
              </span>
            </Tooltip>
          )}
          <IconButton size="small" onClick={handleDelete} disabled={deleteLoading || deleting}>
            <Delete fontSize="small" />
          </IconButton>
        </Stack>
      </TableCell>
    </TableRow>
  );
};

const PaymentList = () => {
  const { t } = useTranslation("dashboard");
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"));
  const dispatch = useAppDispatch();
  const [loading, setLoading] = useState(true);
  const [payments, setPayments] = useState<Payment[]>([]);
  const [page, setPage] = useQueryState(PageQuery, { defaultValue: "1" });
  const [pageSize, setPageSize] = useQueryState(PageSizeQuery, { defaultValue: "10" });
  const [orderBy, setOrderBy] = useQueryState(OrderByQuery, { defaultValue: "" });
  const [orderDirection, setOrderDirection] = useQueryState(OrderDirectionQuery, { defaultValue: "desc" });
  const [tradeNo, setTradeNo] = useQueryState(PaymentTradeNoQuery, { defaultValue: "" });
  const [productType, setProductType] = useQueryState(PaymentProductTypeQuery, { defaultValue: "" });
  const [provider, setProvider] = useQueryState(PaymentProviderQuery, { defaultValue: "" });
  const [status, setStatus] = useQueryState(PaymentStatusQuery, { defaultValue: "" });
  const [user, setUser] = useQueryState(PaymentUserQuery, { defaultValue: "" });
  const [count, setCount] = useState(0);
  const [selected, setSelected] = useState<readonly number[]>([]);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [userDialogOpen, setUserDialogOpen] = useState(false);
  const [userDialogID, setUserDialogID] = useState<number>();
  const filterPopupState = usePopupState({ variant: "popover", popupId: "paymentFilterPopover" });
  const pageInt = parseInt(page) ?? 1;
  const pageSizeInt = parseInt(pageSize) ?? 10;

  useEffect(() => {
    dispatch(loadSiteConfig("vas"));
  }, [dispatch]);

  const clearFilters = useCallback(() => {
    setUser("");
    setTradeNo("");
    setProductType("");
    setProvider("");
    setStatus("");
  }, [setUser, setTradeNo, setProductType, setProvider, setStatus]);

  const fetchPayments = useCallback(() => {
    setLoading(true);
    setSelected([]);
    const params: AdminListService = {
      page: pageInt,
      page_size: pageSizeInt,
      order_by: orderBy ?? "",
      order_direction: orderDirection ?? "desc",
      conditions: {
        payment_user_id: user,
        payment_product_type: productType,
        payment_provider_id: provider,
        payment_status: status,
        payment_trade_no: tradeNo,
      },
    };

    dispatch(adminListPayments(params))
      .then((res) => {
        setPayments(res.payments ?? []);
        setPageSize(res.pagination?.page_size?.toString() ?? "10");
        setCount(res.pagination?.total_items ?? 0);
      })
      .finally(() => setLoading(false));
  }, [dispatch, pageInt, pageSizeInt, orderBy, orderDirection, user, productType, provider, status, tradeNo, setPageSize]);

  useEffect(() => {
    fetchPayments();
  }, [fetchPayments]);

  const handleSelectAll = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSelected(e.target.checked ? payments.map((item) => item.id) : []);
  };

  const handleSelect = useCallback((id: number) => {
    setSelected((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  }, []);

  const handleBatchDelete = () => {
    setDeleteLoading(true);
    dispatch(confirmOperation(t("vas.confirmDelete")))
      .then(() => {
        dispatch(adminBatchDeletePayments(Array.from(selected))).then(() => fetchPayments());
      })
      .finally(() => setDeleteLoading(false));
  };

  const idSorted = orderBy === "id" || orderBy === "";
  const direction = (orderDirection as "asc" | "desc") ?? "desc";
  const handleSort = (column: string) => () => {
    const current = orderBy === column || (column === "id" && idSorted);
    setOrderBy(column);
    setOrderDirection(current && direction === "asc" ? "desc" : "asc");
  };
  const hasActiveFilters = !!(user || tradeNo || productType || provider || status);
  const openUserDialog = useCallback((id?: number) => {
    setUserDialogID(id);
    setUserDialogOpen(true);
  }, []);

  return (
    <PageContainer>
      <UserDialog open={userDialogOpen} onClose={() => setUserDialogOpen(false)} userID={userDialogID} />
      <Container maxWidth="xl">
        <PageHeader title={t("dashboard:vas.orders")} />
        <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
          <PaymentFilterPopover
            {...bindPopover(filterPopupState)}
            user={user}
            setUser={setUser}
            tradeNo={tradeNo}
            setTradeNo={setTradeNo}
            productType={productType}
            setProductType={setProductType}
            providerID={provider}
            setProviderID={setProvider}
            status={status}
            setStatus={setStatus}
            clearFilters={clearFilters}
          />
          <SecondaryButton onClick={fetchPayments} disabled={loading} variant="contained" startIcon={<ArrowSync />}>
            {t("node.refresh")}
          </SecondaryButton>
          <Badge color="primary" variant="dot" invisible={!hasActiveFilters}>
            <SecondaryButton startIcon={<Filter />} variant="contained" {...bindTrigger(filterPopupState)}>
              {t("user.filter")}
            </SecondaryButton>
          </Badge>
          {selected.length > 0 && !isMobile && (
            <>
              <Divider orientation="vertical" flexItem />
              <Button startIcon={<Delete />} variant="contained" color="error" onClick={handleBatchDelete} disabled={deleteLoading}>
                {t("payment.deleteXPayments", { num: selected.length })}
              </Button>
            </>
          )}
        </Stack>
        {isMobile && selected.length > 0 && (
          <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
            <Button startIcon={<Delete />} variant="contained" color="error" onClick={handleBatchDelete} disabled={deleteLoading}>
              {t("payment.deleteXPayments", { num: selected.length })}
            </Button>
          </Stack>
        )}
        <TableContainer component={StyledTableContainerPaper} sx={{ mt: 2 }}>
          <Table size="small" stickyHeader sx={{ width: "100%", tableLayout: "fixed" }}>
            <TableHead>
              <TableRow>
                <TableCell padding="checkbox" sx={{ width: "36px!important" }} width={50}>
                  <Checkbox
                    size="small"
                    indeterminate={selected.length > 0 && selected.length < payments.length}
                    checked={payments.length > 0 && selected.length === payments.length}
                    onChange={handleSelectAll}
                  />
                </TableCell>
                <TableCell width={80}>
                  <TableSortLabel active={idSorted} direction={idSorted ? direction : "asc"} onClick={handleSort("id")}>
                    {t("group.#")}
                  </TableSortLabel>
                </TableCell>
                <TableCell width={250}>{t("vas.orderName")}</TableCell>
                <TableCell width={100}>{t("vas.product")}</TableCell>
                <TableCell width={250}>{t("vas.orderNumber")}</TableCell>
                <TableCell width={150}>{t("vas.amount")}</TableCell>
                <TableCell width={100}>{t("vas.status")}</TableCell>
                <TableCell width={100}>{t("vas.paidBy")}</TableCell>
                <TableCell width={150}>{t("vas.orderOwner")}</TableCell>
                <TableCell width={180}>
                  <TableSortLabel
                    active={orderBy === "created_at"}
                    direction={orderBy === "created_at" ? direction : "asc"}
                    onClick={handleSort("created_at")}
                  >
                    {t("file.createdAt")}
                  </TableSortLabel>
                </TableCell>
                <TableCell width={80} />
              </TableRow>
            </TableHead>
            <TableBody>
              {!loading &&
                payments.map((payment) => (
                  <PaymentRow
                    key={payment.id}
                    payment={payment}
                    deleting={deleteLoading}
                    selected={selected.includes(payment.id)}
                    onSelect={handleSelect}
                    onDelete={fetchPayments}
                    onFulfill={fetchPayments}
                    openUserDialog={openUserDialog}
                  />
                ))}
              {loading &&
                (payments.length > 0 ? payments.slice(0, 10) : Array.from(Array(10))).map((payment, index) => (
                  <PaymentRow key={`loading-${payment?.id ?? index}`} loading />
                ))}
            </TableBody>
          </Table>
        </TableContainer>
        {count > 0 && (
          <Box sx={{ mt: 1 }}>
            <TablePagination
              page={pageInt}
              totalItems={count}
              rowsPerPage={pageSizeInt}
              rowsPerPageOptions={[10, 25, 50, 100, 200, 500]}
              onRowsPerPageChange={(value) => setPageSize(value.toString())}
              onChange={(_, value) => setPage(value.toString())}
            />
          </Box>
        )}
      </Container>
    </PageContainer>
  );
};

export default PaymentList;
