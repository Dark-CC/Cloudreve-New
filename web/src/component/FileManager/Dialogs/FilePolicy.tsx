import { DialogContent, MenuItem, Stack, TextField } from "@mui/material";
import { useSnackbar } from "notistack";
import { useCallback, useContext, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { getUserStoragePolicies, updateFilePolicy } from "../../../api/api.ts";
import { UserStoragePolicy } from "../../../api/user.ts";
import { closeFilePolicyDialog } from "../../../redux/globalStateSlice.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { refreshFileList } from "../../../redux/thunks/filemanager.ts";
import { getFileLinkedUri } from "../../../util";
import { DefaultCloseAction } from "../../Common/Snackbar/snackbar.tsx";
import DraggableDialog from "../../Dialogs/DraggableDialog.tsx";
import Storage from "../../Icons/Storage.tsx";
import { FmIndexContext } from "../FmIndexContext.tsx";

const FilePolicy = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const fmIndex = useContext(FmIndexContext);
  const open = useAppSelector((state) => state.globalState.filePolicyDialogOpen);
  const target = useAppSelector((state) => state.globalState.filePolicyDialogFile);
  const [loading, setLoading] = useState(false);
  const [policyID, setPolicyID] = useState("");
  const [policies, setPolicies] = useState<UserStoragePolicy[]>([]);
  const requested = useRef(false);

  useEffect(() => {
    if (!open || requested.current) {
      return;
    }
    requested.current = true;
    setPolicyID("");
    dispatch(getUserStoragePolicies()).then((res) => {
      const available = res ?? [];
      setPolicies(available);
      const currentPolicy = target?.extended_info?.storage_policy?.id;
      if (currentPolicy && available.some((policy) => policy.id === currentPolicy)) {
        setPolicyID(currentPolicy);
      } else if (available.length > 0) {
        setPolicyID(available[0].id);
      }
    });
  }, [open, dispatch, target]);

  useEffect(() => {
    if (!open) {
      requested.current = false;
    }
  }, [open]);

  const onClose = useCallback(() => {
    dispatch(closeFilePolicyDialog());
  }, [dispatch]);

  const onAccept = useCallback(() => {
    if (!target || !policyID) {
      return;
    }
    setLoading(true);
    dispatch(updateFilePolicy({ uri: getFileLinkedUri(target), policy_id: policyID }))
      .then(() => {
        enqueueSnackbar({
          message: t("application:fileManager.storagePolicyUpdated"),
          variant: "success",
          action: DefaultCloseAction,
        });
        dispatch(refreshFileList(fmIndex));
        dispatch(closeFilePolicyDialog());
      })
      .finally(() => setLoading(false));
  }, [dispatch, enqueueSnackbar, fmIndex, policyID, target, t]);

  return (
    <DraggableDialog
      title={t("application:fileManager.switchStoragePolicy")}
      showActions
      showCancel
      loading={loading}
      disabled={!policyID || policies.length === 0}
      onAccept={onAccept}
      dialogProps={{
        open: open ?? false,
        onClose,
        fullWidth: true,
        maxWidth: "xs",
        disableRestoreFocus: true,
      }}
    >
      <DialogContent sx={{ pt: 1 }}>
        <Stack spacing={3}>
          <TextField
            select
            variant="outlined"
            fullWidth
            label={t("application:fileManager.storagePolicy")}
            value={policyID}
            onChange={(event) => setPolicyID(event.target.value)}
            slotProps={{
              input: {
                startAdornment: <Storage sx={{ mr: 1, color: "action.active" }} />,
              },
            }}
            helperText={t("application:fileManager.switchStoragePolicyHint")}
          >
            {policies.map((policy) => (
              <MenuItem key={policy.id} value={policy.id}>
                {policy.name}
              </MenuItem>
            ))}
          </TextField>
        </Stack>
      </DialogContent>
    </DraggableDialog>
  );
};

export default FilePolicy;
