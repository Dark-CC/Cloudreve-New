import { User } from "./user.ts";
import { PaginationResults, Share } from "./explorer.ts";

export interface ShareInfo {
  id: string;
  name?: string;
  source_type?: number;
  remain_downloads?: number;
  visited?: number;
  downloaded?: number;
  expires?: string;
  created_at?: string;
  unlocked: boolean;
  owner: User;
  expired?: boolean;
}

export interface ListShareService {
  page_size: number;
  order_by?: string;
  order_direction?: string;
  next_page_token?: string;
}

export interface ListShareResponse {
  shares: Share[];
  pagination: PaginationResults;
}

export interface SharePermission {
  can_list: boolean;
  can_read: boolean;
  can_update: boolean;
  can_create: boolean;
  can_delete: boolean;
  can_share: boolean;
  discoverable: boolean;
}

export interface BuiltinSharePermission {
  id: number;
  share_id: number;
  audience: "anonymous" | "logged_in" | "same_group" | "other_group";
  permission: SharePermission;
}

export interface GroupSharePermission {
  id: number;
  share_id: number;
  group_id: number;
  permission: SharePermission;
}

export interface UserSharePermission {
  id: number;
  share_id: number;
  user_id: number;
  user_hash_id?: string;
  permission: SharePermission;
}

export interface ShareACL {
  builtin: BuiltinSharePermission[];
  group: GroupSharePermission[];
  user: UserSharePermission[];
}
