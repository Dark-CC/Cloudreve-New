import { DialogContent, IconButton, MenuItem, Stack, Tooltip } from "@mui/material";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { createUploadLink, UploadLinkCreateResponse } from "../../../api/uploadLink";
import { closeUploadLinkDialog } from "../../../redux/globalStateSlice";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks";
import { copyToClipboard, sendLink } from "../../../util";
import { FilledTextField } from "../../Common/StyledComponents";
import DraggableDialog from "../../Dialogs/DraggableDialog";
import CopyOutlined from "../../Icons/CopyOutlined";
import Share from "../../Icons/Share";

const UploadLinkDialog = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const target = useAppSelector((state) => state.globalState.uploadLinkDialogFile);
  const open = useAppSelector((state) => state.globalState.uploadLinkDialogOpen);
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<UploadLinkCreateResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Optional limits offered before the link is issued. Zero means "no cap"
  // for size and "server default" for expiry.
  const [expiry, setExpiry] = useState(0);
  const [maxSizeMB, setMaxSizeMB] = useState(0);

  const onClose = useCallback(() => {
    if (!loading) {
      setResult(null);
      setError(null);
      dispatch(closeUploadLinkDialog());
    }
  }, [dispatch, loading]);

  const onAccept = useCallback(async () => {
    if (!target || loading) {
      return;
    }
    if (result) {
      copyToClipboard(result.url);
      return;
    }

    setLoading(true);
    setError(null);
    try {
      // The folder URI is the directory that will receive uploads.
      const created = await createUploadLink({
        uri: target.path,
        expires: expiry || undefined,
        max_size: maxSizeMB > 0 ? maxSizeMB * 1024 * 1024 : undefined,
      });
      setResult(created);
    } catch (e: any) {
      setError(e?.response?.data?.msg || e?.message || t("uploadLink.createFailed"));
    } finally {
      setLoading(false);
    }
  }, [target, loading, result, expiry, maxSizeMB, t]);

  return (
    <DraggableDialog
      title={t("uploadLink.dialogTitle")}
      showActions
      showCancel
      loading={loading}
      onAccept={onAccept}
      dialogProps={{
        open: open ?? false,
        onClose,
        fullWidth: true,
        maxWidth: "xs",
      }}
      secondaryAction={
        result ? (
          <Tooltip title={t("uploadLink.send")}>
            <IconButton onClick={() => sendLink(target?.name ?? "", result.url)}>
              <Share />
            </IconButton>
          </Tooltip>
        ) : undefined
      }
    >
      <DialogContent sx={{ pb: 0 }}>
        <Stack spacing={1.5}>
          {!result && (
            <Stack spacing={1}>
              <FilledTextField
                label={t("uploadLink.folder")}
                value={target?.name ?? ""}
                disabled
                fullWidth
                variant="filled"
              />
              <FilledTextField
                select
                label={t("uploadLink.expiry")}
                value={expiry}
                onChange={(e) => setExpiry(Number(e.target.value))}
                fullWidth
                variant="filled"
              >
                <MenuItem value={0}>{t("uploadLink.expiryDefault")}</MenuItem>
                <MenuItem value={3600}>{t("uploadLink.expiryHour")}</MenuItem>
                <MenuItem value={86400}>{t("uploadLink.expiryDay")}</MenuItem>
                <MenuItem value={604800}>{t("uploadLink.expiryWeek")}</MenuItem>
                <MenuItem value={2592000}>{t("uploadLink.expiryMonth")}</MenuItem>
              </FilledTextField>
              <FilledTextField
                select
                label={t("uploadLink.maxFileSize")}
                value={maxSizeMB}
                onChange={(e) => setMaxSizeMB(Number(e.target.value))}
                fullWidth
                variant="filled"
              >
                <MenuItem value={0}>{t("uploadLink.sizeUnlimited")}</MenuItem>
                <MenuItem value={10}>{`10 MB`}</MenuItem>
                <MenuItem value={100}>{`100 MB`}</MenuItem>
                <MenuItem value={1024}>{`1 GB`}</MenuItem>
                <MenuItem value={5120}>{`5 GB`}</MenuItem>
              </FilledTextField>
              {error && <FilledTextField error value={error} fullWidth variant="filled" />}
            </Stack>
          )}
          {result && (
            <Stack spacing={1}>
              <FilledTextField
                variant="filled"
                inputProps={{ readOnly: true }}
                label={t("uploadLink.link")}
                fullWidth
                value={result.url}
                onFocus={(e) => e.target.select()}
                slotProps={{
                  input: {
                    endAdornment: (
                      <IconButton onClick={() => copyToClipboard(result.url)} size="small">
                        <CopyOutlined />
                      </IconButton>
                    ),
                  },
                }}
              />
            </Stack>
          )}
        </Stack>
      </DialogContent>
    </DraggableDialog>
  );
};

export default UploadLinkDialog;
