import { Box, Button, ListItemText, Popover, PopoverProps, Stack } from "@mui/material";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { DenseFilledTextField, DenseSelect } from "../../Common/StyledComponents";
import { SquareMenuItem } from "../../FileManager/ContextMenu/ContextMenu";
import SettingForm from "../../Pages/Setting/SettingForm";

export interface AbuseReportFilterPopoverProps extends PopoverProps {
  user: string;
  setUser: (value: string) => void;
  reporter: string;
  setReporter: (value: string) => void;
  share: string;
  setShare: (value: string) => void;
  reason: string;
  setReason: (value: string) => void;
  clearFilters: () => void;
}

const AbuseReportFilterPopover = ({
  user,
  setUser,
  reporter,
  setReporter,
  share,
  setShare,
  reason,
  setReason,
  clearFilters,
  onClose,
  open,
  ...rest
}: AbuseReportFilterPopoverProps) => {
  const { t } = useTranslation("dashboard");

  const [localUser, setLocalUser] = useState(user);
  const [localReporter, setLocalReporter] = useState(reporter);
  const [localShare, setLocalShare] = useState(share);
  const [localReason, setLocalReason] = useState(reason);

  useEffect(() => {
    if (open) {
      setLocalUser(user);
      setLocalReporter(reporter);
      setLocalShare(share);
      setLocalReason(reason);
    }
  }, [open]);

  const handleApplyFilters = () => {
    setUser(localUser);
    setReporter(localReporter);
    setShare(localShare);
    setReason(localReason);
    onClose?.({}, "backdropClick");
  };

  const handleResetFilters = () => {
    setLocalUser("");
    setLocalReporter("");
    setLocalShare("");
    setLocalReason("");
    clearFilters();
    onClose?.({}, "backdropClick");
  };

  const reasons = t("application:vas.reportReasonOptions", { returnObjects: true }) as string[];

  return (
    <Popover
      anchorOrigin={{
        vertical: "bottom",
        horizontal: "left",
      }}
      transformOrigin={{
        vertical: "top",
        horizontal: "left",
      }}
      slotProps={{
        paper: {
          sx: {
            p: 2,
            width: 300,
            maxWidth: "100%",
          },
        },
      }}
      onClose={onClose}
      open={open}
      {...rest}
    >
      <Stack spacing={2}>
        <SettingForm title={t("abuseReport.reporterID")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localReporter}
            onChange={(e) => setLocalReporter(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>

        <SettingForm title={t("abuseReport.reportedUserID")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localUser}
            onChange={(e) => setLocalUser(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>

        <SettingForm title={t("abuseReport.shareID")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localShare}
            onChange={(e) => setLocalShare(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>

        <SettingForm title={t("abuseReport.reason")} noContainer lgWidth={12}>
          <DenseSelect
            fullWidth
            size="small"
            value={localReason}
            onChange={(e) => setLocalReason(e.target.value as string)}
            displayEmpty
          >
            {reasons.map((label, index) => (
              <SquareMenuItem key={index} value={index.toString()}>
                <ListItemText primaryTypographyProps={{ variant: "body2" }}>{label}</ListItemText>
              </SquareMenuItem>
            ))}
            <SquareMenuItem value="">
              <ListItemText primaryTypographyProps={{ variant: "body2", fontStyle: "italic" }}>
                {t("user.all")}
              </ListItemText>
            </SquareMenuItem>
          </DenseSelect>
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

export default AbuseReportFilterPopover;
