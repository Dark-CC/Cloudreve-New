import { FormControl, InputLabel, MenuItem, Select, Stack, Typography } from "@mui/material";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getAvailableNodes } from "../../../api/api.ts";
import { GroupPermission } from "../../../api/user.ts";
import { NodeCapability, NodeSummary } from "../../../api/workflow.ts";
import { useAppDispatch } from "../../../redux/hooks.ts";
import SessionManager from "../../../session";
import Boolset from "../../../util/boolset.ts";

export interface WorkflowNodeSelectorProps {
  capability: number;
  value?: string;
  onChange: (value?: string) => void;
}

const WorkflowNodeSelector = ({ capability, value, onChange }: WorkflowNodeSelectorProps) => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const [nodes, setNodes] = useState<NodeSummary[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (!SessionManager.currentUserGroupPermission().enabled(GroupPermission.select_node)) {
      setNodes([]);
      setLoaded(true);
      return;
    }
    dispatch(getAvailableNodes())
      .then((res) => {
        setNodes(
          (res ?? []).filter((node) => new Boolset(node.capabilities).enabled(capability)),
        );
      })
      .finally(() => setLoaded(true));
  }, [dispatch, capability]);

  if (loaded && nodes.length === 0) {
    return null;
  }

  return (
    <FormControl fullWidth size="small">
      <InputLabel>{t("application:modals.processNode")}</InputLabel>
      <Select
        label={t("application:modals.processNode")}
        value={value ?? ""}
        onChange={(e) => onChange((e.target.value as string) || undefined)}
      >
        <MenuItem value="">
          <Stack>
            <Typography variant="body2">{t("application:modals.remoteDownloadNodeAuto")}</Typography>
          </Stack>
        </MenuItem>
        {nodes.map((node) => (
          <MenuItem key={node.id} value={node.id}>
            {node.name}
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );
};

export { NodeCapability };
export default WorkflowNodeSelector;
