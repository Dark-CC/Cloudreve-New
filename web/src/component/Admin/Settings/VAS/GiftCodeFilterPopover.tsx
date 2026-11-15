import { Box, Button, MenuItem, Popover, PopoverProps, Stack } from "@mui/material";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { DenseFilledTextField, DenseSelect } from "../../../Common/StyledComponents";
import SettingForm from "../../../Pages/Setting/SettingForm";

export interface GiftCodeFilters {
  code: string;
  used: string;
  userID: string;
}

export const emptyGiftCodeFilters = (): GiftCodeFilters => ({
  code: "",
  used: "",
  userID: "",
});

export interface GiftCodeFilterPopoverProps extends PopoverProps {
  filters: GiftCodeFilters;
  onApply: (filters: GiftCodeFilters) => void;
  onReset: () => void;
}

const GiftCodeFilterPopover = ({ filters, onApply, onReset, onClose, open, ...rest }: GiftCodeFilterPopoverProps) => {
  const { t } = useTranslation("dashboard");
  const [local, setLocal] = useState<GiftCodeFilters>(filters);

  useEffect(() => {
    if (open) {
      setLocal(filters);
    }
  }, [open, filters]);

  const apply = () => {
    onApply(local);
    onClose?.({}, "backdropClick");
  };

  const reset = () => {
    setLocal(emptyGiftCodeFilters());
    onReset();
    onClose?.({}, "backdropClick");
  };

  return (
    <Popover
      anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
      transformOrigin={{ vertical: "top", horizontal: "left" }}
      slotProps={{ paper: { sx: { p: 2, width: 320, maxWidth: "100%" } } }}
      onClose={onClose}
      open={open}
      {...rest}
    >
      <Stack spacing={2}>
        <SettingForm title={t("giftCodes.giftCode")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            size="small"
            value={local.code}
            onChange={(e) => setLocal((prev) => ({ ...prev, code: e.target.value }))}
            placeholder={t("user.emptyNoFilter")}
          />
        </SettingForm>
        <SettingForm title={t("giftCodes.giftCodeStatus")} noContainer lgWidth={12}>
          <DenseSelect
            fullWidth
            size="small"
            value={local.used}
            onChange={(e) => setLocal((prev) => ({ ...prev, used: e.target.value as string }))}
          >
            <MenuItem value="">{t("user.emptyNoFilter")}</MenuItem>
            <MenuItem value="false">{t("giftCodes.giftCodeUnused")}</MenuItem>
            <MenuItem value="true">{t("giftCodes.giftCodeUsed")}</MenuItem>
          </DenseSelect>
        </SettingForm>
        <SettingForm title={t("giftCodes.giftCodeUsedBy")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            size="small"
            value={local.userID}
            onChange={(e) => setLocal((prev) => ({ ...prev, userID: e.target.value }))}
            placeholder={t("user.emptyNoFilter")}
          />
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

export default GiftCodeFilterPopover;
