export function maskEmail(email: string): string {
  const trimmed = email.trim();
  if (!trimmed) return "";
  const at = trimmed.indexOf("@");
  if (at < 0) {
    return `${trimmed.slice(0, Math.min(2, trimmed.length))}***`;
  }
  const local = trimmed.slice(0, at);
  const domain = trimmed.slice(at + 1);
  const visible = local.length <= 1 ? local.slice(0, 1) : local.slice(0, 2);
  return `${visible || "*"}***@${domain}`;
}
