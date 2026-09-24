# OpenCode Console usage APIs after the 2026 redesign

Research note, 2026-09-24. Question: how can switchboard-go read per-model usage from
the OpenCode Console programmatically after the redesign that replaced the SolidStart
console with a Vite SPA at https://opencode.ai/console/.

Where this file lives: this repo has no research note convention (only `docs/*.md`
user docs). I created `docs/research/` and used a date prefix so these notes sit next
to the docs they inform and can be superseded by later dated notes.

Method: public sources only. I read the deployed console bundle, the deployed marketing
bundle, the official docs (HTML and Markdown variants), docs source on GitHub, and the
public branch trees of `anomalyco/opencode`. No cookie or credential was used.
A follow-up session the same day verified authenticated behavior live with a service
account key. See "Live verification results" at the end.

## Summary

Per-model usage is available two ways:

| Path | Auth | Shape | Status |
| --- | --- | --- | --- |
| `GET /console/api/v1/usage/export` | Service account key `oc_sk_...` via `Authorization: Bearer` | Per-request CSV rows with `provider`, `model`, tokens, `cost_micro_cents`, `billing_source`; aggregate per model client-side | Documented, versioned, `v1` |
| `GET /console/api/usage/models` | Console session cookie `__Host-console_session` plus `x-org-id` header | Server-aggregated per-model JSON (`provider`, `model`, requests, token counters, `totalCostMicroCents`), paginated | Undocumented, internal, changes with the SPA |
| `GET /console/api/go/status` | Same session cookie | Go subscription `access.meters.{fiveHour,week,month}` with `limitMicroCents` and `usedMicroCents` | Undocumented, internal; Go subscribers only |
| `GET /console/api/v1/budgets/members` | Service account key with `all` permission | Monthly member budgets (`limit_micro_cents`, `spent_micro_cents`, `exceeded`, `resets_at`, `source`) | Documented, versioned, `v1` |

The old rolling/weekly/monthly quota windows with `quotaCost`, `multiplier` and
`contributionPercent` are gone from the new console bundle and have no direct
replacement API. The nearest equivalents are the usage ranges (`24h`, `7d`, `30d`,
internal also `all`), monthly member budgets, and the Go subscription meters.

## What the old scraper depended on

`workspace.go` discovered SolidStart server function hashes from `https://opencode.ai/`
JS bundles and called `/_server?id=<hash>` with an `auth` cookie, then parsed seroval
frames (`seroval.go`). The functions were `getWorkspaces`, `queryLiteSubscription` and
`queryLiteUsageDetails` (`workspace.go:21`), and the data contract was windows
`rolling`, `weekly`, `monthly` with `usage_usd`, `limit_usd`, `usage_percent`,
`reset_in_sec` and per-model rows with `cost`, `quota_cost`, `multiplier`,
`contribution_percent`, `estimated` (`workspace.go:25-42`).

That path is dead:

- The deployed console bundle contains none of those strings:
  `grep -c -E 'getWorkspaces|queryLiteSubscription|queryLiteUsageDetails|createServerReference'`
  returns 0 against the local copy of the bundle.
- The deployed console bundle has no `quota`, `rollingUsage`, `quotaCost`,
  `contributionPercent`, `multiplier` or `weekly` strings. Only unrelated
  `document.scrollingElement` matches for "rolling".
- The marketing bundle `entry-client.js` also has no server function references.

The old SolidStart console source is still public on the repo for reference:
`packages/console/app` (routes and UI) and `packages/console/core/src` (server
functions) on branch `dev`, e.g.
https://github.com/anomalyco/opencode/tree/dev/packages/console/app/src/routes/workspace/%5Bid%5D/usage
and https://github.com/anomalyco/opencode/blob/dev/packages/console/core/src/lite.ts.
Branches `v2` and `2.0` contain the same old console, not the new SPA.

## The new console SPA

The console at https://opencode.ai/console/ loads one entry bundle,
`/console/assets/index-DYlSq2C2.js` (from the HTML returned on 2026-09-24; 595,433 bytes,
md5 `891135dca9ef8750804db1c2c55d9796` of the local copy). The SPA hardcodes its base
path in the bundle:

- `gs=vg("/console/")` and `qS=e=>{const t=zS(e);return typeof window>"u"?t:new URL(t,window.location.origin).toString()}`
  so API paths written as `/api/...` become `https://opencode.ai/console/api/...`.

The bundle defines three Effect HttpApi documents. The annotations state their
compatibility status:

- Public: `class sZ extends Cf("public").add(g_).add(m_).annotate(Xc,"OpenCode Console Public API")...annotate(Zr,"Supported public management operations for OpenCode Console. Only operations in this allowlisted composition carry public compatibility guarantees.")`
- Internal: `class __ extends Cf("console")...annotate(Zr,"Combined typed contract for the Console browser and server. Inclusion here does not establish public compatibility; use the separate PublicApi document for supported external management operations.")`
- Combined docs: `class oZ extends Cf("console-documentation")...annotate(Zr,"Combined engineering reference for Console browser, server, and public typed routes. Inclusion here does not establish public compatibility.")`

Bundle strings below are quoted from the deployed file. The local copy path is
`/private/var/folders/f2/hx9z4xmj4816tkd36xknyxdm0000gn/T/opencode/console-index.js`
(offsets in this note refer to that copy; verify by searching the exact strings).

## Documented API: usage export (stable)

Source: https://opencode.ai/v2/docs/console/usage/ (deployed) and the docs source
`services/www/src/docs/content/console/usage.mdx` on branch `v2`:
https://github.com/anomalyco/opencode/blob/v2/services/www/src/docs/content/console/usage.mdx

```
GET /console/api/v1/usage/export
Authorization: Bearer oc_sk_...
Accept: text/csv
```

- Every request requires `scope` and `range`. Ranges are `24h`, `7d`, `30d`, and they
  start at midnight UTC rather than being rolling windows. The full range is streamed
  as one CSV, regardless of size (docs, "Endpoint and scopes").
- Scopes and extra parameters (docs table):

| Scope | Extra parameters | Result |
| --- | --- | --- |
| `organization` | none | All usage in the workspace |
| `member` | `user_email` | Usage for one member |
| `service_account` | `service_account_id` | Usage for one service account |
| `model` | `provider` and `model` | Usage for one provider/model pair |

- Filters are mutually exclusive. Extra filters return HTTP 400, not ignored.
- `scope=model` needs both `provider` and `model`; `provider` is the catalog key
  (`anthropic`, `deepseek`, `opencode`, ...). A valid filter with no rows returns HTTP
  200 and a header-only CSV.
- Rows come newest first. `cost_micro_cents` is Console's charge, 100,000,000 per USD.
  Free, BYOK and legacy unclassified rows have a zero charge. Organization and member
  exports include web search rows, which carry `service=web-search` and blank
  provider/model/token fields. Model and provider filters return inference rows only.
- CSV fields: `id`, `user_email`, `service_account_name`, `app`, `provider`, `model`,
  `input_tokens`, `output_tokens`, `reasoning_tokens`, `cache_read_tokens`,
  `cache_write_5m_tokens`, `cache_write_1h_tokens`, `reasoning_mode`,
  `reasoning_effort`, `reasoning_budget_tokens`, `reasoning_source`, `billing_source`,
  `cost_micro_cents`, `created_at`, `service`, `quantity`.
- `billing_source` values seen in the docs: `managed-inference`, `credit`, `byok`,
  `free`. The internal bundle adds `go` and `go-plus` (`qV=X(["managed-inference","free","byok",...Ri.literals,"credit","seat-credit"])`
  where `Ri=X(["go","go-plus"])`), which is worth expecting in Go workspaces.
- Errors: `400` missing/invalid parameters or filters that do not match the scope,
  `401` missing/invalid/expired/revoked key, `403` the service account is not allowed
  to read usage.

The bundle definition matches the docs:

```
class g_ extends Et("public-usage").add(Y("exportCsv","/api/v1/usage/export",{query:Wz,success:f.pipe(MI({contentType:"text/csv; charset=utf-8"})),error:Gz})).annotate(Jl,"public-usage.exportCsv").annotate(Yl,"Export organization usage as CSV").annotate(Zr,"Streams usage rows newest-first for the organization bound to the service API key. A successful response proves authorization and query acceptance; completion of the response body proves all matching rows were read. The operation is read-only and safe to retry.")).middleware(YI).annotate(Xc,"Usage").annotate(Zr,"Stable service-account usage export operations."){}const qz=X(["organization","member","service_account","model"]),Wz=C({scope:qz,range:p_,user_email:m(st),service_account_id:m(Qn),provider:m(ei),model:m(Tt)});class Gz extends ee()("PublicUsageExportQueryError",{message:f},{httpApiStatus:400}){}
```

Unauthenticated probe on 2026-09-24 (no credentials):

```
GET https://opencode.ai/console/api/v1/usage/export?scope=organization&range=24h
HTTP/2 401, body {"_tag":"Unauthorized"}
```

That confirms the route exists and rejects anonymous requests. The docs claim that user
session tokens are rejected by this endpoint; the client bundle cannot confirm that
claim, so test it live with a session cookie.

## Documented API: budgets (stable)

Source: https://opencode.ai/v2/docs/console/budgets/ and
`services/www/src/docs/content/console/budgets.mdx` on branch `v2`.

- `GET /console/api/v1/budgets/members` lists every member, ordered by email, with
  `user_id`, `email`, `limit_micro_cents`, `spent_micro_cents`, `exceeded`,
  `resets_at`, `source` (`custom` or `default`), `updated_at`. Monetary fields are
  decimal strings in microcents. Spend and reset cover the current UTC calendar month.
  A null limit and source mean unlimited.
- `PUT /console/api/v1/budgets/members/:user_id` with `{"budget_dollars":75.25}`
  (non-negative, up to two decimals), idempotent, HTTP 204.
- `DELETE /console/api/v1/budgets/members/:user_id` removes the override, HTTP 204.
- Keys need the `all` permission: "Inference-only keys cannot access budget data or
  change limits." Errors: 400, 401, 403, 404.

The internal console has a richer budget API, `class AV extends Et("budgets")`, under
`/api/budgets` with middleware `Yt`. It includes org defaults, per-user defaults,
per-service-account budgets, and `/api/budgets/service-accounts/status`:

```
class AV extends Et("budgets").add(Y("getOrgBudget","/org",{success:Xn(TV)})).add(jt("setOrgBudget","/org",{payload:ya})).add(rn("removeOrgBudget","/org")).add(Y("listUserBudgets","/users/status",{success:D(SV)})).add(Y("getDefaultUserBudget","/users/default",{success:Xn(u0)})) ... .add(Y("listServiceAccountBudgets","/service-accounts/status",{success:D(OV)})).add(Y("getDefaultServiceAccountBudget","/service-accounts/default",{success:Xn(u0)})) ... .prefix("/api/budgets").middleware(Yt)
```

Only the member routes are exposed publicly.

## Internal JSON usage API (undocumented)

The SPA's usage service is defined in the bundle as:

```
class zz extends Et("usage").add(Y("summary","/summary",{query:YK,success:Oy})).add(Y("costByDay","/cost-by-day",{query:ZK,success:D(vT)})).add(Y("models","/models",{query:eV,success:Mi(bT)})).add(Y("users","/users",{query:XK,success:Mi(jV)})).add(Y("rows","/rows",{query:nV,success:nT(WV)})).add(Y("exportCsv","/export",{query:jz,success:f.pipe(MI({contentType:"text/csv; charset=utf-8"}))})).prefix("/api/usage").middleware(Yt)
```

Resolved paths (base `/console`): `/console/api/usage/summary`, `/cost-by-day`,
`/models`, `/users`, `/rows`, `/export`.

Query schemas, quoted from the bundle:

```
UK=X(["24h","7d","30d","all"])      // internal range enum
p_=X(["24h","7d","30d"])            // public range enum
sY=10,QK=100                        // default page size, max page size
JK=X(["day","hour"])                // cost-by-day bucket
wy=X(["asc","desc"])                // costOrder
aa=f.check(ot(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$/))   // since, ISO 8601 UTC
YK=C({range:m(ia),since:m(aa),userId:m(Ge),serviceAccountId:m(Qn)})                                  // summary
ZK=C({range:m(ia),since:m(aa),bucket:m(JK),userId:m(Ge),serviceAccountId:m(Qn)})                     // cost-by-day
XK=C({range:m(ia),since:m(aa),page:m(Mf),pageSize:m(Xo),costOrder:m(wy)})                            // users
eV=C({range:m(ia),since:m(aa),userId:m(Ge),serviceAccountId:m(Qn),page:m(Mf),pageSize:m(Xo),costOrder:m(wy)})  // models
nV=C({range:m(ia),since:m(aa),userId:m(Ge),serviceAccountId:m(Qn),cursor:m(f),pageSize:m(Xo)})       // rows
jz=C({range:p_,userId:m(Ge),serviceAccountId:m(Qn),provider:m(ei),model:m(Tt)})                      // export
```

Semantics:

- `range` and `since` are both optional. `since` maps to a UTC day boundary when a
  range is used: `if(t==="24h")return dt(r);if(t==="7d"){...s.setUTCDate(s.getUTCDate()-6)...}if(t==="30d"){...-29...}`
  with `r` = today at UTC midnight. The `all` value exists only internally; the UI sends
  no `range` for "All time" and relies on `since` or server defaults.
- `page` is a non-negative integer (`Mf=Po.check(us(),Sn(0))`), `pageSize` is 1 to 100
  with default 10, `costOrder` orders by cost.
- The UI range picker is `$e=[{label:"Today",value:"24h"},{label:"Last 7 days",value:"7d"},{label:"Last 30 days",value:"30d"},{label:"All time",value:"all"}]`;
  its export menu is `At=[{value:"24h",label:"Last 1 day"},{value:"7d",label:"Last 7 days"},{value:"30d",label:"Last 30 days"}]`
  (chunk `model-breakdown-COAplVTa.js`).
- The model table has a per-row CSV export that calls the internal `/export` with
  `{range, userId | serviceAccountId, provider, model}`, which is the same CSV shape as
  the public export (verify live). The org page is admin-only
  (`ce(c.data.role,"admin")` in `page-D2CcQgQN.js`).

Response types, quoted from the bundle:

```
class Oy extends I("UsageSummary")({totalRequests:is,totalInputTokens:vt,totalOutputTokens:vt,totalCacheReadTokens:vt,totalCacheWrite5mTokens:vt,totalCacheWrite1hTokens:vt,totalCostMicroCents:nt,services:D(HV).pipe(lt(re([])))}){get totalTokens(){...}}
class HV extends I("ServiceUsageSummary")({service:V("web-search"),totalRequests:is,totalCostMicroCents:nt})
class vT extends I("DailyCost")({date:f,totalCostMicroCents:nt,totalTokens:vt,totalRequests:is})
class bT extends I("ModelSummary")({model:Tt,provider:ei,totalRequests:is,totalInputTokens:vt,totalOutputTokens:vt,totalCacheReadTokens:vt,totalCacheWrite5mTokens:vt,totalCacheWrite1hTokens:vt,totalCostMicroCents:nt})
class jV extends I("UserUsageSummary")({userId:_(Ge),principalType:XI,serviceUserId:_(Qn),email:_(st),name:_(f),totalRequests:is,totalInputTokens:vt,...,lastActiveAt:_(G)})
class WV extends I("UsageSelect")({id:zV,orgId:Pe,userId:_(Ge),principalType:XI,serviceUserId:_(Qn),serviceApiKeyId:_(vy),appReferrer:_(f),appTitle:_(f),provider:ei,model:Tt,inputTokens:Le,outputTokens:Le,reasoningTokens:Le,cacheReadTokens:Le,cacheWrite5mTokens:Le,cacheWrite1hTokens:Le,reasoningMode:_(UV),reasoningEffort:_(BV),reasoningBudgetTokens:_(KV),reasoningSource:_(VV),billingSource:_(qV),costMicroCents:Ls,createdAt:G})
class tV extends I("PageInfo")({page:c0,pageSize:c0,total:l0,pageCount:l0})
Mi=e=>C({items:D(e),pageInfo:tV})       // page-based envelope for models/users
nT=e=>C({items:D(e),nextCursor:Xn(f)})  // cursor envelope for rows
```

So `/usage/models` returns the same per-model fields that switchboard-go currently
derives from the old `queryLiteUsageDetails` rows: provider, model, request count, token counters
(including cache read and 5m/1h cache writes), and cost in microcents. It does not
include quota cost, multiplier or contribution percent. Token counts are integers;
cost fields are a BigInt-backed microcent schema in the client (`nt` with
`toDollars`), so confirm the JSON wire format (string vs number) live. The public
budgets docs use decimal strings for microcents.

Pagination: `/usage/models` and `/usage/users` return `{items, pageInfo:{page,pageSize,total,pageCount}}`.
`/usage/rows` returns `{items, nextCursor}`. `/usage/summary` and `/usage/cost-by-day`
are not paginated.

Unauthenticated probes on 2026-09-24 (no credentials) returned HTTP 401
`{"_tag":"Unauthorized"}` for `/console/api/usage/models`, `/summary`, `/rows`, `/users`,
`/cost-by-day`, `/export`, `/console/api/go/status`, `/console/api/service-accounts`,
`/console/api/orgs` and `/console/api/orgs/current`.

## Auth

### Session cookie and org scope

- Cookie names in the bundle: `cq="__Host-console_session"` (HTTPS/prod) and
  `lq="console_session"` (non-HTTPS/dev), selected by
  `fq=y_(v_)?cq:lq`. OIDC flow cookies are `__Host-console_oidc_flow` and
  `console_oidc_flow`. The bundle never reads or writes an `auth` cookie (`"auth="` and
  `key:"auth"` are both absent), which matches the observed rejection of the old cookie.
- Session middleware: `w_=z8({in:"cookie",key:fq}), S_=HI; class Wa extends ra()("SessionMiddleware",{error:b_,security:{cookie:w_,bearer:S_}})`.
  `HI=j8({scheme:"Bearer"})` is the bearer security scheme used throughout.
- Auth HTTP API (`Vq`) is prefixed `/auth`: `POST /console/auth/register`,
  `POST /console/auth/login`, `POST /console/auth/logout`, `POST /console/auth/refresh`,
  `GET /console/auth/session`, `GET /console/auth/password-status`, plus device and
  OAuth/OIDC routes (`/auth/device/code`, `/auth/device/token`, `/auth/oauth/token`,
  `/auth/oidc/:providerId/start`, `/auth/social/:providerId/start`, ...).
- `GET /console/auth/session` returns `Ma=C({expiresAt:f,user:Dq,org_id:m(f)})` where
  `Dq=C({id:Ge,email:st})`. Unauthenticated it returns 401
  `{"_tag":"SessionQueryFailed","message":"Not authenticated"}` (probe, 2026-09-24).
- The SPA retries any 401 by POSTing `/console/auth/refresh` and replaying the request,
  except on `/auth/session`, `/auth/refresh`, `/auth/login`, `/auth/register`,
  `/auth/logout`:
  `t=pm(e.post("/auth/refresh")...)` and the URL exclusion list in the same expression.
  A long-running scraper with a cookie can use the same refresh call to extend the
  session instead of re-logging in.
- Workspace scope travels in a header, not the cookie:
  `Qp="x-org-id"`, `jq=(e,t)=>t._tag==="unscoped"?e:wf(e,n=>ry(n,Qp,t.orgId))` adds it
  for org-scoped calls, `zq` rejects callers trying to set it themselves
  (`"${Qp} is reserved; provide workspace scope through ApiScope"`). The selected org is
  persisted client-side under `tg="opencode-console.org-id"`.
- The org ID schema accepts legacy and new prefixes:
  `Pe=f.check(ot(/^(org_|wrk_)/)).pipe(j("OrgId"),...)`. Old `wrk_...` workspace IDs
  still match, which suggests the IDs already configured in switchboard-go remain usable
  as `x-org-id` values. Verify live.
- Auth mutations (`login`, `register`, `logout`, `device/approve`, `change-password`)
  carry `CsrfMiddleware` (`class Or extends ra()("CsrfMiddleware")`). Usage reads do not.
  A bare session cookie should be enough for reads; verify live.

### Service accounts and API keys

Console UI routes: `/console/service-accounts` (list) and
`/console/service-accounts/:id` (detail), both in the SPA route table. The list page is
`chunk page-CK8OkNN1.js`; the detail page is `page-C48IybZx.js`.

Internal endpoints as called by the console UI (middleware `Yt`; the SPA uses the
session cookie):

```
POST   /console/api/service-accounts                          {name}                  -> ServiceAccount
GET    /console/api/service-accounts?page&pageSize&name
GET    /console/api/service-accounts/:serviceAccountId
POST   /console/api/service-accounts/:serviceAccountId/keys  {name,permissions,expiresAt?} -> {key, token}
POST   /console/api/service-accounts/keys/:serviceApiKeyId/revoke
DELETE /console/api/service-accounts/:serviceAccountId
```

Quoted definition:

```
class Kz extends Et("service-accounts").add(Y("list","/api/service-accounts",{query:Uz,success:Mi(a0)})).add(Y("detail","/api/service-accounts/:serviceAccountId",{params:Oh,success:Xn(a0)})).add(he("createServiceAccount","/api/service-accounts",{payload:Nz,success:eT,error:[WK,GK]})).add(he("createServiceApiKey","/api/service-accounts/:serviceAccountId/keys",{params:Oh,payload:Bz,success:zK,error:be})).add(he("revokeServiceApiKey","/api/service-accounts/keys/:serviceApiKeyId/revoke",{params:Fz,success:qK,error:be})).add(rn("deleteServiceAccount","/api/service-accounts/:serviceAccountId",{params:Oh,success:P,error:be})).middleware(Yt)
```

Key details from the bundle:

- Permissions: `Pf=X(["all","inference-only"])`. The detail page renders
  `permissions==="inference-only"?"Inference only":"All"`.
- Create key form fields: name (placeholder in the UI is "GitHub Actions"),
  permissions radio (All, Inference only), optional `expiresAt` date
  (chunk `page-C48IybZx.js`: `options:[{value:"all",label:"All"},{value:"inference-only",label:"Inference only"}]`).
- The full token is returned once; the UI warns "Copy this token now. It will not be
  shown again." and lists only `tokenHint`. Token pattern:
  `ot(/^(?:oc_sk_[0-9a-f]{12}_[A-Za-z0-9_-]{32}|sk-[A-Za-z0-9]{64})$/)`, hint pattern
  `^(?:oc_sk_[0-9a-f]{12}|sk-[A-Za-z0-9]{12})$`. Key statuses: `active`, `revoked`,
  `expired`, plus `lastUsedAt`, `expiresAt`, `revokedAt`.
- Usage export docs require a service account key. They do not name the permission,
  only that a 403 means "not allowed to read usage". Budgets explicitly require `all`;
  assume `all` is also required for usage export until tested (open question).

### Middleware matrix

Three middleware classes gate the API. Each is declared with a bearer security scheme;
actual accepted credentials are server-side and not visible in the client bundle.

| Middleware | Identifier | Used by | Declared errors |
| --- | --- | --- | --- |
| `Yt` | `@console/OrgActorMiddleware` | most internal endpoints: usage, budgets, service accounts, providers, members, sso, billing | `Unauthorized` 401, `Forbidden` 403, `OrgRequired` 400, `NotFound` 404, `SsoRequired` 403 |
| `YI` | `@console/ServiceActorMiddleware` | public `v1` usage export and budgets only | `Unauthorized` 401, `Forbidden` 403 |
| `Af` | `@console/ActorMiddleware` | unscoped endpoints such as `/api/user`, `/api/invites`, `/api/internal/*` | `Unauthorized` 401 |

Quoted:

```
class Yt extends ra()("@console/OrgActorMiddleware",{error:[fy,ce,JI,be,Ze],security:{bearer:yy}}){}class YI extends ra()("@console/ServiceActorMiddleware",{error:[fy,ce],security:{bearer:yy}}){}class Af extends ra()("@console/ActorMiddleware",{error:fy,security:{bearer:yy}})
```

`Ze` is `SsoRequired` (`class Ze extends ee()("SsoRequired",{orgId:Pe,connectionId:m(sa)},{httpApiStatus:403})`),
`JI` is `OrgRequired` 400, `fy` is `Unauthorized` 401, `ce` is `Forbidden` 403,
`be` is `NotFound` 404.

The Go routes use a different middleware name,
`@opencode-ai/console/AuthenticatedWorkspaceAccess/Middleware`, declared with the same
bearer scheme in `chunk queries-Dc1pcTGj.js`:
`class T extends S()("@opencode-ai/console/AuthenticatedWorkspaceAccess/Middleware",{security:{bearer:C},error:[w,f,o,s,t]})`.

What is provable from the client bundle: the browser client authenticates internal calls
with the session cookie (Effect's fetch sends same-origin cookies) and never sets an
`Authorization` header for internal calls; the `x-org-id` header carries org scope. The
bearer scheme declared on `Yt` may be for the OAuth/device access tokens
(`POST /console/auth/oauth/token` returns `{access_token, refresh_token, token_type:"Bearer", expires_in}`,
and device codes approve an `org_id`), or it may accept `oc_sk_` keys. This client bundle
cannot settle it. Live test needed.

## What replaced the quota windows

The old per-window quota model has no direct replacement. Findings:

- No `quota`, `rolling`, `weekly`, `weeklyUsage`, `contribution`, `quotaCost` or
  `multiplier` strings in the new console bundle. The old fields are not rendered or
  served anywhere in the new SPA.
- Usage is now range-based: `24h`, `7d`, `30d` (public) and additionally `all`
  (internal). Ranges start at UTC midnight.
- Limits are now monthly budgets. The documented member budgets API exposes
  `limit_micro_cents`, `spent_micro_cents`, `exceeded` and `resets_at` for a UTC
  calendar month, per member (`GET /console/api/v1/budgets/members`). The internal API
  also has org, user-default and service-account budgets under `/console/api/budgets`.
- Go subscribers have 5-hour, weekly and monthly meters in the Go status payload:

```
nj=C({fiveHour:XH,week:ej,month:tj})
XH=C({startsAt:_(wr),resetsAt:_(wr),limitMicroCents:Li,usedMicroCents:Li})
ej=C({startsAt:wr,resetsAt:wr,limitMicroCents:Li,usedMicroCents:Li})
tj=C({limitMicroCents:Li,usedMicroCents:Li})
rj=C({startsAt:wr,endsAt:wr,cancelAtPeriodEnd:P,meters:nj})
JT=C({subscriberUserID:ua,product:Ri,renewalProduct:Ri,...,access:_(rj),...})   // go status
```

The product revisions in the bundle define Go usage limits:
`go` has `fiveHoursFromFirstUseMicroCents:1200000000n, utcCalendarWeekMicroCents:3000000000n, paidPeriodMicroCents:6000000000n`
($12 / $30 / $60); `go-plus` has $30 / $75 / $150. The docs describe the same windows
as percentages of a per-model monthly limit: "Each model has the following usage
limits: 5-hour (20% of the monthly limit), weekly (50%), and monthly (100%)"
(https://opencode.ai/v2/docs/console/go/#usage-limits). The `/go/status` payload has one
meter object per window, so it looks subscription-level rather than per model, but treat
that as an inference until verified.

Endpoint paths for Go data: internal `GET /console/api/go/status` (used by the SPA,
chunk `queries-Dc1pcTGj.js`; success type `JT`, `sj=_(JT)`), and the support/internal
`GET /console/api/orgs/:orgId/go/status` in the internal orgs service.

Implications for the switchboard-go dashboard contract:

| Old (`workspace.go`) | New source |
| --- | --- |
| workspace list (`getWorkspaces`) | org list `GET /console/api/orgs` (session, unscoped), or keep configured `wrk_` IDs as `x-org-id` |
| `windows.rolling/weekly/monthly` | usage ranges `24h`/`7d`/`30d`/`all`; Go meters `fiveHour`/`week`/`month` for Go workspaces |
| `usage_usd`, `limit_usd` | `totalCostMicroCents` from usage queries; limit from member/service-account budgets or Go meter `limitMicroCents` |
| `usage_percent`, `status` | computed from budget or meter `usedMicroCents` / `limitMicroCents`; budget `exceeded` flag |
| `reset_in_sec` | `resets_at` (budgets, UTC month start) or meter `resetsAt` (Go), computed client-side |
| rows `cost` | `totalCostMicroCents` per model (`/usage/models`) or summed CSV rows |
| rows `quota_cost`, `multiplier`, `contribution_percent`, `estimated` | no replacement; drop from the contract or mark deprecated |

## Stability, errors and rate limits

- Documented and versioned: `GET /console/api/v1/usage/export` and the
  `/console/api/v1/budgets/members` routes. The bundle marks the public document as
  "Only operations in this allowlisted composition carry public compatibility
  guarantees" and the public usage export as "Stable service-account usage export
  operations". The docs say to "Use the versioned endpoint in scripts, reporting jobs,
  and billing integrations".
- Undocumented: everything under `/console/api/usage/*`, `/console/api/service-accounts`,
  `/console/api/go/*`, `/console/api/budgets/*`, and the me-group routes (`/api/user`,
  `/api/orgs`, `/api/invites`). The bundle annotation for the internal document says
  inclusion "does not establish public compatibility". Paths and query names can change
  with any SPA deploy.
- Internal error bodies are Effect tagged unions: `{"_tag":"Unauthorized"}` for 401,
  `{"_tag":"SessionQueryFailed","message":"Not authenticated"}` for the session
  endpoint. Public API errors follow the HTTP status table in the docs.
- Rate limits: no rate limits are documented for the usage export or budgets pages
  (checked the deployed pages and the MDX sources). The bundle does define a
  `RateLimited` error and an in-memory RateLimiter, wired into auth flows
  (`Bl=Yy.pipe(Js(429))` on register/login). The `retry-after` header is exposed via
  CORS (`access-control-expose-headers: retry-after,x-request-id,x-opencode-log-id`)
  on responses. Treat public API rate limits as unknown and back off on 429.

## Is the new console SPA source public?

No source for the new SPA or its API was found in the public repo on 2026-09-24.

Checked:

- Default branch of `anomalyco/opencode` is `dev`
  (https://api.github.com/repos/anomalyco/opencode). The `dev`, `v2` and `2.0` trees
  contain the old SolidStart console only: `packages/console/app`,
  `packages/console/core`, `packages/console/function`, and no paths matching
  `service-account`, `svcacct`, `console/api` or `OrgActor`. Tree listings:
  https://github.com/anomalyco/opencode/tree/dev and https://github.com/anomalyco/opencode/tree/v2.
- Branches matching `console` (via `git ls-remote --heads`) are feature branches for the
  old console or docs, for example `console-auth-refresh`,
  `kit/console-org-switcher`, `migrate-console-app-to-nextjs`. Trees for those three
  contain no new SPA paths either.
- `anomalyco/console` is the old SST dashboard ("A web based dashboard for your SST
  apps"), unrelated. See https://github.com/anomalyco/console.

So the new SPA, its Effect HttpApi server and the `/console/api` implementation are
presumably in a private repository. The only public artifacts are the deployed bundle
and the docs.

Public docs source for citations (branch `v2`):
https://github.com/anomalyco/opencode/tree/v2/services/www/src/docs/content/console
(`index.mdx`, `models.mdx`, `websearch.mdx`, `go.mdx`, `inference.mdx`, `byok.mdx`,
`usage.mdx`, `budgets.mdx`).

## Options for switchboard-go, ranked

1. Public CSV export plus local aggregation. Use one `all`-permission service account
   key (`Authorization: Bearer oc_sk_...`) and
   `GET /console/api/v1/usage/export?scope=organization&range=24h|7d|30d`, then group
   rows by `provider` + `model` and sum `cost_micro_cents` and token fields. This is the
   only documented, versioned path, it returns per-model data after aggregation
   (including per-model token breakdowns), and it
   works without touching cookies. Caveats: record-level volume for 30d, UTC-midnight
   boundaries instead of rolling windows, no limit/reset information, CSV parsing and
   dedup on `id`, web-search rows need filtering (`service=web-search`, blank model),
   and `organization` scope is bound to the key's workspace, so one key per workspace.
2. Internal `/console/api/usage/models` with the session cookie and `x-org-id`. This is
   the closest match to the dashboard's existing shape (provider, model, requests,
   tokens, `totalCostMicroCents`, pagination, cost order). Use `/summary` for totals,
   `/cost-by-day` for the chart, `/users` for per-user rows. Caveats: undocumented and
   unversioned, session cookie expiry (mitigate by POSTing `/console/auth/refresh`),
   one `x-org-id` per workspace, microcent wire format to confirm, and it may break on
   any console deploy. It is still far simpler than the seroval scraping it replaces.
3. Go status meters, if the workspace has a Go subscription. `GET /console/api/go/status`
   returns `access.meters.{fiveHour,week,month}` with used and limit microcents, which
   restores the window-style status the dashboard used to show, at subscription level
   rather than per model. Session-only, undocumented, single member per workspace
   subscription.
4. Documented budgets for limits and resets. `GET /console/api/v1/budgets/members` with
   an `all` key supplies `limit_micro_cents`, `spent_micro_cents`, `exceeded` and
   `resets_at` per member for the current UTC month. Pair with option 1 or 2 for
   per-model breakdown; there is no public per-service-account budget listing.

Practical combination: option 4 for limit/status/reset fields plus option 1 for
per-model cost, with option 2 as an optional low-latency path if a monitored session
cookie is acceptable. The session path in option 2 is the only one that avoids
aggregating CSV in switchboard-go, but it puts the project back on undocumented
internal endpoints.

## Live verification results

Verified on 2026-09-24 against https://opencode.ai with a service account key
(`oc_sk_...`, permission `all`) on a Go-plan workspace. The key and the raw responses
stayed out of this repo; the observations below are summarized.

Resolved:

- `oc_sk_...` works as `Authorization: Bearer` on the internal JSON endpoints.
  `/console/api/usage/{summary,cost-by-day,models,users,rows,export}` and
  `/console/api/go/status` all returned 200 with the service key. No session cookie or
  `x-org-id` header was needed.
- Internal microcent fields are decimal strings (`"totalCostMicroCents":"542071899"`),
  matching the public budgets API. Token counts are also strings.
- Internal microcent fields and the Go meters share one unit. For the same `since`
  boundary, `/usage/summary` and `/usage/models` returned identical totals
  (365 requests, 109551037 microcents), and the five-hour meter read 114418852
  microcents at nearly the same time. So `totalCostMicroCents` is quota cost, not a
  separate raw charge.
- `since` alone works on `/usage/models` without `range`, and returns that window's
  per-model rows.
- The internal `/console/api/usage/export` CSV starts with the same header row as the
  public `v1/usage/export` (all 21 columns, `cost_micro_cents` included).
- `x-org-id: wrk_...` is accepted on `/console/api/go/status` (200 with and without).
- `/console/api/service-accounts` returns both the org ID (`wrk_...`) and key metadata
  (name, permissions, `tokenHint`, `expiresAt`) with an `all` key.
- Meter limits for this Go workspace: fiveHour 1200000000, week 3000000000, month
  6000000000 microcents ($12 / $30 / $60). The month meter carries no `startsAt` or
  `resetsAt`; `access.endsAt` is the only monthly boundary in the response.
- Week and month meters read higher than the sum of usage records in the same window
  (week 1013236711 vs 614213292 recorded; month 5084232386 vs 614213292). The five-hour
  window matched within 1 percent. The record store in this workspace only goes back to
  2026-09-22 while the subscription started 2026-08-26, so older usage is missing from
  the records, not from the meters.

Still open:

- Required permission for `v1/usage/export` and for internal reads with an
  `inference-only` key. Only an `all` key was tested.
- Rate limits and payload caps on either export path, and behavior of
  `scope=organization&range=30d` on a busy workspace.
- Whether a console session cookie alone passes the internal middleware, and whether
  `POST /console/auth/refresh` extends a cookie session indefinitely. The pre-redesign
  `auth` cookie is rejected by the new console (401).
- What `GET /console/api/orgs` returns for a service key (401 with one), and the full
  `OrgRequired` error body.
- Whether the old `wrk_...` ID in `x-org-id` selects a workspace or is ignored when the
  key is already org-bound.

## Appendix: bundle strings used

All from https://opencode.ai/console/assets/index-DYlSq2C2.js on 2026-09-24, local copy
`console-index.js` md5 `891135dca9ef8750804db1c2c55d9796`. Offsets refer to that copy.

| Offset | String |
| --- | --- |
| 431223 | `class Yt extends ra()("@console/OrgActorMiddleware",{error:[fy,ce,JI,be,Ze],security:{bearer:yy}}){}class YI extends ra()("@console/ServiceActorMiddleware",{error:[fy,ce],security:{bearer:yy}}){}class Af extends ra()("@console/ActorMiddleware",{error:fy,security:{bearer:yy}})` |
| 430001 | `class Ze extends ee()("SsoRequired",{orgId:Pe,connectionId:m(sa)},{httpApiStatus:403})` |
| 433477 | `const UK=X(["24h","7d","30d","all"])` and the `ia` range transform with `toSince` |
| 435988 | `sY=10,QK=100,...Xo=Po.check(us(),jm({minimum:1,maximum:QK}))` |
| 436001 | `aa=f.check(ot(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$/))` |
| 436264 | `YK=C({range:m(ia),since:m(aa),userId:m(Ge),serviceAccountId:m(Qn)})...jz=C({range:p_,userId:m(Ge),serviceAccountId:m(Qn),provider:m(ei),model:m(Tt)})` |
| 503950 | `class zz extends Et("usage")...prefix("/api/usage").middleware(Yt)` |
| 504317 | `class g_ extends Et("public-usage")...middleware(YI).annotate(Xc,"Usage").annotate(Zr,"Stable service-account usage export operations.")` |
| 447941 | `class Oy extends I("UsageSummary")...` and neighboring `UserUsageSummary`, `DailyCost`, `ModelSummary` definitions |
| 449553 | `class WV extends I("UsageSelect")...` |
| 503011 | `class Kz extends Et("service-accounts")...middleware(Yt)` |
| 434261 | `Pf=X(["all","inference-only"])`, `BK=...oc_sk_[0-9a-f]{12}...`, `ServiceAPIKey.Token` pattern |
| 423929 | `Qp="x-org-id"` and `Pe=f.check(ot(/^(org_|wrk_)/))` |
| 519105 | `jq=(e,t)=>t._tag==="unscoped"?e:wf(e,n=>ry(n,Qp,t.orgId))`; at 519200 `zq` throws `` `${Qp} is reserved; provide workspace scope through ApiScope` `` |
| 446160 | `usageLimits:{fiveHoursFromFirstUseMicroCents:1200000000n,utcCalendarWeekMicroCents:3000000000n,paidPeriodMicroCents:6000000000n}` |
| 482806 | `nj=C({fiveHour:XH,week:ej,month:tj})...rj=C({startsAt:wr,endsAt:wr,cancelAtPeriodEnd:P,meters:nj})` |
| 515234 | `Ma=C({expiresAt:f,user:Dq,org_id:m(f)})` and the auth service endpoints |
| 419147 | refresh-on-401 logic and the `/auth/refresh` POST |
| 510330 | `cq="__Host-console_session",lq="console_session",uq="__Host-console_oidc_flow",dq="console_oidc_flow"` |
| 510957 | `w_=z8({in:"cookie",key:fq}),S_=HI;class Wa extends ra()("SessionMiddleware",...)` |
| 153235 | `gs=vg("/console/")` and `qS` base URL resolution |
