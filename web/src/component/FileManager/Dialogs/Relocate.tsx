import { DialogContent, MenuItem, Stack, TextField } from "@mui/material";
import { useSnackbar } from "notistack";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { getUserStoragePolicies, sendRelocateWorkflow } from "../../../api/api.ts";
import { UserStoragePolicy } from "../../../api/user.ts";
import { closeRelocateDialog } from "../../../redux/globalStateSlice.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { getFileLinkedUri } from "../../../util";
import { ViewTaskAction } from "../../Common/Snackbar/snackbar.tsx";
import DraggableDialog from "../../Dialogs/DraggableDialog.tsx";
import Storage from "../../Icons/Storage.tsx";

const Relocate = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();

  const open = useAppSelector((state) => state.globalState.relocateDialogOpen);
  const files = useAppSelector((state) => state.globalState.relocateDialogFiles);

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
    dispatch(getUserStoragePolicies()).then((resp) => {
      const available = resp ?? [];
      setPolicies(available);
      if (available.length > 0) {
        setPolicyID(available[0].id);
      }
    });
  }, [open, dispatch]);

  const selectedPolicyName = useMemo(() => {
    return policies.find((policy) => policy.id === policyID)?.name ?? "";
  }, [policies, policyID]);

  const onClose = useCallback(() => {
    dispatch(closeRelocateDialog());
  }, [dispatch]);

  const onAccept = useCallback(() => {
    if (!files || files.length === 0 || !policyID) {
      return;
    }

    setLoading(true);
    dispatch(
      sendRelocateWorkflow({
        src: files.map((file) => getFileLinkedUri(file)),
        dst_policy_id: policyID,
      }),
    )
      .then(() => {
        dispatch(closeRelocateDialog());
        enqueueSnackbar({
          message: t("modals.taskCreated"),
          variant: "success",
          action: ViewTaskAction(),
        });
      })
      .finally(() => {
        setLoading(false);
      });
  }, [files, policyID, dispatch, enqueueSnackbar, t]);

  return (
    <DraggableDialog
      title={t("application:fileManager.migrateStoragePolicy")}
      showActions
      showCancel
      loading={loading}
      disabled={!policyID || policies.length === 0}
      onAccept={onAccept}
      dialogProps={{
        open: open ?? false,
        onClose: onClose,
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
            helperText={selectedPolicyName}
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

export default Relocate;
