import { Box, DialogContent, DialogProps } from "@mui/material";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppSelector } from "../../redux/hooks.ts";
import { ConfigLoadState } from "../../redux/siteConfigSlice.ts";
import SessionManager from "../../session";
import DraggableDialog from "../Dialogs/DraggableDialog.tsx";

const readNoticeKey = "read_notice";

// Mirrors Pro's announcement fingerprint so an already-acknowledged
// announcement is not shown again until its content changes.
const noticeFingerprint = (notice: string, seed = 0): number => {
  let a = 3735928559 ^ seed;
  let b = 1103547991 ^ seed;

  for (let i = 0; i < notice.length; i++) {
    const code = notice.charCodeAt(i);
    a = Math.imul(a ^ code, 2654435761);
    b = Math.imul(b ^ code, 1597334677);
  }

  a = Math.imul(a ^ (a >>> 16), 2246822507);
  a ^= Math.imul(b ^ (b >>> 13), 3266489909);
  b = Math.imul(b ^ (b >>> 16), 2246822507);
  b ^= Math.imul(a ^ (a >>> 13), 3266489909);

  return 4294967296 * (2097151 & b) + (a >>> 0);
};

const SiteAnnouncement = () => {
  const { t } = useTranslation("application");
  const notice = useAppSelector((state) => state.siteConfig.basic.config.site_notice);
  const basicLoaded = useAppSelector((state) => state.siteConfig.basic.loaded);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const user = SessionManager.currentLoginOrNull();
    if (!notice || user == null || basicLoaded === ConfigLoadState.NotLoaded) {
      setOpen(false);
      return;
    }

    const acknowledged = SessionManager.get(readNoticeKey);
    if (acknowledged !== noticeFingerprint(notice)) {
      setOpen(true);
    }
  }, [notice, basicLoaded]);

  const dismiss = useCallback(
    (updateReadState: boolean) => {
      if (updateReadState && notice) {
        SessionManager.set(readNoticeKey, noticeFingerprint(notice));
      }
      setOpen(false);
    },
    [notice],
  );

  const handleClose: DialogProps["onClose"] = () => dismiss(true);

  return (
    <DraggableDialog
      title={t("vas.announcement")}
      showActions
      showCancel
      onAccept={() => dismiss(false)}
      cancelText={t("vas.dontShowAgain")}
      dialogProps={{
        open,
        fullWidth: true,
        maxWidth: "sm",
        onClose: handleClose,
      }}
    >
      <DialogContent>
        <Box sx={{ overflowWrap: "break-word" }} dangerouslySetInnerHTML={{ __html: notice ?? "" }} />
      </DialogContent>
    </DraggableDialog>
  );
};

export default SiteAnnouncement;
