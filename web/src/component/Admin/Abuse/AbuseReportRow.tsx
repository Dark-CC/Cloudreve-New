import { Box, Checkbox, IconButton, Link, Skeleton, Stack, TableCell, TableRow, Typography } from "@mui/material";
import { useMemo } from "react";
import { Trans, useTranslation } from "react-i18next";
import { batchDeleteAbuseReports } from "../../../api/api";
import { AbuseReport } from "../../../api/dashboard";
import { FileType } from "../../../api/explorer";
import { useAppDispatch } from "../../../redux/hooks";
import { confirmOperation } from "../../../redux/thunks/dialog";
import TimeBadge from "../../Common/TimeBadge";
import UserAvatar from "../../Common/User/UserAvatar";
import FileBadge from "../../FileManager/FileBadge";
import Delete from "../../Icons/Delete";

export interface AbuseReportRowProps {
  abuseReport?: AbuseReport;
  loading?: boolean;
  deleting?: boolean;
  selected?: boolean;
  onDelete?: () => void;
  onSelect?: (id: number) => void;
  openUserDialog?: (id?: number) => void;
  openShareDialog?: (id?: number) => void;
}

const AbuseReportRow = ({
  abuseReport,
  loading,
  deleting,
  selected,
  onDelete,
  onSelect,
  openUserDialog,
  openShareDialog,
}: AbuseReportRowProps) => {
  const { t } = useTranslation(["dashboard", "application"]);
  const dispatch = useAppDispatch();

  const handleDelete = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    if (!abuseReport?.id) return;
    dispatch(confirmOperation(t("dashboard:abuseReport.confirmDelete"))).then(() => {
      dispatch(batchDeleteAbuseReports({ ids: [abuseReport.id] })).then(() => onDelete?.());
    });
  };

  const target = useMemo(() => {
    if (abuseReport?.file_uri) {
      if (abuseReport?.edges?.share) {
        return (
          <Trans
            i18nKey="dashboard:abuseReport.shareLink"
            values={{ id: abuseReport.edges.share.id }}
            components={[
              <Link
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  openShareDialog?.(abuseReport.edges?.share?.id);
                }}
                underline="hover"
                href="#/"
              />,
            ]}
          />
        );
      }
      return (
        <Typography variant="inherit" sx={{ textDecoration: "line-through" }} color="text.secondary">
          {t("dashboard:abuseReport.deletedShare")}
        </Typography>
      );
    }

    if (abuseReport?.edges?.reported_user) {
      const reportedUser = abuseReport.edges.reported_user;
      return (
        <Stack direction="row" alignItems="center" spacing={1}>
          <UserAvatar
            sx={{ width: 24, height: 24 }}
            overwriteTextSize
            user={{
              id: abuseReport.user_hash_id ?? abuseReport.reported_user_hash_id ?? "",
              nickname: reportedUser.nick ?? "",
              created_at: reportedUser.created_at ?? "",
            }}
          />
          <Typography variant="inherit">
            <Link
              sx={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                openUserDialog?.(reportedUser.id);
              }}
              underline="hover"
              href="#/"
            >
              {reportedUser.nick}
            </Link>
          </Typography>
        </Stack>
      );
    }

    return (
      <Typography variant="inherit" sx={{ textDecoration: "line-through" }} color="text.secondary">
        {t("dashboard:abuseReport.deletedUser")}
      </Typography>
    );
  }, [abuseReport, openShareDialog, openUserDialog, t]);

  if (loading) {
    return (
      <TableRow>
        <TableCell padding="checkbox">
          <Skeleton variant="circular" width={20} height={20} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={50} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={200} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={150} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={50} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={200} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={50} />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width={50} />
        </TableCell>
        <TableCell>
          <Skeleton variant="circular" width={24} height={24} />
        </TableCell>
      </TableRow>
    );
  }

  const reasons = t("application:vas.reportReasonOptions", { returnObjects: true }) as string[];

  return (
    <TableRow hover selected={selected}>
      <TableCell padding="checkbox">
        <Checkbox
          disabled={deleting}
          size="small"
          disableRipple
          color="primary"
          onClick={() => abuseReport?.id && onSelect?.(abuseReport.id)}
          checked={!!selected}
        />
      </TableCell>
      <TableCell>
        <Typography variant="inherit">{abuseReport?.id}</Typography>
      </TableCell>
      <TableCell>{target}</TableCell>
      <TableCell>
        {abuseReport?.file_uri && (
          <FileBadge simplifiedFile={{ path: abuseReport.file_uri, type: FileType.folder }} />
        )}
      </TableCell>
      <TableCell>
        <Typography variant="inherit">{reasons[abuseReport?.category ?? 0]}</Typography>
      </TableCell>
      <TableCell>{abuseReport?.description}</TableCell>
      <TableCell>
        {abuseReport?.edges?.reporter && (
          <Stack direction="row" alignItems="center" spacing={1}>
            <UserAvatar
              sx={{ width: 24, height: 24 }}
              overwriteTextSize
              user={{
                id: abuseReport.reporter_hash_id ?? "",
                nickname: abuseReport.edges.reporter.nick ?? "",
                created_at: abuseReport.edges.reporter.created_at ?? "",
              }}
            />
            <Typography variant="inherit">
              <Link
                sx={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  openUserDialog?.(abuseReport.edges?.reporter?.id);
                }}
                underline="hover"
                href="#/"
              >
                {abuseReport.edges.reporter.nick}
              </Link>
            </Typography>
          </Stack>
        )}
      </TableCell>
      <TableCell>
        <Typography variant="inherit">
          <TimeBadge datetime={abuseReport?.created_at ?? ""} variant="inherit" timeAgoThreshold={0} />
        </Typography>
      </TableCell>
      <TableCell>
        <Box display="flex" alignItems="center" gap={1}>
          <IconButton size="small" onClick={handleDelete} disabled={deleting}>
            <Delete fontSize="small" />
          </IconButton>
        </Box>
      </TableCell>
    </TableRow>
  );
};

export default AbuseReportRow;
