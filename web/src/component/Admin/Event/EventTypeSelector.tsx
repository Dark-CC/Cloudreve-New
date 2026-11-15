import { Box, Chip, ListItemText, MenuItem, Select, SelectChangeEvent, Typography } from "@mui/material";
import { useTranslation } from "react-i18next";
import { AuditLogType } from "../../../api/explorer";

export interface EventGroup {
  title: string;
  description: string;
  events: number[];
}

export const eventCategories: EventGroup[] = [
  {
    title: "settings.systemEvents",
    description: "settings.systemEventsDes",
    events: [AuditLogType.server_start],
  },
  {
    title: "settings.userEvents",
    description: "settings.userEventsDes",
    events: [
      AuditLogType.user_signup,
      AuditLogType.user_activated,
      AuditLogType.user_login,
      AuditLogType.user_login_failed,
      AuditLogType.user_token_refresh,
      AuditLogType.user_changed,
      AuditLogType.user_exceed_quota_notified,
      AuditLogType.change_nick,
      AuditLogType.change_avatar,
      AuditLogType.change_password,
      AuditLogType.enable_2fa,
      AuditLogType.disable_2fa,
      AuditLogType.add_passkey,
      AuditLogType.remove_passkey,
      AuditLogType.link_account,
      AuditLogType.unlink_account,
      AuditLogType.report_abuse,
      AuditLogType.oauth_grant_create,
      AuditLogType.oauth_token_exchange,
      AuditLogType.oauth_grant_revoke,
    ],
  },
  {
    title: "settings.fileEvents",
    description: "settings.fileEventsDes",
    events: [
      AuditLogType.file_create,
      AuditLogType.file_imported,
      AuditLogType.file_rename,
      AuditLogType.set_file_permission,
      AuditLogType.entity_uploaded,
      AuditLogType.entity_downloaded,
      AuditLogType.copy_from,
      AuditLogType.copy_to,
      AuditLogType.move_to,
      AuditLogType.delete_file,
      AuditLogType.move_to_trash,
      AuditLogType.update_metadata,
      AuditLogType.get_direct_link,
      AuditLogType.delete_direct_link,
      AuditLogType.update_view,
    ],
  },
  {
    title: "settings.shareEvents",
    description: "settings.shareEventsDes",
    events: [AuditLogType.share, AuditLogType.share_link_viewed, AuditLogType.edit_share, AuditLogType.delete_share],
  },
  {
    title: "settings.versionEvents",
    description: "settings.versionEventsDes",
    events: [AuditLogType.set_current_version, AuditLogType.delete_version],
  },
  {
    title: "settings.mediaEvents",
    description: "settings.mediaEventsDes",
    events: [AuditLogType.thumb_generated, AuditLogType.live_photo_uploaded],
  },
  {
    title: "settings.filesystemEvents",
    description: "settings.filesystemEventsDes",
    events: [
      AuditLogType.mount,
      AuditLogType.relocate,
      AuditLogType.create_archive,
      AuditLogType.extract_archive,
    ],
  },
  {
    title: "settings.webdavEvents",
    description: "settings.webdavEventsDes",
    events: [
      AuditLogType.webdav_login_failed,
      AuditLogType.webdav_account_create,
      AuditLogType.webdav_account_update,
      AuditLogType.webdav_account_delete,
    ],
  },
  {
    title: "settings.paymentEvents",
    description: "settings.paymentEventsDes",
    events: [
      AuditLogType.payment_created,
      AuditLogType.points_change,
      AuditLogType.payment_paid,
      AuditLogType.payment_fulfilled,
      AuditLogType.payment_fulfill_failed,
      AuditLogType.storage_added,
      AuditLogType.group_changed,
      AuditLogType.membership_unsubscribe,
      AuditLogType.redeem_gift_code,
    ],
  },
  {
    title: "settings.emailEvents",
    description: "settings.emailEventsDes",
    events: [AuditLogType.email_sent],
  },
];

export const eventCategoryIndex = (type: number): number => {
  const index = eventCategories.findIndex((category) => category.events.includes(type));
  return index >= 0 ? index : eventCategories.length;
};

export const eventName = (type: number): string => {
  const entry = Object.entries(AuditLogType).find(([, value]) => value === type);
  return entry?.[0] ?? `event_${type}`;
};

export const getEventCategoryName = (type: number): string => {
  const index = eventCategoryIndex(type);
  return index < eventCategories.length ? eventCategories[index].title : "event.unknownCategory";
};

export interface EventCategorySelectorProps {
  value: number[];
  onChange: (value: number[]) => void;
  helperText?: string;
  showAllOption?: boolean;
  displayEmpty?: boolean;
}

const EventCategorySelector = ({
  value,
  onChange,
  helperText,
  showAllOption = false,
  displayEmpty = false,
}: EventCategorySelectorProps) => {
  const { t } = useTranslation("dashboard");

  return (
    <Box>
      <Select
        multiple
        fullWidth
        value={value}
        onChange={(event: SelectChangeEvent<number[]>) => onChange(event.target.value as number[])}
        displayEmpty={displayEmpty}
        renderValue={(selected) => (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
            {(selected as number[]).map((type) => (
              <Chip key={type} size="small" label={t(`settings.event.${eventName(type)}`)} />
            ))}
          </Box>
        )}
        MenuProps={{ PaperProps: { sx: { maxHeight: 400 } } }}
      >
        {showAllOption && (
          <MenuItem value={[]} disabled>
            <ListItemText
              primary={t("event.allEventTypes")}
              slotProps={{ primary: { variant: "body2", fontStyle: "italic" } }}
            />
          </MenuItem>
        )}
        {eventCategories.map((category) => [
          <MenuItem key={category.title} disabled sx={{ fontWeight: 600, px: "12px" }}>
            <Typography variant="body2" color="primary" fontWeight={600}>
              {t(category.title)}
            </Typography>
          </MenuItem>,
          ...category.events.map((type) => (
            <MenuItem key={type} value={type}>
              <ListItemText
                primary={t(`settings.event.${eventName(type)}`)}
                slotProps={{ primary: { variant: "body2" } }}
              />
            </MenuItem>
          )),
        ])}
      </Select>
      {helperText && (
        <Box component="span" sx={{ display: "block", mt: 0.5, color: "text.secondary", fontSize: "0.75rem" }}>
          {helperText}
        </Box>
      )}
    </Box>
  );
};

export default EventCategorySelector;
