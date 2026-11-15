import { Icon } from "@iconify/react";
import {
  Box,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { UserSettings } from "../../../../api/user";
import { useAppSelector } from "../../../../redux/hooks";
import TimeBadge from "../../../Common/TimeBadge";
import Password from "../../../Icons/Password";
import PersonPasskey from "../../../Icons/PersonPasskey";
import PhoneLaptopOutlined from "../../../Icons/PhoneLaptopOutlined";
import SSO from "../../../Icons/SSO";

interface LoginActivityTableProps {
  settings: UserSettings;
}

const LoginActivityTable = ({ settings }: LoginActivityTableProps) => {
  const { t } = useTranslation();
  const loginProviders = useAppSelector((s) => s.siteConfig.login.config.login_providers ?? []);

  const columns = useMemo(
    () => [
      t("setting.loginWith"),
      t("setting.result"),
      t("setting.device"),
      t("setting.ip"),
      t("setting.time"),
    ],
    [t],
  );

  const renderLoginWith = (activity: NonNullable<UserSettings["login_activity"]>[number]) => {
    if (activity.webdav) {
      return (
        <>
          <PhoneLaptopOutlined fontSize="small" />
          {t("modals.webdav")}
        </>
      );
    }

    if (activity.login_with === "passkey") {
      return (
        <>
          <PersonPasskey fontSize="small" />
          {t("setting.loginWithPasskey", { name: activity.passkey })}
        </>
      );
    }

    if (activity.login_with === "openid") {
      const provider = loginProviders.find((item) => item.id === activity.open_id_provider);
      if (!provider) {
        return null;
      }
      return (
        <>
          {provider.icon ? <Icon icon={provider.icon} style={{ fontSize: 20 }} /> : <SSO fontSize="small" />}
          {t(provider.name)}
        </>
      );
    }

    return (
      <>
        <Password fontSize="small" />
        {t("login.password")}
      </>
    );
  };

  return (
    <TableContainer sx={{ mt: 1 }}>
      <Table size="small" sx={{ width: "100%" }}>
        <TableHead>
          <TableRow>
            {columns.map((column) => (
              <TableCell key={column}>{column}</TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {settings.login_activity?.map((activity, index) => (
            <TableRow key={index} hover>
              <TableCell>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>{renderLoginWith(activity)}</Box>
              </TableCell>
              <TableCell>
                <Chip
                  color={activity.success ? "success" : "error"}
                  size="small"
                  label={t(`setting.${activity.success ? "success" : "failed"}`)}
                />
              </TableCell>
              <TableCell>{`${activity.browser ?? ""} - ${activity.os ?? ""} - ${activity.device ?? ""}`}</TableCell>
              <TableCell>{activity.ip}</TableCell>
              <TableCell>
                <TimeBadge datetime={activity.created_at} variant="inherit" />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {!settings.login_activity?.length && (
        <Box sx={{ p: 1, width: "100%", textAlign: "center" }}>
          <Typography variant="caption" color="text.secondary">
            {t("application:setting.listEmpty")}
          </Typography>
        </Box>
      )}
    </TableContainer>
  );
};

export default LoginActivityTable;
