import { Button, DialogContent, Stack, Typography } from "@mui/material";
import { DateTimePicker, LocalizationProvider } from "@mui/x-date-pickers";
import { AdapterDayjs } from "@mui/x-date-pickers/AdapterDayjs";
import dayjs from "dayjs";
import { useSnackbar } from "notistack";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  archiveAuditLogs,
  cleanupAuditLogs,
  deleteArchiveJob,
  getArchiveJobStatus,
} from "../../../api/api";
import { ArchiveEventsService, ArchiveJob } from "../../../api/dashboard";
import { useAppDispatch } from "../../../redux/hooks";
import { confirmOperation } from "../../../redux/thunks/dialog";
import { DefaultCloseAction } from "../../Common/Snackbar/snackbar";
import DraggableDialog from "../../Dialogs/DraggableDialog";
import SettingForm from "../../Pages/Setting/SettingForm";
import EventCategorySelector from "./EventTypeSelector";

export interface EventArchiveDialogProps {
  open: boolean;
  onClose: () => void;
  onComplete?: () => void;
}

const waitForArchive = async (
  initial: ArchiveJob,
  poll: (id: string) => Promise<ArchiveJob>,
): Promise<ArchiveJob> => {
  let current = initial;
  const deadline = Date.now() + 24 * 60 * 60 * 1000;
  let delay = 1000;
  while (Date.now() < deadline) {
    if (current.status === "completed") {
      return current;
    }
    if (
      current.status === "failed" ||
      ((current.status === "pending" || current.status === "running") && Date.now() > Date.parse(current.expires))
    ) {
      throw new Error(current.error || "archive failed");
    }
    await new Promise((resolve) => window.setTimeout(resolve, delay));
    delay = Math.min(Math.round(delay * 1.5), 10000);
    current = await poll(initial.id);
  }
  throw new Error("archive timed out");
};

const EventArchiveDialog = ({ open, onClose, onComplete }: EventArchiveDialogProps) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const [notBefore, setNotBefore] = useState<dayjs.Dayjs | null>(null);
  const [notAfter, setNotAfter] = useState<dayjs.Dayjs | null>(null);
  const [categories, setCategories] = useState<number[]>([]);
  const [loading, setLoading] = useState(false);
  const valid = useMemo(() => !!notBefore && !!notAfter && notBefore.isBefore(notAfter), [notBefore, notAfter]);

  const handleReset = () => {
    setNotBefore(null);
    setNotAfter(null);
    setCategories([]);
  };

  const handleArchive = async () => {
    if (!notBefore || !notAfter) {
      return;
    }
    setLoading(true);
    try {
      const args: ArchiveEventsService = {
        not_before: notBefore.toISOString(),
        not_after: notAfter.toISOString(),
        categories: categories.length > 0 ? categories : undefined,
      };
      const job = await dispatch(archiveAuditLogs(args));
      const completed = await waitForArchive(job, async (id) => await dispatch(getArchiveJobStatus(id)));
      if (completed.url) {
        const iframe = document.createElement("iframe");
        iframe.hidden = true;
        iframe.src = completed.url;
        document.body.appendChild(iframe);
        window.setTimeout(() => iframe.remove(), 60000);
      }

      try {
        await dispatch(confirmOperation(t("event.confirmDeleteArchivedLogs")));
      } catch {
        onClose();
        return;
      }
      await dispatch(deleteArchiveJob(completed.id));
      onComplete?.();
      onClose();
      enqueueSnackbar({ message: t("event.archiveDownloadSuccess"), variant: "success", action: DefaultCloseAction });
    } catch (error) {
      enqueueSnackbar({
        message: t("event.archiveDownloadFailed") + (error instanceof Error ? `: ${error.message}` : ""),
        variant: "error",
        action: DefaultCloseAction,
      });
      onClose();
    } finally {
      setLoading(false);
    }
  };

  return (
    <DraggableDialog
      title={t("event.archiveAuditLog")}
      dialogProps={{ open, onClose, maxWidth: "sm", fullWidth: true }}
      showActions
      showCancel
      onAccept={handleArchive}
      loading={loading}
      disabled={!valid}
      okText={t("event.archive")}
      secondaryAction={
        <Button onClick={handleReset} disabled={loading}>
          {t("user.reset")}
        </Button>
      }
    >
      <DialogContent>
        <Stack spacing={3}>
          <Typography variant="body2" color="text.secondary">
            {t("event.archiveAuditLogDescription")}
          </Typography>
          <SettingForm title={t("event.archiveNotBefore")} noContainer lgWidth={12}>
            <LocalizationProvider dateAdapter={AdapterDayjs}>
              <DateTimePicker
                value={notBefore}
                maxDateTime={notAfter ?? undefined}
                onChange={(value) => setNotBefore(value)}
                slotProps={{ textField: { fullWidth: true, size: "small" } }}
              />
            </LocalizationProvider>
          </SettingForm>
          <SettingForm title={t("event.archiveNotAfter")} noContainer lgWidth={12}>
            <LocalizationProvider dateAdapter={AdapterDayjs}>
              <DateTimePicker
                value={notAfter}
                minDateTime={notBefore ?? undefined}
                onChange={(value) => setNotAfter(value)}
                slotProps={{ textField: { fullWidth: true, size: "small" } }}
              />
            </LocalizationProvider>
          </SettingForm>
          <SettingForm title={t("event.type")} noContainer lgWidth={12}>
            <EventCategorySelector
              value={categories}
              onChange={setCategories}
              helperText={t("event.archiveEventTypesDes")}
              showAllOption
              displayEmpty
            />
          </SettingForm>
        </Stack>
      </DialogContent>
    </DraggableDialog>
  );
};

export default EventArchiveDialog;
