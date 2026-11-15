import { Box, IconButton, Stack, Typography } from "@mui/material";
import { useContext, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getLoadBalanceTargets, getStoragePolicyList, updateLoadBalanceTargets } from "../../../../api/api";
import { StoragePolicy } from "../../../../api/dashboard";
import { useAppDispatch } from "../../../../redux/hooks";
import { DenseFilledTextField, SecondaryButton } from "../../../Common/StyledComponents";
import Delete from "../../../Icons/Delete";
import SettingForm from "../../../Pages/Setting/SettingForm";
import { NoMarginHelperText, SettingSection, SettingSectionContent } from "../../Settings/Settings";
import { StoragePolicySettingContext } from "./StoragePolicySettingWrapper";
import SinglePolicySelectionInput from "../../Common/SinglePolicySelectionInput";

interface LoadBalanceTarget {
  target_policy_id: number;
  weight: number;
}

const LoadBalanceSection = () => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const { values } = useContext(StoragePolicySettingContext);
  const [policies, setPolicies] = useState<StoragePolicy[]>([]);
  const [targets, setTargets] = useState<LoadBalanceTarget[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!values.id || values.type !== "load_balance") {
      return;
    }
    setLoading(true);
    Promise.all([
      dispatch(getStoragePolicyList({ page: 1, page_size: 1000, order_by: "id", order_direction: "asc" })),
      dispatch(getLoadBalanceTargets(values.id)),
    ])
      .then(([policyRes, targetRes]) => {
        setPolicies(policyRes.policies);
        setTargets(
          (targetRes.targets ?? []).map((target) => ({
            target_policy_id: target.target_policy_id,
            weight: target.weight,
          })),
        );
      })
      .finally(() => setLoading(false));
  }, [values.id, values.type]);

  const availablePolicies = useMemo(() => {
    return policies.filter((policy) => policy.id !== values.id && policy.type !== "load_balance");
  }, [policies, values.id]);

  const updateTarget = (index: number, patch: Partial<LoadBalanceTarget>) => {
    setTargets((prev) => prev.map((target, i) => (i === index ? { ...target, ...patch } : target)));
  };

  const save = () => {
    dispatch(updateLoadBalanceTargets(values.id, targets)).then((res) => {
      setTargets(res.targets ?? []);
    });
  };

  return (
    <SettingSection>
      <Typography variant="h6" gutterBottom>
        {t("policy.load_balance")}
      </Typography>
      <SettingSectionContent>
        <SettingForm lgWidth={10}>
          <Stack spacing={1.5}>
            {targets.map((target, index) => (
              <Box key={index} sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                <Box sx={{ flexGrow: 1 }}>
                  <SinglePolicySelectionInput
                    value={target.target_policy_id}
                    onChange={(value) => updateTarget(index, { target_policy_id: value })}
                  />
                </Box>
                <Box sx={{ width: 120 }}>
                  <DenseFilledTextField
                    fullWidth
                    type="number"
                    label={t("policy.weight")}
                    value={target.weight}
                    onChange={(e) => updateTarget(index, { weight: parseInt(e.target.value) || 1 })}
                  />
                </Box>
                <IconButton onClick={() => setTargets((prev) => prev.filter((_, i) => i !== index))}>
                  <Delete fontSize="small" />
                </IconButton>
              </Box>
            ))}
            <Box sx={{ display: "flex", gap: 1 }}>
              <SecondaryButton
                variant="outlined"
                onClick={() =>
                  setTargets((prev) => [...prev, { target_policy_id: availablePolicies[0]?.id ?? 0, weight: 1 }])
                }
                disabled={loading || availablePolicies.length === 0}
              >
                {t("policy.addTargetPolicy")}
              </SecondaryButton>
              <SecondaryButton variant="contained" onClick={save} disabled={loading}>
                {t("policy.save")}
              </SecondaryButton>
            </Box>
            <NoMarginHelperText>{t("policy.loadBalanceDes")}</NoMarginHelperText>
          </Stack>
        </SettingForm>
      </SettingSectionContent>
    </SettingSection>
  );
};

export default LoadBalanceSection;
