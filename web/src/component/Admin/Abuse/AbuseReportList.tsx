import { Delete } from "@mui/icons-material";
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
import { batchDeleteAbuseReports, getAbuseReportList } from "../../../api/api";
import { AbuseReport, AdminListService } from "../../../api/dashboard";
import { useAppDispatch } from "../../../redux/hooks";
import { confirmOperation } from "../../../redux/thunks/dialog";
import { NoWrapTableCell, SecondaryButton, StyledTableContainerPaper } from "../../Common/StyledComponents";
import ArrowSync from "../../Icons/ArrowSync";
import Filter from "../../Icons/Filter";
import PageContainer from "../../Pages/PageContainer";
import PageHeader from "../../Pages/PageHeader";
import TablePagination from "../Common/TablePagination";
import ShareDialog from "../Share/ShareDialog/ShareDialog";
import { OrderByQuery, OrderDirectionQuery, PageQuery, PageSizeQuery } from "../StoragePolicy/StoragePolicySetting";
import UserDialog from "../User/UserDialog/UserDialog";
import AbuseReportFilterPopover from "./AbuseReportFilterPopover";
import AbuseReportRow from "./AbuseReportRow";

export const AbuseUserQuery = "user";
export const AbuseReporterQuery = "reporter";
export const AbuseShareQuery = "share";
export const AbuseReasonQuery = "reason";

const AbuseReportList = () => {
  const { t } = useTranslation("dashboard");
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"));
  const dispatch = useAppDispatch();
  const [loading, setLoading] = useState(true);
  const [reports, setReports] = useState<AbuseReport[]>([]);
  const [page, setPage] = useQueryState(PageQuery, { defaultValue: "1" });
  const [pageSize, setPageSize] = useQueryState(PageSizeQuery, { defaultValue: "10" });
  const [orderBy, setOrderBy] = useQueryState(OrderByQuery, { defaultValue: "" });
  const [orderDirection, setOrderDirection] = useQueryState(OrderDirectionQuery, { defaultValue: "desc" });
  const [user, setUser] = useQueryState(AbuseUserQuery, { defaultValue: "" });
  const [reporter, setReporter] = useQueryState(AbuseReporterQuery, { defaultValue: "" });
  const [share, setShare] = useQueryState(AbuseShareQuery, { defaultValue: "" });
  const [reason, setReason] = useQueryState(AbuseReasonQuery, { defaultValue: "" });

  const [count, setCount] = useState(0);
  const [selected, setSelected] = useState<readonly number[]>([]);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [userDialogOpen, setUserDialogOpen] = useState(false);
  const [userDialogID, setUserDialogID] = useState<number | undefined>(undefined);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);
  const [shareDialogID, setShareDialogID] = useState<number | undefined>(undefined);

  const filterPopupState = usePopupState({
    variant: "popover",
    popupId: "abuseReportFilterPopover",
  });

  const pageInt = parseInt(page) ?? 1;
  const pageSizeInt = parseInt(pageSize) ?? 10;

  useEffect(() => {
    fetchReports();
  }, [page, pageSize, orderBy, orderDirection, user, reporter, share, reason]);

  const clearFilters = useCallback(() => {
    setUser("");
    setReporter("");
    setShare("");
    setReason("");
  }, [setUser, setReporter, setShare, setReason]);

  const fetchReports = () => {
    setLoading(true);
    setSelected([]);

    const params: AdminListService = {
      page: pageInt,
      page_size: pageSizeInt,
      order_by: orderBy ?? "",
      order_direction: orderDirection ?? "desc",
      conditions: {
        abuse_report_user: user,
        abuse_report_reporter: reporter,
        abuse_report_share: share,
        abuse_report_category: reason,
      },
    };

    dispatch(getAbuseReportList(params))
      .then((res) => {
        setReports(res.reports ?? []);
        setPageSize(res.pagination.page_size.toString());
        setCount(res.pagination.total_items ?? 0);
      })
      .finally(() => {
        setLoading(false);
      });
  };

  const handleDelete = useCallback(() => {
    setDeleteLoading(true);
    dispatch(confirmOperation(t("abuseReport.confirmBatchDelete", { num: selected.length })))
      .then(() => {
        dispatch(batchDeleteAbuseReports({ ids: Array.from(selected) })).then(() => {
          fetchReports();
        });
      })
      .finally(() => {
        setDeleteLoading(false);
      });
  }, [dispatch, selected, t]);

  const handleSelectAllClick = (event: React.ChangeEvent<HTMLInputElement>) => {
    if (event.target.checked) {
      setSelected(reports.map((report) => report.id));
      return;
    }
    setSelected([]);
  };

  const handleSelect = useCallback(
    (id: number) => {
      const index = selected.indexOf(id);
      let next: readonly number[] = [];
      if (index === -1) {
        next = [...selected, id];
      } else {
        next = selected.filter((item) => item !== id);
      }
      setSelected(next);
    },
    [selected],
  );

  const hasActiveFilters = useMemo(
    () => !!(user || reporter || share || reason),
    [user, reporter, share, reason],
  );

  const idSorted = orderBy === "id" || orderBy === "";
  const direction = (orderDirection as "asc" | "desc") ?? "desc";

  const handleSort = (column: string) => () => {
    const nextDirection = idSorted && column === "id" && direction === "asc" ? "desc" : "asc";
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

  const openShareDialog = useCallback((id?: number) => {
    if (!id) {
      return;
    }
    setShareDialogID(id);
    setShareDialogOpen(true);
  }, []);

  return (
    <PageContainer>
      <Container maxWidth="xl">
        <PageHeader title={t("dashboard:nav.abuseReport")} />
        <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
          <AbuseReportFilterPopover
            {...bindPopover(filterPopupState)}
            user={user}
            setUser={setUser}
            reporter={reporter}
            setReporter={setReporter}
            share={share}
            setShare={setShare}
            reason={reason}
            setReason={setReason}
            clearFilters={clearFilters}
          />
          <SecondaryButton onClick={fetchReports} disabled={loading} variant="contained" startIcon={<ArrowSync />}>
            {t("node.refresh")}
          </SecondaryButton>
          <Badge color="primary" variant="dot" invisible={!hasActiveFilters}>
            <SecondaryButton startIcon={<Filter />} variant="contained" {...bindTrigger(filterPopupState)}>
              {t("user.filter")}
            </SecondaryButton>
          </Badge>
          {selected.length > 0 && !isMobile && (
            <>
              <Divider orientation="vertical" flexItem />
              <Button startIcon={<Delete />} variant="contained" color="error" onClick={handleDelete}>
                {t("abuseReport.deleteXAbuseReports", { num: selected.length })}
              </Button>
            </>
          )}
        </Stack>
        {isMobile && selected.length > 0 && (
          <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
            <Button startIcon={<Delete />} variant="contained" color="error" onClick={handleDelete}>
              {t("abuseReport.deleteXAbuseReports", { num: selected.length })}
            </Button>
          </Stack>
        )}
        <UserDialog open={userDialogOpen} onClose={() => setUserDialogOpen(false)} userID={userDialogID} />
        <ShareDialog open={shareDialogOpen} onClose={() => setShareDialogOpen(false)} shareID={shareDialogID} />
        <TableContainer component={StyledTableContainerPaper} sx={{ mt: 2 }}>
          <Table size="small" stickyHeader sx={{ width: "100%", tableLayout: "fixed" }}>
            <TableHead>
              <TableRow>
                <TableCell padding="checkbox" sx={{ width: "36px!important" }} width={50}>
                  <Checkbox
                    size="small"
                    indeterminate={selected.length > 0 && selected.length < reports.length}
                    checked={reports.length > 0 && selected.length === reports.length}
                    onChange={handleSelectAllClick}
                  />
                </TableCell>
                <NoWrapTableCell width={60} sortDirection={idSorted ? direction : false}>
                  <TableSortLabel active={idSorted} direction={direction} onClick={handleSort("id")}>
                    {t("group.#")}
                  </TableSortLabel>
                </NoWrapTableCell>
                <NoWrapTableCell width={200}>{t("application:vas.reportTarget")}</NoWrapTableCell>
                <NoWrapTableCell width={150}>{t("abuseReport.folderPath")}</NoWrapTableCell>
                <NoWrapTableCell width={100}>{t("application:vas.reportReason")}</NoWrapTableCell>
                <NoWrapTableCell width={300}>{t("application:vas.reportDescription")}</NoWrapTableCell>
                <NoWrapTableCell width={100}>{t("abuseReport.reporter")}</NoWrapTableCell>
                <NoWrapTableCell width={150}>
                  <TableSortLabel
                    active={orderBy === "created_at"}
                    direction={direction}
                    onClick={handleSort("created_at")}
                  >
                    {t("file.createdAt")}
                  </TableSortLabel>
                </NoWrapTableCell>
                <NoWrapTableCell width={60}></NoWrapTableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {!loading && reports.map((report) => (
                <AbuseReportRow
                  key={report.id}
                  abuseReport={report}
                  deleting={deleteLoading}
                  selected={selected.indexOf(report.id) !== -1}
                  onSelect={handleSelect}
                  onDelete={fetchReports}
                  openUserDialog={openUserDialog}
                  openShareDialog={openShareDialog}
                />
              ))}
              {loading &&
                (reports.length > 0 ? reports : Array.from(Array(pageSizeInt))).map((report, index) => (
                  <AbuseReportRow key={`loading-${report?.id ?? index}`} loading={true} />
                ))}
              {!loading && reports.length === 0 && (
                <TableRow>
                  <TableCell colSpan={9} align="center" sx={{ py: 6 }}>
                    {t("abuseReport.empty")}
                  </TableCell>
                </TableRow>
              )}
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

export default AbuseReportList;
