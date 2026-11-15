import { LoadingButton } from "@mui/lab";
import {
  Box,
  Button,
  DialogContent,
  Link,
  List,
  ListItem,
  ListItemText,
  Skeleton,
  Stack,
  Typography,
} from "@mui/material";
import { useSnackbar } from "notistack";
import { useCallback, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { refreshLicenseAuto, refreshLicenseManual } from "../../../api/api.ts";
import { License } from "../../../api/dashboard.ts";
import { useAppDispatch } from "../../../redux/hooks.ts";
import TimeBadge from "../../Common/TimeBadge.tsx";
import { DenseFilledTextField } from "../../Common/StyledComponents.tsx";
import { DefaultCloseAction } from "../../Common/Snackbar/snackbar.tsx";
import DraggableDialog from "../../Dialogs/DraggableDialog.tsx";
import ArrowSync from "../../Icons/ArrowSync.tsx";
import Setting from "../../Icons/Setting.tsx";

export interface LicenseCardProps {
  license?: License;
  onLicenseRefreshed: (license: License) => void;
}

export const LicenseCard = ({ license, onLicenseRefreshed }: LicenseCardProps) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const [loading, setLoading] = useState(false);
  const [manualOpen, setManualOpen] = useState(false);
  const [manualLicense, setManualLicense] = useState("");

  const handleRefreshed = useCallback(
    (refreshed: License) => {
      onLicenseRefreshed(refreshed);
      enqueueSnackbar({
        message: t("summary.refreshSuccessfully"),
        variant: "success",
        action: DefaultCloseAction,
      });
    },
    [enqueueSnackbar, onLicenseRefreshed, t],
  );

  const handleAutoRefresh = useCallback(() => {
    setLoading(true);
    dispatch(refreshLicenseAuto())
      .then(handleRefreshed)
      .catch(() => {
        setManualOpen(true);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [dispatch, handleRefreshed]);

  const handleManualRefresh = useCallback(() => {
    if (!manualLicense.trim()) {
      return;
    }
    setLoading(true);
    dispatch(refreshLicenseManual({ license: manualLicense }))
      .then((refreshed) => {
        setManualOpen(false);
        setManualLicense("");
        handleRefreshed(refreshed);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [dispatch, handleRefreshed, manualLicense]);

  return (
    <>
      <DraggableDialog
        title={t("summary.manualRefresh")}
        showActions
        showCancel
        onAccept={handleManualRefresh}
        loading={loading}
        disabled={!manualLicense.trim()}
        dialogProps={{
          open: manualOpen,
          onClose: () => setManualOpen(false),
          fullWidth: true,
          maxWidth: "sm",
        }}
      >
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 1 }}>
            <Trans
              ns="dashboard"
              i18nKey="summary.manualRefreshDes"
              components={[
                <Link href="/admin/settings" target="_blank" underline="hover">
                  {" "}
                </Link>,
              ]}
            />
          </Typography>
          <DenseFilledTextField
            fullWidth
            multiline
            rows={6}
            required
            value={manualLicense}
            onChange={(e) => setManualLicense(e.target.value)}
          />
        </DialogContent>
      </DraggableDialog>
      {!license && (
        <Box>
          <Skeleton variant="text" width="70%" />
          <Skeleton variant="text" width="50%" />
        </Box>
      )}
      {license && (
        <>
          <List disablePadding>
            <ListItem>
              <ListItemText
                primary={t("summary.licenseExpireAt")}
                secondary={t("summary.permanentLicense")}
              />
            </ListItem>
            <ListItem>
              <ListItemText
                primary={t("summary.offlineLicenseExpireAy")}
                secondary={
                  <Box component="span">
                    <TimeBadge datetime={license.expired_at} variant="inherit" />
                    <Typography variant="caption" component="p" sx={{ mt: 0.5 }}>
                      {t("summary.offlineLicenseDes")}
                    </Typography>
                  </Box>
                }
              />
            </ListItem>
            <ListItem>
              <ListItemText
                primary={t("summary.licensedDomains")}
                secondary={license.domains.join(", ")}
              />
            </ListItem>
            <ListItem>
              <ListItemText
                primary={t("summary.iosVol")}
                secondary={
                  <Box component="span">
                    {license.vol_domains.join(", ")}
                    <Typography variant="caption" component="p" sx={{ mt: 0.5 }}>
                      <Trans
                        ns="dashboard"
                        i18nKey="summary.volPurchase"
                        components={[
                          <Link href="/admin/settings" target="_blank" underline="hover">
                            {" "}
                          </Link>,
                          <Link href="https://cloudreve.org/ios" target="_blank" underline="hover">
                            {" "}
                          </Link>,
                        ]}
                      />
                    </Typography>
                  </Box>
                }
              />
            </ListItem>
          </List>
          <Stack spacing={1} direction="row" sx={{ mt: 2 }}>
            <Button
              variant="contained"
              startIcon={<Setting />}
              onClick={() => window.open("/admin/settings")}
            >
              {t("summary.manageLicense")}
            </Button>
            <LoadingButton
              variant="contained"
              startIcon={<ArrowSync />}
              onClick={handleAutoRefresh}
              loading={loading}
            >
              {t("summary.renew")}
            </LoadingButton>
          </Stack>
        </>
      )}
    </>
  );
};

export default LicenseCard;
