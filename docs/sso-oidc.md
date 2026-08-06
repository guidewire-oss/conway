# SSO with OpenID Connect (OIDC)

Conway can delegate **staff** sign-in (admin / facilitator / manager) to an
external OIDC identity provider — Okta, Google Workspace, Entra ID, Auth0, or
any spec-compliant provider. Roles are derived from a group claim in the token
(RBAC), and an account is created **just-in-time** on first login for anyone who
maps to a recognized role.

Teams still **join a game by code** — that flow is unchanged and never touches
SSO. The built-in `admin` password account is retained as a **break-glass**
fallback, so a misconfigured IdP can't lock you out.

## How it works

```
Browser ──▶ /api/oidc/start ──▶ (Authorization Code + PKCE) ──▶ IdP login
   ▲                                                               │
   │                                                               ▼
   └──── /#sso=<session-token> ◀── /api/oidc/callback ◀── redirect w/ code
```

1. The login overlay shows **Sign in with SSO** when OIDC is configured.
2. `/api/oidc/start` mints `state` + `nonce` + a PKCE (S256) pair, stores the
   flow server-side (10-min TTL, single-use), and returns the provider's
   authorize URL.
3. After the user authenticates at the IdP, the browser returns to
   `/api/oidc/callback` with a code. The server:
   - validates `state` (CSRF), exchanges the code (with the PKCE verifier),
   - **verifies the ID token**: RS256 signature against the provider JWKS, plus
     `iss` / `aud` / `exp` (60s skew) / `nonce`,
   - reads the groups claim and maps it to Conway roles,
   - **denies** the login if no group maps to a role (no account is created),
   - otherwise **JIT-provisions** a passwordless account (roles re-synced from
     the IdP on every login) and mints Conway's own 12h HMAC session token.
4. The token is delivered in the URL **fragment** (`/#sso=…`), so it never hits
   server logs or `Referer`. `auth.js` reads it, stores it, and strips it.

Downstream nothing changes: the minted token is the same one the password path
issues, so `withAuth` and every role gate work identically.

## Roles (RBAC)

| Conway role | Grants |
|-------------|--------|
| `admin` | Superuser — passes every gate; manages users & roles |
| `facilitator` | Run games (board, rounds, leaderboard, game CRUD) |
| `manager` | Plan pillar (plans, Jira import, snapshots, rosters) |

`player` is **not** an SSO role — players are teams that join by code.

The IdP owns group membership; `CONWAY_OIDC_ROLE_MAP` decides what each group
means in Conway. A user with several matching groups gets all the mapped roles.

## Configuration

Set these environment variables on the `conway` service:

| Variable | Required | Meaning |
|----------|----------|---------|
| `CONWAY_PUBLIC_URL` | yes | Externally reachable base URL. Redirect URI is `<it>/api/oidc/callback`. |
| `CONWAY_OIDC_ISSUER` | yes | Issuer URL; discovery is `<issuer>/.well-known/openid-configuration`. |
| `CONWAY_OIDC_CLIENT_ID` | yes | OAuth client ID registered with the IdP. |
| `CONWAY_OIDC_CLIENT_SECRET` | no | **Leave unset.** Only for providers that force a confidential client — PKCE is the default and recommended auth method (see below). |
| `CONWAY_OIDC_ROLE_MAP` | yes | `group=role,group=role,…` e.g. `conway-admins=admin,conway-facils=facilitator`. Group names match **case-insensitively**. Empty ⇒ SSO stays disabled. |
| `CONWAY_OIDC_GROUPS_CLAIM` | no | Token claim holding groups. Default `groups`. |
| `CONWAY_OIDC_SCOPES` | no | Space-separated scopes. Default `openid profile email groups`. |

SSO turns on only when `CONWAY_OIDC_ISSUER`, `CONWAY_OIDC_CLIENT_ID`, and a
non-empty `CONWAY_OIDC_ROLE_MAP` are all present **and** discovery succeeds at
boot. Otherwise Conway logs a warning and runs with password login only.

### Client authentication: PKCE, not a client secret

Conway authenticates the authorization-code exchange with **PKCE** (Proof Key
for Code Exchange, S256) — a fresh, single-use verifier per login. This is the
current recommendation for browser-driven web apps: there is no long-lived
client secret to store, rotate, or leak.

**Register Conway as a public client** (no client authentication) and enable
PKCE — the default for SPA/native app types, and a checkbox ("Proof Key for
Code Exchange" / "Require PKCE", "None" for token endpoint auth) on web app
types. Then leave `CONWAY_OIDC_CLIENT_SECRET` unset.

PKCE is **always** sent, whether or not a secret is configured, so the boot log
reports the active mode:

```
OIDC SSO configured (issuer …, redirect …, public client, PKCE only)
```

Set `CONWAY_OIDC_CLIENT_SECRET` only if your provider cannot issue a public
client for this redirect (some Google "Web application" clients, for instance,
always require a secret). PKCE still applies on top — the exchange then sends
both.

### Register the redirect URI

At the IdP, register exactly: `${CONWAY_PUBLIC_URL}/api/oidc/callback`
(e.g. `https://conway.example.com/api/oidc/callback`).

### Emit the groups claim

Most providers don't include groups by default:

- **Okta** — add a `groups` claim to the app's ID token (filter to the
  `conway-*` groups), or use an Okta authorization server with a groups claim.
- **Google Workspace** — Google ID tokens don't carry groups; use a custom
  claim populated from directory groups and set `CONWAY_OIDC_GROUPS_CLAIM` to
  its name, or map by another attribute.
- **Entra ID** — add the `groups` (or `roles`) claim to the token configuration
  and set `CONWAY_OIDC_GROUPS_CLAIM` accordingly.

## Example (Okta, public client + PKCE)

Register the Okta app as a **SPA** (or a Web app with client authentication set
to **None** and PKCE required). No client secret:

```bash
CONWAY_PUBLIC_URL=https://conway.example.com
CONWAY_OIDC_ISSUER=https://acme.okta.com
CONWAY_OIDC_CLIENT_ID=0oaXXXXXXXXXXXX
CONWAY_OIDC_ROLE_MAP=conway-admins=admin,conway-facilitators=facilitator,conway-managers=manager
# CONWAY_OIDC_CLIENT_SECRET is intentionally unset — PKCE authenticates the exchange.
```

## Operational notes

- **Break-glass:** the `admin` password account still works. Keep its password
  safe; it's your recovery path if OIDC breaks.
- **Revocation:** removing a user from all Conway groups in the IdP denies their
  *next* login. An already-issued session token remains valid until it expires
  (≤12h). For immediate cut-off, revoke the account in the Admin panel too.
- **Admin panel:** SSO accounts show an `SSO` badge and no expiry/extend control
  (they're governed by the IdP). "Revoke" deletes the row; it will be recreated
  on their next successful SSO login if they still map to a role.
- **Security posture:** RS256-only ID-token verification, JWKS cached 1h with a
  forced refresh on an unknown `kid`; PKCE S256; single-use, expiring login
  state; token delivered via fragment. Stdlib-only crypto, matching the rest of
  the auth package.
