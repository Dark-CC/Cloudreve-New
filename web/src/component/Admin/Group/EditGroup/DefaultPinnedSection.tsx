import { Add, ArrowDownward, ArrowUpward, Close, FolderOff } from "@mui/icons-material";
import {
  Autocomplete,
  Box,
  CircularProgress,
  Collapse,
  FormControl,
  IconButton,
  Link,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import { DndProvider } from "react-dnd";
import { HTML5Backend } from "react-dnd-html5-backend";
import { useCallback, useContext, useEffect, useMemo, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { Link as RouterLink } from "react-router-dom";
import { getShareList } from "../../../../api/api";
import { GroupEnt, PinnedShareSetting, Share } from "../../../../api/dashboard";
import { useAppDispatch } from "../../../../redux/hooks";
import { DenseFilledTextField, NoWrapCell, SquareChip, StyledTableContainerPaper } from "../../../Common/StyledComponents";
import FileTypeIcon from "../../../FileManager/Explorer/FileTypeIcon";
import { NoMarginHelperText, SettingSection, SettingSectionContent } from "../../Settings/Settings";
import { AnonymousGroupID } from "../GroupRow";
import { GroupSettingContext } from "./GroupSettingWrapper";

const DefaultPinnedSection = () => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const { values, setGroup } = useContext(GroupSettingContext);
  const pinned = useMemo(() => values.settings?.default_pinned_shares ?? [], [values.settings?.default_pinned_shares]);
  const [shares, setShares] = useState<Record<number, Share>>({});
  const [options, setOptions] = useState<number[]>([]);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const ids = pinned.map((item) => item.share_id).filter((id) => id > 0 && !shares[id]);
    if (ids.length === 0) {
      return;
    }
    let cancelled = false;
    dispatch(
      getShareList({
        page: 1,
        page_size: 100,
        order_by: "id",
        order_direction: "desc",
        conditions: { share_id: ids.join(",") },
      }),
    )
      .then((res) => {
        if (!cancelled) {
          setShares((prev) => ({
            ...prev,
            ...(res.shares ?? []).reduce<Record<number, Share>>((acc, share) => {
              acc[share.id] = share;
              return acc;
            }, {}),
          }));
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [dispatch, pinned, shares]);

  useEffect(() => {
    if (search === "") {
      setOptions([]);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      setLoading(true);
      dispatch(
        getShareList({
          page: 1,
          page_size: 50,
          order_by: "id",
          order_direction: "desc",
          conditions: { share_id: search.trim() },
        }),
      )
        .then((res) => {
          if (!cancelled) {
            setOptions((res.shares ?? []).map((share) => share.id));
            setShares((prev) => ({
              ...prev,
              ...(res.shares ?? []).reduce<Record<number, Share>>((acc, share) => {
                acc[share.id] = share;
                return acc;
              }, {}),
            }));
          }
        })
        .finally(() => {
          if (!cancelled) {
            setLoading(false);
          }
        });
    }, 400);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [dispatch, search]);

  const updatePinned = useCallback(
    (next: PinnedShareSetting[]) => {
      setGroup((prev: GroupEnt) => ({
        ...prev,
        settings: { ...prev.settings, default_pinned_shares: next.length ? next : undefined },
      }));
    },
    [setGroup],
  );

  const move = (from: number, to: number) => {
    if (from === to) return;
    const next = [...pinned];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    updatePinned(next);
  };

  const addShare = (shareID: number) => {
    if (!shareID || pinned.some((item) => item.share_id === shareID)) return;
    updatePinned([...pinned, { share_id: shareID }]);
    setSearch("");
  };

  const rename = (index: number, name: string) => {
    const next = [...pinned];
    next[index] = { ...next[index], name: name.trim() || undefined };
    updatePinned(next);
  };

  if (values?.id == AnonymousGroupID) {
    return null;
  }

  return (
    <SettingSection>
      <Typography variant="h6" gutterBottom>
        {t("group.defaultPinned")}
      </Typography>
      <SettingSectionContent>
        <FormControl fullWidth sx={{ maxWidth: 520 }}>
          <Autocomplete<number, false, false, false>
            value={null}
            options={options}
            loading={loading}
            blurOnSelect
            clearOnBlur
            onChange={(_e, value) => {
              if (typeof value === "number") addShare(value);
            }}
            onInputChange={(_e, value) => setSearch(value)}
            getOptionDisabled={(option) => pinned.some((item) => item.share_id === option)}
            noOptionsText={t("application:modals.noResults")}
            renderOption={(props, option) => {
              const share = shares[option];
              return (
                <Box component="li" {...props} key={option} sx={{ display: "flex", width: "100%", alignItems: "center" }}>
                  {share?.edges?.file ? (
                    <FileTypeIcon name={share.edges.file.name ?? ""} fileType={share.edges.file.type ?? 0} />
                  ) : (
                    <FolderOff />
                  )}
                  <Typography
                    variant="body2"
                    sx={{ width: "100%", ml: 2, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
                  >
                    {share?.edges?.file?.name ?? t("application:share.expiredLink")}
                  </Typography>
                </Box>
              );
            }}
            renderInput={(params) => (
              <DenseFilledTextField
                {...params}
                variant="outlined"
                margin="dense"
                placeholder={t("settings.searchShare")}
                InputProps={{
                  ...params.InputProps,
                  endAdornment: (
                    <>
                      {loading ? <CircularProgress color="inherit" size={18} /> : null}
                      {params.InputProps.endAdornment}
                    </>
                  ),
                }}
              />
            )}
          />
          <NoMarginHelperText>
            <Trans
              i18nKey="group.defaultPinnedDes"
              ns="dashboard"
              components={[<Link component={RouterLink} to="/admin/share" />]}
            />
          </NoMarginHelperText>
        </FormControl>
        <Collapse in={pinned.length > 0} unmountOnExit>
          <TableContainer component={StyledTableContainerPaper} sx={{ mt: 2 }}>
            <DndProvider backend={HTML5Backend}>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <NoWrapCell>{t("group.pinnedShare")}</NoWrapCell>
                    <NoWrapCell>{t("group.pinnedDisplayName")}</NoWrapCell>
                    <NoWrapCell>{t("settings.actions")}</NoWrapCell>
                    <NoWrapCell />
                  </TableRow>
                </TableHead>
                <TableBody>
                  {pinned.map((item, index) => {
                    const share = shares[item.share_id];
                    const fileName = share?.edges?.file?.name ?? t("application:share.expiredLink");
                    return (
                      <TableRow key={`${item.share_id}-${index}`}>
                        <TableCell>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                            {share?.edges?.file ? (
                              <FileTypeIcon name={fileName} fileType={share.edges.file.type ?? 0} />
                            ) : (
                              <FolderOff color="action" />
                            )}
                            <SquareChip size="small" label={`#${item.share_id}`} />
                            <Typography variant="body2" noWrap>
                              {fileName}
                            </Typography>
                          </Box>
                        </TableCell>
                        <TableCell>
                          <DenseFilledTextField
                            variant="outlined"
                            size="small"
                            defaultValue={item.name ?? ""}
                            placeholder={fileName}
                            onChange={(e) => rename(index, e.target.value)}
                            fullWidth
                          />
                        </TableCell>
                        <TableCell>
                          <IconButton
                            size="small"
                            onClick={() => updatePinned(pinned.filter((_, i) => i !== index))}
                          >
                            <Close fontSize="small" />
                          </IconButton>
                        </TableCell>
                        <TableCell>
                          <IconButton size="small" onClick={() => move(index, index - 1)} disabled={index === 0}>
                            <ArrowUpward sx={{ fontSize: 18 }} />
                          </IconButton>
                          <IconButton
                            size="small"
                            onClick={() => move(index, index + 1)}
                            disabled={index === pinned.length - 1}
                          >
                            <ArrowDownward sx={{ fontSize: 18 }} />
                          </IconButton>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </DndProvider>
          </TableContainer>
        </Collapse>
      </SettingSectionContent>
    </SettingSection>
  );
};

export default DefaultPinnedSection;
