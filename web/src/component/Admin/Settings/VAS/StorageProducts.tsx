import {
  Box,
  DialogContent,
  FormControl,
  FormControlLabel,
  IconButton,
  Switch,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { DenseFilledTextField, NoWrapCell, NoWrapTableCell, SecondaryButton, StyledTableContainerPaper } from "../../../Common/StyledComponents";
import SizeInput from "../../../Common/SizeInput";
import DraggableDialog from "../../../Dialogs/DraggableDialog";
import Add from "../../../Icons/Add";
import ArrowDown from "../../../Icons/ArrowDown";
import Delete from "../../../Icons/Delete";
import Edit from "../../../Icons/Edit";
import SettingForm from "../../../Pages/Setting/SettingForm";
import { formatPrice, sizeToString } from "../../../../util";
import { StorageProduct } from "./types";

export interface StorageProductsProps {
  value: string;
  onChange: (value: string) => void;
  currencySymbol?: string;
  currencyUnit?: number;
}

const newID = () => {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
};

const parseValue = (value: string): StorageProduct[] => {
  try {
    const parsed = JSON.parse(value || "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
};

interface ProductDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (product: StorageProduct) => void;
  product?: StorageProduct;
  currencySymbol: string;
  currencyUnit: number;
}

const ProductDialog = ({ open, onClose, onSave, product, currencySymbol, currencyUnit }: ProductDialogProps) => {
  const { t } = useTranslation("dashboard");
  const formRef = useRef<HTMLFormElement>(null);
  const [usePoints, setUsePoints] = useState(false);
  const [values, setValues] = useState<StorageProduct>();

  useEffect(() => {
    if (product) {
      setValues({ ...product });
      setUsePoints(!!product.points);
    }
    if (!open) {
      setTimeout(() => {
        setValues(undefined);
        setUsePoints(false);
      }, 100);
    }
  }, [open, product]);

  const submit = () => {
    if (!formRef.current?.checkValidity()) {
      formRef.current?.reportValidity();
      return;
    }
    if (!values) {
      return;
    }
    const next = { ...values };
    next.chip = values.chip?.trim() ? values.chip : undefined;
    next.points = usePoints && values.points ? values.points : undefined;
    onSave(next);
    onClose();
  };

  return (
    <DraggableDialog
      title={t(product ? "settings.editStorageProduct" : "settings.addStorageProduct")}
      showActions
      showCancel
      onAccept={submit}
      dialogProps={{ open, onClose, fullWidth: true, maxWidth: "sm" }}
    >
      <DialogContent>
        {values && (
          <Box component="form" ref={formRef} sx={{ display: "flex", flexDirection: "column", gap: 2, mt: 1 }}>
            <SettingForm title={t("settings.displayName")} lgWidth={12}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  value={values.name}
                  onChange={(e) => setValues({ ...values, name: e.target.value })}
                  required
                />
              </FormControl>
            </SettingForm>
            <SettingForm title={t("settings.storageSize")} lgWidth={12}>
              <FormControl fullWidth>
                <SizeInput
                  variant="outlined"
                  required
                  allowZero={false}
                  value={values.size ?? 0}
                  onChange={(size) => setValues({ ...values, size })}
                />
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
                  {t("settings.storageSizeBytes")}
                </Typography>
              </FormControl>
            </SettingForm>
            <SettingForm title={t("settings.duration")} lgWidth={12}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  value={values.time ?? 0}
                  inputProps={{ min: 1 }}
                  type="number"
                  required
                  onChange={(e) => setValues({ ...values, time: parseInt(e.target.value) || 0 })}
                />
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
                  {t("settings.durationSeconds")}
                </Typography>
              </FormControl>
            </SettingForm>
            <SettingForm title={t("settings.priceInUnits")} lgWidth={12}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  value={values.price ?? 0}
                  inputProps={{ min: 1 }}
                  type="number"
                  required
                  onChange={(e) => setValues({ ...values, price: parseInt(e.target.value) || 0 })}
                />
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
                  {`${t("settings.priceInUnitsDes")} ${formatPrice(currencySymbol, values.price ?? 0, currencyUnit)}`}
                </Typography>
              </FormControl>
            </SettingForm>
            <SettingForm title={t("settings.chipLabel")} lgWidth={12}>
              <FormControl fullWidth>
                <DenseFilledTextField
                  value={values.chip ?? ""}
                  onChange={(e) => setValues({ ...values, chip: e.target.value })}
                />
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
                  {t("settings.chipLabelHelp")}
                </Typography>
              </FormControl>
            </SettingForm>
            <SettingForm lgWidth={12}>
              <FormControl>
                <FormControlLabel
                  control={<Switch checked={usePoints} onChange={(e) => setUsePoints(e.target.checked)} />}
                  label={t("settings.usePoints")}
                />
              </FormControl>
            </SettingForm>
            {usePoints && (
              <SettingForm title={t("settings.points")} lgWidth={12}>
                <FormControl fullWidth>
                  <DenseFilledTextField
                    value={values.points ?? 0}
                    inputProps={{ min: 1 }}
                    type="number"
                    required={usePoints}
                    onChange={(e) => setValues({ ...values, points: parseInt(e.target.value) || 0 })}
                  />
                  <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
                    {t("settings.pointsHelp")}
                  </Typography>
                </FormControl>
              </SettingForm>
            )}
          </Box>
        )}
      </DialogContent>
    </DraggableDialog>
  );
};

const StorageProducts = ({
  value,
  onChange,
  currencySymbol = "¥",
  currencyUnit = 100,
}: StorageProductsProps) => {
  const { t } = useTranslation("dashboard");
  const [products, setProducts] = useState<StorageProduct[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<StorageProduct>();

  useEffect(() => {
    setProducts(parseValue(value));
  }, [value]);

  const save = useCallback(
    (next: StorageProduct[]) => {
      setProducts(next);
      onChange(JSON.stringify(next));
    },
    [onChange],
  );

  const remove = useCallback(
    (id: string) => {
      save(products.filter((product) => product.id !== id));
    },
    [products, save],
  );

  const upsert = useCallback(
    (product: StorageProduct) => {
      const index = products.findIndex((item) => item.id === product.id);
      if (index >= 0) {
        const next = [...products];
        next[index] = product;
        save(next);
      } else {
        save([...products, product]);
      }
    },
    [products, save],
  );

  const move = useCallback(
    (from: number, to: number) => {
      if (from === to || to < 0 || to >= products.length) {
        return;
      }
      const next = [...products];
      const [moved] = next.splice(from, 1);
      next.splice(to, 0, moved);
      save(next);
    },
    [products, save],
  );

  const openAdd = () => {
    setEditing({ id: newID(), name: "", size: 0, time: 0, price: 0, chip: "", points: 0 });
    setDialogOpen(true);
  };

  const openEdit = (product: StorageProduct) => {
    setEditing(product);
    setDialogOpen(true);
  };

  return (
    <Box>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
        <SecondaryButton variant="contained" startIcon={<Add />} onClick={openAdd}>
          {t("settings.addStorageProduct")}
        </SecondaryButton>
      </Box>

      <TableContainer component={StyledTableContainerPaper}>
        <Table sx={{ width: "100%" }} size="small">
          <TableHead>
            <TableRow>
              <NoWrapTableCell>{t("settings.displayName")}</NoWrapTableCell>
              <NoWrapTableCell>{t("settings.price")}</NoWrapTableCell>
              <NoWrapTableCell>{t("settings.duration")}</NoWrapTableCell>
              <NoWrapTableCell>{t("settings.storageSize")}</NoWrapTableCell>
              <NoWrapTableCell>{t("settings.actions")}</NoWrapTableCell>
              <NoWrapTableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {products.map((product, index) => (
              <TableRow key={product.id} hover>
                <NoWrapCell>
                  {product.name}
                  {product.chip && (
                    <Box
                      component="span"
                      sx={{
                        ml: 1,
                        px: 1,
                        py: 0.5,
                        borderRadius: 1,
                        bgcolor: "primary.main",
                        color: "primary.contrastText",
                        fontSize: "0.75rem",
                      }}
                    >
                      {product.chip}
                    </Box>
                  )}
                </NoWrapCell>
                <NoWrapCell>
                  {product.price > 0 && <span>{formatPrice(currencySymbol, product.price, currencyUnit)}</span>}
                  {product.points ? (
                    <span style={{ marginLeft: product.price > 0 ? 8 : 0 }}>
                      {product.points} {t("settings.pointsUnit")}
                    </span>
                  ) : null}
                </NoWrapCell>
                <NoWrapCell>
                  {t("application:vas.validDurationDays", { num: Math.ceil((product.time || 0) / 86400) })}
                </NoWrapCell>
                <NoWrapCell>{sizeToString(product.size || 0)}</NoWrapCell>
                <NoWrapCell>
                  <IconButton size="small" onClick={() => openEdit(product)}>
                    <Edit fontSize="small" />
                  </IconButton>
                  <IconButton size="small" onClick={() => remove(product.id)}>
                    <Delete fontSize="small" />
                  </IconButton>
                </NoWrapCell>
                <NoWrapCell>
                  <IconButton size="small" onClick={() => move(index, index - 1)} disabled={index === 0}>
                    <ArrowDown sx={{ width: 18, height: 18, transform: "rotate(180deg)" }} />
                  </IconButton>
                  <IconButton
                    size="small"
                    onClick={() => move(index, index + 1)}
                    disabled={index === products.length - 1}
                  >
                    <ArrowDown sx={{ width: 18, height: 18 }} />
                  </IconButton>
                </NoWrapCell>
              </TableRow>
            ))}
            {products.length === 0 && (
              <TableRow>
                <NoWrapCell colSpan={6} align="center">
                  <Typography variant="caption" color="text.secondary">
                    {t("application:setting.listEmpty")}
                  </Typography>
                </NoWrapCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <ProductDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        onSave={upsert}
        product={editing}
        currencySymbol={currencySymbol}
        currencyUnit={currencyUnit}
      />
    </Box>
  );
};

export default StorageProducts;
