import {
  Badge,
  Box,
  Button,
  Checkbox,
  Container,
  Divider,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TableSortLabel,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import { bindPopover, bindTrigger, usePopupState } from "material-ui-popup-state/hooks";
import { useQueryState } from "nuqs";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { batchDeleteAuditLogs, getAuditLogList } from "../../../api/api";
import { AdminListService, AuditLog } from "../../../api/dashboard";
import { useAppDispatch } from "../../../redux/hooks";
import { confirmOperation } from "../../../redux/thunks/dialog";
import { NoWrapTableCell, SecondaryButton, StyledTableContainerPaper } from "../../Common/StyledComponents";
import ArrowSync from "../../Icons/ArrowSync";
import Broom from "../../Icons/Broom";
import Filter from "../../Icons/Filter";
import PageContainer from "../../Pages/PageContainer";
import PageHeader from "../../Pages/PageHeader";
import TablePagination from "../Common/TablePagination";
import { OrderByQuery, OrderDirectionQuery, PageQuery, PageSizeQuery } from "../StoragePolicy/StoragePolicySetting";
import UserDialog from "../User/UserDialog/UserDialog";
import EventArchiveDialog from "./EventArchiveDialog";
import Archive from "../../Icons/Archive";
import Delete from "../../Icons/Delete";
import EventCleanupDialog from "./EventCleanupDialog";
import EventDetailDialog from "./EventDetailDialog";
import EventFilterPopover from "./EventFilterPopover";
import EventRow from "./EventRow";
import EntityDialog from "../Entity/EntityDialog/EntityDialog";

export const AuditUserQuery = "user";
export const AuditIPQuery = "ip";
export const AuditCategoryQuery = "category";
export const AuditCorrelationQuery = "correlation_id";
export const AuditFileQuery = "file";

const AuditLogList = () => {
  const { t } = useTranslation("dashboard");
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"));
  const dispatch = useAppDispatch();
  const [loading, setLoading] = useState(true);
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [page, setPage] = useQueryState(PageQuery, { defaultValue: "1" });
  const [pageSize, setPageSize] = useQueryState(PageSizeQuery, { defaultValue: "10" });
  const [orderBy, setOrderBy] = useQueryState(OrderByQuery, { defaultValue: "" });
  const [orderDirection, setOrderDirection] = useQueryState(OrderDirectionQuery, { defaultValue: "desc" });
  const [user, setUser] = useQueryState(AuditUserQuery, { defaultValue: "" });
  const [ip, setIp] = useQueryState(AuditIPQuery, { defaultValue: "" });
  const [categories, setCategories] = useQueryState(AuditCategoryQuery, { defaultValue: "" });
  const [correlationID, setCorrelationID] = useQueryState(AuditCorrelationQuery, { defaultValue: "" });
  const [fileID, setFileID] = useQueryState(AuditFileQuery, { defaultValue: "" });

  const [count, setCount] = useState(0);
  const [selected, setSelected] = useState<readonly number[]>([]);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [userDialogOpen, setUserDialogOpen] = useState(false);
  const [userDialogID, setUserDialogID] = useState<number | undefined>(undefined);
  const [cleanupOpen, setCleanupOpen] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailID, setDetailID] = useState<number>();
  const [entityDialogOpen, setEntityDialogOpen] = useState(false);
  const [entityDialogID, setEntityDialogID] = useState<number>();

  const filterPopupState = usePopupState({
    variant: "popover",
    popupId: "eventFilterPopover",
  });

  const pageInt = parseInt(page) ?? 1;
  const pageSizeInt = parseInt(pageSize) ?? 10;
  const selectedCategories = useMemo(
    () =>
      categories
        .split(",")
        .map((value) => parseInt(value))
        .filter((value) => !Number.isNaN(value) && value >= 0),
    [categories],
  );

  const fetchLogs = useCallback(() => {
    setLoading(true);
    setSelected([]);

    const params: AdminListService = {
      page: pageInt,
      page_size: pageSizeInt,
      order_by: orderBy ?? "",
      order_direction: orderDirection ?? "desc",
      conditions: {
        audit_log_category: selectedCategories.join(","),
        audit_log_user: user,
        audit_log_ip: ip,
        audit_log_correlation_id: correlationID,
        audit_log_file: fileID,
      },
    };

    dispatch(getAuditLogList(params))
      .then((res) => {
        setLogs(res.logs ?? []);
        setPageSize(res.pagination.page_size.toString());
        setCount(res.pagination.total_items ?? 0);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [pageInt, pageSizeInt, orderBy, orderDirection, user, ip, correlationID, fileID, selectedCategories, dispatch, setPageSize]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  const clearFilters = useCallback(() => {
    setUser("");
    setIp("");
    setCategories("");
    setCorrelationID("");
    setFileID("");
  }, [setUser, setIp, setCategories, setCorrelationID, setFileID]);

  const handleDelete = useCallback(() => {
    setDeleteLoading(true);
    dispatch(confirmOperation(t("event.confirmBatchDelete", { num: selected.length })))
      .then(() => {
        dispatch(batchDeleteAuditLogs(Array.from(selected))).then(() => {
          fetchLogs();
        });
      })
      .finally(() => {
        setDeleteLoading(false);
      });
  }, [dispatch, selected, t, fetchLogs]);

  const handleSelectAllClick = (event: React.ChangeEvent<HTMLInputElement>) => {
    if (event.target.checked) {
      setSelected(logs.map((log) => log.id));
      return;
    }
    setSelected([]);
  };

  const handleSelect = useCallback(
    (id: number) => {
      const index = selected.indexOf(id);
      if (index === -1) {
        setSelected([...selected, id]);
      } else {
        setSelected(selected.filter((item) => item !== id));
      }
    },
    [selected],
  );

  const hasActiveFilters = !!(user || ip || categories || correlationID || fileID);
  const idSorted = orderBy === "id" || orderBy === "";
  const direction = (orderDirection as "asc" | "desc") ?? "desc";

  const handleSort = (column: string) => () => {
    const sameColumn = orderBy === column || (column === "id" && idSorted);
    const nextDirection = sameColumn && direction === "asc" ? "desc" : "asc";
    setOrderBy(column);
    setOrderDirection(nextDirection);
  };

  const openUserDialog = useCallback((id?: number) => {
    if (!id) {
      return;
    }
    setUserDialogID(id);
    setUserDialogOpen(true);
  }, []);

  const openEntityDialog = useCallback((id?: number) => {
    if (!id) {
      return;
    }
    setEntityDialogID(id);
    setEntityDialogOpen(true);
  }, []);

  return (
    <PageContainer>
      <Container maxWidth="xl">
        <PageHeader title={t("dashboard:nav.events")} />
        <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
          <EventFilterPopover
            {...bindPopover(filterPopupState)}
            user={user}
            setUser={setUser}
            ip={ip}
            setIp={setIp}
            categories={selectedCategories}
            setCategories={(value) => setCategories(value.join(","))}
            correlationID={correlationID}
            setCorrelationID={setCorrelationID}
            fileID={fileID}
            setFileID={setFileID}
            clearFilters={clearFilters}
          />
          <SecondaryButton onClick={fetchLogs} disabled={loading} variant="contained" startIcon={<ArrowSync />}>
            {t("node.refresh")}
          </SecondaryButton>
          <Badge color="primary" variant="dot" invisible={!hasActiveFilters}>
            <SecondaryButton startIcon={<Filter />} variant="contained" {...bindTrigger(filterPopupState)}>
              {t("user.filter")}
            </SecondaryButton>
          </Badge>
          <SecondaryButton startIcon={<Broom />} variant="contained" onClick={() => setCleanupOpen(true)}>
            {t("event.cleanup")}
          </SecondaryButton>
          <SecondaryButton startIcon={<Archive />} variant="contained" onClick={() => setArchiveOpen(true)}>
            {t("event.archive")}
          </SecondaryButton>
          {selected.length > 0 && !isMobile && (
            <>
              <Divider orientation="vertical" flexItem />
              <Button startIcon={<Delete />} variant="contained" color="error" onClick={handleDelete}>
                {t("event.deleteXEvents", { num: selected.length })}
              </Button>
            </>
          )}
        </Stack>
        {isMobile && selected.length > 0 && (
          <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
            <Button startIcon={<Delete />} variant="contained" color="error" onClick={handleDelete}>
              {t("event.deleteXEvents", { num: selected.length })}
            </Button>
          </Stack>
        )}
        <UserDialog open={userDialogOpen} onClose={() => setUserDialogOpen(false)} userID={userDialogID} />
        <EntityDialog
          open={entityDialogOpen}
          onClose={() => setEntityDialogOpen(false)}
          entityID={entityDialogID}
        />
        <EventCleanupDialog open={cleanupOpen} onClose={() => setCleanupOpen(false)} onComplete={fetchLogs} />
        <EventArchiveDialog open={archiveOpen} onClose={() => setArchiveOpen(false)} onComplete={fetchLogs} />
        <EventDetailDialog
          open={detailOpen}
          onClose={() => setDetailOpen(false)}
          eventID={detailID}
        />
        <TableContainer component={StyledTableContainerPaper} sx={{ mt: 2 }}>
          <Table size="small" stickyHeader sx={{ width: "100%", tableLayout: "fixed" }}>
            <TableHead>
              <TableRow>
                <TableCell padding="checkbox" sx={{ width: "36px!important" }} width={50}>
                  <Checkbox
                    size="small"
                    indeterminate={selected.length > 0 && selected.length < logs.length}
                    checked={logs.length > 0 && selected.length === logs.length}
                    onChange={handleSelectAllClick}
                  />
                </TableCell>
                <NoWrapTableCell width={60}>
                  <TableSortLabel active={idSorted} direction={direction} onClick={handleSort("id")}>
                    {t("group.#")}
                  </TableSortLabel>
                </NoWrapTableCell>
                <NoWrapTableCell width={300}>{t("event.event")}</NoWrapTableCell>
                <NoWrapTableCell width={100}>{t("event.ip")}</NoWrapTableCell>
                <NoWrapTableCell width={150}>{t("event.correlationId")}</NoWrapTableCell>
                <NoWrapTableCell width={100}>{t("event.initiator")}</NoWrapTableCell>
                <NoWrapTableCell width={150}>
                  <TableSortLabel
                    active={orderBy === "created_at"}
                    direction={direction}
                    onClick={handleSort("created_at")}
                  >
                    {t("file.createdAt")}
                  </TableSortLabel>
                </NoWrapTableCell>
                <NoWrapTableCell width={60} align="right"></NoWrapTableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {!loading &&
                logs.map((log) => (
                  <EventRow
                    key={log.id}
                    log={log}
                    deleting={deleteLoading}
                    selected={selected.indexOf(log.id) !== -1}
                    onSelect={handleSelect}
                    onDelete={fetchLogs}
                    onDetails={(id) => {
                      setDetailID(id);
                      setDetailOpen(true);
                    }}
                    openUserDialog={openUserDialog}
                    openEntityDialog={openEntityDialog}
                  />
                ))}
              {loading &&
                (logs.length > 0 ? logs.slice(0, 10) : Array.from(Array(10))).map((log, index) => (
                  <EventRow key={`loading-${log?.id ?? index}`} loading={true} />
                ))}
            </TableBody>
          </Table>
        </TableContainer>
        {count > 0 && (
          <Box sx={{ mt: 1 }}>
            <TablePagination
              page={pageInt}
              totalItems={count}
              rowsPerPage={pageSizeInt}
              rowsPerPageOptions={[10, 25, 50, 100, 200, 500]}
              onRowsPerPageChange={(value) => setPageSize(value.toString())}
              onChange={(_, value) => setPage(value.toString())}
            />
          </Box>
        )}
      </Container>
    </PageContainer>
  );
};

export default AuditLogList;
