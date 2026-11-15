import { LoadingButton } from "@mui/lab";
import { Box, Grow, styled } from "@mui/material";
import * as React from "react";
import { createContext, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { CSSTransition, SwitchTransition } from "react-transition-group";
import { getSettings, sendSetSetting } from "../../../api/api.ts";
import { useAppDispatch } from "../../../redux/hooks.ts";
import FacebookCircularProgress from "../../Common/CircularProgress.tsx";
import { SecondaryButton } from "../../Common/StyledComponents.tsx";
import ArrowHookUpRight from "../../Icons/ArrowHookUpRight.tsx";
import Save from "../../Icons/Save.tsx";

export interface SettingsWrapperProps {
  settings: string[];
  children: React.ReactNode;
}

export interface SettingContextProps {
  values: {
    [key: string]: string;
  };
  setSettings: (settings: { [key: string]: string }) => void;
  formRef?: React.RefObject<HTMLFormElement>;
}

const SavingFloatContainer = styled(Box)(({ theme }) => ({
  padding: theme.spacing(2),
  border: `1px solid ${theme.palette.divider}`,
  borderRadius: theme.shape.borderRadius,
  marginTop: theme.spacing(2),
  position: "fixed",
  backgroundColor: theme.palette.background.paper,
  bottom: 23,
  zIndex: theme.zIndex.modal,
}));

export interface SavingFloatProps {
  disabled?: boolean;
  in: boolean;
  submitting: boolean;
  revert: () => void;
  submit: () => void;
}

export const SavingFloat = ({ in: inProp, submitting, revert, submit, disabled }: SavingFloatProps) => {
  const { t } = useTranslation("dashboard");
  return (
    <>
      <Box sx={{ height: 70 }} />
      <Grow in={inProp}>
        <SavingFloatContainer>
          <LoadingButton
            loading={submitting}
            onClick={submit}
            variant={"contained"}
            startIcon={<Save />}
            disabled={disabled}
          >
            <span>{t("settings.save")}</span>
          </LoadingButton>
          <SecondaryButton
            disabled={submitting}
            onClick={revert}
            sx={{ ml: 1 }}
            variant={"contained"}
            startIcon={<ArrowHookUpRight />}
          >
            {t("settings.revert")}
          </SecondaryButton>
        </SavingFloatContainer>
      </Grow>
    </>
  );
};

export const SettingContext = createContext<SettingContextProps>({
  values: {},
  setSettings: () => {},
});

const canonicalizeSettings = (settings: { [key: string]: string }): { [key: string]: string } => {
  const res = { ...settings };
  const aliases: [string, string][] = [
    ["score_enabled", "vas_credit_enabled"],
    ["score_price", "vas_credit_per_unit"],
    ["share_score_rate", "vas_share_score_rate"],
    ["anonymous_purchase", "vas_anonymous_purchase"],
    ["shop_nav_enabled", "vas_shop_nav_enabled"],
    ["storage_products", "vas_storage_products"],
    ["group_sell_data", "vas_group_products"],
    ["cron_notify_user", "cron_vas_notify_user"],
    ["cron_ban_user", "cron_vas_ban_user"],
    ["mail_domain_filter", "email_provider_filter_mode"],
    ["mail_domain_filter_list", "email_provider_filter_rules"],
  ];
  aliases.forEach(([key, fallback]) => {
    if (!res[key] && res[fallback]) {
      res[key] = res[fallback];
    }
  });
  return res;
};

const SettingsWrapper = ({ settings, children }: SettingsWrapperProps) => {
  const dispatch = useAppDispatch();
  const { t } = useTranslation("dashboard");
  const [values, setValues] = useState<{ [key: string]: string }>({});
  const [modifiedValues, setModifiedValues] = useState<{
    [key: string]: string;
  }>({});
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const setSettings = (settings: { [key: string]: string }) => {
    setModifiedValues((prev) => ({ ...prev, ...settings }));
  };
  const formRef = useRef<HTMLFormElement>(null);

  const showSaveButton = useMemo(() => {
    return JSON.stringify(modifiedValues) !== JSON.stringify(values);
  }, [modifiedValues, values]);

  useEffect(() => {
    setLoading(true);
    dispatch(
      getSettings({
        keys: settings,
      }),
    )
      .then((res) => {
        const normalized = canonicalizeSettings(res);
        setValues(normalized);
        setModifiedValues(normalized);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [settings]);

  const revert = () => {
    setModifiedValues(values);
  };

  const submit = () => {
    if (formRef.current) {
      if (!formRef.current.checkValidity()) {
        formRef.current.reportValidity();
        return;
      }
    }

    const modified: { [key: string]: string } = {};
    Object.keys(modifiedValues).forEach((key) => {
      if (modifiedValues[key] !== values[key]) {
        modified[key] = modifiedValues[key];
      }
    });

    setSubmitting(true);
    dispatch(
      sendSetSetting({
        settings: modified,
      }),
    )
      .then((res) => {
        setValues((s) => ({ ...s, ...res }));
        setModifiedValues((s) => ({ ...s, ...res }));
      })
      .finally(() => {
        setSubmitting(false);
      });
  };

  return (
    <SettingContext.Provider
      value={{
        values: modifiedValues,
        setSettings,
        formRef,
      }}
    >
      <SwitchTransition>
        <CSSTransition
          addEndListener={(node, done) => node.addEventListener("transitionend", done, false)}
          classNames="fade"
          key={`${loading}`}
        >
          <Box sx={{ mt: 3 }}>
            {loading && (
              <Box
                sx={{
                  pt: 20,
                  height: "100%",
                  display: "flex",
                  justifyContent: "center",
                  alignItems: "center",
                }}
              >
                <FacebookCircularProgress />
              </Box>
            )}
            {!loading && (
              <Box>
                {children}
                <SavingFloat in={showSaveButton} submitting={submitting} revert={revert} submit={submit} />
              </Box>
            )}
          </Box>
        </CSSTransition>
      </SwitchTransition>
    </SettingContext.Provider>
  );
};

export default SettingsWrapper;
