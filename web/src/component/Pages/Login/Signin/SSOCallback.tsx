import { Box, Typography } from "@mui/material";
import { enqueueSnackbar } from "notistack";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { exchangeOIDCTicket, exchangeProOpenID } from "../../../../api/api.ts";
import { AppError } from "../../../../api/request.ts";
import { useAppDispatch } from "../../../../redux/hooks.ts";
import { refreshUserSession } from "../../../../redux/thunks/session.ts";
import { useQuery } from "../../../../util";
import { DefaultCloseAction } from "../../../Common/Snackbar/snackbar.tsx";
import DismissCircleFilled from "../../../Icons/DismissCircleFilled.tsx";

export interface SSOCallbackProps {
  providerId?: number;
}

const SSOCallback = ({ providerId }: SSOCallbackProps) => {
  const { t } = useTranslation(["application", "dashboard"]);
  const dispatch = useAppDispatch();
  const query = useQuery();
  const params = useParams();
  const [error, setError] = useState<string>();

  useEffect(() => {
    const errorCode = query.get("error");
    const description = query.get("error_description");
    const ticket = query.get("ticket");
    const code = query.get("code");
    const state = query.get("state");
    const linked = query.get("linked");
    const redirect = query.get("redirect");

    if (errorCode) {
      setError(description || errorCode);
      return;
    }

    if (linked === "1") {
      enqueueSnackbar({
        message: t("setting.ssoLinked", { ns: "application" }),
        variant: "success",
        action: DefaultCloseAction,
      });
      window.location.href = redirect || "/setting?tab=security";
      return;
    }

    if (ticket) {
      dispatch(exchangeOIDCTicket(ticket))
        .then((session) => {
          dispatch(refreshUserSession(session, redirect));
        })
        .catch((e) => {
          setError(e instanceof AppError ? e.message : t("login.ssoLoginFailed", { ns: "application" }));
        });
      return;
    }

    const parsedProvider = Number.parseInt(params.provider ?? "", 10);
    if (code) {
      dispatch(exchangeProOpenID(providerId !== undefined ? providerId : Number.isNaN(parsedProvider) ? 0 : parsedProvider, code, state ?? ""))
        .then((session) => {
          if (session?.user) {
            dispatch(refreshUserSession(session, redirect));
          } else {
            enqueueSnackbar({
              message: t("setting.ssoLinked", { ns: "application" }),
              variant: "success",
              action: DefaultCloseAction,
            });
            window.location.href = redirect || "/setting?tab=security";
          }
        })
        .catch((e) => {
          setError(e instanceof AppError ? e.message : t("login.ssoLoginFailed", { ns: "application" }));
        });
      return;
    }

    setError(t("login.ssoInvalidCallback", { ns: "application" }));
  }, []);

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        pt: 7,
        pb: 9,
      }}
    >
      {error ? (
        <>
          <DismissCircleFilled fontSize={"large"} color={"error"} />
          <Typography variant={"body2"} sx={{ color: (theme) => theme.palette.error.main, mt: 2 }}>
            {error}
          </Typography>
        </>
      ) : (
        <Typography variant={"body2"} color={"text.secondary"}>
          {t("login.ssoCompleting", { ns: "application" })}
        </Typography>
      )}
    </Box>
  );
};

export default SSOCallback;
