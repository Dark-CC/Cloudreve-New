import {
  Box,
  DialogContent,
  Grid2 as Grid,
  Link,
  Typography,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import { lazy, Suspense, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getAuditLogDetail } from "../../../api/api";
import { AuditLog } from "../../../api/dashboard";
import { AuditLogType, FileType } from "../../../api/explorer";
import { useAppDispatch } from "../../../redux/hooks";
import AutoHeight from "../../Common/AutoHeight";
import FacebookCircularProgress from "../../Common/CircularProgress";
import TimeBadge from "../../Common/TimeBadge";
import UserAvatar from "../../Common/User/UserAvatar";
import DraggableDialog from "../../Dialogs/DraggableDialog";
import FileTypeIcon from "../../FileManager/Explorer/FileTypeIcon";
import SettingForm from "../../Pages/Setting/SettingForm";
import EntityDialog from "../Entity/EntityDialog/EntityDialog";
import FileDialog from "../File/FileDialog/FileDialog";
import ShareDialog from "../Share/ShareDialog/ShareDialog";
import UserDialog from "../User/UserDialog/UserDialog";

const MonacoEditor = lazy(() => import("../../Viewers/CodeViewer/MonacoEditor"));

export interface EventDetailDialogProps {
  open: boolean;
  onClose: () => void;
  eventID?: number;
}

const eventName = (type?: number): string => {
  const entry = Object.entries(AuditLogType).find(([, value]) => value === type);
  return entry?.[0] ?? `event_${type}`;
};

const EventDetailDialog = ({ open, onClose, eventID }: EventDetailDialogProps) => {
  const dispatch = useAppDispatch();
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));
  const { t } = useTranslation("dashboard");
  const [values, setValues] = useState<AuditLog>({ id: 0, edges: {} });
  const [loading, setLoading] = useState(true);
  const [userDialogOpen, setUserDialogOpen] = useState(false);
  const [userDialogID, setUserDialogID] = useState<number>();
  const [fileDialogOpen, setFileDialogOpen] = useState(false);
  const [fileDialogID, setFileDialogID] = useState<number>();
  const [entityDialogOpen, setEntityDialogOpen] = useState(false);
  const [entityDialogID, setEntityDialogID] = useState<number>();
  const [shareDialogOpen, setShareDialogOpen] = useState(false);
  const [shareDialogID, setShareDialogID] = useState<number>();

  useEffect(() => {
    if (!eventID || !open) {
      return;
    }
    let cancelled = false;
    setLoading(true);
    dispatch(getAuditLogDetail(eventID))
      .then((res) => {
        if (!cancelled) {
          setValues(res);
        }
      })
      .catch(() => {
        if (!cancelled) {
          onClose();
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [open, eventID]);

  const name = eventName(values.type);

  return (
    <>
      <UserDialog open={userDialogOpen} onClose={() => setUserDialogOpen(false)} userID={userDialogID} />
      <FileDialog open={fileDialogOpen} onClose={() => setFileDialogOpen(false)} fileID={fileDialogID} />
      <EntityDialog open={entityDialogOpen} onClose={() => setEntityDialogOpen(false)} entityID={entityDialogID} />
      <ShareDialog open={shareDialogOpen} onClose={() => setShareDialogOpen(false)} shareID={shareDialogID} />
      <DraggableDialog
        title={t("event.eventDialogTitle")}
        dialogProps={{ fullWidth: true, maxWidth: "md", open, onClose }}
      >
        <DialogContent>
          <AutoHeight>
            <Box>
                  {loading && (
                    <Box
                      sx={{
                        py: 15,
                        height: "100%",
                        display: "flex",
                        justifyContent: "center",
                        alignItems: "center",
                      }}
                    >
                      <FacebookCircularProgress />
                    </Box>
                  )}
                  {!loading && (
                    <Grid container spacing={isMobile ? 2 : 3} alignItems="stretch">
                      <SettingForm title={t("file.id")} noContainer lgWidth={2}>
                        <Typography variant="body2" color="text.secondary">
                          {values.id}
                        </Typography>
                      </SettingForm>
                      <SettingForm title={t("event.type")} noContainer lgWidth={2}>
                        <Typography variant="body2" color="text.secondary">
                          {t(`settings.event.${name}`, name)}
                        </Typography>
                      </SettingForm>
                      <SettingForm title={t("event.ip")} noContainer lgWidth={2}>
                        <Typography variant="body2" color="text.secondary" sx={{ overflowWrap: "break-word" }}>
                          {values.ip ?? "-"}
                        </Typography>
                      </SettingForm>
                      <SettingForm title={t("event.datetime")} noContainer lgWidth={2}>
                        <Typography variant="body2" color="text.secondary">
                          <TimeBadge variant="inherit" datetime={values.created_at ?? ""} timeAgoThreshold={0} />
                        </Typography>
                      </SettingForm>
                      <SettingForm title={t("event.correlationId")} noContainer lgWidth={4}>
                        <Typography variant="body2" color="text.secondary">
                          {values.correlation_id ?? "-"}
                        </Typography>
                      </SettingForm>
                      <SettingForm title={t("event.linkedUser")} noContainer lgWidth={4}>
                        {values.edges?.user ? (
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                            <UserAvatar
                              sx={{ width: 24, height: 24 }}
                              overwriteTextSize
                              user={{
                                id: values.user_hash_id ?? "",
                                nickname: values.edges.user.nick ?? "",
                                created_at: values.edges.user.created_at ?? "",
                              }}
                            />
                            <Typography variant="body2" color="text.secondary">
                              <Link
                                sx={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}
                                onClick={(e) => {
                                  e.preventDefault();
                                  setUserDialogID(values.edges.user?.id);
                                  setUserDialogOpen(true);
                                }}
                                underline="hover"
                                href="#/"
                              >
                                {values.edges.user.nick}
                              </Link>
                            </Typography>
                          </Box>
                        ) : (
                          <Typography variant="body2" color="text.secondary">
                            -
                          </Typography>
                        )}
                      </SettingForm>
                      <SettingForm title={t("event.linkedFile")} noContainer lgWidth={4}>
                        {values.edges?.file ? (
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                            <FileTypeIcon name={values.edges.file.name ?? ""} fileType={FileType.file} />
                            <Typography variant="body2" color="text.secondary">
                              <Link
                                sx={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}
                                onClick={(e) => {
                                  e.preventDefault();
                                  setFileDialogID(values.edges.file?.id);
                                  setFileDialogOpen(true);
                                }}
                                underline="hover"
                                href="#/"
                              >
                                {values.edges.file.name}
                              </Link>
                            </Typography>
                          </Box>
                        ) : (
                          <Typography variant="body2" color="text.secondary">
                            -
                          </Typography>
                        )}
                      </SettingForm>
                      <SettingForm title={t("event.linkedEntity")} noContainer lgWidth={2}>
                        {values.edges?.entity ? (
                          <Link
                            href="#/"
                            onClick={(e) => {
                              e.preventDefault();
                              setEntityDialogID(values.edges.entity?.id);
                              setEntityDialogOpen(true);
                            }}
                          >
                            #{values.edges.entity.id}
                          </Link>
                        ) : (
                          <Typography variant="body2" color="text.secondary">
                            -
                          </Typography>
                        )}
                      </SettingForm>
                      <SettingForm title={t("event.linkedShare")} noContainer lgWidth={2}>
                        {values.edges?.share ? (
                          <Link
                            href="#/"
                            onClick={(e) => {
                              e.preventDefault();
                              setShareDialogID(values.edges.share?.id);
                              setShareDialogOpen(true);
                            }}
                          >
                            #{values.edges.share.id}
                          </Link>
                        ) : (
                          <Typography variant="body2" color="text.secondary">
                            -
                          </Typography>
                        )}
                      </SettingForm>
                      <SettingForm title={t("event.userAgent")} noContainer lgWidth={12}>
                        <Typography variant="body2" color="text.secondary">
                          {values.content?.user_agent ?? "-"}
                        </Typography>
                      </SettingForm>
                      <SettingForm title={t("event.rawContent")} noContainer lgWidth={12}>
                        <Suspense fallback={<FacebookCircularProgress />}>
                          <MonacoEditor
                            theme={theme.palette.mode === "dark" ? "vs-dark" : "vs"}
                            language="json"
                            value={JSON.stringify(values, null, 4)}
                            height="400px"
                            minHeight="400px"
                            options={{
                              wordWrap: "on",
                              minimap: { enabled: false },
                              scrollBeyondLastLine: false,
                              readOnly: true,
                            }}
                          />
                        </Suspense>
                      </SettingForm>
                    </Grid>
                  )}
            </Box>
          </AutoHeight>
        </DialogContent>
      </DraggableDialog>
    </>
  );
};

export default EventDetailDialog;
