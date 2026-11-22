import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Container,
  LinearProgress,
  Paper,
  Stack,
  Typography,
} from "@mui/material";
import axios, { AxiosProgressEvent } from "axios";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import {
  completePublicUpload,
  createPublicUploadSession,
  getUploadLinkInfo,
  PublicUploadSession,
  refreshPublicUpload,
  UploadLinkInfo,
} from "../../../api/uploadLink";
import Logo from "../../Common/Logo";
import Upload from "../../Icons/Upload";

interface UploadItem {
  file: File;
  progress: number;
  status: "pending" | "uploading" | "done" | "error";
  message?: string;
}

const firstPart = 1;
const maxTicketRefreshes = 3;

class SessionExpiredError extends Error {}

const UploadLinkPage = () => {
  const { t } = useTranslation();
  const { token = "" } = useParams();
  const [info, setInfo] = useState<UploadLinkInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [items, setItems] = useState<UploadItem[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);

  useEffect(() => {
    if (!token) {
      setLoading(false);
      setLoadError("invalid link");
      return;
    }
    getUploadLinkInfo(token)
      .then(setInfo)
      .catch((e) => setLoadError(e?.response?.data?.msg || e.message || "invalid link"))
      .finally(() => setLoading(false));
  }, [token]);

  const maxSizeLabel = useMemo(() => {
    if (!info?.max_size) {
      return "";
    }
    return `${(info.max_size / 1024 / 1024).toFixed(0)} MB`;
  }, [info?.max_size]);

  const addFiles = useCallback(
    (files: FileList | null) => {
      if (!files) {
        return;
      }
      const next: UploadItem[] = Array.from(files).map((file) => ({
        file,
        progress: 0,
        // 1yunpan needs at least two bytes, and an oversized file is known to
        // fail before the first chunk is even sent.
        status:
          file.size < 2 || (info?.max_size && file.size > info.max_size) ? "error" : "pending",
        message:
          file.size < 2
            ? t("uploadLink.tooSmall")
            : info?.max_size && file.size > info.max_size
              ? t("uploadLink.tooLarge")
              : undefined,
      }));
      setItems((prev) => [...prev, ...next]);
    },
    [info?.max_size, t],
  );

  const updateItem = (index: number, patch: Partial<UploadItem>) => {
    setItems((prev) => prev.map((item, i) => (i === index ? { ...item, ...patch } : item)));
  };

  const uploadOne = async (item: UploadItem, index: number) => {
    let data = await createPublicUploadSession(token, {
      name: item.file.name,
      size: item.file.size,
      last_modified: item.file.lastModified,
      mime_type: item.file.type,
    });

    // The sealed edge ticket embeds a 1yunpan login. If that login ages out
    // mid-upload the chunk request comes back as 401/403; ask the server for a
    // fresh ticket and restart the hash chain from byte zero.
    for (let refreshes = 0; ; refreshes++) {
      try {
        await streamFile(item, index, data);
        break;
      } catch (e) {
        if (!(e instanceof SessionExpiredError) || refreshes >= maxTicketRefreshes) {
          throw e;
        }
        const refreshed = await refreshPublicUpload(token, data.session_id, data.callback_secret);
        data = { ...data, edge_ticket: refreshed.edge_ticket };
        updateItem(index, { progress: 0, status: "uploading" });
      }
    }

    await completePublicUpload(token, data.session_id, data.callback_secret);
    updateItem(index, { progress: 100, status: "done" });
  };

  const streamFile = async (item: UploadItem, index: number, data: PublicUploadSession) => {
    const edge = data.upload_urls?.[0];
    if (!edge || !data.edge_ticket) {
      throw new Error(t("uploadLink.edgeUnavailable"));
    }

    const chunkSize = Math.max(1, data.chunk_size || item.file.size);
    const plan: number[] = [firstPart];
    for (let offset = firstPart; offset < item.file.size; offset += chunkSize) {
      plan.push(Math.min(chunkSize, item.file.size - offset));
    }

    let offset = 0;
    let next = "";
    let uploaded = 0;
    for (let i = 0; i < plan.length; i++) {
      const end = offset + plan[i];
      const isFinal = i === plan.length - 1;
      const headers: Record<string, string> = {
        "content-type": "application/octet-stream",
        "content-range": `${offset}-${end - 1}`,
        "x-cr-ticket": data.edge_ticket,
      };
      if (next) {
        headers["x-next-token"] = next;
      }

      const res = await axios.request({
        url: edge,
        method: "post",
        headers,
        data: item.file.slice(offset, end),
        onUploadProgress: (e: AxiosProgressEvent) => {
          const loaded = Math.min(e.loaded || 0, plan[i]);
          const percent = Math.min(100, ((uploaded + loaded) / item.file.size) * 100);
          updateItem(index, { progress: percent, status: "uploading" });
        },
        validateStatus: () => true,
      });

      if (res.status === 401 || res.status === 403 || res.headers?.["x-cr-session-expired"] === "1") {
        throw new SessionExpiredError("1yunpan session expired");
      }
      if (res.status < 200 || res.status >= 300 || res.data?.status === false) {
        throw new Error(res.data?.message || `upload failed (${res.status})`);
      }

      uploaded = end;
      offset = end;
      if (!isFinal) {
        next = res.data?.next || "";
        if (!next) {
          throw new Error(t("uploadLink.missingToken"));
        }
      }
    }
  };

  const startUpload = async () => {
    setUploading(true);
    for (let i = 0; i < items.length; i++) {
      if (items[i].status === "done" || items[i].status === "error") {
        continue;
      }
      updateItem(i, { status: "uploading" });
      try {
        await uploadOne(items[i], i);
      } catch (e: any) {
        updateItem(i, { status: "error", message: e?.response?.data?.msg || e.message });
      }
    }
    setUploading(false);
  };

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 12 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (loadError || !info) {
    return (
      <Container maxWidth="sm">
        <Alert severity="error" sx={{ mt: 8 }}>
          {loadError || t("uploadLink.invalid")}
        </Alert>
      </Container>
    );
  }

  return (
    <Container maxWidth="sm">
      <Box sx={{ py: 6 }}>
        <Stack spacing={2} alignItems="center" sx={{ mb: 3 }}>
          <Logo sx={{ maxHeight: 40 }} />
          <Typography variant="h5">{info.note || t("uploadLink.title")}</Typography>
          <Typography color="text.secondary" variant="body2">
            {t("uploadLink.subtitle", { owner: info.owner })}
          </Typography>
        </Stack>
        <Paper variant="outlined" sx={{ p: 2 }}>
          <Box
            sx={{
              border: "1px dashed",
              borderColor: "divider",
              borderRadius: 2,
              p: 4,
              textAlign: "center",
              cursor: "pointer",
            }}
            onClick={() => inputRef.current?.click()}
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => {
              e.preventDefault();
              addFiles(e.dataTransfer.files);
            }}
          >
            <Upload sx={{ fontSize: 40 }} color="action" />
            <Typography sx={{ mt: 1 }}>{t("uploadLink.dropHint")}</Typography>
            <Typography color="text.secondary" variant="caption">
              {maxSizeLabel ? t("uploadLink.maxSize", { size: maxSizeLabel }) : ""}
            </Typography>
            <input
              ref={inputRef}
              hidden
              type="file"
              multiple
              onChange={(e) => {
                addFiles(e.target.files);
                e.target.value = "";
              }}
            />
          </Box>

          <Stack spacing={1.5} sx={{ mt: 2 }}>
            {items.map((item, index) => (
              <Box key={`${item.file.name}-${index}`}>
                <Stack direction="row" justifyContent="space-between">
                  <Typography variant="body2" noWrap sx={{ maxWidth: "70%" }}>
                    {item.file.name}
                  </Typography>
                  <Typography variant="caption" color={item.status === "error" ? "error" : "text.secondary"}>
                    {item.status === "done"
                      ? t("uploadLink.done")
                      : item.status === "error"
                        ? item.message
                        : `${Math.round(item.progress)}%`}
                  </Typography>
                </Stack>
                <LinearProgress
                  variant="determinate"
                  value={item.progress}
                  color={item.status === "error" ? "error" : item.status === "done" ? "success" : "primary"}
                />
              </Box>
            ))}
          </Stack>

          <Button
            fullWidth
            variant="contained"
            sx={{ mt: 2 }}
            disabled={uploading || items.filter((i) => i.status === "pending").length === 0}
            onClick={startUpload}
          >
            {uploading ? t("uploadLink.uploading") : t("uploadLink.start")}
          </Button>
        </Paper>
      </Box>
    </Container>
  );
};

export default UploadLinkPage;
