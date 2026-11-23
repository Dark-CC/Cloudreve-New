import axios, { AxiosResponse } from "axios";
import { UploadCredential } from "../../../../api/explorer";
import { removeResumeCtx } from "../utils/helper";
import { requestAPI } from "../utils/request";
import { Status } from "./base";
import Chunk, { ChunkInfo, ChunkProgress } from "./chunk";

interface OneyunpanChunkResponse {
  status?: boolean;
  message?: string;
  next?: string;
}

interface RefreshTicketResponse {
  edge_ticket: string;
}

const FIRST_PART_SIZE = 1;
const MAX_REFRESH_ATTEMPTS = 3;
const CALLBACK_RETRIES = 4;
const MAX_PART_ATTEMPTS = 3;
const MAX_CHAIN_RESTARTS = 2;

class SessionExpiredError extends Error {}

/**
 * The chain token can only move forward, so a part whose response is lost
 * leaves us without the token the upstream expects next. Replaying with the
 * stale token is rejected upstream, and there is no way to resume, so the only
 * safe recovery is to start a brand new chain from byte zero.
 */
class ChainRestartError extends Error {}

/**
 * Browser-side uploader for the 1yunpan edge route.
 *
 * 1yunpan has an unusual chunk protocol:
 *   - the first part must be exactly one byte,
 *   - every later part is chained with the `X-Next-Token` returned before it,
 *   - the final part carries the target path and file metadata.
 *
 * The chain is strictly sequential, so chunks never run in parallel. The edge
 * only decrypts the sealed ticket and forwards bytes, which means the account
 * never leaves Cloudreve. If the session inside the ticket expires, the browser
 * asks Cloudreve for a re-sealed ticket and restarts the chain from byte zero.
 */
export default class Oneyunpan extends Chunk {
  private nextToken = "";
  private plan: number[] = [];

  protected getConcurrency(): number {
    return 1;
  }

  public upload = async (): Promise<any> => {
    // The chain token cannot survive a page reload, so a cached session would
    // silently resume at the wrong offset. Always start a fresh chain.
    removeResumeCtx(this.task, this.logger);

    let refreshes = 0;
    let restarts = 0;
    for (;;) {
      try {
        await this.streamFile();
        return;
      } catch (e) {
        if (e instanceof SessionExpiredError) {
          if (refreshes >= MAX_REFRESH_ATTEMPTS) {
            throw e;
          }
          refreshes++;
          this.logger.warn(`1yunpan session expired, refreshing edge ticket (${refreshes}/${MAX_REFRESH_ATTEMPTS})`);
          await this.refreshTicket();
          continue;
        }

        if (e instanceof ChainRestartError) {
          if (restarts >= MAX_CHAIN_RESTARTS) {
            throw e;
          }
          restarts++;
          this.logger.warn(
            `1yunpan chunk chain was interrupted, restarting from the first byte (${restarts}/${MAX_CHAIN_RESTARTS})`,
          );
          // Restarting with the same ticket replays the chain inside the same
          // upstream session, which the edge accepts (the first part reopens
          // the chain). Asking for a new ticket would instead leave the
          // abandoned partial object behind upstream.
          continue;
        }

        throw e;
      }
    }
  };

  private edgeEndpoint(): string {
    const edge = this.task.session?.upload_urls?.[0];
    if (!edge) {
      throw new Error("1yunpan edge upload session is missing its endpoint");
    }
    return edge;
  }

  private buildPlan(): number[] {
    const total = this.task.blob.size;
    if (total < 2) {
      throw new Error("1yunpan requires files to be at least 2 bytes");
    }

    // Every part is a separate request that can only be sent after the
    // previous one returned its chain token, so the part count directly
    // multiplies the odds of hitting a transient edge reset. Large files use
    // the biggest part the edge accepts to keep that count (and the total
    // time) down; small files are unaffected because the last part is
    // clamped to the remaining bytes.
    const totalMiB = total / (1024 * 1024);
    const preferred = totalMiB >= 512 ? 64 : totalMiB >= 128 ? 32 : 16;
    const requested = Math.floor((this.task.session?.chunk_size || 0) / (1024 * 1024));
    const chunkMiB = Math.max(preferred, Math.min(88, requested));
    const chunkSize = Math.max(1, Math.min(total, chunkMiB * 1024 * 1024));

    const plan: number[] = [FIRST_PART_SIZE];
    let remaining = total - FIRST_PART_SIZE;
    while (remaining > 0) {
      const size = Math.min(chunkSize, remaining);
      plan.push(size);
      remaining -= size;
    }
    return plan;
  }

  private resetProgress(plan: number[]) {
    this.plan = plan;
    this.nextToken = "";
    this.task.chunkProgress = plan.map(
      (_, index): ChunkProgress => ({
        loaded: 0,
        index,
      }),
    );
    this.publishProgress(0);
  }

  private publishProgress(loaded: number) {
    const total = this.task.blob.size;
    this.progress = {
      total: {
        size: total,
        loaded,
        percent: total === 0 ? 100 : Math.min(100, (loaded / total) * 100),
      },
      chunks: this.task.chunkProgress.map((item, index) => {
        const size = this.plan[index] ?? 0;
        return {
          size,
          loaded: item.loaded,
          percent: size === 0 ? 100 : (item.loaded / size) * 100,
        };
      }),
    };
    this.subscriber.onProgress(this.progress);
  }

  private async streamFile() {
    const edge = this.edgeEndpoint();
    const plan = this.buildPlan();
    this.resetProgress(plan);

    let offset = 0;
    for (let index = 0; index < plan.length; index++) {
      const end = offset + plan[index];
      const isFinal = index === plan.length - 1;
      await this.sendPart(edge, this.task.blob.slice(offset, end), offset, end, isFinal, index);
      offset = end;
    }
  }

  private async sendPart(edge: string, blob: Blob, start: number, end: number, isFinal: boolean, index: number) {
    const ticket = this.task.session?.edge_ticket || this.task.session?.credential;
    if (!ticket) {
      throw new Error("1yunpan edge upload session is missing its ticket");
    }

    const chainToken = this.nextToken;
    let res: AxiosResponse<OneyunpanChunkResponse> | undefined;

    for (let attempt = 1; ; attempt++) {
      const headers: Record<string, string> = {
        "content-type": "application/octet-stream",
        "content-range": `${start}-${end - 1}`,
        "x-cr-ticket": ticket,
      };
      if (chainToken) {
        headers["x-next-token"] = chainToken;
      }

      try {
        res = await axios.request<OneyunpanChunkResponse>({
          url: edge,
          method: "post",
          headers,
          data: blob,
          onUploadProgress: (e) => {
            const loaded = Math.min(e.loaded, blob.size);
            this.task.chunkProgress[index].loaded = loaded;
            this.publishProgress(start + loaded);
          },
          // `cancelToken.token` is an axios CancelToken, not an AbortSignal.
          // Passing it as `signal` made axios call `addEventListener` on it.
          cancelToken: this.cancelToken.token,
          validateStatus: () => true,
        });
      } catch (e: any) {
        if (this.cancelToken.token.reason) {
          throw e;
        }
        // No response at all: the edge or Cloudflare dropped the connection.
        // A dropped part leaves the chain where it was, so replaying the same
        // bytes with the same token is safe (verified against the real edge).
        if (attempt >= MAX_PART_ATTEMPTS) {
          throw new ChainRestartError(`1yunpan edge dropped the connection: ${e?.message ?? e}`);
        }
        this.logger.warn(
          `1yunpan part ${index} lost its connection, retrying (${attempt}/${MAX_PART_ATTEMPTS})`,
          e?.message,
        );
        await new Promise((resolve) => setTimeout(resolve, 800 * attempt));
        continue;
      }

      if (res.status === 401 || res.status === 403 || res.headers?.["x-cr-session-expired"] === "1") {
        throw new SessionExpiredError("1yunpan session expired");
      }

      if (res.status >= 500) {
        // A 5xx is an upstream hiccup on a part that has not been consumed;
        // the chain is still positioned here, so the same request can simply
        // be replayed. Only give up and restart the whole chain once the
        // retries are exhausted, because a fresh chain is expensive.
        if (attempt < MAX_PART_ATTEMPTS) {
          this.logger.warn(
            `1yunpan part ${index} got a transient upstream error, retrying (${attempt}/${MAX_PART_ATTEMPTS})`,
            res.status,
          );
          await new Promise((resolve) => setTimeout(resolve, 1000 * attempt));
          continue;
        }
        throw new ChainRestartError(`1yunpan upstream kept failing this part (status ${res.status})`);
      }

      if (res.status < 200 || res.status >= 300) {
        const message = typeof res.data === "string" ? res.data : res.data?.message;
        // 4xx means the edge refused the chain itself (expired/rotated token,
        // ticket mismatch). Nothing can be resumed from here.
        throw new ChainRestartError(message || `1yunpan edge rejected the chain (status ${res.status})`);
      }

      break;
    }

    if (!res) {
      throw new ChainRestartError("1yunpan edge returned no response");
    }
    if (res.data?.status === false) {
      throw new Error(res.data?.message || "1yunpan edge upload failed");
    }

    this.task.chunkProgress[index].loaded = blob.size;
    this.publishProgress(end);

    if (!isFinal) {
      this.nextToken = res.data?.next || "";
      if (!this.nextToken) {
        throw new Error("1yunpan edge did not return the next chunk token");
      }
    }
  }

  private async refreshTicket() {
    const session = this.task.session!;
    const res = await requestAPI<RefreshTicketResponse>(
      `/callback/oneyunpan/${session.session_id}/${session.callback_secret}/refresh`,
      { method: "POST" },
    );
    const ticket = res.data?.data?.edge_ticket;
    if (!ticket) {
      throw new Error("Failed to refresh the 1yunpan upload ticket");
    }
    const refreshed = this.task.session as UploadCredential;
    refreshed.edge_ticket = ticket;
    refreshed.credential = ticket;
  }

  protected async afterUpload(): Promise<any> {
    const session = this.task.session!;
    this.transit(Status.finishing);

    let lastError: unknown;
    for (let attempt = 0; attempt < CALLBACK_RETRIES; attempt++) {
      try {
        await requestAPI(`/callback/oneyunpan/${session.session_id}/${session.callback_secret}`, {
          method: "POST",
        });
        return;
      } catch (e) {
        lastError = e;
        await new Promise((resolve) => setTimeout(resolve, 500 * (attempt + 1)));
      }
    }
    throw lastError;
  }

  /** Chunking is fully handled in `upload`; the base pool never calls this. */
  protected async uploadChunk(_chunkInfo: ChunkInfo): Promise<any> {
    return undefined;
  }
}
