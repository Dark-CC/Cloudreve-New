import { PolicyType, StoragePolicy } from "../../../../api/explorer";
import UploadManager from "../index";
import { Task } from "../types";
import Base from "./base";

/**
 * Uploader for a load balance storage policy.
 *
 * A load balance policy has no storage of its own: the server picks one of its
 * weighted targets while creating the upload session and reports the concrete
 * policy back on the credential. So the session is always created here first,
 * and the task is then handed over to the uploader that matches the resolved
 * policy.
 */
export default class LoadBalance extends Base {
  constructor(task: Task, manager: UploadManager) {
    super(task, manager);
  }

  public run = async () => {
    if (!(await this.prepare())) {
      return;
    }

    const resolved = this.task.session?.storage_policy;
    if (!resolved || resolved.type === PolicyType.load_balance) {
      this.setError(new Error("Storage policy could not be resolved"));
      return;
    }

    const task: Task = {
      ...this.task,
      policy: {
        ...this.task.policy,
        ...resolved,
        relay: resolved.relay ?? this.task.policy.relay,
      } as StoragePolicy,
    };

    const uploader = this.manager.dispatchUploader(task);
    uploader.subscribe({
      onTransition: (status) => this.transit(status),
      onError: (err) => {
        this.status = uploader.status;
        this.error = err;
        this.subscriber.onError(err);
      },
      onProgress: (progress) => {
        this.progress = progress;
        this.subscriber.onProgress(progress);
      },
      onMsg: (msg, color) => this.subscriber.onMsg(msg, color),
    });
    uploader.promiseId = this.promiseId;
    this.promiseId = undefined;

    await uploader.runPrepared();
  };

  public upload = async () => {
    return;
  };
}
