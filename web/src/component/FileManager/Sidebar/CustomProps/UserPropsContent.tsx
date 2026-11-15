import Autocomplete from "@mui/material/Autocomplete";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getSearchUser } from "../../../../api/api.ts";
import { User } from "../../../../api/user.ts";
import { NoLabelFilledTextField } from "../../../Common/StyledComponents.tsx";
import UserAvatar from "../../../Common/User/UserAvatar.tsx";
import { useAppDispatch } from "../../../../redux/hooks.ts";
import { PropsContentProps } from "./CustomPropsItem.tsx";

const UserPropsContent = ({ prop, onChange, loading, readOnly, fullSize }: PropsContentProps) => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const [selected, setSelected] = useState<User | null>(null);
  const [inputValue, setInputValue] = useState("");
  const [options, setOptions] = useState<User[]>([]);
  const [searching, setSearching] = useState(false);
  const currentValue = prop.value;

  useEffect(() => {
    setSelected(null);
  }, [currentValue]);

  useEffect(() => {
    if (inputValue.trim().length < 2) {
      setOptions([]);
      return;
    }

    let cancelled = false;
    setSearching(true);
    dispatch(getSearchUser(inputValue.trim()))
      .then((users) => {
        if (!cancelled) {
          setOptions(users ?? []);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setSearching(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [dispatch, inputValue]);

  const filterOptions = useMemo(
    () => (candidate: User[], { inputValue: query }: { inputValue: string }) => {
      if (!query) {
        return candidate;
      }
      const lower = query.toLowerCase();
      return candidate.filter((option) =>
        `${option.nickname} ${option.email ?? option.id}`.toLowerCase().includes(lower),
      );
    },
    [],
  );

  const handleSelect = useCallback(
    (_event: unknown, value: User | null) => {
      setSelected(value);
      if (value) {
        setInputValue("");
        if (value.id !== currentValue) {
          onChange(value.id);
        }
      }
    },
    [currentValue, onChange],
  );

  const handleClear = useCallback(() => {
    setSelected(null);
    setInputValue("");
    if (currentValue) {
      onChange("");
    }
  }, [currentValue, onChange]);

  if (readOnly) {
    return currentValue ? <UserAvatar uid={currentValue} sx={{ width: 20, height: 20 }} /> : null;
  }

  if (currentValue && !selected) {
    return (
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        <UserAvatar uid={currentValue} sx={{ width: 20, height: 20 }} />
        <Typography
          variant="body2"
          color="error"
          sx={{ cursor: "pointer", textDecoration: "underline" }}
          onClick={handleClear}
        >
          {t("application:fileManager.delete")}
        </Typography>
      </Box>
    );
  }

  if (selected) {
    return (
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        <UserAvatar user={selected} sx={{ width: 20, height: 20 }} />
        <Typography
          variant="body2"
          color="error"
          sx={{ cursor: "pointer", textDecoration: "underline" }}
          onClick={handleClear}
        >
          {t("application:fileManager.delete")}
        </Typography>
      </Box>
    );
  }

  return (
    <Autocomplete
      options={options}
      filterOptions={filterOptions}
      loading={searching}
      value={null}
      inputValue={inputValue}
      onInputChange={(_event, value) => setInputValue(value)}
      onChange={handleSelect}
      getOptionLabel={(option) => option.nickname}
      noOptionsText={
        inputValue.trim().length < 2
          ? t("application:fileManager.typeToSearch")
          : t("application:modals.noResults")
      }
      renderOption={(props, option) => (
        <li {...props} key={option.id}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <UserAvatar user={option} sx={{ width: 24, height: 24 }} />
            <Box>
              <Typography variant="body2">{option.nickname}</Typography>
              <Typography variant="caption" color="text.secondary">
                {option.email ?? option.id}
              </Typography>
            </Box>
          </Box>
        </li>
      )}
      renderInput={(params) => (
        <NoLabelFilledTextField
          {...params}
          variant="filled"
          placeholder={t("application:fileManager.searchUser")}
          disabled={loading}
          fullWidth
          fullSize={fullSize}
          onClick={(event) => event.stopPropagation()}
        />
      )}
      ListboxProps={{ style: { maxHeight: 200 } }}
    />
  );
};

export default UserPropsContent;
