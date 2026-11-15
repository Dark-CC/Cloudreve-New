import { Box, Checkbox, IconButton, Link, Skeleton, Stack, TableCell, TableRow, Typography } from "@mui/material";
import { useTranslation } from "react-i18next";
import { batchDeleteAuditLogs } from "../../../api/api";
import { AuditLog } from "../../../api/dashboard";
import { useAppDispatch } from "../../../redux/hooks";
import { confirmOperation } from "../../../redux/thunks/dialog";
import TimeBadge from "../../Common/TimeBadge";
import UserAvatar from "../../Common/User/UserAvatar";
import { NoWrapTypography } from "../../Common/StyledComponents";
import Delete from "../../Icons/Delete";
import { AuditLogType } from "../../../api/explorer";
import { sizeToString } from "../../../util";
import EventDescription from "./EventDescription";

export interface EventRowProps {
  log?: AuditLog;
  loading?: boolean;
  deleting?: boolean;
  selected?: boolean;
  onDelete?: () => void;
  onDetails?: (id: number) => void;
  onSelect?: (id: number) => void;
  openUserDialog?: (id?: number) => void;
  openEntityDialog?: (id?: number) => void;
}

const eventName = (type?: number): string => {
  const entry = Object.entries(AuditLogType).find(([, value]) => value === type);
  return entry?.[0] ?? `event_${type}`;
};

const EventRow = ({
  log,
  loading,
  deleting,
  selected,
  onDelete,
  onDetails,
  onSelect,
  openUserDialog,
  openEntityDialog,
}: EventRowProps) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();

  const handleDelete = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    e.preventDefault();
    if (!log?.id) return;
    dispatch(confirmOperation(t("event.confirmDelete"))).then(() => {
      dispatch(batchDeleteAuditLogs([log.id])).then(() => onDelete?.());
    });
  };

  const handleDetails = () => {
    if (log?.id) {
      onDetails?.(log.id);
    }
  };

  const handleSelect = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    e.preventDefault();
    if (log?.id) {
      onSelect?.(log.id);
    }
  };

  if (loading) {
    return (
      <TableRow sx={{ height: "43px" }}>
        {Array.from({ length: 8 }).map((_, index) => (
          <TableCell key={index}>
            <Skeleton variant="text" width={index === 7 ? 24 : index === 2 ? 200 : 80} />
          </TableCell>
        ))}
      </TableRow>
    );
  }

  const name = eventName(log?.type);
  const content = log?.content;
  let summary = t(`settings.event.${name}`, name);
  let description: JSX.Element | undefined;

  switch (log?.type) {
    case AuditLogType.email_sent:
      summary = content?.error
        ? t("event.emailFailed")
        : t("event.emailSend", { title: content?.email_title ?? "", email: content?.email_to ?? "" });
      break;
    case AuditLogType.user_login_failed:
      summary = t("event.signinFailed", { reason: content?.error ?? "" });
      break;
    case AuditLogType.webdav_account_create:
      summary = t("event.createDavAccount", { account: content?.account ?? "" });
      break;
    case AuditLogType.webdav_account_update:
      summary = t("event.updateDavAccount", { account: content?.account ?? "" });
      break;
    case AuditLogType.webdav_account_delete:
      summary = t("event.deleteDavAccount", { account: content?.account ?? "" });
      break;
    case AuditLogType.points_change:
      summary = t("event.pointsChange", {
        points: (content?.points_change ?? 0) > 0 ? `+${content?.points_change}` : `${content?.points_change ?? 0}`,
      });
      break;
    case AuditLogType.storage_added:
      summary = t("event.storageAdded", {
        size: sizeToString(content?.storage_size ?? 0),
      });
      break;
    case AuditLogType.change_nick:
      summary = t("event.nickChange", { old: content?.exts?.from ?? "", new: content?.exts?.to ?? "" });
      break;
    default:
      const entityID = log?.edges?.entity?.id;
      description = (
        <EventDescription
          type={log?.type}
          content={content}
          edgeFile={log?.edges?.file}
          versionID={entityID ? entityID.toString() : undefined}
          onOpenVersion={
            entityID
              ? () => {
                  openEntityDialog?.(entityID);
                }
              : undefined
          }
          fallback={summary}
        />
      );
  }

  return (
    <TableRow hover sx={{ cursor: "pointer" }} selected={selected} onClick={handleDetails}>
      <TableCell padding="checkbox">
        <Checkbox
          disabled={deleting}
          size="small"
          disableRipple
          color="primary"
          onClick={handleSelect}
          checked={!!selected}
        />
      </TableCell>
      <TableCell>
        <Typography variant="inherit">{log?.id}</Typography>
      </TableCell>
      <TableCell>
        <Typography variant="body2" color="text.secondary">
          {description ?? summary}
        </Typography>
      </TableCell>
      <TableCell>
        <Typography variant="inherit">{log?.ip || ""}</Typography>
      </TableCell>
      <TableCell>
        <NoWrapTypography variant="inherit">{log?.correlation_id || ""}</NoWrapTypography>
      </TableCell>
      <TableCell sx={{ overflow: "hidden" }}>
        {log?.edges?.user ? (
          <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
            <UserAvatar
              sx={{ width: 24, height: 24, flexShrink: 0 }}
              overwriteTextSize
              user={{
                id: log.user_hash_id ?? "",
                nickname: log.edges.user.nick ?? "",
                created_at: log.edges.user.created_at ?? "",
              }}
            />
            <Typography variant="inherit" component="div" noWrap sx={{ minWidth: 0 }}>
              <Link
                sx={{ display: "block", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  openUserDialog?.(log.edges?.user?.id);
                }}
                underline="hover"
                href="#/"
              >
                {log.edges.user.nick}
              </Link>
            </Typography>
          </Stack>
        ) : null}
      </TableCell>
      <TableCell>
        <NoWrapTypography variant="inherit">
          <TimeBadge datetime={log?.created_at ?? ""} variant="inherit" timeAgoThreshold={0} />
        </NoWrapTypography>
      </TableCell>
      <TableCell align="right">
        <Box display="flex" alignItems="center" justifyContent="flex-end">
          <IconButton size="small" onClick={handleDelete} disabled={deleting}>
            <Delete fontSize="small" />
          </IconButton>
        </Box>
      </TableCell>
    </TableRow>
  );
};

export default EventRow;
