import { Box, useTheme } from "@mui/material";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getShareList } from "../../../api/api.ts";
import { Share } from "../../../api/dashboard.ts";
import { useAppDispatch } from "../../../redux/hooks.ts";
import { DenseAutocomplete, DenseFilledTextField, NoWrapBox, SquareChip } from "../../Common/StyledComponents.tsx";
import FileTypeIcon from "../../FileManager/Explorer/FileTypeIcon.tsx";

export interface SharesInputProps {
  value: number[];
  onChange: (value: number[]) => void;
}

const SharesInput = ({ value, onChange }: SharesInputProps) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const [shares, setShares] = useState<Share[]>([]);

  useEffect(() => {
    dispatch(
      getShareList({
        page: 1,
        page_size: 1000,
        order_by: "id",
        order_direction: "desc",
      }),
    ).then((res) => setShares(res.shares ?? []));
  }, [dispatch]);

  return (
    <DenseAutocomplete
      value={shares.filter((share) => value.includes(share.id))}
      onChange={(_event, newValue) => onChange(newValue.map((share) => share.id))}
      multiple
      options={shares}
      getOptionLabel={(option) => `#${option.id} ${option.edges?.file?.name ?? ""}`}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      blurOnSelect
      renderOption={(props, option) => (
        <Box component="li" {...props}>
          <SquareChip size="small" label={`#${option.id}`} sx={{ mr: 1 }} />
          <NoWrapBox>{option.edges?.file?.name}</NoWrapBox>
        </Box>
      )}
      renderTags={(selected, getTagProps) => {
        return selected.map((option, index) => {
          const { key, ...tagProps } = getTagProps({ index });
          return (
            <SquareChip
              key={key}
              {...tagProps}
              size="small"
              icon={<FileTypeIcon name={option.edges?.file?.name ?? ""} fileType={option.edges?.file?.type ?? 0} />}
              label={`#${option.id} ${option.edges?.file?.name ?? ""}`}
            />
          );
        });
      }}
      renderInput={(params) => (
        <DenseFilledTextField
          {...params}
          sx={{
            "& .MuiInputBase-root": {},
            "& .MuiInputBase-root.MuiOutlinedInput-root": {
              paddingTop: theme.spacing(0.6),
              paddingBottom: theme.spacing(0.6),
            },
            mt: 0,
          }}
          variant="outlined"
          margin="dense"
          placeholder={t("dashboard:settings.searchShare")}
          type="text"
          fullWidth
        />
      )}
    />
  );
};

export default SharesInput;
