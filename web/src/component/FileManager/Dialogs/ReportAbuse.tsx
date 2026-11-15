import { DialogContent, ListItemText, Stack, Typography } from "@mui/material";
import { useSnackbar } from "notistack";
import { useCallback, useContext, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { sendSiteAbuseReport } from "../../../api/api.ts";
import { FileType } from "../../../api/explorer.ts";
import { User } from "../../../api/user.ts";
import { closeReportAbuseDialog } from "../../../redux/globalStateSlice.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { refreshFileList } from "../../../redux/thunks/filemanager.ts";
import { Captcha, CaptchaParams } from "../../Common/Captcha/Captcha.tsx";
import { DefaultCloseAction } from "../../Common/Snackbar/snackbar.tsx";
import { DenseFilledTextField, DenseSelect } from "../../Common/StyledComponents.tsx";
import DraggableDialog from "../../Dialogs/DraggableDialog.tsx";
import SettingForm from "../../Pages/Setting/SettingForm.tsx";
import { SquareMenuItem } from "../ContextMenu/ContextMenu.tsx";
import FileBadge from "../FileBadge.tsx";
import { FmIndexContext } from "../FmIndexContext.tsx";
import UserAvatar from "../../Common/User/UserAvatar.tsx";

const ReportAbuse = () => {
  const { t } = useTranslation(["application", "dashboard"]);
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const fmIndex = useContext(FmIndexContext);

  const open = useAppSelector((state) => state.globalState.reportAbuseDialogOpen);
  const share = useAppSelector((state) => state.globalState.reportAbuseDialogShare);
  const user = useAppSelector((state) => state.globalState.reportAbuseDialogUser);
  const abuseCaptcha = useAppSelector((state) => state.siteConfig.login.config.abuse_report_captcha);

  const [category, setCategory] = useState(0);
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(false);
  const [captchaGen, setCaptchaGen] = useState(0);
  const captchaState = useRef<CaptchaParams>();

  const reasons = t("vas.reportReasonOptions", { ns: "application", returnObjects: true }) as string[];

  useEffect(() => {
    if (open) {
      setCategory(0);
      setDescription("");
      setCaptchaGen((prev) => prev + 1);
    }
  }, [open]);

  const onClose = useCallback(() => {
    if (!loading) {
      dispatch(closeReportAbuseDialog());
    }
  }, [dispatch, loading]);

  const onAccept = useCallback(() => {
    if (!share && !user) {
      return;
    }

    setLoading(true);
    dispatch(
      sendSiteAbuseReport({
        category,
        description: description.trim() || undefined,
        file_uri: share?.source_uri,
        share_id: share?.id,
        user_id: user?.id,
        password: share?.password,
        ...captchaState.current,
      }),
    )
      .then(() => {
        enqueueSnackbar({
          message: t("vas.reportAbuseSuccess", { ns: "application" }),
          variant: "success",
          action: DefaultCloseAction,
        });
        dispatch(closeReportAbuseDialog());
        dispatch(refreshFileList(fmIndex));
      })
      .catch(() => {
        setCaptchaGen((prev) => prev + 1);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [category, description, dispatch, enqueueSnackbar, fmIndex, share, t, user]);

  return (
    <DraggableDialog
      title={t("vas.report", { ns: "application" })}
      showActions
      showCancel
      loading={loading}
      onAccept={onAccept}
      dialogProps={{
        open: open ?? false,
        onClose: onClose,
        fullWidth: true,
        maxWidth: "xs",
      }}
    >
      <DialogContent>
        <Stack spacing={2}>
          <SettingForm title={t("vas.reportTarget", { ns: "application" })} lgWidth={12} noContainer>
            {share?.source_uri && !user && (
              <FileBadge
                clickable={false}
                variant="outlined"
                simplifiedFile={{
                  path: share.source_uri,
                  type: share.source_type === FileType.file ? FileType.file : FileType.folder,
                }}
              />
            )}
            {user && (
              <Stack direction="row" alignItems="center" spacing={1}>
                <UserAvatar sx={{ width: 24, height: 24 }} overwriteTextSize user={user} />
                <Typography variant="body2">{user.nickname}</Typography>
              </Stack>
            )}
          </SettingForm>

          <SettingForm title={t("vas.reportReason", { ns: "application" })} lgWidth={12} noContainer>
            <DenseSelect
              fullWidth
              value={category}
              onChange={(e) => setCategory(Number(e.target.value))}
            >
              {reasons.map((reason, index) => (
                <SquareMenuItem key={index} value={index}>
                  <ListItemText primaryTypographyProps={{ variant: "body2" }}>{reason}</ListItemText>
                </SquareMenuItem>
              ))}
            </DenseSelect>
          </SettingForm>

          <SettingForm title={t("vas.reportDescription", { ns: "application" })} lgWidth={12} noContainer>
            <DenseFilledTextField
              fullWidth
              multiline
              minRows={4}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              slotProps={{ htmlInput: { maxLength: 512 } }}
            />
          </SettingForm>

          {abuseCaptcha && (
            <SettingForm title={t("login.captcha", { ns: "application" })} lgWidth={12} noContainer>
              <Captcha
                key={captchaGen}
                generation={captchaGen}
                required
                fullWidth
                onStateChange={(state) => (captchaState.current = state)}
              />
            </SettingForm>
          )}
        </Stack>
      </DialogContent>
    </DraggableDialog>
  );
};

export default ReportAbuse;
