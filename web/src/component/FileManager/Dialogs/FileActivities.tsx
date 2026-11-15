import { Box, DialogContent, ListItemText, Menu, MenuItem, Stack, Typography } from "@mui/material";
import { useCallback, useContext, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { getFileActivities } from "../../../api/api.ts";
import { AuditLogType, FileActivity, FileActivityFilter, FileResponse } from "../../../api/explorer.ts";
import { closeFileActivityDialog } from "../../../redux/globalStateSlice.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import TimeBadge from "../../Common/TimeBadge.tsx";
import UserAvatar, { AnonymousUser } from "../../Common/User/UserAvatar.tsx";
import DraggableDialog from "../../Dialogs/DraggableDialog.tsx";
import { SecondaryButton } from "../../Common/StyledComponents.tsx";
import ArrowDown from "../../Icons/ArrowDown.tsx";
import EventDescription from "../../Admin/Event/EventDescription.tsx";

const pageSize = 20;

const filterLabels: Record<string, string> = {
  [FileActivityFilter.all]: "fileManager.activityFilterAll",
  [FileActivityFilter.my]: "fileManager.activityFilterMine",
  [FileActivityFilter.updates]: "fileManager.activityFilterUpdates",
  [FileActivityFilter.reads]: "fileManager.activityFilterReads",
};

const ActivityRow = ({
  activity,
  file,
  t,
}: {
  activity: FileActivity;
  file?: FileResponse;
  t: (key: string, options?: any) => string;
}) => {
  const actor = activity.user ?? activity.edges?.user;
  const displayUser = actor ?? { ...AnonymousUser, nickname: t("application:fileManager.anonymous") };
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, py: 1, borderBottom: 1, borderColor: "divider" }}>
      <UserAvatar
        overwriteTextSize
        user={displayUser}
        sx={{ width: 32, height: 32 }}
      />
      <Box sx={{ minWidth: 0, flexGrow: 1 }}>
        <Typography variant="body2" noWrap>
          {actor?.nickname ?? t("application:fileManager.anonymous")}
          {activity.created_at && (
            <TimeBadge datetime={activity.created_at} variant="inherit" color="text.secondary" sx={{ ml: 1 }} />
          )}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          <EventDescription
            type={activity.type}
            content={activity.content}
            edgeFile={activity.edges?.file}
            file={file}
            versionID={activity.edges?.file?.primary_entity?.toString()}
            fallback={t("application:fileManager.logFileActivityUnknown")}
          />
        </Typography>
      </Box>
    </Box>
  );
};

const FileActivities = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const open = useAppSelector((state) => state.globalState.fileActivityDialogOpen);
  const target = useAppSelector((state) => state.globalState.fileActivityDialogFile);
  const [activities, setActivities] = useState<FileActivity[]>([]);
  const [nextToken, setNextToken] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [filter, setFilter] = useState<string>(FileActivityFilter.all);
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
  const loadedFor = useRef<string>();

  const fetchPage = useCallback(
    (token?: string) => {
      if (!target) {
        return;
      }
      setLoading(true);
      dispatch(
        getFileActivities({
          uri: target.path,
          page_size: pageSize,
          order_direction: "desc",
          filter,
          next_page_token: token,
        }),
      )
        .then((res) => {
          setActivities((prev) => (token ? [...prev, ...(res.activities ?? [])] : res.activities ?? []));
          setNextToken(res.pagination?.next_token);
        })
        .finally(() => setLoading(false));
    },
    [dispatch, filter, target],
  );

  useEffect(() => {
    if (!open || !target) {
      loadedFor.current = undefined;
      return;
    }
    const key = `${target.path}:${filter}`;
    if (loadedFor.current === key) {
      return;
    }
    loadedFor.current = key;
    setActivities([]);
    setNextToken(undefined);
    fetchPage();
  }, [open, target, filter, fetchPage]);

  const onClose = useCallback(() => {
    dispatch(closeFileActivityDialog());
  }, [dispatch]);

  return (
    <DraggableDialog
      title={t("application:fileManager.fileActivities")}
      showActions
      showCancel
      cancelText={t("common:close")}
      onAccept={onClose}
      dialogProps={{
        open: open ?? false,
        onClose,
        fullWidth: true,
        maxWidth: "sm",
        disableRestoreFocus: true,
      }}
    >
      <DialogContent sx={{ pt: 1 }}>
        <Stack spacing={1}>
          <Box>
            <SecondaryButton
              variant="outlined"
              size="small"
              onClick={(event) => setMenuAnchor(event.currentTarget)}
              endIcon={<ArrowDown sx={{ fontSize: "12px!important" }} />}
            >
              {t(filterLabels[filter] ?? filterLabels[FileActivityFilter.all])}
            </SecondaryButton>
            <Menu anchorEl={menuAnchor} open={!!menuAnchor} onClose={() => setMenuAnchor(null)}>
              {Object.entries(filterLabels).map(([value, label]) => (
                <MenuItem
                  dense
                  key={value}
                  selected={value === filter}
                  onClick={() => {
                    setFilter(value);
                    setMenuAnchor(null);
                  }}
                >
                  <ListItemText>{t(label)}</ListItemText>
                </MenuItem>
              ))}
            </Menu>
          </Box>
          <Box sx={{ minHeight: 120, maxHeight: "60vh", overflow: "auto" }}>
            {activities.map((activity) => (
              <ActivityRow key={activity.id} activity={activity} file={target} t={t} />
            ))}
            {!loading && activities.length === 0 && (
              <Typography variant="body2" color="text.secondary" sx={{ py: 3, textAlign: "center" }}>
                {t("application:setting.listEmpty")}
              </Typography>
            )}
            {nextToken && (
              <Box sx={{ display: "flex", justifyContent: "center", py: 1 }}>
                <SecondaryButton size="small" disabled={loading} onClick={() => fetchPage(nextToken)}>
                  {t("application:fileManager.loadMore")}
                </SecondaryButton>
              </Box>
            )}
          </Box>
        </Stack>
      </DialogContent>
    </DraggableDialog>
  );
};

export default FileActivities;
