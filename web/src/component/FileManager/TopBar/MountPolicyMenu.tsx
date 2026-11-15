import { Divider, ListItemIcon, ListItemText, MenuProps, Skeleton, Stack, Typography } from "@mui/material";
import { useSnackbar } from "notistack";
import { useCallback, useContext, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { updateFilePolicy } from "../../../api/api.ts";
import { NavigatorCapability } from "../../../api/explorer.ts";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { loadPolicyOptions, refreshFileList } from "../../../redux/thunks/filemanager.ts";
import Boolset from "../../../util/boolset.ts";
import { DefaultCloseAction } from "../../Common/Snackbar/snackbar.tsx";
import Checkmark from "../../Icons/Checkmark.tsx";
import WarningOutlined from "../../Icons/WarningOutlined.tsx";
import { SquareMenu, SquareMenuItem } from "../ContextMenu/ContextMenu.tsx";
import { FmIndexContext } from "../FmIndexContext.tsx";

const MountPolicyMenu = ({ onClose, ...rest }: MenuProps) => {
  const { t } = useTranslation();
  const fmIndex = useContext(FmIndexContext);
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const list = useAppSelector((state) => state.fileManager[fmIndex].list);
  const purePath = useAppSelector((state) => state.fileManager[fmIndex].pure_path);
  const policyOptions = useAppSelector((state) => state.globalState.policyOptionCache);
  // Pro gates the switcher purely on the folder's navigator capability, with
  // no extra group-permission check, so migrated groups keep working exactly
  // as they did before.
  const canMount = useMemo(() => {
    if (!list?.props?.capability || list.single_file_view) {
      return false;
    }
    return new Boolset(list.props.capability).enabled(NavigatorCapability.mount);
  }, [list?.props?.capability, list?.single_file_view]);
  const isOwner = !!list?.parent?.owned;
  const currentPolicyID = list?.storage_policy?.id;

  useEffect(() => {
    if (canMount && isOwner && !policyOptions) {
      dispatch(loadPolicyOptions()).catch(() => undefined);
    }
  }, [canMount, dispatch, isOwner, policyOptions]);

  const onMountClicked = useCallback(
    (policy: { id: string; name: string }) => {
      if (!purePath || policy.id === currentPolicyID) {
        return;
      }
      dispatch(updateFilePolicy({ uri: purePath, policy_id: policy.id }))
        .then(() => {
          enqueueSnackbar({
            message: t("application:vas.folderPolicySwitched", { name: policy.name }),
            variant: "success",
            action: DefaultCloseAction,
          });
          dispatch(refreshFileList(fmIndex));
          onClose && onClose({}, "escapeKeyDown");
        })
        .catch(() => undefined);
    },
    [currentPolicyID, dispatch, enqueueSnackbar, fmIndex, onClose, purePath, t],
  );

  return (
    <SquareMenu
      MenuListProps={{ dense: true, sx: { py: 0.5, maxWidth: 250 } }}
      anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
      transformOrigin={{ vertical: "top", horizontal: "left" }}
      onClose={onClose}
      {...rest}
    >
      {!canMount && (
        <SquareMenuItem disabled sx={{ py: 0.5, px: 1 }}>
          <ListItemIcon>
            <WarningOutlined className="MuiSvgIcon-root" sx={{ fontSize: "18px" }} />
          </ListItemIcon>
          <Typography variant="body2" color="text.secondary">
            {t("application:fileManager.noActionsCanBeDone")}
          </Typography>
        </SquareMenuItem>
      )}
      {canMount && !isOwner && (
        <SquareMenuItem disabled sx={{ py: 1 }}>
          <Stack sx={{ px: 1 }}>
            <Typography variant="body2" fontWeight={600}>
              {t("application:uploader.storagePolicy")}
            </Typography>
            <Typography variant="body2">{list?.storage_policy?.name ?? "-"}</Typography>
            <Divider sx={{ my: 0.5 }} />
            <Typography variant="body2" color="text.secondary">
              {t("application:fileManager.mountOwner")}
            </Typography>
          </Stack>
        </SquareMenuItem>
      )}
      {canMount && isOwner && (
        <>
          {policyOptions &&
            policyOptions.map((policy) => (
              <SquareMenuItem key={policy.id} onClick={() => onMountClicked(policy)}>
                {policy.id === currentPolicyID && (
                  <ListItemIcon>
                    <Checkmark fontSize="small" />
                  </ListItemIcon>
                )}
                <ListItemText inset={policy.id !== currentPolicyID}>
                  <Typography variant="body2" sx={{ whiteSpace: "normal" }}>
                    {policy.name}
                  </Typography>
                </ListItemText>
              </SquareMenuItem>
            ))}
          {(!policyOptions || policyOptions.length === 0) && (
            <>
              <SquareMenuItem disabled>
                <ListItemText inset>
                  <Skeleton variant="text" width={100} />
                </ListItemText>
              </SquareMenuItem>
              <SquareMenuItem disabled>
                <ListItemText inset>
                  <Skeleton variant="text" width={100} />
                </ListItemText>
              </SquareMenuItem>
            </>
          )}
        </>
      )}
    </SquareMenu>
  );
};

export default MountPolicyMenu;
