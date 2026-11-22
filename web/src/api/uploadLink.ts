import { defaultOpts, requestOpts, send } from "./request";
import { store } from "../redux/store";

// Public upload endpoints never require a login. Suppressing credentials also
// keeps a stale local session from triggering the global "session expired"
// sign-out flow while a visitor is uploading.
//
// Kept as a function: request.ts and api.ts form a cycle, so reading
// `defaultOpts` while this module is being evaluated can hit its TDZ and blank
// the whole app.
const publicOpts = (): requestOpts => ({
  ...defaultOpts,
  noCredential: true,
});

export interface UploadLinkInfo {
  owner: string;
  expires_at: number;
  max_size: number;
  note?: string;
}

export interface UploadLinkCreateRequest {
  uri: string;
  policy_id?: string;
  expires?: number;
  max_size?: number;
  note?: string;
}

export interface UploadLinkCreateResponse {
  token: string;
  url: string;
  expires: number;
  expires_at: number;
  max_size?: number;
  note?: string;
}

export async function getUploadLinkInfo(token: string): Promise<UploadLinkInfo> {
  return await store.dispatch(
    send<UploadLinkInfo>(`/upload-link/${encodeURIComponent(token)}`, undefined, publicOpts()),
  );
}

export async function createUploadLink(req: UploadLinkCreateRequest): Promise<UploadLinkCreateResponse> {
  return await store.dispatch(
    send<UploadLinkCreateResponse>(`/file/upload-link`, {
      method: "POST",
      data: req,
    }),
  );
}

export interface PublicUploadSession {
  session_id: string;
  chunk_size: number;
  upload_urls?: string[];
  edge_ticket?: string;
  callback_secret: string;
}

export async function createPublicUploadSession(token: string, req: { name: string; size: number; last_modified?: number; mime_type?: string }): Promise<PublicUploadSession> {
  return await store.dispatch(
    send<PublicUploadSession>(
      `/upload-link/${encodeURIComponent(token)}/session`,
      {
        method: "POST",
        data: { token, ...req },
      },
      publicOpts(),
    ),
  );
}

export async function completePublicUpload(token: string, sessionID: string, secret: string): Promise<void> {
  await store.dispatch(
    send<void>(
      `/upload-link/${encodeURIComponent(token)}/complete/${sessionID}/${secret}`,
      { method: "POST" },
      publicOpts(),
    ),
  );
}

export interface PublicUploadRefresh {
  edge_ticket: string;
  expires: number;
}

/**
 * refreshPublicUpload re-issues the sealed upstream ticket after the edge
 * reports that the embedded 1yunpan session has expired.
 */
export async function refreshPublicUpload(
  token: string,
  sessionID: string,
  secret: string,
): Promise<PublicUploadRefresh> {
  return await store.dispatch(
    send<PublicUploadRefresh>(
      `/upload-link/${encodeURIComponent(token)}/refresh/${sessionID}/${secret}`,
      { method: "POST" },
      publicOpts(),
    ),
  );
}
