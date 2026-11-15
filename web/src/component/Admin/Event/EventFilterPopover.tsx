import { Box, Button, ListItemText, Popover, PopoverProps, Stack } from "@mui/material";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { DenseFilledTextField } from "../../Common/StyledComponents";
import { SquareMenuItem } from "../../FileManager/ContextMenu/ContextMenu";
import SettingForm from "../../Pages/Setting/SettingForm";
import EventCategorySelector from "./EventTypeSelector";

export interface EventFilterPopoverProps extends PopoverProps {
  user: string;
  setUser: (user: string) => void;
  ip: string;
  setIp: (ip: string) => void;
  categories: number[];
  setCategories: (categories: number[]) => void;
  correlationID: string;
  setCorrelationID: (correlationID: string) => void;
  fileID: string;
  setFileID: (fileID: string) => void;
  clearFilters: () => void;
}

const EventFilterPopover = ({
  user,
  setUser,
  ip,
  setIp,
  categories,
  setCategories,
  correlationID,
  setCorrelationID,
  fileID,
  setFileID,
  clearFilters,
  onClose,
  open,
  ...rest
}: EventFilterPopoverProps) => {
  const { t } = useTranslation("dashboard");

  const [localUser, setLocalUser] = useState(user);
  const [localIp, setLocalIp] = useState(ip);
  const [localCategories, setLocalCategories] = useState<number[]>(categories);
  const [localCorrelationID, setLocalCorrelationID] = useState(correlationID);
  const [localFileID, setLocalFileID] = useState(fileID);

  useEffect(() => {
    if (open) {
      setLocalUser(user);
      setLocalIp(ip);
      setLocalCategories(categories);
      setLocalCorrelationID(correlationID);
      setLocalFileID(fileID);
    }
  }, [open]);

  const handleApplyFilters = () => {
    setUser(localUser);
    setIp(localIp);
    setCategories(localCategories);
    setCorrelationID(localCorrelationID);
    setFileID(localFileID);
    onClose?.({}, "backdropClick");
  };

  const handleResetFilters = () => {
    setLocalUser("");
    setLocalIp("");
    setLocalCategories([]);
    setLocalCorrelationID("");
    setLocalFileID("");
    clearFilters();
    onClose?.({}, "backdropClick");
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

        <SettingForm title={t("event.ip")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localIp}
            onChange={(e) => setLocalIp(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>

        <SettingForm title={t("event.type")} noContainer lgWidth={12}>
          <EventCategorySelector value={localCategories} onChange={setLocalCategories} displayEmpty />
        </SettingForm>

        <SettingForm title={t("event.correlationId")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localCorrelationID}
            onChange={(e) => setLocalCorrelationID(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>

        <SettingForm title={t("event.fileID")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localFileID}
            onChange={(e) => setLocalFileID(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>

        <Box display="flex" justifyContent="space-between">
          <Button variant="outlined" size="small" onClick={handleResetFilters}>
            {t("user.reset")}
          </Button>
          <Button variant="contained" size="small" onClick={handleApplyFilters}>
            {t("user.apply")}
          </Button>
        </Box>
      </Stack>
    </Popover>
  );
};

export default EventFilterPopover;
