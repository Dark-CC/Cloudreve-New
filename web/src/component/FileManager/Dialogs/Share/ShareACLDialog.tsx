import {
  Autocomplete,
  Box,
  Checkbox,
  DialogContent,
  Divider,
  FormControlLabel,
  FormGroup,
  IconButton,
  Link,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { useSnackbar } from "notistack";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  getGroupList,
  getSearchUser,
  getShareACL,
  getUserInfo,
  updateShareACL,
} from "../../../../api/api.ts";
import {
  BuiltinSharePermission,
  GroupSharePermission,
  ShareACL,
  SharePermission,
  UserSharePermission,
} from "../../../../api/share.ts";
import { User } from "../../../../api/user.ts";
import { GroupEnt } from "../../../../api/dashboard.ts";
import { useAppDispatch } from "../../../../redux/hooks.ts";
import { DefaultCloseAction } from "../../../Common/Snackbar/snackbar.tsx";
import DraggableDialog from "../../../Dialogs/DraggableDialog.tsx";

type PermissionKey = "can_list" | "can_read" | "can_update" | "can_create" | "can_delete" | "can_share";
type Audience = BuiltinSharePermission["audience"];

const permissionKeys: PermissionKey[] = [
  "can_list",
  "can_read",
  "can_update",
  "can_create",
  "can_delete",
  "can_share",
];

const audiences: Audience[] = ["anonymous", "logged_in", "same_group", "other_group"];

const inheritedPermission = (): SharePermission => ({
  can_list: true,
  can_read: true,
  can_update: false,
  can_create: false,
  can_delete: false,
  can_share: false,
  discoverable: false,
});

const defaultACL = (): ShareACL => ({ builtin: [], group: [], user: [] });

const emptyPermission = (): SharePermission => ({
  can_list: false,
  can_read: false,
  can_update: false,
  can_create: false,
  can_delete: false,
  can_share: false,
  discoverable: false,
});

const builtinPermission = (
  acl: ShareACL,
  audience: Audience,
): SharePermission | null => {
  const found = acl.builtin.find((item) => item.audience === audience);
  return found ? { ...found.permission } : null;
};

const clonePermission = (permission?: SharePermission): SharePermission =>
  permission ? { ...permission } : emptyPermission();

export interface ShareACLDialogProps {
  open: boolean;
  shareID?: string;
  onClose: () => void;
}

const ShareACLDialog = ({ open, shareID, onClose }: ShareACLDialogProps) => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const { enqueueSnackbar } = useSnackbar();
  const [loading, setLoading] = useState(false);
  const [acl, setAcl] = useState<ShareACL>(defaultACL);
  const [builtin, setBuiltin] = useState<Record<Audience, SharePermission | null>>({
    anonymous: null,
    logged_in: null,
    same_group: null,
    other_group: null,
  });
  const [groupRules, setGroupRules] = useState<GroupSharePermission[]>([]);
  const [userRules, setUserRules] = useState<UserSharePermission[]>([]);
  const [groups, setGroups] = useState<GroupEnt[]>([]);
  const [selectedGroupID, setSelectedGroupID] = useState("");
  const [userOptions, setUserOptions] = useState<User[]>([]);
  const [knownUsers, setKnownUsers] = useState<Record<string, User>>({});
  const [userSearch, setUserSearch] = useState("");

  useEffect(() => {
    if (!open || !shareID) {
      return;
    }
    setLoading(true);
    dispatch(getShareACL(shareID))
      .then((res) => {
        const next = { ...(res ?? defaultACL()), group: res?.group ?? [], user: res?.user ?? [] };
        setAcl(next);
        setBuiltin({
          anonymous: builtinPermission(next, "anonymous"),
          logged_in: builtinPermission(next, "logged_in"),
          same_group: builtinPermission(next, "same_group"),
          other_group: builtinPermission(next, "other_group"),
        });
        setGroupRules(next.group ?? []);
        setUserRules(next.user ?? []);
        next.user.forEach((rule) => {
          const userHashID = rule.user_hash_id ?? rule.user_id.toString();
          dispatch(getUserInfo(userHashID))
            .then((user) => setKnownUsers((prev) => ({ ...prev, [userHashID]: user })))
            .catch(() => undefined);
        });
      })
      .finally(() => setLoading(false));
  }, [open, shareID, dispatch]);

  useEffect(() => {
    if (!open) {
      return;
    }
    dispatch(getGroupList({ page: 1, page_size: 1000, order_by: "id", order_direction: "asc" }))
      .then((res) => setGroups(res.groups ?? []))
      .catch(() => setGroups([]));
  }, [open, dispatch]);

  useEffect(() => {
    const keyword = userSearch.trim();
    if (!open || keyword.length < 2) {
      setUserOptions([]);
      return;
    }
    let active = true;
    const timer = setTimeout(() => {
      dispatch(getSearchUser(keyword))
        .then((res) => {
          if (active) {
            setUserOptions(res ?? []);
          }
        })
        .catch(() => {
          if (active) {
            setUserOptions([]);
          }
        });
    }, 300);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [open, userSearch, dispatch]);

  const onAccept = useCallback(() => {
    if (!shareID) {
      return;
    }
    setLoading(true);
    const nextBuiltin = audiences
      .filter((audience) => builtin[audience] !== null)
      .map((audience) => ({
        audience,
        permission: builtin[audience] as SharePermission,
      }));

    dispatch(
      updateShareACL(shareID, {
        builtin: nextBuiltin as unknown as BuiltinSharePermission[],
        group: groupRules,
        user: userRules,
      }),
    )
      .then(() => {
        enqueueSnackbar(t("application:fileManager.sharePermissionsSaved"), {
          variant: "success",
          action: DefaultCloseAction,
        });
        onClose();
      })
      .finally(() => setLoading(false));
  }, [shareID, builtin, groupRules, userRules, dispatch, enqueueSnackbar, t, onClose]);

  const updatePermission = (audience: Audience, key: PermissionKey | "discoverable", value: boolean) => {
    setBuiltin((prev) => {
      const permission = { ...(prev[audience] ?? inheritedPermission()), [key]: value };
      if (key === "can_read" && value) {
        permission.can_list = true;
      }
      return { ...prev, [audience]: permission };
    });
  };

  const setAudienceMode = (audience: Audience, mode: "inherit" | "read" | "deny") => {
    setBuiltin((prev) => ({
      ...prev,
      [audience]: mode === "inherit" ? null : mode === "read" ? inheritedPermission() : emptyPermission(),
    }));
  };

  const restoreInherited = useCallback(() => {
    setBuiltin({ anonymous: null, logged_in: null, same_group: null, other_group: null });
    setGroupRules([]);
    setUserRules([]);
    setSelectedGroupID("");
  }, []);

  const addGroupRule = useCallback(() => {
    const groupID = Number(selectedGroupID);
    if (!groupID || groupRules.some((rule) => rule.group_id === groupID)) {
      return;
    }
    setGroupRules((prev) => [
      ...prev,
      {
        id: 0,
        share_id: 0,
        group_id: groupID,
        permission: clonePermission(inheritedPermission()),
      },
    ]);
    setSelectedGroupID("");
  }, [selectedGroupID, groupRules]);

  const addUserRule = useCallback((user: User | null) => {
    if (!user) {
      return;
    }
    const userHashID = user.id;
    if (!userHashID || userRules.some((rule) => (rule.user_hash_id ?? rule.user_id.toString()) === userHashID)) {
      return;
    }
    setUserRules((prev) => [
      ...prev,
      {
        id: 0,
        share_id: 0,
        user_id: 0,
        user_hash_id: userHashID,
        permission: clonePermission(inheritedPermission()),
      },
    ]);
    setUserSearch("");
    setUserOptions([]);
    setKnownUsers((prev) => ({ ...prev, [userHashID]: user }));
  }, [userRules]);

  const updateRulePermission = (
    kind: "group" | "user",
    id: string | number,
    key: PermissionKey | "discoverable",
    value: boolean,
  ) => {
    const updatePermission = (permission: SharePermission) => {
      const next = { ...permission, [key]: value };
      if (key === "can_read" && value) {
        next.can_list = true;
      }
      return next;
    };
    if (kind === "group") {
      setGroupRules((prev) =>
        prev.map((rule) =>
          rule.group_id === id ? { ...rule, permission: updatePermission(rule.permission) } : rule,
        ),
      );
    } else {
      setUserRules((prev) =>
        prev.map((rule) =>
          (rule.user_hash_id ?? rule.user_id.toString()) === id.toString()
            ? { ...rule, permission: updatePermission(rule.permission) }
            : rule,
        ),
      );
    }
  };

  const denyEveryone = () => {
    const denied = emptyPermission();
    setBuiltin({
      anonymous: denied,
      logged_in: { ...denied },
      same_group: { ...denied },
      other_group: { ...denied },
    });
    setGroupRules([]);
    setUserRules([]);
  };

  const permissionCheckboxes = (
    permission: SharePermission,
    onChange: (key: PermissionKey | "discoverable", value: boolean) => void,
  ) => (
    <FormGroup row>
      {permissionKeys.map((key) => (
        <FormControlLabel
          key={key}
          control={
            <Checkbox
              size="small"
              checked={permission[key]}
              onChange={(e) => onChange(key, e.target.checked)}
            />
          }
          label={t(`application:fileManager.permission_${key.replace("can_", "")}`)}
        />
      ))}
      <FormControlLabel
        control={
          <Checkbox
            size="small"
            checked={permission.discoverable}
            onChange={(e) => onChange("discoverable", e.target.checked)}
          />
        }
        label={t("application:fileManager.discoverable")}
      />
    </FormGroup>
  );

  return (
    <DraggableDialog
      title={t("application:fileManager.sharePermissions")}
      showActions
      showCancel
      loading={loading}
      onAccept={onAccept}
      dialogProps={{
        open,
        onClose: loading ? undefined : onClose,
        fullWidth: true,
        maxWidth: "sm",
      }}
    >
      <DialogContent sx={{ pt: 1 }}>
        <Typography variant="body2" color="text.secondary" sx={{ pb: 2 }}>
          {t("application:fileManager.sharePermissionsHint")}
        </Typography>
        <Stack direction="row" justifyContent="flex-end" sx={{ pb: 1 }}>
          <Stack direction="row" spacing={2}>
            <Link component="button" variant="body2" onClick={denyEveryone}>
              {t("application:fileManager.sharePermissionsDenyAll")}
            </Link>
            <Link component="button" variant="body2" onClick={restoreInherited}>
              {t("application:fileManager.sharePermissionsRestore")}
            </Link>
          </Stack>
        </Stack>
        <Stack spacing={2}>
          {audiences.map((audience) => {
            const permission = builtin[audience];
            const mode = permission === null ? "inherit" : permission.can_list || permission.can_read ? "read" : "deny";
            return (
              <Stack key={audience} spacing={0.5}>
                <Stack direction="row" spacing={2} alignItems="center">
                  <Typography variant="subtitle2" sx={{ minWidth: 100 }}>
                    {t(`application:fileManager.shareAudience_${audience}`)}
                  </Typography>
                  <TextField
                    select
                    size="small"
                    value={mode}
                    onChange={(e) => setAudienceMode(audience, e.target.value as "inherit" | "read" | "deny")}
                    sx={{ minWidth: 180 }}
                  >
                    <MenuItem value="inherit">{t("application:fileManager.sharePermissionModeInherit")}</MenuItem>
                    <MenuItem value="read">{t("application:fileManager.sharePermissionModeRead")}</MenuItem>
                    <MenuItem value="deny">{t("application:fileManager.sharePermissionModeDeny")}</MenuItem>
                  </TextField>
                </Stack>
                {permission && permissionCheckboxes(permission, (key, value) => updatePermission(audience, key, value))}
              </Stack>
            );
          })}

          <Divider />
          <Typography variant="subtitle2">{t("application:fileManager.shareExplicitRules")}</Typography>
          <Stack direction="row" spacing={1} alignItems="center">
            <TextField
              select
              size="small"
              value={selectedGroupID}
              onChange={(e) => setSelectedGroupID(e.target.value)}
              sx={{ minWidth: 200 }}
              label={t("application:fileManager.shareAddGroup")}
            >
              {groups
                .filter((group) => group.id !== 3)
                .map((group) => (
                  <MenuItem key={group.id} value={group.id.toString()}>
                    {group.name}
                  </MenuItem>
                ))}
            </TextField>
            <IconButton size="small" onClick={addGroupRule} disabled={!selectedGroupID}>
              <Typography variant="h6">+</Typography>
            </IconButton>
          </Stack>
          {groupRules.map((rule) => {
            const groupName = groups.find((group) => group.id === rule.group_id)?.name ?? `#${rule.group_id}`;
            return (
              <Box key={`group-${rule.group_id}`} sx={{ pl: 2, borderLeft: 2, borderColor: "divider" }}>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                  <Typography variant="body2">{groupName}</Typography>
                  <IconButton
                    size="small"
                    onClick={() => setGroupRules((prev) => prev.filter((item) => item.group_id !== rule.group_id))}
                  >
                    <Typography variant="body2">×</Typography>
                  </IconButton>
                </Stack>
                {permissionCheckboxes(rule.permission, (key, value) =>
                  updateRulePermission("group", rule.group_id, key, value),
                )}
              </Box>
            );
          })}

          <Autocomplete
            size="small"
            options={userOptions}
            value={null}
            inputValue={userSearch}
            onInputChange={(_event, value) => setUserSearch(value)}
            onChange={(_event, user) => addUserRule(user)}
            getOptionLabel={(option) => `${option.nickname} <${option.email ?? ""}>`}
            renderInput={(params) => (
              <TextField {...params} size="small" label={t("application:fileManager.shareAddUser")} />
            )}
          />
          {userRules.map((rule) => {
            const userHashID = rule.user_hash_id ?? rule.user_id.toString();
            const user = knownUsers[userHashID] ?? userOptions.find((item) => item.id === userHashID);
            return (
              <Box key={`user-${userHashID}`} sx={{ pl: 2, borderLeft: 2, borderColor: "divider" }}>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                  <Typography variant="body2">
                    {user ? `${user.nickname} <${user.email ?? ""}>` : userHashID}
                  </Typography>
                  <IconButton
                    size="small"
                    onClick={() =>
                      setUserRules((prev) =>
                        prev.filter((item) => (item.user_hash_id ?? item.user_id.toString()) !== userHashID),
                      )
                    }
                  >
                    <Typography variant="body2">×</Typography>
                  </IconButton>
                </Stack>
                {permissionCheckboxes(rule.permission, (key, value) =>
                  updateRulePermission("user", userHashID, key, value),
                )}
              </Box>
            );
          })}
        </Stack>
      </DialogContent>
    </DraggableDialog>
  );
};

export default ShareACLDialog;
