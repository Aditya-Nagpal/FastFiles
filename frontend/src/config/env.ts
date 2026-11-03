const gatewayUrl = import.meta.env.VITE_API_GATEWAY_URL?.trim();

if (!gatewayUrl) {
  throw new Error('Missing VITE_API_GATEWAY_URL. Check your .env.local file.');
}

const configuredCooldown = Number(
  import.meta.env.VITE_RESEND_OTP_COOLDOWN_SECONDS,
);

export const appConfig = {
  apiGatewayUrl: gatewayUrl.replace(/\/+$/, ''),
  resendOtpCooldownSeconds:
    Number.isFinite(configuredCooldown) && configuredCooldown > 0
      ? configuredCooldown
      : 30,
};