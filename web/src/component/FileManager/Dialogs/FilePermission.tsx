import {
  Checkbox,
  DialogContent,
  FormControlLabel,
  FormGroup,
  Stack,
  Typography,
} from "@mui/material";
import { useSnackbar } from "notistack";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { deleteFilePermission, getFileInfo, setFilePermission } from "../../../api/api.ts";
import { FileResponse, PermissionSetting } from "../../../api/explorer.ts";
import { GroupPermission } from "../../../api/user.ts";
import { closePermissionDialog } from "../../../redux/globalStateSlice.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import SessionManager from "../../../session";
import { getFileLinkedUri } from "../../../util";
import Boolset from "../../../util/boolset.ts";
import { DefaultCloseAction } from "../../Common/Snackbar/snackbar.tsx";
import DraggableDialog from "../../Dialogs/DraggableDialog.tsx";

const PermissionBits = {
  list: 0,
  read: 1,
  update: 2,
  create: 3,
  delete: 4,
  share: 5,
};

type PermissionKey = keyof typeof PermissionBits;
type PermissionForm = Record<PermissionKey, boolean>;

const emptyPermission = (): PermissionForm => ({
  list: true,
  read: true,
  update: false,
  create: false,
  delete: false,
  share: false,
});

const decode = (raw?: string): PermissionForm => {
  if (!raw) {
    return emptyPermission();
  }
  const bs = new Boolset(raw);
  return {
    list: bs.enabled(PermissionBits.list) || bs.enabled(PermissionBits.read),
    read: bs.enabled(PermissionBits.read),
    update: bs.enabled(PermissionBits.update),
    create: bs.enabled(PermissionBits.create),
    delete: bs.enabled(PermissionBits.delete),
    share: bs.enabled(PermissionBits.share),
  };
};

const encode = (permission: PermissionForm): string => {
  const bs = new Boolset();
  Object.entries(PermissionBits).forEach(([key, bit]) => {
    const enabled = key === "list" ? permission.list || permission.read : permission[key as PermissionKey];
    bs.set(bit, enabled);
  });
  return bs.toString();
};

const PermissionEditor = ({
  title,
  value,
  onChange,
  disableHigher,
  disableShare,
}: {
  title: string;
  value: PermissionForm;
  onChange: (value: PermissionForm) => void;
  disableHigher?: boolean;
  disableShare?: boolean;
}) => {
  const { t } = useTranslation();
  const keys: PermissionKey[] = ["list", "read", "update", "create", "delete", "share"];
  return (
    <Stack spacing={0.5}>
      <Typography variant="subtitle2">{title}</Typography>
      <FormGroup row>
        {keys.map((key) => {
          const disabled =
            (key === "share" && disableShare) ||
            (disableHigher && ["update", "create", "delete"].includes(key));
          return (
            <FormControlLabel
              key={key}
              control={
                <Checkbox
                  size="small"
                  checked={value[key]}
                  disabled={disabled}
                  onChange={(e) => {
                    const next = { ...value, [key]: e.target.checked };
                    if (key === "read" && e.target.checked) {
                      next.list = true;
                    }
                    onChange(next);
                  }}
                />
              }
              label={t(`application:fileManager.permission_${key}`)}
            />
          );
        })}
      </FormGroup>
    </Stack>
  );
};

const FilePermission = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const open = useAppSelector((state) => state.globalState.permissionDialogOpen);
  const targets = useAppSelector((state) => state.globalState.permissionDialogFiles);
  const [loading, setLoading] = useState(false);
  const [anonymous, setAnonymous] = useState<PermissionForm>(emptyPermission());
  const [everyone, setEveryone] = useState<PermissionForm>(emptyPermission());
  const [sameGroup, setSameGroup] = useState<PermissionForm>(emptyPermission());
  const [other, setOther] = useState<PermissionForm>(emptyPermission());
  const [existing, setExisting] = useState<PermissionSetting | undefined>();
  const [notSet, setNotSet] = useState(true);

  const canSetHigher = useMemo(() => {
    const bs = SessionManager.currentUserGroupPermission();
    return bs.enabled(GroupPermission.set_anonymous_permission);
  }, [open]);

  useEffect(() => {
    if (!open || !targets || targets.length !== 1) {
      return;
    }
    setLoading(true);
    dispatch(
      getFileInfo({
        uri: getFileLinkedUri(targets[0]),
        extended: true,
      }),
    )
      .then((file: FileResponse) => {
        const setting = file.extended_info?.permissions;
        setExisting(setting);
        setNotSet(!setting);
        setAnonymous(decode(setting?.anonymous));
        setEveryone(decode(setting?.everyone));
        setSameGroup(decode(setting?.same_group));
        setOther(decode(setting?.other));
      })
      .finally(() => setLoading(false));
  }, [open, targets, dispatch]);

  const onClose = useCallback(() => {
    if (!loading) {
      dispatch(closePermissionDialog());
    }
  }, [dispatch, loading]);

  const onAccept = useCallback(() => {
    if (!targets || targets.length === 0) {
      return;
    }
    setLoading(true);
    dispatch(
      setFilePermission({
        uris: targets.map((target) => getFileLinkedUri(target)),
        setting: {
          ...(existing ?? {}),
          anonymous: encode(anonymous),
          everyone: encode(everyone),
          same_group: encode(sameGroup),
          other: encode(other),
        },
      }),
    )
      .then(() => {
        enqueueSnackbar(t("application:fileManager.permissionSaved"), {
          variant: "success",
          action: DefaultCloseAction,
        });
        dispatch(closePermissionDialog());
      })
      .finally(() => setLoading(false));
  }, [targets, existing, anonymous, everyone, sameGroup, other, dispatch, enqueueSnackbar, t]);

  const onClear = useCallback(() => {
    if (!targets || targets.length === 0) {
      return;
    }
    setLoading(true);
    dispatch(deleteFilePermission({ uris: targets.map((target) => getFileLinkedUri(target)) }))
      .then(() => {
        setNotSet(true);
        setAnonymous(emptyPermission());
        setEveryone(emptyPermission());
        setSameGroup(emptyPermission());
        setOther(emptyPermission());
        setExisting(undefined);
      })
      .finally(() => setLoading(false));
  }, [targets, dispatch]);

  return (
    <DraggableDialog
      title={t("application:fileManager.permissions")}
      showActions
      showCancel
      loading={loading}
      onAccept={onAccept}
      dialogProps={{
        open: open ?? false,
        onClose,
        fullWidth: true,
        maxWidth: "sm",
      }}
      secondaryAction={notSet ? undefined : (
        <Typography
          component="button"
          variant="body2"
          color="error"
          onClick={onClear}
          sx={{ border: 0, background: "none", cursor: "pointer" }}
        >
          {t("application:fileManager.clearPermissions")}
        </Typography>
      )}
    >
      <DialogContent sx={{ pt: 1 }}>
        {targets && targets.length > 1 && (
          <Typography variant="body2" color="text.secondary" sx={{ pb: 2 }}>
            {t("application:fileManager.permissionBatchHint")}
          </Typography>
        )}
        <Stack spacing={2}>
          <PermissionEditor
            title={t("application:fileManager.anonymous")}
            value={anonymous}
            onChange={setAnonymous}
            disableHigher={!canSetHigher}
            disableShare
          />
          <PermissionEditor
            title={t("application:fileManager.everyone")}
            value={everyone}
            onChange={setEveryone}
            disableHigher={!canSetHigher}
            disableShare
          />
          <PermissionEditor title={t("application:fileManager.sameGroup")} value={sameGroup} onChange={setSameGroup} />
          <PermissionEditor title={t("application:fileManager.otherGroups")} value={other} onChange={setOther} />
        </Stack>
      </DialogContent>
    </DraggableDialog>
  );
};

export default FilePermission;
