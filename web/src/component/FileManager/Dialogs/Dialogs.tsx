import DeleteConfirmation from "./DeleteConfirmation.tsx";
import DesktopMountSetup from "./DesktopMountSetup.tsx";
import AggregatedErrorDetail from "../../Dialogs/AggregatedErrorDetail.tsx";
import LockConflictDetails from "./LockConflictDetails.tsx";
import Rename from "./Rename.tsx";
import PathSelection from "./PathSelection.tsx";
import Tags from "./Tags.tsx";
import ChangeIcon from "./ChangeIcon.tsx";
import ShareDialog from "./Share/ShareDialog.tsx";
import UploadLinkDialog from "./UploadLinkDialog.tsx";
import VersionControl from "./VersionControl.tsx";
import ManageShares from "./Share/ManageShares.tsx";
import StaleVersionConfirm from "./StaleVersionConfirm.tsx";
import SaveAs from "./SaveAs.tsx";
import Photopea from "../../Viewers/Photopea/Photopea.tsx";
import OpenWith from "./OpenWith.tsx";
import Wopi from "../../Viewers/Wopi.tsx";
import ArchivePreview from "../../Viewers/ArchivePreview/ArchivePreview.tsx";
import CodeViewer from "../../Viewers/CodeViewer/CodeViewer.tsx";
import DrawIOViewer from "../../Viewers/DrawIO/DrawIOViewer.tsx";
import MarkdownViewer from "../../Viewers/MarkdownEditor/MarkdownViewer.tsx";
import VideoViewer from "../../Viewers/Video/VideoViewer.tsx";
import PdfViewer from "../../Viewers/PdfViewer.tsx";
import CustomViewer from "../../Viewers/CustomViewer.tsx";
import EpubViewer from "../../Viewers/EpubViewer/EpubViewer.tsx";
import ExcalidrawViewer from "../../Viewers/Excalidraw/ExcalidrawViewer.tsx";
import CreateNew from "./CreateNew.tsx";
import { useAppSelector } from "../../../redux/hooks.ts";
import CreateArchive from "./CreateArchive.tsx";
import ExtractArchive from "./ExtractArchive.tsx";
import Relocate from "./Relocate.tsx";
import CreateRemoteDownload from "./CreateRemoteDownload.tsx";
import AdvanceSearch from "../Search/AdvanceSearch/AdvanceSearch.tsx";
import React from "react";
import ColumnSetting from "../Explorer/ListView/ColumnSetting.tsx";
import DirectLinks from "./DirectLinks/DirectLinks.tsx";
import DirectLinksControl from "./DirectLinksControl.tsx";
import ReportAbuse from "./ReportAbuse.tsx";
import FilePermission from "./FilePermission.tsx";
import FileActivities from "./FileActivities.tsx";
import FilePolicy from "./FilePolicy.tsx";

const Dialogs = () => {
  const showCreateArchive = useAppSelector((state) => state.globalState.createArchiveDialogOpen);
  const showExtractArchive = useAppSelector((state) => state.globalState.extractArchiveDialogOpen);
  const showRelocate = useAppSelector((state) => state.globalState.relocateDialogOpen);
  const showRemoteDownload = useAppSelector((state) => state.globalState.remoteDownloadDialogOpen);
  const showAdvancedSearch = useAppSelector((state) => state.globalState.advanceSearchOpen);
  const showListViewColumnSetting = useAppSelector((state) => state.globalState.listViewColumnSettingDialogOpen);
  const directLink = useAppSelector((state) => state.globalState.directLinkDialogOpen);
  const excalidrawViewer = useAppSelector((state) => state.globalState.excalidrawViewer);
  const directLinkManagement = useAppSelector((state) => state.globalState.directLinkManagementDialogOpen);
  const archivePreview = useAppSelector((state) => state.globalState.archiveViewer);
  const desktopMountSetup = useAppSelector((state) => state.globalState.desktopMountSetupDialogOpen);
  const showPermission = useAppSelector((state) => state.globalState.permissionDialogOpen);
  const showFileActivity = useAppSelector((state) => state.globalState.fileActivityDialogOpen);
  const showFilePolicy = useAppSelector((state) => state.globalState.filePolicyDialogOpen);

  return (
    <>
      <CreateNew />
      <DeleteConfirmation />
      <AggregatedErrorDetail />
      <LockConflictDetails />
      <Rename />
      <PathSelection />
      <Tags />
      <ChangeIcon />
      <ShareDialog />
      <UploadLinkDialog />
      <VersionControl />
      <ManageShares />
      <StaleVersionConfirm />
      <SaveAs />
      <Photopea />
      <OpenWith />
      <Wopi />
      <CodeViewer />
      <DrawIOViewer />
      <MarkdownViewer />
      <VideoViewer />
      <PdfViewer />
      <CustomViewer />
      <EpubViewer />
      {showCreateArchive != undefined && <CreateArchive />}
      {showExtractArchive != undefined && <ExtractArchive />}
      {showRelocate != undefined && <Relocate />}
      {showPermission != undefined && <FilePermission />}
      {showFileActivity != undefined && <FileActivities />}
      {showFilePolicy != undefined && <FilePolicy />}
      {showRemoteDownload != undefined && <CreateRemoteDownload />}
      {showAdvancedSearch != undefined && <AdvanceSearch />}
      {showListViewColumnSetting != undefined && <ColumnSetting />}
      {directLink != undefined && <DirectLinks />}
      {excalidrawViewer != undefined && <ExcalidrawViewer />}
      {directLinkManagement != undefined && <DirectLinksControl />}
      {archivePreview != undefined && <ArchivePreview />}
      {desktopMountSetup != undefined && <DesktopMountSetup />}
      <ReportAbuse />
    </>
  );
};

export default Dialogs;
