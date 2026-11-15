import { Link } from "@mui/material";
import { Trans, useTranslation } from "react-i18next";
import { LogEntry } from "../../../api/dashboard";
import { AuditLogType, FileResponse, FileType } from "../../../api/explorer";
import { setDirectLinkManagementDialog, setVersionControlDialog } from "../../../redux/globalStateSlice";
import { useAppDispatch } from "../../../redux/hooks";
import CrUri from "../../../util/uri";
import TimeBadge from "../../Common/TimeBadge";
import FileBadge from "../../FileManager/FileBadge";

export interface EventDescriptionProps {
  type?: number;
  content?: LogEntry;
  edgeFile?: { name?: string; path?: string; type?: number };
  file?: FileResponse;
  versionID?: string;
  onOpenVersion?: (id: string) => void;
  fallback?: string;
}

const Redacted = "redacted";

const parseUri = (path?: string): CrUri | undefined => {
  if (!path) {
    return;
  }
  try {
    return new CrUri(path);
  } catch {
    return;
  }
};

const isRedacted = (uri?: CrUri) => uri?.id() === Redacted;

const PathBadge = ({ path, type, clickable }: { path: string; type: number; clickable?: boolean }) => (
  <FileBadge variant="outlined" simplifiedFile={{ path, type }} clickable={clickable} />
);

const EventDescription = ({
  type,
  content,
  file,
  versionID,
  onOpenVersion,
  fallback,
}: EventDescriptionProps) => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const from = parseUri(content?.from);
  const to = parseUri(content?.to);
  const fromRedacted = isRedacted(from);
  const toRedacted = isRedacted(to);
  const fromType = fromRedacted ? FileType.file : FileType.folder;

  const version = content?.entity_create_time ? (
    <TimeBadge variant="inherit" datetime={content.entity_create_time} timeAgoThreshold={0} />
  ) : (
    <span />
  );
  const directLinkID = content?.direct_link_id;
  const directLink =
    directLinkID && file ? (
      <Link
        href="#"
        underline="hover"
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          dispatch(setDirectLinkManagementDialog({ open: true, file, highlight: directLinkID }));
        }}
      />
    ) : (
      <span />
    );
  const versionLink = versionID ? (
    <Link
      href="#"
      underline="hover"
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        if (file) {
          dispatch(setVersionControlDialog({ open: true, file, highlight: versionID }));
        } else {
          onOpenVersion?.(versionID);
        }
      }}
    >
      {t("application:fileManager.lookForThisVersion")}
    </Link>
  ) : null;

  switch (type) {
    case AuditLogType.file_create:
      return <>{t("application:fileManager.logFileCreate")}</>;
    case AuditLogType.file_rename:
      return (
        <>
          {t("application:fileManager.logFileRename", {
            originalName: content?.original_name ?? "",
            newName: content?.new_name ?? "",
          })}
        </>
      );
    case AuditLogType.set_file_permission:
      return <>{t("application:fileManager.logFileSetPermission")}</>;
    case AuditLogType.entity_uploaded:
      return versionLink ? (
        <>
          {t("application:fileManager.logFileEntityUpload")} {versionLink}
        </>
      ) : (
        <>{t("application:fileManager.logFileEntityUpload")}</>
      );
    case AuditLogType.file_imported:
      return <>{t("application:fileManager.logFileImported")}</>;
    case AuditLogType.thumb_generated:
      return <>{t("application:fileManager.logFileThumbGenerated")}</>;
    case AuditLogType.live_photo_uploaded:
      return <>{t("application:fileManager.logFileLivePhotoUploaded")}</>;
    case AuditLogType.update_view:
      return <>{t("application:fileManager.logUpdateView")}</>;
    case AuditLogType.delete_direct_link:
      return <>{t("application:fileManager.logDeleteDirectLink")}</>;
    case AuditLogType.entity_downloaded:
      return content?.direct_link_id ? (
        <Trans
          ns="application"
          i18nKey="fileManager.logDirectLinkDownloaded"
          components={[directLink]}
        />
      ) : (
        <>{t("application:fileManager.logEntityDownloaded")}</>
      );
    case AuditLogType.copy_from:
      return (
        <Trans
          ns="application"
          i18nKey="fileManager.logFileCopyFrom"
          components={[
            <PathBadge path={from?.toString() ?? ""} type={fromType} clickable={false} />,
            <PathBadge path={to?.toString() ?? ""} type={FileType.folder} clickable={!toRedacted} />,
          ]}
        />
      );
    case AuditLogType.copy_to:
      return (
        <Trans
          ns="application"
          i18nKey="fileManager.logFileCopyTo"
          components={[
            <PathBadge path={from?.toString() ?? ""} type={fromType} clickable={!fromRedacted} />,
            <PathBadge path={to?.toString() ?? ""} type={FileType.folder} clickable={!toRedacted} />,
          ]}
        />
      );
    case AuditLogType.move_to:
      return (
        <Trans
          ns="application"
          i18nKey="fileManager.logFileMoveTo"
          components={[
            <PathBadge path={from?.parent().toString() ?? ""} type={FileType.folder} clickable={!fromRedacted} />,
            <PathBadge path={to?.toString() ?? ""} type={FileType.folder} clickable={!toRedacted} />,
          ]}
        />
      );
    case AuditLogType.delete_file:
    case AuditLogType.move_to_trash:
      return (
        <Trans
          ns="application"
          i18nKey="fileManager.logFileMoveToTrash"
          components={[
            <PathBadge path={from?.parent().toString() ?? ""} type={FileType.folder} clickable={!fromRedacted} />,
          ]}
        />
      );
    case AuditLogType.share:
    case AuditLogType.share_link_viewed:
    case AuditLogType.edit_share:
    case AuditLogType.delete_share:
      return (
        <>
          {t(
            type === AuditLogType.share
              ? "application:fileManager.logFileShare"
              : type === AuditLogType.share_link_viewed
                ? "application:fileManager.logFileShareViewed"
                : type === AuditLogType.edit_share
                  ? "application:fileManager.logFileEditShare"
                  : "application:fileManager.logFileDeleteShare",
          )}
        </>
      );
    case AuditLogType.mount:
      return (
        <>
          {t("application:fileManager.logFileMount", {
            name: content?.storage_policy_name ?? content?.storage_policy_id ?? "",
          })}
        </>
      );
    case AuditLogType.relocate:
      return (
        <>
          {t("application:fileManager.logRelocate", {
            newPolicy: content?.storage_policy_name ?? content?.storage_policy_id ?? "",
          })}
        </>
      );
    case AuditLogType.create_archive:
      return (
        <Trans
          ns="application"
          i18nKey="fileManager.logCreateArchive"
          components={[<PathBadge path={to?.toString() ?? ""} type={FileType.file} clickable={!toRedacted} />]}
        />
      );
    case AuditLogType.extract_archive:
      return (
        <Trans
          ns="application"
          i18nKey="fileManager.logExtractArchive"
          components={[<PathBadge path={to?.toString() ?? ""} type={FileType.folder} clickable={!toRedacted} />]}
        />
      );
    case AuditLogType.update_metadata:
      return <>{t("application:fileManager.logFileUpdateMetadata")}</>;
    case AuditLogType.set_current_version:
      return content?.entity_create_time ? (
        <>
          <Trans ns="application" i18nKey="fileManager.logFileSetCurrentVersion" components={[version]} />
          {versionLink ? <> {versionLink}</> : null}
        </>
      ) : (
        <>{fallback}</>
      );
    case AuditLogType.delete_version:
      return content?.entity_create_time ? (
        <Trans ns="application" i18nKey="fileManager.logFileDeleteVersion" components={[version]} />
      ) : (
        <>{fallback}</>
      );
    case AuditLogType.get_direct_link:
      return (
        <Trans
          ns="application"
          i18nKey="fileManager.logGetDirectLink"
          components={[content?.direct_link_id ? directLink : <span />]}
        />
      );
    default:
      return fallback ? <>{fallback}</> : null;
  }
};

export default EventDescription;
