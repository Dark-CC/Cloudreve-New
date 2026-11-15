import { Box, CircularProgress, ListItemText, Typography } from "@mui/material";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getNodeList } from "../../../../api/api";
import { Node } from "../../../../api/dashboard";
import { useAppDispatch } from "../../../../redux/hooks";
import { DenseSelect, SquareChip } from "../../../Common/StyledComponents";
import { SquareMenuItem } from "../../../FileManager/ContextMenu/ContextMenu";

export interface MultipleNodeSelectionInputProps {
  value: number[];
  onChange: (value: number[]) => void;
}

const MultipleNodeSelectionInput = ({ value, onChange }: MultipleNodeSelectionInputProps) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    dispatch(
      getNodeList({
        page_size: 1000,
        page: 1,
        order_by: "id",
        order_direction: "desc",
        conditions: {},
      }),
    )
      .then((res) => setNodes(res.nodes ?? []))
      .finally(() => setLoading(false));
  }, [dispatch]);

  const nodeMap = nodes.reduce<Record<number, Node>>((acc, node) => {
    acc[node.id] = node;
    return acc;
  }, {});

  return (
    <DenseSelect
      multiple
      displayEmpty
      disabled={loading}
      value={value}
      onChange={(e) => {
        const selected = e.target.value;
        onChange(typeof selected === "string" ? [] : selected);
      }}
      sx={{
        minHeight: 39,
      }}
      MenuProps={{
        PaperProps: { sx: { maxWidth: 230 } },
        MenuListProps: {
          sx: {
            "& .MuiMenuItem-root": {
              whiteSpace: "normal",
            },
          },
        },
      }}
      renderValue={(selected) => {
        if (loading) {
          return <CircularProgress size={20} sx={{ mt: "1px" }} />;
        }
        if (selected.length === 0) {
          return (
            <ListItemText
              primary={<em>{t("group.allNodes")}</em>}
              slotProps={{
                primary: { color: "textSecondary", variant: "body2" },
              }}
            />
          );
        }
        return (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
            {selected.map((id) => (
              <SquareChip
                key={id}
                size="small"
                label={nodeMap[id] ? nodeMap[id].name : t("group.deletedNode", { id })}
              />
            ))}
          </Box>
        );
      }}
    >
      {nodes.map((node) => (
        <SquareMenuItem key={node.id} value={node.id}>
          <Box sx={{ display: "flex", flexDirection: "column" }}>
            <Typography variant="body2" fontWeight={600}>
              {node.name}
            </Typography>
            <Typography variant="caption" color="textSecondary">
              {node.server}
            </Typography>
          </Box>
        </SquareMenuItem>
      ))}
      {value
        .filter((id) => !nodes.find((node) => node.id === id))
        .map((id) => (
          <SquareMenuItem key={id} value={id}>
            <Typography variant="body2" fontWeight={600} color="textSecondary">
              {t("group.deletedNode", { id })}
            </Typography>
          </SquareMenuItem>
        ))}
    </DenseSelect>
  );
};

export default MultipleNodeSelectionInput;
