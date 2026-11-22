import { Button, Stack } from "@mui/material";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { StoragePolicy } from "../../../../../api/dashboard";
import { PolicyType } from "../../../../../api/explorer";
import { DenseFilledTextField } from "../../../../Common/StyledComponents";
import SettingForm from "../../../../Pages/Setting/SettingForm";
import { NoMarginHelperText } from "../../../Settings/Settings";
import { AddWizardProps } from "../../AddWizardDialog";

// API / download / upload endpoints are fixed by 1yunpan and resolved on the
// server, so the wizard only asks for the account. The edge upload settings are
// site-wide (Admin -> Settings -> Server) and are not part of a storage policy.
const OneYunPanWizard = ({ onSubmit }: AddWizardProps) => {
  const { t } = useTranslation("dashboard");
  const formRef = useRef<HTMLFormElement>(null);
  const [policy, setPolicy] = useState<StoragePolicy>({
    id: 0,
    node_id: 0,
    name: "",
    type: PolicyType.oneyunpan,
    dir_name_rule: "cloudreve/{uid}/{path}",
    file_name_rule: "{uuid}_{originname}",
    settings: {
      relay: true,
      chunk_size: 25 << 20,
      chunk_concurrency: 1,
      thumb_generator_proxy: true,
      media_meta_generator_proxy: true,
    },
    edges: {},
  });

  const handleSubmit = () => {
    if (!formRef.current?.checkValidity()) {
      formRef.current?.reportValidity();
      return;
    }
    onSubmit(policy);
  };

  return (
    <form ref={formRef} onSubmit={handleSubmit}>
      <Stack spacing={2}>
        <SettingForm title={t("policy.name")} lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            required
            value={policy.name}
            onChange={(e) => setPolicy({ ...policy, name: e.target.value })}
          />
          <NoMarginHelperText>{t("policy.policyName")}</NoMarginHelperText>
        </SettingForm>
        <SettingForm title={t("policy.oneyunpanAccount")} lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            required
            value={policy.access_key}
            onChange={(e) => setPolicy({ ...policy, access_key: e.target.value })}
          />
          <NoMarginHelperText>{t("policy.oneyunpanCredentialDes")}</NoMarginHelperText>
        </SettingForm>
        <SettingForm title={t("policy.oneyunpanPassword")} lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            required
            type="password"
            value={policy.secret_key}
            onChange={(e) => setPolicy({ ...policy, secret_key: e.target.value })}
          />
          <NoMarginHelperText>{t("policy.oneyunpanAutoConfig")}</NoMarginHelperText>
        </SettingForm>
      </Stack>
      <Button variant="contained" color="primary" sx={{ mt: 2 }} onClick={handleSubmit}>
        {t("policy.create")}
      </Button>
    </form>
  );
};

export default OneYunPanWizard;
