import { Button, Divider, FormControl, Link, Stack, Typography } from "@mui/material";
import { Icon } from "@iconify/react";
import { useTranslation } from "react-i18next";
import { LoginResponse, PrepareLoginResponse } from "../../../../api/user.ts";
import { startProOpenID } from "../../../../api/api.ts";
import { useAppDispatch, useAppSelector } from "../../../../redux/hooks.ts";
import { Captcha, CaptchaParams } from "../../../Common/Captcha/Captcha.tsx";
import { OutlineIconTextField } from "../../../Common/Form/OutlineIconTextField.tsx";
import Password from "../../../Icons/Password.tsx";
import QQ from "../../../Icons/QQ.tsx";
import SSO from "../../../Icons/SSO.tsx";
import PasskeyLoginButton from "../Signin/PasskeyLoginButton.tsx";
import { Control } from "../Signin/SignIn.tsx";

interface PhaseCollectPasswordProps {
  pwd: string;
  email: string;
  setPwd: (pwd: string) => void;
  control?: Control;
  loginOptions?: PrepareLoginResponse;
  captchaGen: number;
  setCaptchaState: (state: CaptchaParams) => void;
  onForget?: () => void;
  onOAuthPasskeyLogin?: (response: LoginResponse) => void;
}

const PhaseCollectPassword = ({
  pwd,
  email,
  setPwd,
  control,
  loginOptions,
  captchaGen,
  setCaptchaState,
  onForget,
  onOAuthPasskeyLogin,
}: PhaseCollectPasswordProps) => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const { login_captcha, authn, login_providers } = useAppSelector((state) => state.siteConfig.login.config);
  const providers = (login_providers ?? []).filter(
    (provider) => provider.enabled && loginOptions?.openid_providers?.includes(provider.id),
  );

  const moreOptions = (loginOptions?.webauthn_enabled && authn) || providers.length > 0;
  return (
    <>
      {loginOptions?.password_enabled && (
        <>
          <Typography color={"text.secondary"}>{t("login.enterPasswordHint", { email: email })}</Typography>
          <FormControl variant="standard" margin="normal" required fullWidth>
            <OutlineIconTextField
              autoFocus={true}
              variant={"outlined"}
              label={t("login.password")}
              inputProps={{
                name: "password",
                type: "password",
                id: "password",
                required: "true",
                maxLength: 128,
                minLength: 4,
              }}
              onChange={(e) => setPwd(e.target.value)}
              icon={<Password />}
              value={pwd}
              autoComplete={"true"}
            />
            <Link sx={{ mt: 1 }} underline="hover" variant="body2" href={"#"} onClick={onForget}>
              {t("login.forgetPassword")}
            </Link>
          </FormControl>
          {login_captcha && (
            <FormControl variant="standard" margin="normal" required fullWidth>
              <Captcha generation={captchaGen} required={true} fullWidth={true} onStateChange={setCaptchaState} />
            </FormControl>
          )}
          {control?.submit}
          {moreOptions && (
            <Divider sx={{ my: 2 }} role="presentation">
              <Typography variant="body2" color={"text.secondary"}>
                {t("login.or")}
              </Typography>
            </Divider>
          )}
        </>
      )}
      {!loginOptions?.password_enabled && (
        <Typography color={"text.secondary"} sx={{ mb: 2 }}>
          {t("login.paswordlessHint", { email: email })}
        </Typography>
      )}
      <Stack spacing={1}>
        {providers.map((provider) => (
          <Button
            key={`${provider.type}-${provider.id}`}
            variant="outlined"
            fullWidth
            startIcon={provider.icon ? <Icon icon={provider.icon} /> : provider.type === "qq" ? <QQ /> : <SSO />}
            onClick={() =>
              dispatch(startProOpenID(provider.id)).then((url) => {
                window.location.href = url;
              })
            }
          >
            {t("vas.loginWith", { name: t(provider.name) })}
          </Button>
        ))}
        {loginOptions?.webauthn_enabled && authn && <PasskeyLoginButton onLoginSuccess={onOAuthPasskeyLogin} />}
      </Stack>
      {control?.back}
    </>
  );
};

export default PhaseCollectPassword;
