import { Button, DialogContent, Stack, Typography } from "@mui/material";
import { DateTimePicker, LocalizationProvider } from "@mui/x-date-pickers";
import { AdapterDayjs } from "@mui/x-date-pickers/AdapterDayjs";
import dayjs from "dayjs";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { cleanupAuditLogs } from "../../../api/api";
import { CleanupEventsService } from "../../../api/dashboard";
import { useAppDispatch } from "../../../redux/hooks";
import DraggableDialog from "../../Dialogs/DraggableDialog";
import SettingForm from "../../Pages/Setting/SettingForm";
import EventCategorySelector from "./EventTypeSelector";

export interface EventCleanupDialogProps {
  open: boolean;
  onClose: () => void;
  onComplete?: () => void;
}

const EventCleanupDialog = ({ open, onClose, onComplete }: EventCleanupDialogProps) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const [notAfter, setNotAfter] = useState<dayjs.Dayjs | null>(null);
  const [categories, setCategories] = useState<number[]>([]);
  const [loading, setLoading] = useState(false);

  const handleReset = () => {
    setNotAfter(null);
    setCategories([]);
  };

  const handleCleanup = async () => {
    if (!notAfter) {
      return;
    }
    setLoading(true);
    try {
      const args: CleanupEventsService = {
        not_after: notAfter.toISOString(),
        categories: categories.length > 0 ? categories : undefined,
      };
      await dispatch(cleanupAuditLogs(args));
      onComplete?.();
      onClose();
    } finally {
      setLoading(false);
    }
  };

  return (
    <DraggableDialog
      title={t("event.cleanupAuditLog")}
      dialogProps={{ open, onClose, maxWidth: "sm", fullWidth: true }}
      showActions
      showCancel
      onAccept={handleCleanup}
      loading={loading}
      disabled={!notAfter}
      okText={t("event.cleanup")}
      secondaryAction={
        <Button onClick={handleReset} disabled={loading}>
          {t("user.reset")}
        </Button>
      }
    >
      <DialogContent>
        <Stack spacing={3}>
          <Typography variant="body2" color="text.secondary">
            {t("event.cleanupAuditLogDescription")}
          </Typography>
          <SettingForm title={t("event.cleanupNotAfter")} noContainer lgWidth={12}>
            <LocalizationProvider dateAdapter={AdapterDayjs}>
              <DateTimePicker
                value={notAfter}
                onChange={(value) => setNotAfter(value)}
                slotProps={{ textField: { fullWidth: true, size: "small" } }}
              />
            </LocalizationProvider>
          </SettingForm>
          <SettingForm title={t("event.type")} noContainer lgWidth={12}>
            <EventCategorySelector
              value={categories}
              onChange={setCategories}
              helperText={t("event.cleanupEventTypesDes")}
              showAllOption
              displayEmpty
            />
          </SettingForm>
        </Stack>
      </DialogContent>
    </DraggableDialog>
  );
};

export default EventCleanupDialog;
