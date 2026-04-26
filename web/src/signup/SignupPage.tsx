import { AlertTriangle, LogIn } from "lucide-react";
import { Link, useLocation } from "react-router";
import "../styles/reader.css";

export function SignupPage() {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const invite = (params.get("invite") ?? "").trim();
  const returnTo = safeReturnTo(params.get("return_to") ?? "/");
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
        <h1 id="signup-title">Join workspace</h1>
        {invite ? (
          <>
            <dl className="signup-panel__meta">
              <div>
                <dt>Invite</dt>
                <dd>{inviteSummary(invite)}</dd>
              </div>
            </dl>
            <a className="signup-panel__button" href={loginHref}>
              <LogIn size={18} aria-hidden="true" />
              <span>Sign in with GitHub</span>
            </a>
          </>
        ) : (
          <div className="signup-panel__notice" role="alert">
            <AlertTriangle size={18} aria-hidden="true" />
            <div>
              <strong>Invite required</strong>
              <p>Open the signup link from your workspace invite.</p>
            </div>
          </div>
        )}
        <Link className="signup-panel__secondary" to="/">
          Back to Reader
        </Link>
      </section>
    </main>
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

function inviteSummary(invite: string): string {
  if (invite.length <= 14) return invite;
  return `${invite.slice(0, 6)}...${invite.slice(-6)}`;
}
