import { Alert, AlertTitle, Box, Button, CircularProgress, styled, Typography } from "@mui/material";
import React, { memo, useCallback, useContext, useMemo, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { getShareInfo, purchaseShare } from "../../../api/api.ts";
import { Share } from "../../../api/explorer.ts";
import { AppError, Code, Response, rememberPurchaseTicket } from "../../../api/request.ts";
import SessionManager from "../../../session";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { navigateToPath, refreshFileList, retrySharePassword } from "../../../redux/thunks/filemanager.ts";
import CrUri, { Filesystem } from "../../../util/uri.ts";
import { FilledTextField, SecondaryButton } from "../../Common/StyledComponents.tsx";
import ArrowLeft from "../../Icons/ArrowLeft.tsx";
import LinkDismiss from "../../Icons/LinkDismiss.tsx";
import LockClosed from "../../Icons/LockClosed.tsx";
import { FmIndexContext } from "../FmIndexContext.tsx";
import CoinStack from "../../Icons/CoinStack.tsx";

const PriceBadge = styled("span")(({ theme }) => ({
  boxShadow: `inset 0 0 0 1px ${theme.palette.primary.main}`,
  color: theme.palette.primary.main,
  padding: "0 6px",
  alignItems: "center",
  display: "inline-flex",
  borderRadius: "8px",
  gap: 4,
  verticalAlign: "middle",
  margin: "0 4px",
}));

const SharePrice = ({ price, size }: { price: number; size?: number }) => {
  const { t } = useTranslation();
  return (
    <PriceBadge>
      <CoinStack sx={{ height: size ?? 22, width: size ?? 22, py: "1px" }} />
      {t("application:share.points", { num: price.toLocaleString() })}
    </PriceBadge>
  );
};

interface ExplorerErrorProps {
  error?: Response<any>;
  [key: string]: any;
}

const RetryPassword = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const fmIndex = useContext(FmIndexContext);
  const [password, setPassword] = useState("");

  return (
    <Box sx={{ textAlign: "center" }} onClick={(e) => e.stopPropagation()} onMouseDown={(e) => e.stopPropagation()}>
      <LockClosed sx={{ fontSize: 80 }} color={"action"} />
      <Box sx={{ mt: 1 }}>
        <FilledTextField
          variant={"filled"}
          autoFocus
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          label={t("application:share.enterPassword")}
        />
        <Button
          disabled={password == ""}
          onClick={() => dispatch(retrySharePassword(fmIndex, password))}
          variant={"contained"}
          sx={{ ml: 1, height: "56px" }}
        >
          <ArrowLeft
            sx={{
              transform: "scaleX(-1)",
            }}
          />
        </Button>
      </Box>
    </Box>
  );
};

const PurchaseShare = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const navigate = useNavigate();
  const fmIndex = useContext(FmIndexContext);
  const path = useAppSelector((state) => state.fileManager[fmIndex].path);
  const [share, setShare] = useState<Share>();
  const [loading, setLoading] = useState(false);
  const signedIn = !!SessionManager.currentLoginOrNull();

  const shareID = useMemo(() => {
    if (!path) {
      return "";
    }
    try {
      return new CrUri(path).id();
    } catch {
      return "";
    }
  }, [path]);

  React.useEffect(() => {
    if (!shareID) {
      return;
    }
    dispatch(getShareInfo(shareID))
      .then(setShare)
      .catch(() => undefined);
  }, [dispatch, shareID]);

  const purchase = useCallback(async () => {
    if (!shareID || loading) {
      return;
    }
    setLoading(true);
    try {
      const res = await dispatch(purchaseShare(shareID));
      rememberPurchaseTicket(shareID, res.ticket);
      await dispatch(refreshFileList(fmIndex));
    } finally {
      setLoading(false);
    }
  }, [dispatch, fmIndex, loading, shareID]);

  const signIn = useCallback(() => {
    navigate("/session?redirect=" + encodeURIComponent(window.location.pathname + window.location.search));
  }, [navigate]);

  return (
    <Box sx={{ textAlign: "center" }} onClick={(e) => e.stopPropagation()} onMouseDown={(e) => e.stopPropagation()}>
      <LockClosed sx={{ fontSize: 80 }} color={"action"} />
      <Typography color={"text.secondary"} sx={{ mt: 1 }}>
        <Trans
          i18nKey={"application:vas.sharePurchaseTitle"}
          components={[<SharePrice price={share?.price ?? 0} />]}
        />
      </Typography>
      <Button
        variant={"contained"}
        disabled={loading || !shareID}
        onClick={signedIn ? purchase : signIn}
        sx={{ mt: 3 }}
        startIcon={loading ? <CircularProgress size={18} color={"inherit"} /> : undefined}
      >
        {signedIn ? t("application:vas.purchaseNow") : t("application:login.signIn")}
      </Button>
    </Box>
  );
};

const ExplorerError = memo(
  React.forwardRef(({ error, ...rest }: ExplorerErrorProps, ref) => {
    const dispatch = useAppDispatch();
    const navigate = useNavigate();
    const fmIndex = useContext(FmIndexContext);
    const fs = useAppSelector((state) => state.fileManager[fmIndex].current_fs);
    const previousPath = useAppSelector((state) => state.fileManager[fmIndex].previous_path);
    const { t } = useTranslation("application");
    const appErr = useMemo(() => {
      if (error) {
        return new AppError(error);
      }

      return undefined;
    }, [error]);
    const navigateBack = useCallback(() => {
      previousPath && dispatch(navigateToPath(fmIndex, previousPath));
    }, [dispatch, fmIndex, previousPath]);

    const signIn = useCallback(() => {
      navigate("/session?redirect=" + encodeURIComponent(window.location.pathname + window.location.search));
    }, [navigate]);

    const innerError = () => {
      switch (error?.code) {
        case Code.AnonymouseAccessDenied:
          return (
            <Box sx={{ textAlign: "center" }}>
              <LockClosed sx={{ fontSize: 60 }} color={"action"} />
              <Typography color={"text.secondary"}>{t("application:fileManager.anonymousAccessDenied")}</Typography>
              <SecondaryButton variant={"contained"} color={"inherit"} onClick={signIn} sx={{ mt: 4 }}>
                {t("application:login.signIn")}
              </SecondaryButton>
            </Box>
          );
        case Code.IncorrectPassword:
          return <RetryPassword />;
        case Code.PurchaseRequired:
          if (fs == Filesystem.share) {
            return <PurchaseShare />;
          }
          break;
        // @ts-ignore
        case Code.NodeFound:
          if (fs == Filesystem.share) {
            return (
              <Box sx={{ textAlign: "center" }}>
                <LinkDismiss sx={{ fontSize: 60 }} color={"action"} />
                <Typography color={"text.secondary"}>{t("application:share.shareNotExist")}</Typography>
              </Box>
            );
          }
        default:
          return (
            <Alert severity="warning">
              <AlertTitle> {t("application:fileManager.listError")}</AlertTitle>
              {appErr && appErr.message}
              {error?.correlation_id && (
                <Box sx={{ typography: "caption", mt: 2, opacity: 0.5 }}>
                  <code>{t("common:requestID", { id: error.correlation_id })}</code>
                </Box>
              )}
            </Alert>
          );
      }
    };
    return (
      <Box
        ref={ref}
        {...rest}
        sx={{
          p: 2,
          height: "100%",
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
          ...rest.sx,
        }}
      >
        {innerError()}
      </Box>
    );
  }),
);

export default ExplorerError;
