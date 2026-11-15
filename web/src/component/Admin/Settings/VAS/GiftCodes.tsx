import {
  Box,
  Chip,
  DialogContent,
  FormControl,
  IconButton,
  ListItemText,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import { useSnackbar } from "notistack";
import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  adminCreateGiftCodes,
  adminDeleteGiftCode,
  adminListGiftCodes,
  adminUpdateGiftCode,
} from "../../../../api/api";
import { GiftCode, GiftCodeProduct, GiftCodeProductType } from "../../../../api/dashboard";
import { useAppDispatch } from "../../../../redux/hooks";
import { confirmOperation } from "../../../../redux/thunks/dialog";
import { DefaultCloseAction } from "../../../Common/Snackbar/snackbar";
import UserAvatar from "../../../Common/User/UserAvatar";
import {
  DenseFilledTextField,
  DenseSelect,
  NoWrapCell,
  NoWrapTableCell,
  SecondaryButton,
  StyledTableContainerPaper,
} from "../../../Common/StyledComponents";
import DraggableDialog from "../../../Dialogs/DraggableDialog";
import Add from "../../../Icons/Add";
import Delete from "../../../Icons/Delete";
import Edit from "../../../Icons/Edit";
import SettingForm from "../../../Pages/Setting/SettingForm";
import TablePagination from "../../Common/TablePagination";
import UserDialog from "../../User/UserDialog/UserDialog";
import { GroupProduct, StorageProduct } from "./types";

interface GiftCodesProps {
  storageProductsConfig: string;
  groupProductsConfig: string;
}

interface PaginationParams {
  page: number;
  perPage: number;
  total: number;
}

const parseProducts = <T,>(value: string): T[] => {
  try {
    const parsed = JSON.parse(value || "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
};

const GiftCodeStatusChip = ({ used }: { used: boolean }) => {
  const { t } = useTranslation("dashboard");

  return (
    <Chip
      color={used ? "default" : "success"}
      label={used ? t("giftCodes.giftCodeUsed") : t("giftCodes.giftCodeUnused")}
      size="small"
    />
  );
};

const CreateGiftCodesDialog = ({
  open,
  onClose,
  storageProducts,
  groupProducts,
  editCode,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  storageProducts: StorageProduct[];
  groupProducts: GroupProduct[];
  editCode?: GiftCode;
  onCreated: () => void;
}) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const formRef = useRef<HTMLFormElement>(null);
  const [type, setType] = useState<"points" | "storage" | "group">("points");
  const [productID, setProductID] = useState("");
  const [qyt, setQyt] = useState(0);
  const [num, setNum] = useState(1);
  const [loading, setLoading] = useState(false);
  const [generatedCodes, setGeneratedCodes] = useState<GiftCode[]>([]);
  const [resultOpen, setResultOpen] = useState(false);

  useEffect(() => {
    if (open && editCode) {
      const nextType =
        editCode.product?.type === GiftCodeProductType.storage
          ? "storage"
          : editCode.product?.type === GiftCodeProductType.group
            ? "group"
            : "points";
      const skuID =
        editCode.product?.sku_id ??
        (nextType === "storage"
          ? storageProducts.find((item) => item.name === editCode.product?.name)?.id
          : nextType === "group"
            ? groupProducts.find((item) => item.name === editCode.product?.name)?.id
            : "") ??
        "";
      setType(nextType);
      setProductID(skuID);
      setQyt(editCode.qyt ?? 0);
      setNum(1);
    }
    if (!open) {
      setTimeout(() => {
        setType("points");
        setProductID("");
        setQyt(0);
        setNum(1);
      }, 100);
    }
  }, [open, editCode, storageProducts, groupProducts]);

  useEffect(() => {
    if (!editCode) {
      setProductID("");
      setQyt(0);
    }
  }, [type, editCode]);

  const submit = () => {
    if (!formRef.current?.checkValidity()) {
      formRef.current?.reportValidity();
      return;
    }

    if (type !== "points" && !productID) {
      return;
    }

    let product: GiftCodeProduct = { type: GiftCodeProductType.points };
    if (type === "storage") {
      product = {
        type: GiftCodeProductType.storage,
        sku_id: productID,
      };
    } else if (type === "group") {
      product = {
        type: GiftCodeProductType.group,
        sku_id: productID,
      };
    }

    setLoading(true);
    const request = editCode
      ? dispatch(adminUpdateGiftCode({ id: editCode.id, qyt, product }))
      : dispatch(adminCreateGiftCodes({ num, qyt, product }));
    request
      .then((codes) => {
        if (!editCode) {
          setGeneratedCodes((codes as GiftCode[]) ?? []);
          setResultOpen(true);
        } else {
          enqueueSnackbar({
            message: t("giftCodes.giftCodeUpdated"),
            variant: "success",
            action: DefaultCloseAction,
          });
        }
        onCreated();
        onClose();
      })
      .finally(() => setLoading(false));
  };

  return (
    <>
      <DraggableDialog
        title={t(editCode ? "giftCodes.editGiftCode" : "giftCodes.generateGiftCodes")}
        showActions
        showCancel
        loading={loading}
        onAccept={submit}
        dialogProps={{ open, onClose, fullWidth: true, maxWidth: "sm" }}
      >
        <DialogContent>
          <Box component="form" ref={formRef} sx={{ display: "flex", flexDirection: "column", gap: 2, mt: 1 }}>
            <SettingForm title={t("giftCodes.giftCodeQuantity")} lgWidth={12}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  value={num}
                  onChange={(e) => setNum(parseInt(e.target.value) || 1)}
                  type="number"
                  inputProps={{ min: 1 }}
                  required
                  disabled={!!editCode}
                />
                <Typography variant="caption" color="text.secondary">
                  {t("giftCodes.giftCodeQuantityHelp")}
                </Typography>
              </FormControl>
            </SettingForm>
            <SettingForm title={t("giftCodes.giftCodeProductType")} lgWidth={12}>
              <FormControl fullWidth>
                <DenseSelect value={type} onChange={(e) => setType(e.target.value as "points" | "storage" | "group")}>
                  <MenuItem value="points">
                    <ListItemText slotProps={{ primary: { variant: "body2" } }}>
                      {t("giftCodes.giftCodeTypePoints")}
                    </ListItemText>
                  </MenuItem>
                  {storageProducts.length > 0 && (
                    <MenuItem value="storage">
                      <ListItemText slotProps={{ primary: { variant: "body2" } }}>
                        {t("giftCodes.giftCodeTypeStorage")}
                      </ListItemText>
                    </MenuItem>
                  )}
                  {groupProducts.length > 0 && (
                    <MenuItem value="group">
                      <ListItemText slotProps={{ primary: { variant: "body2" } }}>
                        {t("giftCodes.giftCodeTypeGroup")}
                      </ListItemText>
                    </MenuItem>
                  )}
                </DenseSelect>
              </FormControl>
            </SettingForm>
            {type === "points" && (
              <SettingForm title={t("giftCodes.giftCodePointsAmount")} lgWidth={12}>
                <FormControl fullWidth>
                  <DenseFilledTextField
                    value={qyt}
                    onChange={(e) => setQyt(parseInt(e.target.value) || 0)}
                    type="number"
                    inputProps={{ min: 1 }}
                    required
                  />
                  <Typography variant="caption" color="text.secondary">
                    {t("giftCodes.giftCodePointsAmountHelp")}
                  </Typography>
                </FormControl>
              </SettingForm>
            )}
            {type === "storage" && (
              <SettingForm title={t("giftCodes.giftCodeProduct")} lgWidth={12}>
                <FormControl fullWidth>
                  <DenseSelect value={productID} onChange={(e) => setProductID(e.target.value as string)} required>
                    {storageProducts.map((product) => (
                      <MenuItem key={product.id} value={product.id}>
                        <ListItemText slotProps={{ primary: { variant: "body2" } }}>{product.name}</ListItemText>
                      </MenuItem>
                    ))}
                  </DenseSelect>
                </FormControl>
              </SettingForm>
            )}
            {type === "group" && (
              <SettingForm title={t("giftCodes.giftCodeProduct")} lgWidth={12}>
                <FormControl fullWidth>
                  <DenseSelect value={productID} onChange={(e) => setProductID(e.target.value as string)} required>
                    {groupProducts.map((product) => (
                      <MenuItem key={product.id} value={product.id}>
                        <ListItemText slotProps={{ primary: { variant: "body2" } }}>{product.name}</ListItemText>
                      </MenuItem>
                    ))}
                  </DenseSelect>
                </FormControl>
              </SettingForm>
            )}
            {type !== "points" && (
              <SettingForm title={t("giftCodes.duratonTimes")} lgWidth={12}>
                <FormControl fullWidth>
                  <DenseFilledTextField
                    value={qyt}
                    onChange={(e) => setQyt(parseInt(e.target.value) || 0)}
                    type="number"
                    inputProps={{ min: 1 }}
                    required
                  />
                  <Typography variant="caption" color="text.secondary">
                    {t("giftCodes.duratonTimesDes")}
                  </Typography>
                </FormControl>
              </SettingForm>
            )}
          </Box>
        </DialogContent>
      </DraggableDialog>

      <DraggableDialog
        title={t("giftCodes.generatedCodesTitle")}
        showActions
        onAccept={() => setResultOpen(false)}
        dialogProps={{ open: resultOpen, onClose: () => setResultOpen(false), fullWidth: true, maxWidth: "md" }}
      >
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 2 }}>
            {t("giftCodes.generatedCodesDescription")}
          </Typography>
          <DenseFilledTextField
            fullWidth
            multiline
            rows={10}
            value={generatedCodes.map((code) => code.code).join("\n")}
            slotProps={{ input: { readOnly: true, sx: { fontFamily: "monospace" } } }}
          />
        </DialogContent>
      </DraggableDialog>
    </>
  );
};

const GiftCodes = ({ storageProductsConfig, groupProductsConfig }: GiftCodesProps) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingCode, setEditingCode] = useState<GiftCode>();
  const [giftCodes, setGiftCodes] = useState<GiftCode[]>([]);
  const [loading, setLoading] = useState(false);
  const [userDialogOpen, setUserDialogOpen] = useState(false);
  const [userDialogID, setUserDialogID] = useState<number>();
  const [pagination, setPagination] = useState<PaginationParams>({ page: 1, perPage: 10, total: 0 });
  const storageProducts = parseProducts<StorageProduct>(storageProductsConfig);
  const groupProducts = parseProducts<GroupProduct>(groupProductsConfig);

  const fetchCodes = useCallback(() => {
    setLoading(true);
    dispatch(
      adminListGiftCodes({
        page: pagination.page,
        page_size: pagination.perPage,
        order_by: "id",
        order_direction: "desc",
        conditions: {},
        searches: {},
      }),
    )
      .then((res) => {
        setGiftCodes(res.gift_codes ?? []);
        setPagination((prev) => ({
          ...prev,
          total: res.pagination?.total_items ?? 0,
          page: (res.pagination?.page ?? 0) + 1,
          perPage: res.pagination?.page_size ?? prev.perPage,
        }));
      })
      .finally(() => setLoading(false));
  }, [dispatch, pagination.page, pagination.perPage]);

  useEffect(() => {
    fetchCodes();
  }, [fetchCodes]);

  const handleDelete = (id: number) => {
    dispatch(confirmOperation(t("giftCodes.giftCodeDeleted"))).then(() => {
      dispatch(adminDeleteGiftCode(id)).then(() => {
        enqueueSnackbar({ message: t("giftCodes.giftCodeDeleted"), variant: "success", action: DefaultCloseAction });
        fetchCodes();
      });
    });
  };

  const handleEdit = (code: GiftCode) => {
    setEditingCode(code);
    setDialogOpen(true);
  };

  const productName = (code: GiftCode) => {
    const props = code.product_props ?? code.product;
    if (!props || props.type === GiftCodeProductType.points) {
      return t("giftCodes.giftCodeTypePoints");
    }
    if (props.type === GiftCodeProductType.storage && props.sku_id) {
      return storageProducts.find((item) => item.id === props.sku_id)?.name ?? t("giftCodes.giftCodeTypeStorage");
    }
    if (props.type === GiftCodeProductType.group && props.sku_id) {
      return groupProducts.find((item) => item.id === props.sku_id)?.name ?? t("giftCodes.giftCodeTypeGroup");
    }
    return t("giftCodes.giftCodeTypePoints");
  };

  return (
    <Stack spacing={2}>
      <UserDialog
        open={userDialogOpen}
        onClose={() => setUserDialogOpen(false)}
        userID={userDialogID}
      />
      <Stack direction="row" spacing={1}>
        <SecondaryButton variant="contained" startIcon={<Add />} onClick={() => setDialogOpen(true)}>
          {t("giftCodes.generateGiftCodes")}
        </SecondaryButton>
      </Stack>

      <StyledTableContainerPaper>
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <NoWrapTableCell>#</NoWrapTableCell>
                <NoWrapTableCell>{t("giftCodes.giftCodeProduct")}</NoWrapTableCell>
                <NoWrapTableCell>{t("giftCodes.giftCodeAmount")}</NoWrapTableCell>
                <NoWrapTableCell>{t("giftCodes.giftCode")}</NoWrapTableCell>
                <NoWrapTableCell>{t("giftCodes.giftCodeStatus")}</NoWrapTableCell>
                <NoWrapTableCell>{t("giftCodes.giftCodeUsedBy")}</NoWrapTableCell>
                <NoWrapTableCell align="right" />
              </TableRow>
            </TableHead>
            <TableBody>
              {giftCodes.map((code) => (
                <TableRow key={code.id}>
                  <NoWrapCell>{code.id}</NoWrapCell>
                  <NoWrapCell>{productName(code)}</NoWrapCell>
                  <NoWrapCell>{code.qyt}</NoWrapCell>
                  <NoWrapCell sx={{ fontFamily: "monospace" }}>{code.code}</NoWrapCell>
                  <NoWrapCell>
                    <GiftCodeStatusChip used={code.used} />
                  </NoWrapCell>
                  <NoWrapCell>
                    {code.edges?.user && (
                      <Stack direction="row" alignItems="center" spacing={1}>
                        <UserAvatar
                          sx={{ width: 24, height: 24 }}
                          overwriteTextSize
                          user={{
                            id: code.user_hash_id ?? "",
                            nickname: code.edges.user.nick ?? "",
                            created_at: code.edges.user.created_at ?? "",
                          }}
                        />
                        <Typography
                          variant="body2"
                          noWrap
                          sx={{ cursor: "pointer" }}
                          onClick={() => {
                            setUserDialogID(code.used_by);
                            setUserDialogOpen(true);
                          }}
                        >
                          {code.edges.user.nick}
                        </Typography>
                      </Stack>
                    )}
                  </NoWrapCell>
                  <NoWrapCell align="right">
                    <IconButton size="small" disabled={code.used} onClick={() => handleEdit(code)}>
                      <Edit fontSize="small" />
                    </IconButton>
                    <IconButton size="small" disabled={code.used} onClick={() => handleDelete(code.id)}>
                      <Delete fontSize="small" />
                    </IconButton>
                  </NoWrapCell>
                </TableRow>
              ))}
              {giftCodes.length === 0 && !loading && (
                <TableRow>
                  <NoWrapCell colSpan={7} align="center">
                    {t("giftCodes.noGiftCodes")}
                  </NoWrapCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
        {pagination.total > 0 && (
          <Box sx={{ px: 1 }}>
            <TablePagination
              totalItems={pagination.total}
              page={pagination.page}
              rowsPerPage={pagination.perPage}
              rowsPerPageOptions={[10, 25, 50, 100]}
              onRowsPerPageChange={(size) => setPagination({ page: 1, perPage: size, total: pagination.total })}
              onChange={(_, page) => setPagination({ ...pagination, page })}
            />
          </Box>
        )}
      </StyledTableContainerPaper>

      <CreateGiftCodesDialog
        open={dialogOpen}
        onClose={() => {
          setDialogOpen(false);
          setEditingCode(undefined);
        }}
        storageProducts={storageProducts}
        groupProducts={groupProducts}
        editCode={editingCode}
        onCreated={fetchCodes}
      />
    </Stack>
  );
};

export default GiftCodes;
