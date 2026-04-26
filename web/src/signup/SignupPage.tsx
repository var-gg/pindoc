import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, Loader2, LogIn } from "lucide-react";
import { Link, useLocation } from "react-router";
import { api, type InviteJoinInfo } from "../api/client";
import { useI18n } from "../i18n";
import "../styles/reader.css";

export function SignupPage() {
  const { t, lang } = useI18n();
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const invite = (params.get("invite") ?? "").trim();
  const requestedReturnTo = safeReturnTo(params.get("return_to") ?? "");
  const [info, setInfo] = useState<InviteJoinInfo | null>(null);
  const [loading, setLoading] = useState(Boolean(invite));
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    if (!invite) {
      setLoading(false);
      setInfo(null);
      setErrorCode(null);
      setErrorMessage(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setErrorCode(null);
    setErrorMessage(null);
    api.inviteInfo(invite)
      .then((next) => {
        if (!cancelled) setInfo(next);
      })
      .catch((e) => {
        const err = e as Error & { error_code?: string };
        if (!cancelled) {
          setInfo(null);
          setErrorCode(err.error_code ?? "UNKNOWN");
          setErrorMessage(err.message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [invite]);

  const expiresAt = useMemo(() => {
    if (!info?.expires_at) return "";
    return new Intl.DateTimeFormat(lang === "ko" ? "ko-KR" : "en-US", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(info.expires_at));
  }, [info?.expires_at, lang]);

  const returnTo =
    requestedReturnTo !== "/"
      ? requestedReturnTo
      : info
        ? `/signup/complete?project=${encodeURIComponent(info.project_slug)}`
        : "/";
  const loginHref = invite
    ? `/auth/github/login?${new URLSearchParams({ invite, return_to: returnTo }).toString()}`
    : "";

  return (
    <main className="signup-page">
      <section className="signup-panel" aria-labelledby="signup-title">
        <div className="signup-panel__brand">
          <img src="/design-system/assets/logo.svg" alt="" width={24} height={24} />
          <span>Pindoc</span>
        </div>
        <h1 id="signup-title">{t("signup.title")}</h1>

        {!invite && (
          <SignupNotice
            title={t("signup.missing.title")}
            body={t("signup.missing.body")}
          />
        )}

        {invite && loading && (
          <div className="signup-panel__notice signup-panel__notice--neutral" role="status">
            <Loader2 className="lucide signup-panel__spinner" size={18} aria-hidden="true" />
            <div>
              <strong>{t("signup.loading.title")}</strong>
              <p>{t("signup.loading.body")}</p>
            </div>
          </div>
        )}

        {invite && !loading && errorCode && (
          <SignupNotice
            title={t("signup.invalid.title")}
            body={inviteErrorBody(t, errorCode, errorMessage)}
          />
        )}

        {invite && !loading && info && (
          <>
            <dl className="signup-panel__meta">
              <div>
                <dt>{t("signup.meta.project")}</dt>
                <dd>{info.project_name}</dd>
              </div>
              <div>
                <dt>{t("signup.meta.role")}</dt>
                <dd>{t(`invite.role.${info.role}`)}</dd>
              </div>
              <div>
                <dt>{t("signup.meta.expires")}</dt>
                <dd>{expiresAt}</dd>
              </div>
            </dl>
            <a className="signup-panel__button" href={loginHref}>
              <LogIn size={18} aria-hidden="true" />
              <span>{t("signup.github")}</span>
            </a>
          </>
        )}

        <Link className="signup-panel__secondary" to={info ? `/p/${info.project_slug}/today` : "/"}>
          {t("signup.back")}
        </Link>
      </section>
    </main>
  );
}

function SignupNotice({ title, body }: { title: string; body: string }) {
  return (
    <div className="signup-panel__notice" role="alert">
      <AlertTriangle size={18} aria-hidden="true" />
      <div>
        <strong>{title}</strong>
        <p>{body}</p>
      </div>
    </div>
  );
}

function safeReturnTo(raw: string): string {
  const value = raw.trim();
  if (!value || value.startsWith("//")) return "/";
  if (value.startsWith("/")) return value;
  try {
    const u = new URL(value, window.location.origin);
    if (u.origin !== window.location.origin) return "/";
    return `${u.pathname}${u.search}${u.hash}`;
  } catch {
    return "/";
  }
}

function inviteErrorBody(
  t: (key: string, ...args: Array<string | number>) => string,
  errorCode: string,
  errorMessage: string | null,
): string {
  if (errorCode === "INVITE_TOKEN_INACTIVE") return t("signup.invalid.inactive");
  if (errorCode === "INVITE_TOKEN_NOT_FOUND") return t("signup.invalid.not_found");
  if (errorCode === "INVITE_TOKEN_REQUIRED") return t("signup.missing.body");
  return errorMessage || t("signup.invalid.unknown");
}
