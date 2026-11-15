import { AxiosProgressEvent, CancelToken } from "axios";
import { EncryptedBlob } from "../component/Uploader/core/uploader/encrypt/blob.ts";
import i18n from "../i18n.ts";
import {
  AdminListGroupResponse,
  AdminListService,
  ListShareResponse as AdminListShareResponse,
  ListAbuseReportResponse,
  AbuseReport,
  AuditLog,
  ListAuditLogResponse,
  StoragePolicy as AdminStoragePolicy,
  BatchIDService,
  CleanupTaskService,
  CreateStoragePolicyCorsService,
  Entity,
  FetchWOPIDiscoveryService,
  File as FileEnt,
  FinishOauthCallbackService,
  GetOAuthClientResponse,
  GetOauthRedirectService,
  GetSettingService,
  GroupEnt,
  HomepageSummary,
  ListEntityResponse,
  ListFileResponse,
  ListNodeResponse,
  ListOAuthClientResponse,
  ListStoragePolicyResponse,
  ListTaskResponse,
  ListUserResponse,
  License,
  ManualRefreshLicenseService,
  Node,
  OauthCredentialStatus,
  QueueMetric,
  SetSettingService,
  Share as ShareEnt,
  Task,
  TestNodeDownloaderService,
  TestNodeService,
  TestSMTPService,
  ThumbGeneratorTestService,
  UpsertFileService,
  UpsertGroupService,
  UpsertNodeService,
  UpsertOAuthClientService,
  UpsertStoragePolicyService,
  UpsertUserService,
  User as UserEnt,
  CreditResponse,
  CreditChange,
  ListCreditChangesResponse,
  Payment,
  ListPaymentResponse,
  GiftCode,
  ListGiftCodeResponse,
  CreateGiftCodesService,
  UpdateGiftCodeService,
  ArchiveEventsService,
  CleanupEventsService,
  ArchiveJob,
  StoragePack,
  ListStoragePackResponse,
  PurchaseShareResponse,
  CreatePaymentService,
  ProductType,
} from "./dashboard.ts";
import {
  ArchiveListFilesResponse,
  ArchiveListFilesService,
  CreateFileService,
  CreateViewerSessionService,
  DeleteFileService,
  DeleteFilePermissionService,
  DeleteUploadSessionService,
  DirectLink,
  FileResponse,
  FileThumbResponse,
  FileUpdateService,
  FileURLResponse,
  FileURLService,
  GetFileInfoService,
  ListFileActivityResponse,
  ListFileActivityService,
  ListFileService,
  ListResponse,
  MoveFileService,
  MultipleUriService,
  PatchMetadataService,
  PatchViewSyncService,
  PinFileService,
  RenameFileService,
  SetFilePermissionService,
  Share,
  ShareCreateService,
  UnlockFileService,
  UpdateFilePolicyService,
  UploadCredential,
  UploadSessionRequest,
  VersionControlService,
  ViewerGroup,
  ViewerSessionResponse,
} from "./explorer.ts";
import { AppError, Code, CrHeaders, defaultOpts, isRequestAbortedError, send, ThunkResponse } from "./request.ts";
import { CreateDavAccountService, DavAccount, ListDavAccountsResponse, ListDavAccountsService } from "./setting.ts";
import { ListShareResponse, ListShareService, ShareACL } from "./share.ts";
import { CaptchaResponse, SiteConfig } from "./site.ts";
import {
  AppRegistration,
  Capacity,
  FinishPasskeyLoginService,
  FinishPasskeyRegistrationService,
  GrantResponse,
  GrantService,
  LoginResponse,
  Passkey,
  PasskeyCredentialOption,
  PasswordLoginRequest,
  PatchUserSetting,
  PrepareLoginResponse,
  PreparePasskeyLoginResponse,
  RefreshTokenRequest,
  ResetPasswordService,
  SendResetEmailService,
  SignUpService,
  SSOStatus,
  Token,
  TwoFALoginRequest,
  User,
  UserStoragePolicy,
  UserSettings,
} from "./user.ts";
import {
  ArchiveWorkflowService,
  DownloadWorkflowService,
  ImportWorkflowService,
  ListTaskService,
  NodeSummary,
  RebuildFTSIndexWorkflowService,
  RelocateWorkflowService,
  SetDownloadFilesService,
  TaskListResponse,
  TaskProgresses,
  TaskResponse,
} from "./workflow.ts";

export function getSiteConfig(section: string): ThunkResponse<SiteConfig> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/site/config/" + section,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
          bypassSnackbar: (e) => isRequestAbortedError(e),
          errorSnackbarMsg: (e) => i18n.t("errLoadingSiteConfig", { ns: "common" }) + e.message,
        },
      ),
    );
  };
}

export function sendPrepareLogin(email: string): ThunkResponse<PrepareLoginResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/session/prepare",
        {
          params: {
            email: email,
          },
          method: "GET",
        },
        {
          ...defaultOpts,
          noCredential: true,
          bypassSnackbar: (e) => e instanceof AppError && e.code == Code.NodeFound,
        },
      ),
    );
  };
}

export function startOIDCLogin(redirect?: string): ThunkResponse<{ url: string }> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/session/sso/oidc",
        {
          method: "POST",
          data: { redirect },
        },
        {
          ...defaultOpts,
          noCredential: true,
        },
      ),
    );
  };
}

// startProOpenID starts any configured provider by its Pro provider id.
export function startProOpenID(
  provider: number,
  linking = false,
  hint?: string,
): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/session/openid",
        {
          method: "PUT",
          data: { provider, linking, hint },
        },
        {
          ...defaultOpts,
          noCredential: !linking,
        },
      ),
    );
  };
}

export function exchangeOIDCTicket(ticket: string): ThunkResponse<LoginResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/session/sso/oidc/exchange",
        {
          method: "POST",
          data: { ticket },
        },
        {
          ...defaultOpts,
          noCredential: true,
        },
      ),
    );
  };
}

export function exchangeProOpenID(
  providerID: number,
  code: string,
  sessionID: string,
): ThunkResponse<LoginResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/session/openid",
        {
          method: "POST",
          data: { provider_id: providerID, code, session_id: sessionID },
        },
        {
          ...defaultOpts,
          noCredential: true,
          bypassSnackbar: () => true,
        },
      ),
    );
  };
}

export function getSSOStatus(): ThunkResponse<SSOStatus> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send("/user/sso/oidc", { method: "GET" }, { ...defaultOpts }),
    );
  };
}

export function startOIDCLink(redirect?: string): ThunkResponse<{ url: string }> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/user/sso/oidc/link",
        {
          method: "POST",
          data: { redirect },
        },
        { ...defaultOpts },
      ),
    );
  };
}

export function unlinkOIDC(): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send("/user/sso/oidc", { method: "DELETE" }, { ...defaultOpts }),
    );
  };
}

export function unlinkProOpenID(id: number): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(`/session/openid/${id}`, { method: "DELETE" }, { ...defaultOpts }),
    );
  };
}

export function getCaptcha(): ThunkResponse<CaptchaResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/site/captcha",
        {
          method: "GET",
        },
        {
          ...defaultOpts,
          noCredential: true,
          errorSnackbarMsg: (e) => i18n.t("login.captchaError", { ns: "application" }) + e.message,
        },
      ),
    );
  };
}

export function sendLogin(req: PasswordLoginRequest): ThunkResponse<LoginResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/session/token",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          noCredential: true,
          bypassSnackbar: (e) => e instanceof AppError && e.code == Code.Continue,
        },
      ),
    );
  };
}

export function send2FALogin(req: TwoFALoginRequest): ThunkResponse<LoginResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/session/token/2fa",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          noCredential: true,
        },
      ),
    );
  };
}

export function getUserMe(): ThunkResponse<User> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/user/me",
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendRefreshToken(req: RefreshTokenRequest): ThunkResponse<Token> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/session/token/refresh",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          bypassSnackbar: (_e) => true,
          noCredential: true,
        },
      ),
    );
  };
}

export function sendSignout(req: RefreshTokenRequest): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/session/token",
        {
          data: req,
          method: "DELETE",
        },
        {
          ...defaultOpts,
          noCredential: true,
        },
      ),
    );
  };
}

export function getFileList(req: ListFileService, skipSnackbar = true): ThunkResponse<ListResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file",
        {
          params: req,
          method: "GET",
        },
        {
          ...defaultOpts,
          withPurchaseTicket: true,
          bypassSnackbar: (_e) => skipSnackbar,
        },
      ),
    );
  };
}

export function getFileThumb(path: string, contextHint?: string): ThunkResponse<FileThumbResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/thumb",
        {
          params: { uri: path },
          method: "GET",
          headers: contextHint
            ? {
                [CrHeaders.context_hint]: contextHint,
              }
            : {},
        },
        {
          ...defaultOpts,
          withPurchaseTicket: true,
          bypassSnackbar: (_e) => true,
        },
      ),
    );
  };
}

export function getUserInfo(uid: string): ThunkResponse<User> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/user/info/" + uid,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
          bypassSnackbar: (_e) => true,
        },
      ),
    );
  };
}

export function getUserCapacity(): ThunkResponse<Capacity> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/user/capacity",
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendDeleteFiles(req: DeleteFileService): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file",
        {
          data: req,
          method: "DELETE",
        },
        {
          ...defaultOpts,
          withPurchaseTicket: true,
          skipBatchError: req.uris.length == 1,
        },
      ),
    );
  };
}

export function sendEmptyTrash(): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/trash",
        {
          method: "DELETE",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendUnlockFiles(req: UnlockFileService): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/lock",
        {
          data: req,
          method: "DELETE",
        },
        {
          ...defaultOpts,
          skipLockConflict: true,
        },
      ),
    );
  };
}

export function sendRenameFile(req: RenameFileService): ThunkResponse<FileResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/rename",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          bypassSnackbar: (e) => isRequestAbortedError(e),
        },
      ),
    );
  };
}

export function sendPinFile(req: PinFileService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/pin",
        {
          data: req,
          method: "PUT",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendUnpinFile(req: PinFileService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/pin",
        {
          data: req,
          method: "DELETE",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendMoveFile(req: MoveFileService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/move",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          skipBatchError: req.uris.length == 1,
        },
      ),
    );
  };
}

export function sendRestoreFile(req: DeleteFileService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/restore",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          skipBatchError: req.uris.length == 1,
        },
      ),
    );
  };
}

export function sendMetadataPatch(req: PatchMetadataService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/metadata",
        {
          data: req,
          method: "PATCH",
        },
        {
          ...defaultOpts,
          skipBatchError: req.uris.length == 1,
        },
      ),
    );
  };
}

export function getSearchUser(keyword: string): ThunkResponse<User[]> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/user/search?keyword=" + encodeURIComponent(keyword),
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendCreateShare(req: ShareCreateService): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/share",
        {
          data: req,
          method: "PUT",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendUpdateShare(req: ShareCreateService, id: string): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/share/" + id,
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendDeleteShare(id: string): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/share/" + id,
        {
          method: "DELETE",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getShareACL(id: string): ThunkResponse<ShareACL> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/share/${id}/acl`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function updateShareACL(id: string, acl: ShareACL): ThunkResponse<ShareACL> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/share/${id}/acl`,
        {
          method: "PUT",
          data: { acl },
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getShareInfo(
  id: string,
  password?: string,
  count_views?: boolean,
  owner_extended?: boolean,
): ThunkResponse<Share> {
  return async (dispatch, _getState) => {
    let uri = "/share/info/" + id;
    const query = new URLSearchParams();
    if (password && password != "") {
      query.set("password", password);
    }
    if (count_views) {
      query.set("count_views", "true");
    }
    if (owner_extended) {
      query.set("owner_extended", "true");
    }
    if (query.toString() != "") {
      uri += "?" + query.toString();
    }
    return await dispatch(
      send(
        uri,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
          bypassSnackbar: (_e) => true,
        },
      ),
    );
  };
}

export function sendCreateFile(req: CreateFileService): ThunkResponse<FileResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/create",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getFileEntityUrl(req: FileURLService): ThunkResponse<FileURLResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/url",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          skipBatchError: req.uris.length == 1,
        },
      ),
    );
  };
}

export function getFileInfo(req: GetFileInfoService, skipError = false): ThunkResponse<FileResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/info",
        {
          method: "GET",
          params: req,
        },
        {
          ...defaultOpts,
          withPurchaseTicket: true,
          bypassSnackbar: () => skipError,
        },
      ),
    );
  };
}

export function setFilePermission(req: SetFilePermissionService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/permission",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          skipBatchError: req.uris.length == 1,
        },
      ),
    );
  };
}

export function deleteFilePermission(req: DeleteFilePermissionService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/permission",
        {
          data: req,
          method: "DELETE",
        },
        {
          ...defaultOpts,
          skipBatchError: req.uris.length == 1,
        },
      ),
    );
  };
}

export function setCurrentVersion(req: VersionControlService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/version/current",
        {
          method: "POST",
          data: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function deleteVersion(req: VersionControlService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/version",
        {
          method: "DELETE",
          data: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendUpdateFile(req: FileUpdateService, data: any): ThunkResponse<FileResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/content",
        {
          data,
          params: req,
          method: "PUT",
          headers: {
            "Content-Type": "application/octet-stream",
          },
        },
        {
          bypassSnackbar: (e) => e instanceof AppError && e.code == Code.StaleVersion,
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendCreateViewerSession(req: CreateViewerSessionService): ThunkResponse<ViewerSessionResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/viewerSession",
        {
          data: req,
          method: "PUT",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendCreateUploadSession(req: UploadSessionRequest): ThunkResponse<UploadCredential> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/upload",
        {
          data: req,
          method: "PUT",
        },
        {
          ...defaultOpts,
          bypassSnackbar: (_e) => true,
        },
      ),
    );
  };
}

export function sendUploadChunk(
  sessionID: string,
  chunk: Blob,
  index: number,
  cancel?: CancelToken,
  onProgress?: (progressEvent: AxiosProgressEvent) => void,
): ThunkResponse<UploadCredential> {
  return async (dispatch, _getState) => {
    const streaming = chunk instanceof EncryptedBlob;
    return await dispatch(
      send(
        `/file/upload/${sessionID}/${index}`,
        {
          adapter: streaming ? "fetch" : "xhr",
          data: streaming ? chunk.stream() : chunk,
          cancelToken: cancel,
          onUploadProgress: onProgress,
          method: "POST",
          headers: {
            "Content-Type": "application/octet-stream",
            ...(streaming && { "X-Expected-Entity-Length": chunk.size?.toString() ?? "0" }),
          },
        },
        {
          ...defaultOpts,
          bypassSnackbar: (_e) => true,
        },
      ),
    );
  };
}

export function sendDeleteUploadSession(req: DeleteUploadSessionService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/file/upload`,
        {
          data: req,
          method: "DELETE",
        },
        {
          ...defaultOpts,
          bypassSnackbar: (_e) => true,
        },
      ),
    );
  };
}

export function sendS3LikeCompleteUpload(policyType: string, sessionId: string, sessionKey: string): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/callback/${policyType}/${sessionId}/${sessionKey}`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
          bypassSnackbar: (_e) => true,
        },
      ),
    );
  };
}

export function sendOneDriveCompleteUpload(sessionId: string, sessionKey: string): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/callback/onedrive/${sessionId}/${sessionKey}`,
        {
          method: "POST",
        },
        {
          ...defaultOpts,
          bypassSnackbar: (_e) => true,
        },
      ),
    );
  };
}

export function sendCreateArchive(req: ArchiveWorkflowService): ThunkResponse<TaskResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow/archive",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendExtractArchive(req: ArchiveWorkflowService): ThunkResponse<TaskResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow/extract",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendRelocateWorkflow(req: RelocateWorkflowService): ThunkResponse<TaskResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow/relocate",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getUserStoragePolicies(): ThunkResponse<UserStoragePolicy[]> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/user/setting/policies",
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getAvailableNodes(): ThunkResponse<NodeSummary[]> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/user/setting/nodes",
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getTasks(req: ListTaskService): ThunkResponse<TaskListResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow",
        {
          params: req,
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getTasksPhaseProgress(id: string): ThunkResponse<TaskProgresses> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow/progress/" + id,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendCreateRemoteDownload(req: DownloadWorkflowService): ThunkResponse<TaskResponse[]> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow/download",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          skipBatchError: (req.src?.length ?? 0) <= 1,
        },
      ),
    );
  };
}

export function sendSetDownloadTarget(id: string, req: SetDownloadFilesService): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow/download/" + id,
        {
          data: req,
          method: "PATCH",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendCancelDownloadTask(id: string): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow/download/" + id,
        {
          method: "DELETE",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getShares(req: ListShareService): ThunkResponse<ListShareResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/share",
        {
          method: "GET",
          params: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getDavAccounts(req: ListDavAccountsService): ThunkResponse<ListDavAccountsResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/devices/dav",
        {
          method: "GET",
          params: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendCreateDavAccounts(req: CreateDavAccountService): ThunkResponse<DavAccount> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/devices/dav",
        {
          method: "PUT",
          data: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendUpdateDavAccounts(id: string, req: CreateDavAccountService): ThunkResponse<DavAccount> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/devices/dav/${id}`,
        {
          method: "PATCH",
          data: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendDeleteDavAccount(id: string): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/devices/dav/${id}`,
        {
          method: "DELETE",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getFileDirectLinks(req: MultipleUriService): ThunkResponse<DirectLink[]> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/file/source",
        {
          data: req,
          method: "PUT",
        },
        {
          ...defaultOpts,
          skipBatchError: req.uris.length == 1,
          acceptBatchPartialSuccess: true,
        },
      ),
    );
  };
}

export function sendDeleteDirectLink(id: string): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(send(`/file/source/${id}`, { method: "DELETE" }, { ...defaultOpts }));
  };
}

export function getUserShares(req: ListShareService, uid: string): ThunkResponse<ListShareResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/shares/${uid}`,
        {
          method: "GET",
          params: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getUserSettings(): ThunkResponse<UserSettings> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/setting`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendUploadAvatar(avatar?: Blob, contentType?: string): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/setting/avatar`,
        {
          method: "PUT",
          data: avatar,
          headers: contentType
            ? {
                "Content-Type": contentType,
              }
            : undefined,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendUpdateUserSetting(settings: PatchUserSetting): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/setting`,
        {
          method: "PATCH",
          data: settings,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function get2FAInitSecret(): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/setting/2fa`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendPreparePasskeyRegistration(): ThunkResponse<PasskeyCredentialOption> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/authn`,
        {
          method: "PUT",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendFinishPasskeyRegistration(req: FinishPasskeyRegistrationService): ThunkResponse<Passkey> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/authn`,
        {
          method: "POST",
          data: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendDeletePasskey(id: string): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/authn?id=${encodeURIComponent(id)}`,
        {
          method: "DELETE",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendFinishPasskeyLogin(req: FinishPasskeyLoginService): ThunkResponse<LoginResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/session/authn`,
        {
          method: "POST",
          data: req,
        },
        {
          ...defaultOpts,
          noCredential: true,
        },
      ),
    );
  };
}

export function sendPreparePasskeyLogin(): ThunkResponse<PreparePasskeyLoginResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/session/authn`,
        {
          method: "PUT",
        },
        {
          ...defaultOpts,
          noCredential: true,
        },
      ),
    );
  };
}

export function sendSinUp(req: SignUpService): ThunkResponse<User> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/user",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
          noCredential: true,
          bypassSnackbar: (e) => e instanceof AppError && e.code == Code.Continue,
        },
      ),
    );
  };
}

export function sendEmailActivate(id: string, sign: string): ThunkResponse<User> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/activate/${id}?sign=${encodeURIComponent(sign)}`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
          noCredential: true,
        },
      ),
    );
  };
}

export function sendResetEmail(req: SendResetEmailService): ThunkResponse<User> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/reset`,
        {
          method: "POST",
          data: req,
        },
        {
          ...defaultOpts,
          noCredential: true,
        },
      ),
    );
  };
}

export function sendReset(uid: string, req: ResetPasswordService): ThunkResponse<User> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/reset/${uid}`,
        {
          method: "PATCH",
          data: req,
        },
        {
          ...defaultOpts,
          noCredential: true,
        },
      ),
    );
  };
}

export function getDashboardSummary(generateCharts?: boolean): ThunkResponse<HomepageSummary> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/summary?generate=${!!generateCharts}`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function refreshLicenseAuto(): ThunkResponse<License> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/license/refreshAuto`,
        {
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function refreshLicenseManual(req: ManualRefreshLicenseService): ThunkResponse<License> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/license/refreshManual`,
        {
          method: "POST",
          data: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getSettings(keys: GetSettingService): ThunkResponse<{
  [key: string]: string;
}> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/settings`,
        {
          method: "POST",
          data: keys,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendSetSetting(keys: SetSettingService): ThunkResponse<{
  [key: string]: string;
}> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/settings`,
        {
          method: "PATCH",
          data: keys,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getGroupList(args: AdminListService): ThunkResponse<AdminListGroupResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/group`,
        {
          method: "POST",
          data: args,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getWopiDiscovery(args: FetchWOPIDiscoveryService): ThunkResponse<ViewerGroup> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/tool/wopi`,
        {
          method: "GET",
          params: args,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendTestThumbGeneratorExecutable(args: ThumbGeneratorTestService): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/tool/thumbExecutable`,
        {
          method: "POST",
          data: args,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendTestSMTP(args: TestSMTPService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/tool/mail`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getQueueMetrics(): ThunkResponse<QueueMetric[]> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/queue/metrics`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getStoragePolicyList(args: AdminListService): ThunkResponse<ListStoragePolicyResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getStoragePolicyDetail(id: number, countEntity?: boolean): ThunkResponse<AdminStoragePolicy> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/${id}`,
        { method: "GET", params: { countEntity: countEntity ? true : undefined } },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function upsertStoragePolicy(args: UpsertStoragePolicyService): ThunkResponse<AdminStoragePolicy> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy${args.policy.id ? `/${args.policy.id}` : ""}`,
        { method: "PUT", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getNodeList(args: AdminListService): ThunkResponse<ListNodeResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/node`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getNodeDetail(id: number): ThunkResponse<Node> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/node/${id}`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function upsertNode(args: UpsertNodeService): ThunkResponse<Node> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/node${args.node.id ? `/${args.node.id}` : ""}`,
        {
          method: "PUT",
          data: args,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendClearBlobUrlCache(): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/tool/entityUrlCache`,
        { method: "DELETE" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function createStoragePolicyCors(args: CreateStoragePolicyCorsService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/cors`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getPolicyOauthRedirectUrl(): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/oauth/redirect`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getPolicyOauthCredentialRefreshTime(id: string): ThunkResponse<OauthCredentialStatus> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/oauth/status/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getPolicyOauthUrl(args: GetOauthRedirectService): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/oauth/signin`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function finishOauthCallback(args: FinishOauthCallbackService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/oauth/callback`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getOneDriveDriverRoot(id: number, url: string): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/oauth/root/${id}`,
        { method: "GET", params: { url } },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function deleteStoragePolicy(id: number): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/${id}`,
        { method: "DELETE" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getGroupDetail(id: number, countUser?: boolean): ThunkResponse<GroupEnt> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/group/${id}`,
        { method: "GET", params: { countUser: countUser ? true : undefined } },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function upsertGroup(args: UpsertGroupService): ThunkResponse<GroupEnt> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/group${args.group.id ? `/${args.group.id}` : ""}`,
        { method: "PUT", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function deleteGroup(id: number): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/group/${id}`,
        { method: "DELETE" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export interface GroupPolicyResponse {
  group_id: number;
  policy_ids: number[];
}

export function getGroupPolicies(id: number): ThunkResponse<GroupPolicyResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/group/${id}/policies`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function updateGroupPolicies(id: number, policyIDs: number[]): ThunkResponse<GroupPolicyResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/group/${id}/policies`,
        { method: "PUT", data: { policy_ids: policyIDs } },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export interface LoadBalanceTarget {
  target_policy_id: number;
  weight: number;
}

export interface LoadBalanceResponse {
  policy_id: number;
  targets: LoadBalanceTarget[];
}

export function getLoadBalanceTargets(id: number): ThunkResponse<LoadBalanceResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/${id}/loadBalance`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function updateLoadBalanceTargets(id: number, targets: LoadBalanceTarget[]): ThunkResponse<LoadBalanceResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/policy/${id}/loadBalance`,
        { method: "PUT", data: { targets } },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function deleteNode(id: number): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/node/${id}`,
        { method: "DELETE" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function testNode(args: TestNodeService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/node/test`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function testNodeDownloader(args: TestNodeDownloaderService): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/node/test/downloader`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getUserList(args: AdminListService): ThunkResponse<ListUserResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/user`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getUserDetail(id: number): ThunkResponse<UserEnt> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/user/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function upsertUser(args: UpsertUserService): ThunkResponse<UserEnt> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/user${args.user.id ? `/${args.user.id}` : ""}`,
        { method: "PUT", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function batchDeleteUser(args: BatchIDService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/user/batch/delete`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
          skipBatchError: args.ids.length === 1,
        },
      ),
    );
  };
}

export function getFlattenFileList(args: AdminListService): ThunkResponse<ListFileResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/file`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getFileDetail(id: number): ThunkResponse<FileEnt> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/file/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function upsertFile(args: UpsertFileService): ThunkResponse<FileEnt> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/file${args.file.id ? `/${args.file.id}` : ""}`,
        { method: "PUT", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getFileUrl(id: number): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/file/url/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function batchDeleteFiles(args: BatchIDService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/file/batch/delete`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getEntityList(args: AdminListService): ThunkResponse<ListEntityResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/entity`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getEntityDetail(id: number): ThunkResponse<Entity> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/entity/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getEntityUrl(id: number): ThunkResponse<string> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/entity/url/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function batchDeleteEntities(args: BatchIDService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/entity/batch/delete`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getTaskList(args: AdminListService): ThunkResponse<ListTaskResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/queue`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getTaskDetail(id: number): ThunkResponse<Task> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/queue/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function batchDeleteTasks(args: BatchIDService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/queue/batch/delete`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getShareList(args: AdminListService): ThunkResponse<AdminListShareResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/share`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getShareDetail(id: number): ThunkResponse<ShareEnt> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/share/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function batchDeleteShares(args: BatchIDService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/share/batch/delete`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getAbuseReportList(args: AdminListService): ThunkResponse<ListAbuseReportResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/abuse`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getAbuseReportDetail(id: number): ThunkResponse<AbuseReport> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/abuse/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function batchDeleteAbuseReports(args: BatchIDService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/abuse/batch/delete`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getAuditLogList(args: AdminListService): ThunkResponse<ListAuditLogResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/event`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getAuditLogDetail(id: number): ThunkResponse<AuditLog> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/event/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function archiveAuditLogs(args: ArchiveEventsService): ThunkResponse<ArchiveJob> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/admin/event/archive",
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getArchiveJobStatus(id: string): ThunkResponse<ArchiveJob> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/event/archive/${id}/status`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function deleteArchiveJob(id: string): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/event/archive/${id}`,
        { method: "DELETE" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function cleanupAuditLogs(args: CleanupEventsService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/admin/event/cleanup",
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function batchDeleteAuditLogs(ids: number[]): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/admin/event/batch/delete",
        { method: "POST", data: { ids } },
        {
          ...defaultOpts,
          skipBatchError: ids.length === 1,
        },
      ),
    );
  };
}

export function sendAbuseReport(
  shareID: string,
  args: { category: number; description?: string; file_uri?: string; password?: string },
): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/share/report/${shareID}`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendSiteAbuseReport(args: {
  category: number;
  description?: string;
  file_uri?: string;
  share_id?: string;
  user_id?: string;
  password?: string;
  captcha?: string;
  ticket?: string;
  randstr?: string;
}): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/site/abuse",
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendCalibrateUserStorage(id: number): ThunkResponse<UserEnt> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/user/${id}/calibrate`,
        { method: "POST" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendImport(req: ImportWorkflowService): ThunkResponse<TaskResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow/import",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendPatchViewSync(args: PatchViewSyncService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/file/view`,
        { method: "PATCH", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendCleanupTask(args: CleanupTaskService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/queue/cleanup`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getArchiveListFiles(args: ArchiveListFilesService): ThunkResponse<ArchiveListFilesResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/file/archive`,
        { method: "GET", params: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getOauthAppRegistration(app_id: string): ThunkResponse<AppRegistration> {
  return async (dispatch, _getState) => {
    return await dispatch(send(`/session/oauth/app/${app_id}`, { method: "GET" }, { ...defaultOpts }));
  };
}

export function sendConsentOauthApp(args: GrantService, deny = false): ThunkResponse<GrantResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/session/oauth/consent${deny ? "/deny" : ""}`,
        { method: "POST", data: args },
        { bypassSnackbar: (_e) => true, ...defaultOpts },
      ),
    );
  };
}

export function getOAuthClientList(args: AdminListService): ThunkResponse<ListOAuthClientResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/oauthClient`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getOAuthClientDetail(id: number): ThunkResponse<GetOAuthClientResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/oauthClient/${id}`,
        { method: "GET" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function upsertOAuthClient(args: UpsertOAuthClientService): ThunkResponse<GetOAuthClientResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/oauthClient${args.client.id ? `/${args.client.id}` : ""}`,
        { method: "PUT", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function deleteOAuthClient(id: number): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/oauthClient/${id}`,
        { method: "DELETE" },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function batchDeleteOAuthClients(args: BatchIDService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/oauthClient/batch/delete`,
        { method: "POST", data: args },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendRevokeOAuthGrant(grant_id: string): ThunkResponse {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/session/oauth/grant/${grant_id}`,
        {
          method: "DELETE",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendFullTextSearch(query: string, offset?: number): ThunkResponse {
  const params = new URLSearchParams();
  params.set("query", query);
  if (offset) {
    params.set("offset", offset.toString());
  }
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/file/search`,
        {
          method: "GET",
          params,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function sendRebuildFTSIndex(req: RebuildFTSIndexWorkflowService): ThunkResponse<TaskResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        "/workflow/rebuildFtsIndex",
        {
          data: req,
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getCredit(): ThunkResponse<CreditResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/credit`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getCreditChanges(page: number, pageSize: number): ThunkResponse<ListCreditChangesResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/creditChanges`,
        {
          method: "GET",
          params: { page, page_size: pageSize },
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getPayments(
  page: number,
  pageSize: number,
  nextPageToken?: string,
): ThunkResponse<ListPaymentResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/payments`,
        {
          method: "GET",
          params: { page, page_size: pageSize, next_page_token: nextPageToken },
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getStoragePacks(): ThunkResponse<ListStoragePackResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/user/storagePacks`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function createPayment(req: CreatePaymentService): ThunkResponse<Payment> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/vas/payment`,
        {
          method: "PUT",
          data: req,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getPaymentStatus(tradeNo: string, sign: string): ThunkResponse<Payment> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/vas/payment/status/${tradeNo}/${sign}`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getGiftCode(code: string): ThunkResponse<GiftCode> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/vas/giftcode/${code}`,
        {
          method: "GET",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function redeemGiftCode(code: string): ThunkResponse<GiftCode> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/vas/giftcode/${code}/redeem`,
        {
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function adminListGiftCodes(args: AdminListService): ThunkResponse<ListGiftCodeResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/giftCodes`,
        {
          method: "POST",
          data: args,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function adminCreateGiftCodes(args: CreateGiftCodesService): ThunkResponse<GiftCode[]> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/giftCodes`,
        {
          method: "PUT",
          data: args,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function adminUpdateGiftCode(args: UpdateGiftCodeService): ThunkResponse<GiftCode> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/giftCodes`,
        {
          method: "PATCH",
          data: args,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function getFileActivities(args: ListFileActivityService): ThunkResponse<ListFileActivityResponse> {
  return async (dispatch, _getState) => {
    const params = Object.fromEntries(
      Object.entries(args).filter(([, value]) => value !== undefined && value !== ""),
    );
    return await dispatch(
      send(
        `/file/activities`,
        {
          method: "GET",
          params,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function updateFilePolicy(args: UpdateFilePolicyService): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/file/policy`,
        {
          method: "PATCH",
          data: args,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function adminDeleteGiftCode(id: number): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/giftCodes/${id}`,
        {
          method: "DELETE",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function purchaseShare(shareID: string): ThunkResponse<PurchaseShareResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/share/${shareID}/purchase`,
        {
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function adminListPayments(args: AdminListService): ThunkResponse<ListPaymentResponse> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/payment`,
        {
          method: "POST",
          data: args,
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function adminFulfillPayment(id: number): ThunkResponse<Payment> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/payment/${id}/fulfill`,
        {
          method: "POST",
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}

export function adminBatchDeletePayments(ids: number[]): ThunkResponse<void> {
  return async (dispatch, _getState) => {
    return await dispatch(
      send(
        `/admin/payment/batch/delete`,
        {
          method: "POST",
          data: { ids },
        },
        {
          ...defaultOpts,
        },
      ),
    );
  };
}
