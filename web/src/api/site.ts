import { CustomProps, ViewerGroup } from "./explorer.ts";
import { User } from "./user.ts";

export enum CaptchaType {
  NORMAL = "normal",
  RECAPTCHA = "recaptcha",
  // Deprecated
  TCAPTCHA = "tcaptcha",
  TURNSTILE = "turnstile",
  CAP = "cap",
}

export interface SiteConfig {
  instance_id?: string;
  title?: string;
  site_notice?: string;
  login_captcha?: boolean;
  reg_captcha?: boolean;
  forget_captcha?: boolean;
  abuse_report_captcha?: boolean;
  themes?: string;
  default_theme?: string;
  authn?: boolean;
  user?: User;
  captcha_ReCaptchaKey?: string;
  captcha_type?: CaptchaType;
  turnstile_site_id?: string;
  captcha_cap_instance_url?: string;
  captcha_cap_site_key?: string;
  captcha_cap_secret_key?: string;
  captcha_cap_asset_server?: string;
  register_enabled?: boolean;
  sso_enabled?: boolean;
  sso_name?: string;
  login_providers?: LoginProvider[];
  logo?: string;
  logo_light?: string;
  tos_url?: string;
  privacy_policy_url?: string;
  icons?: string;
  emoji_preset?: string;
  map_provider?: string;
  mapbox_ak?: string;
  google_map_tile_type?: string;
  file_viewers?: ViewerGroup[];
  default_viewer_mapping?: {
    [ext: string]: string;
  };
  max_batch_size?: number;
  app_promotion?: boolean;
  desktop_app_promotion?: boolean;
  app_feedback?: string;
  app_forum?: string;
  shared_with_me_acl?: boolean;
  thumbnail_width?: number;
  thumbnail_height?: number;
  custom_props?: CustomProps[];
  custom_nav_items?: CustomNavItem[];
  custom_html?: CustomHTML;
  thumb_exts?: string[];
  show_encryption_status?: boolean;
  full_text_search?: boolean;
  shop_nav_enabled?: boolean;
  point_enabled?: boolean;
  point_price?: number;
  share_point_gain_rate?: number;
  payment?: PaymentSetting;
  anonymous_purchase?: boolean;
  storage_products?: StorageProduct[];
  group_skus?: GroupProduct[];
}

export interface LoginProvider {
  id: number;
  type: "qq" | "logto" | "oidc" | string;
  name: string;
  icon?: string;
  enabled: boolean;
  direct_sign_in?: boolean;
}

export interface PaymentSetting {
  currency_code: string;
  currency_mark: string;
  currency_unit: number;
  providers: PaymentProvider[];
  local_enabled?: boolean;
  local_payment?: boolean;
}

export interface PaymentProvider {
  id: string;
  name: string;
  type?: string;
  endpoint?: string;
  secret_key?: string;
  secret?: string;
  app_id?: string;
  public_key?: string;
  merchant_id?: string;
  certificate_serial?: string;
  api_private_key?: string;
}

export interface StorageProduct {
  id: string;
  name: string;
  size: number;
  price: number;
  duration: number;
  time?: number;
  points?: number;
  chip?: string;
  highlight?: boolean;
}

export interface GroupProduct {
  id: string;
  name: string;
  group: number;
  group_id?: number;
  price: number;
  duration: number;
  time?: number;
  points?: number;
  chip?: string;
  des?: string[];
  description?: string;
  highlight?: boolean;
}

export interface CaptchaResponse {
  ticket: string;
  image: string;
}

export interface CustomNavItem {
  name: string;
  url: string;
  icon: string;
}

export interface CustomHTML {
  headless_footer?: string;
  headless_bottom?: string;
  sidebar_bottom?: string;
}
