(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const a of document.querySelectorAll('link[rel="modulepreload"]'))n(a);new MutationObserver(a=>{for(const i of a)if(i.type==="childList")for(const f of i.addedNodes)f.tagName==="LINK"&&f.rel==="modulepreload"&&n(f)}).observe(document,{childList:!0,subtree:!0});function s(a){const i={};return a.integrity&&(i.integrity=a.integrity),a.referrerPolicy&&(i.referrerPolicy=a.referrerPolicy),a.crossOrigin==="use-credentials"?i.credentials="include":a.crossOrigin==="anonymous"?i.credentials="omit":i.credentials="same-origin",i}function n(a){if(a.ep)return;a.ep=!0;const i=s(a);fetch(a.href,i)}})();class _ extends Error{status;constructor(t,s){super(s),this.status=t}}function ne(e){return e.trim().replace(/\/+$/,"")}function ae(e){return e?{Authorization:`Bearer ${e}`}:{}}async function Y(e,t,s){const n=await fetch(`${ne(e)}${t}`,{headers:ae(s)});if(n.status===401)throw new _(401,"Invalid or missing proxy API key");if(!n.ok)throw new _(n.status,`Request failed with status ${n.status}`);return await n.json()}async function Q(e,t,s,n){const a=await fetch(`${ne(e)}${t}`,{method:"POST",headers:{...ae(s),"Content-Type":"application/json"},body:n===void 0?void 0:JSON.stringify(n)});if(a.status===401)throw new _(401,"Invalid or missing proxy API key");if(!a.ok)throw new _(a.status,`Request failed with status ${a.status}`);return await a.json()}async function Ne(e,t,s=!1){return Y(e,s?"/usage?refresh=true":"/usage",t)}async function Ce(e){return Y(e,"/dashboard/api/metrics.json","")}async function Ae(e,t){return Q(e,"/admin/validate-keys",t)}async function Ie(e,t,s){return Q(e,"/admin/reset-key",t,{index:s})}async function Te(e,t){return Q(e,"/admin/reset-all-keys",t)}async function Ke(e,t){await Q(e,"/admin/reload",t)}async function Pe(e,t){return Y(e,"/admin/workspace-usage",t)}async function je(e,t){return Y(e,"/admin/config",t)}async function Ee(e,t,s){const n=await fetch(`${ne(e)}/admin/config`,{method:"PATCH",headers:{...ae(t),"Content-Type":"application/json"},body:JSON.stringify(s)});if(n.status===401)throw new _(401,"Invalid or missing proxy API key");if(!n.ok){let a=`Request failed with status ${n.status}`;try{const i=await n.json();i.error?.message&&(a=i.error.message)}catch{}throw new _(n.status,a)}return await n.json()}function c(e){return e.replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;").replace(/'/g,"&#39;")}function D(e){return e===void 0||Number.isNaN(e)?"0.0%":`${e.toFixed(1)}%`}function K(e){return e===void 0||Number.isNaN(e)?"$0.00":"$"+e.toFixed(2)}function oe(e,t=95){return e===void 0||Number.isNaN(e)?"ok":e>=t?"critical":e>=70?"warn":"ok"}function ie(e){const t=Math.max(0,Math.floor(e)),s=Math.floor(t/86400),n=Math.floor(t%86400/3600),a=Math.floor(t%3600/60),i=t%60;return s>0?`${s}d ${n}h ${a}m`:n>0?`${n}h ${a}m`:a>0?`${a}m ${i}s`:`${i}s`}function re(e,t=Date.now()){if(!e)return"—";const s=Date.parse(e);if(Number.isNaN(s))return"—";const n=(s-t)/1e3;return n<=0?"reset due":ie(n)}function Oe(e,t=Date.now()){if(!e)return"never";const s=Date.parse(e);if(Number.isNaN(s))return"—";const n=(t-s)/1e3;return n<0||n<60?"just now":`${ie(n)} ago`}function ue(e,t){return t<=0?"—":`${Math.round(e/t*1e3).toLocaleString("en-US")} ms`}const pe=/^\d+(\.\d+)?(ns|us|µs|ms|s|m|h)$/,ve=[{name:"session_ttl",id:"cfg-session-ttl",label:"Session TTL"},{name:"balanced_idle_timeout",id:"cfg-balanced-idle-timeout",label:"Balanced idle timeout"},{name:"retry_exhausted_after",id:"cfg-retry-exhausted-after",label:"Retry exhausted after"}],ze=["session_sticky","balanced","round_robin","fill_first"];function I(e,t){return e.has(t)?'<span class="env-badge" title="Overridden by an environment variable">env</span>':""}function E(e,t){return e.has(t)?" disabled":""}function ge(e){const t=e===null,s=t?"":String(e.id),n=t?"new key":e.key_hint||"…",a=t?'<input type="password" class="input" data-key-value placeholder="sk-…" autocomplete="new-password" />':'<input type="password" class="input" data-key-rotate placeholder="rotate (optional)" autocomplete="new-password" />';return`
    <div class="cfg-key-row" data-key-row>
      <input type="hidden" data-key-id value="${c(s)}" />
      <span class="num cfg-key-hint">${c(n)}</span>
      <label class="num cfg-inline">pri
        <input type="number" class="input" min="1" data-key-priority value="${e?.priority??1}" />
      </label>
      <label class="num cfg-inline">w
        <input type="number" class="input" min="1" data-key-weight value="${e?.weight??1}" />
      </label>
      ${a}
      <button type="button" class="btn btn-secondary btn-xs" data-remove-key>Remove</button>
    </div>
  `}function ye(e,t,s){return`
    <div class="cfg-alias-row" data-alias-row data-alias-original="${c(s)}">
      <input type="text" class="input" data-alias-from placeholder="alias" value="${c(e)}" />
      <input type="text" class="input" data-alias-to placeholder="target model" value="${c(t)}" />
      <button type="button" class="btn btn-secondary btn-xs" data-remove-alias>Remove</button>
    </div>
  `}function qe(){return ge(null)}function De(){return ye("","","")}function he(e){const t=new Set(e.env_locked),s=e.settings,n=ze.map(u=>`<option value="${u}"${s.routing_strategy===u?" selected":""}>${u}</option>`).join(""),a=ve.map(({name:u,id:o,label:d})=>`
      <div class="field">
        <label for="${o}">${d}${I(t,u)}</label>
        <input type="text" id="${o}" class="input" value="${c(String(s[u]))}"${E(t,u)} />
      </div>`).join(""),i=t.has("keys")?'<p class="text-muted cfg-note">Keys are managed by OPENCODE_GO_API_KEYS.</p>':e.keys.map(u=>ge(u)).join(""),f=t.has("model_aliases")?"":Object.entries(e.model_aliases).map(([u,o])=>ye(u,o,u)).join("");return`
    <div class="cfg-section">
      <div class="kicker">Routing</div>
      <div class="cfg-grid">
        <div class="field">
          <label for="cfg-routing-strategy">Strategy${I(t,"routing_strategy")}</label>
          <select id="cfg-routing-strategy" class="input"${E(t,"routing_strategy")}>${n}</select>
        </div>
        <div class="field">
          <label for="cfg-proactive-threshold">Proactive threshold %${I(t,"proactive_switch_threshold")}</label>
          <input type="number" id="cfg-proactive-threshold" class="input" min="0" max="100" step="0.1" value="${s.proactive_switch_threshold}"${E(t,"proactive_switch_threshold")} />
        </div>
        ${a}
      </div>
    </div>
    <div class="cfg-section">
      <div class="kicker">Polling</div>
      <div class="cfg-grid">
        <div class="field">
          <label for="cfg-usage-check-interval">Usage check interval${I(t,"usage_check_interval")}</label>
          <input type="text" id="cfg-usage-check-interval" class="input" value="${c(s.usage_check_interval)}"${E(t,"usage_check_interval")} />
        </div>
        <div class="field">
          <label for="cfg-disable-polling">Disable usage polling${I(t,"disable_usage_polling")}</label>
          <input type="checkbox" id="cfg-disable-polling"${s.disable_usage_polling?" checked":""}${E(t,"disable_usage_polling")} />
        </div>
      </div>
    </div>
    <div class="cfg-section">
      <div class="kicker">Models</div>
      <div class="cfg-grid">
        <div class="field">
          <label for="cfg-sanitize-role">Sanitize developer role${I(t,"sanitize_developer_role")}</label>
          <input type="checkbox" id="cfg-sanitize-role"${s.sanitize_developer_role?" checked":""}${E(t,"sanitize_developer_role")} />
        </div>
      </div>
      <div class="kicker" style="margin-top:var(--space-4)">Model aliases${I(t,"model_aliases")}</div>
      <div data-alias-list>${f}</div>
      <button type="button" class="btn btn-secondary" data-add-alias${t.has("model_aliases")?" disabled":""}>Add alias</button>
    </div>
    <div class="cfg-section">
      <div class="kicker">Upstream keys${I(t,"keys")}</div>
      <div data-key-list>${i}</div>
      <button type="button" class="btn btn-secondary" data-add-key${t.has("keys")?" disabled":""}>Add key</button>
      <p class="text-muted cfg-note">Existing key values never leave the server. Paste a new value to rotate a key.</p>
    </div>
  `}function H(e,t){return e.querySelector(t)}function He(e,t){const s=new Set(e.env_locked),n=e.settings,a={if_revision:e.revision},i={},f=t.querySelector("#cfg-routing-strategy");f&&!s.has("routing_strategy")&&f.value!==n.routing_strategy&&(i.routing_strategy=f.value);const u=H(t,"#cfg-proactive-threshold");if(u&&!s.has("proactive_switch_threshold")){const p=Number(u.value);if(!Number.isFinite(p)||p<0||p>100)return{error:"proactive_switch_threshold must be between 0 and 100",changed:!1};p!==n.proactive_switch_threshold&&(i.proactive_switch_threshold=p)}for(const{name:p,id:w}of ve){if(s.has(p))continue;const $=H(t,`#${w}`);if(!$)continue;const h=$.value.trim();if(!pe.test(h))return{error:`${p} must be a duration like 30s, 5m, or 2h`,changed:!1};h!==n[p]&&(i[p]=h)}const o=H(t,"#cfg-usage-check-interval");if(o&&!s.has("usage_check_interval")){const p=o.value.trim();if(!pe.test(p))return{error:"usage_check_interval must be a duration like 30s, 5m, or 2h",changed:!1};p!==n.usage_check_interval&&(i.usage_check_interval=p)}const d=H(t,"#cfg-disable-polling");d&&!s.has("disable_usage_polling")&&d.checked!==n.disable_usage_polling&&(i.disable_usage_polling=d.checked);const v=H(t,"#cfg-sanitize-role");if(v&&!s.has("sanitize_developer_role")&&v.checked!==n.sanitize_developer_role&&(i.sanitize_developer_role=v.checked),Object.keys(i).length>0&&(a.settings=i),!s.has("model_aliases")){const p={},w=new Set;for(const $ of t.querySelectorAll("[data-alias-row]")){const h=$.querySelector("[data-alias-from]")?.value.trim()??"",S=$.querySelector("[data-alias-to]")?.value.trim()??"",y=$.dataset.aliasOriginal??"";if(!h){y&&(p[y]=null);continue}if(w.has(h))return{error:`duplicate alias ${h}`,changed:!1};if(w.add(h),y&&y!==h&&(p[y]=null),!S){e.model_aliases[h]!==void 0&&(p[h]=null);continue}e.model_aliases[h]!==S&&(p[h]=S)}Object.keys(p).length>0&&(a.model_aliases=p)}if(!s.has("keys")){const p=[];for(const w of t.querySelectorAll("[data-key-row]")){const $=w.querySelector("[data-key-id]")?.value??"",h=Number(w.querySelector("[data-key-priority]")?.value),S=Number(w.querySelector("[data-key-weight]")?.value);if(!Number.isInteger(h)||h<1)return{error:"key priority must be >= 1",changed:!1};if(!Number.isInteger(S)||S<1)return{error:"key weight must be >= 1",changed:!1};if($===""){const y=w.querySelector("[data-key-value]")?.value.trim()??"";if(!y)return{error:"new keys need a value",changed:!1};p.push({key:y,priority:h,weight:S})}else{const y={id:Number($),priority:h,weight:S},U=w.querySelector("[data-key-rotate]")?.value.trim();U&&(y.key=U),p.push(y)}}if(p.length===0)return{error:"at least one upstream key is required",changed:!1};a.keys=p}return a.settings!==void 0||a.model_aliases!==void 0||a.keys!==void 0?{patch:a,changed:!0}:{changed:!1}}function Z(e,t,s,n,a=!1){const i=s?s.average_percent:t.percent,f=Math.min(100,Math.max(0,i)),u=oe(f,95),o=s?`${D(s.min_percent)} – ${D(s.max_percent)}`:"—",d=s?`${Math.round(s.total_remaining_percent)}%`:"—",v=n??t.resetsAt,m=v?re(v):"—",p=a?"color: var(--color-accent-400);":"color: var(--color-neutral-400);";return`
    <div>
      <div style="display:flex;align-items:baseline;gap:var(--space-2)">
        <span class="num big">${D(i)}</span>
        <span class="num nw" style="font-size:12px;color:var(--color-neutral-500)">avg used</span>
        <span class="kicker" style="margin-left:auto;font-size:10.5px;color:var(--color-neutral-500)">${c(e)}</span>
      </div>
      <div class="bar-track" style="margin:var(--space-4) 0 var(--space-3)">
        <div class="bar-fill ${u}" style="width:${Math.max(1,f)}%"></div>
      </div>
      <div class="num nw" style="font-size:12px;color:var(--color-neutral-500);display:flex;justify-content:space-between;gap:var(--space-3);flex-wrap:wrap">
        <span>min–max ${c(o)}</span>
        <span>remaining ${c(d)}</span>
      </div>
      <div class="num" style="font-size:13px;${p}margin-top:var(--space-3);white-space:nowrap">
        earliest reset ${v?`in <span data-countdown="${c(v)}">${c(m)}</span>`:"—"}
      </div>
    </div>
  `}function Re(e,t){const s=e.summary,n=s.pool_usage,a=s.total_keys,i=s.available_keys,f=s.exhausted_keys,u=s.active_sessions,o=t?t.key_exhaustions.reduce((v,m)=>v+m.count,0):0,d=t?t.key_switches.reduce((v,m)=>v+m.count,0):0;return`
    <div class="pool-header">
      <span class="kicker">Pool usage · averaged across ${a} ${a===1?"key":"keys"}</span>
      <span class="num" style="font-size:11.5px;color:var(--color-neutral-600);letter-spacing:.04em;white-space:nowrap">
        warn ≥ 70% · critical ≥ 95%
      </span>
    </div>

    <div class="pool">
      ${Z("5-hour",e.rolling,n?.rolling,n?.rolling?.earliest_reset_at??e.rolling.resetsAt,!0)}
      ${Z("Weekly",e.weekly,n?.weekly,n?.weekly?.earliest_reset_at??e.weekly.resetsAt,!1)}
      ${Z("Monthly",e.monthly,n?.monthly,n?.monthly?.earliest_reset_at??e.monthly.resetsAt,!1)}
    </div>

    <div class="summary-line num">
      <span><span style="color:var(--color-neutral-100)">${a}</span> ${a===1?"key":"keys"} · ${i} available · ${f} exhausted</span>
      <span><span style="color:var(--color-neutral-100)">${u}</span> active sessions</span>
      <span><span style="color:var(--color-neutral-100)">${o}</span> exhaustions · ${d} switches</span>
      <span class="summary-legend">
        <span class="legend-item"><span class="legend-pill" style="background:var(--color-neutral-400)"></span>ok</span>
        <span class="legend-item"><span class="legend-pill" style="background:var(--color-accent-500)"></span>warn</span>
        <span class="legend-item"><span class="legend-pill" style="background:var(--color-accent-300)"></span>critical</span>
      </span>
    </div>
  `}function X(e,t,s){const n=Math.min(100,Math.max(0,t.percent)),a=oe(n,s),i=t.resetsAt,f=i?re(i):"—";return`
    <div style="flex:1">
      <div class="num klab">
        <span>${c(e)}</span>
        <span style="color:var(--color-neutral-300)">${D(n)}</span>
      </div>
      <div style="height:3px;background:rgba(243,242,242,.14);margin-top:7px;border-radius:1px;overflow:hidden">
        <div class="bar-fill ${a}" style="width:${Math.max(1,n)}%;height:3px"></div>
      </div>
      <div class="num" style="font-size:10.5px;color:var(--color-neutral-600);margin-top:6px">
        ${i?`<span data-countdown="${c(i)}">${c(f)}</span>`:"—"}
      </div>
    </div>
  `}function Ue(e,t,s){const n=e.current,a=e.state==="available"?"tag-available":e.state==="exhausted"?"tag-exhausted":"tag-unknown",i=e.retry_after_seconds!==void 0?`<span style="color:var(--status-bad)" data-retry="${e.retry_after_seconds}">${e.retry_after_seconds}s</span>`:"—",f=e.eligible?`<span style="color:var(--color-accent-400);display:flex">
         <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="display:block">
           <path d="M20 6 9 17l-5-5"></path>
         </svg>
       </span>`:'<span style="color:var(--status-bad);display:flex;font-size:12px;line-height:1">✕</span>',u=n?`<span style="color:var(--color-accent-400);display:flex" title="current key">
         <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="display:block">
           <path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z"></path>
         </svg>
       </span>`:"",o=s===e.index;return`
    <div class="key-card ${n?"current":""}" title="${c(e.error??"")}">
      <div class="key-card-header">
        <span class="num" style="font-size:11.5px;color:var(--color-neutral-600);width:12px">${e.index}</span>
        ${u}
        <span class="num key-hint">${c(e.key_hint??"…")}</span>
        <span class="num tag ${a}">${c(e.state)}</span>
        <div class="menu-container" style="margin-left:auto;">
          <button type="button" class="iconbtn sm" data-key-menu="${e.index}" aria-label="Key ${e.index} actions" title="Key ${e.index} actions">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="display:block">
              <circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none"></circle>
              <circle cx="19" cy="12" r="1.4" fill="currentColor" stroke="none"></circle>
              <circle cx="5" cy="12" r="1.4" fill="currentColor" stroke="none"></circle>
            </svg>
          </button>
          <div class="dropdown-menu" id="key-menu-${e.index}" ${o?"":"hidden"}>
            <div class="menuitem accent-item" data-action="reset-key" data-index="${e.index}">Reset key quota</div>
          </div>
        </div>
      </div>

      <div class="num key-meta-line">
        <span>pri ${e.priority} / w ${e.weight}</span>
        <span style="display:inline-flex;align-items:center;gap:5px">eligible ${f}</span>
        <span>retry ${i}</span>
        <span title="${c(e.last_checked_at??"")}">checked ${c(Oe(e.last_checked_at))}</span>
      </div>

      <div class="krow">
        ${X("5-HOUR",e.rolling,t)}
        ${X("WEEKLY",e.weekly,t)}
        ${X("MONTHLY",e.monthly,t)}
      </div>
    </div>
  `}function se(e,t=null){const s=e.summary.proactive_threshold_percent||95;return`
    <div>
      <div class="kicker" style="margin-bottom:var(--space-4)">Keys</div>
      ${e.keys.map(a=>Ue(a,s,t)).join("")||'<p class="text-muted" style="font-size:13px;">No keys configured.</p>'}
    </div>
  `}function We(e,t){return e?.keys.find(n=>n.index===t)?.key_hint??`key #${t}`}function Be(e,t){const s=new Map;for(const o of e.http_requests){const d=`${o.method} ${o.endpoint}`;let v=s.get(d);v||(v={method:o.method,endpoint:o.endpoint,totalCount:0,errors:[]},s.set(d,v)),v.totalCount+=o.count,o.status!==200&&v.errors.push({status:o.status,count:o.count})}const n=new Map;for(const o of e.http_durations)n.set(`${o.method} ${o.endpoint}`,{sum:o.duration_seconds_sum,count:o.duration_seconds_count});const a=Array.from(s.values()).sort((o,d)=>d.totalCount-o.totalCount),i=[];for(const o of a){const d=n.get(`${o.method} ${o.endpoint}`),v=d&&d.count>0?` · ${ue(d.sum,d.count)}`:"";i.push(`
      <div class="traffic-row">
        <span>${c(o.method)} ${c(o.endpoint)}</span>
        <span style="color:var(--color-neutral-200);white-space:nowrap">${o.totalCount}${v}</span>
      </div>
    `);for(const m of o.errors)i.push(`
        <div class="traffic-row">
          <span style="color:var(--color-accent-300);padding-left:12px">└ ${m.status}</span>
          <span style="color:var(--color-neutral-300)">${m.count}</span>
        </div>
      `)}const f=[...e.upstream_requests].sort((o,d)=>o.key_index-d.key_index||d.count-o.count);for(const o of f){const d=We(t,o.key_index),v=o.duration_seconds_count>0?` · ${ue(o.duration_seconds_sum,o.duration_seconds_count)}`:"",m=o.status!==200?` (${o.status})`:"";i.push(`
      <div class="traffic-row">
        <span>upstream · ${c(d)} pri ${o.priority}${m}</span>
        <span style="color:var(--color-neutral-200);white-space:nowrap">${o.count}${v}</span>
      </div>
    `)}return`
    <div>
      <div class="kicker" style="margin-bottom:var(--space-3)">Traffic · API routes</div>
      <div class="num traffic-lines">
        ${i.length>0?i.join(""):'<div class="text-muted">No requests recorded yet.</div>'}
      </div>
    </div>
  `}function Fe(e,t){const s=e.summary,a=(t||window.location.host||"127.0.0.1:8495").replace(/^https?:\/\//,""),i=s.routing_strategy||"session_sticky",f=s.proactive_threshold_percent||95;return`${c(a)} · ${c(i)} · proactive threshold ${f}%`}const Je=["rolling","weekly","monthly"],Ge={rolling:"5-hour",weekly:"Weekly",monthly:"Monthly"};function B(e,t,s,n){if(!e.enabled)return"";if(e.error&&e.workspaces.length===0)return`
      <div>
        <div class="kicker" style="margin-bottom:var(--space-4)">Workspace</div>
        <div class="banner">Workspace telemetry error: ${c(e.error)}</div>
      </div>
    `;const a=e.workspaces;if(a.length===0)return`
      <div>
        <div class="kicker" style="margin-bottom:var(--space-4)">Workspace</div>
        <p class="text-muted" style="font-size:13px;">No workspace telemetry configured or recorded.</p>
      </div>
    `;const i=a.find(g=>g.id===t)??a[0],f=i.id,u=a.map(g=>{const M=g.id===f;return`
        <label class="segl num ${M?"active":""}" data-ws-id="${c(g.id)}">
          <input type="radio" name="ws-select" ${M?"checked":""} />
          ${c(g.name)}
        </label>
      `}).join(""),o=Je.map(g=>{const M=g===s;return`
      <label class="segl num ${M?"active":""}" data-win-key="${g}">
        <input type="radio" name="win-select" ${M?"checked":""} />
        ${Ge[g]}
      </label>
    `}).join("");if(i.error)return`
      <div>
        <div class="ws-head-controls">
          <span class="kicker">Workspace</span>
          <div class="seg-group">${u}</div>
        </div>
        <div class="banner">Workspace ${c(i.name)}: ${c(i.error)}</div>
      </div>
    `;const d=i.windows[s];if(!d)return`
      <div>
        <div class="ws-head-controls">
          <span class="kicker">Workspace</span>
          <div class="seg-group">${u}</div>
          <div class="seg-group" style="margin-left:auto">${o}</div>
        </div>
        <p class="text-muted" style="font-size:13px;">No data recorded for this window.</p>
      </div>
    `;const v=K(d.usage_usd),m=K(d.limit_usd),p=d.usage_percent,w=K(Math.max(0,d.limit_usd-d.usage_usd)),$=oe(p,95),h=Math.min(100,Math.max(.8,p)),S=ie(d.reset_in_sec),y=d.rows||[],U=y.length>0?`${y.length} models charged`:"no model usage recorded",be=y.length&&Math.max(...y.map(g=>g.contribution_percent))||1,V=y.slice(8),we=V.length>0,ke=(n?y:y.slice(0,8)).map(g=>{const M=g.multiplier&&g.multiplier!==1?`${g.multiplier}×`:"",de=g.estimated?" (est.)":"",Me=Math.max(1.5,g.contribution_percent/be*100),Le=K(Math.max(0,d.limit_usd-g.quota_cost));return`
        <div class="mrow data-row">
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-neutral-200)">
            ${c(g.name)}
            ${M?`<span class="num" style="font-size:11px;color:var(--color-accent-400);margin-left:4px;">${c(M)}</span>`:""}
            ${de?`<span class="num" style="font-size:11px;color:var(--color-neutral-600);margin-left:2px;">${c(de)}</span>`:""}
          </span>
          <span style="display:flex;align-items:center;gap:var(--space-2)">
            <span style="flex:1;height:3px;background:rgba(243,242,242,.14);display:block;overflow:hidden">
              <span style="display:block;height:3px;background:var(--color-accent-400);width:${Me}%"></span>
            </span>
            <span class="num" style="font-size:12.5px;color:var(--color-neutral-200);width:44px;text-align:right">
              ${D(g.contribution_percent)}
            </span>
          </span>
          <span class="num" style="text-align:right;color:var(--color-neutral-500)">${K(g.cost)}</span>
          <span class="num" style="text-align:right;color:var(--color-neutral-200)">${K(g.quota_cost)}</span>
          <span class="num" style="text-align:right;color:var(--color-accent-300)">${Le}</span>
        </div>
      `}).join(""),$e=V.reduce((g,M)=>g+M.quota_cost,0),xe=n?"Show top 8 only":`Show ${V.length} more · ${K($e)} quota cost`,_e=n?"M18 15l-6-6-6 6":"M6 9l6 6 6-6",Se=y.length===0?"No model usage recorded in this window.":`${n?y.length:Math.min(8,y.length)} of ${y.length} models shown.<br>Available = window limit − that model's quota cost.`;return`
    <div>
      <div class="ws-head-controls">
        <span class="kicker">Workspace</span>
        <div class="seg-group" id="ws-tabs-group">${u}</div>
        <div class="seg-group" id="win-tabs-group" style="margin-left:auto">${o}</div>
      </div>

      <div class="ws-big-stats">
        <span class="num ws-big-amount">${v}</span>
        <span class="num" style="font-size:13px;color:var(--color-neutral-500)">of ${m} · ${D(p)} used</span>
        <span class="num" style="margin-left:auto;font-size:13px;color:var(--color-accent-300);white-space:nowrap">${w} available</span>
      </div>

      <div class="bar-track" style="margin:var(--space-3) 0 var(--space-3)">
        <div class="bar-fill ${$}" style="width:${h}%"></div>
      </div>

      <div class="num" style="font-size:12px;color:var(--color-neutral-500)">
        Resets in ${S} · status ${c(d.status||"ok")} · ${U}
      </div>

      <div class="mrow header-row kicker">
        <span>Model</span>
        <span>Contribution</span>
        <span style="text-align:right">Cost</span>
        <span style="text-align:right">Quota cost</span>
        <span style="text-align:right">Available</span>
      </div>

      <div id="ws-model-rows">${ke}</div>

      ${we?`
        <button type="button" id="toggle-models-btn" class="num btn-outline-gold">
          <span>${c(xe)}</span>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display:block">
            <path d="${_e}"></path>
          </svg>
        </button>
      `:""}

      <div class="num" style="font-size:12px;color:var(--color-neutral-500);margin-top:var(--space-3);line-height:1.7">
        ${Se}
      </div>
    </div>
  `}const T=window.__SWB_CONFIG__?.apiKey??"",me="sb_base_url",G="sb_proxy_key",F="sb_interval_ms";function Ye(){const e=localStorage.getItem(F),t=e===null?3e4:Number(e);return{baseUrl:localStorage.getItem(me)??"",apiKey:localStorage.getItem(G)??"",intervalMs:Number.isFinite(t)&&t>=0?t:3e4}}let l=Ye(),j,W=!1,L=null,N=null,k=null,C=null,O=null,z="monthly",q=!1,P=null;const r=e=>{const t=document.querySelector(e);if(!t)throw new Error(`missing element ${e}`);return t};function x(e){const t=r("#error-banner");e?(t.textContent=e,t.hidden=!1):t.hidden=!0}function b(e){const t=r("#toast");t.textContent=e,t.classList.add("show"),window.setTimeout(()=>t.classList.remove("show"),3e3)}function J(){const e=l.intervalMs>0,t=r("#label-auto-on"),s=r("#label-auto-off"),n=r("#radio-auto-on"),a=r("#radio-auto-off"),i=r("#auto-label-text");if(e){t.classList.add("active"),s.classList.remove("active"),n.checked=!0,a.checked=!1;const f=Math.round(l.intervalMs/1e3);i.textContent=`Auto ${f}s`}else t.classList.remove("active"),s.classList.add("active"),n.checked=!1,a.checked=!0}function R(){j!==void 0&&(window.clearInterval(j),j=void 0),l.intervalMs>0&&l.apiKey&&(j=window.setInterval(()=>{A(!1)},l.intervalMs))}async function A(e,t=!1){if(W)return;if(!l.apiKey){x("Set your proxy API key in Settings to load usage data.");return}W=!0;const s=r("#refresh-btn");s.classList.add("loading");try{const[n,a,i]=await Promise.all([Ne(l.baseUrl,l.apiKey,e),Ce(l.baseUrl),Pe(l.baseUrl,l.apiKey)]);L=n,N=a,k=i,x(null);const u=new Date().toISOString().slice(11,19)+"Z";r("#updated-label").textContent=`updated ${u}`,Qe()}catch(n){if(n instanceof _&&n.status===401){if(l.apiKey!==""&&l.apiKey!==T&&T&&!t)return localStorage.removeItem(G),l.apiKey=T,r("#proxy-key").value=T,W=!1,s.classList.remove("loading"),A(e,!0);x("Invalid proxy API key — update it in Settings."),ce()}else n instanceof _?x(`Proxy request failed: ${n.message}`):x(`Cannot reach proxy at ${l.baseUrl||window.location.origin}: ${String(n)}`)}finally{W=!1,s.classList.remove("loading")}}function Qe(){if(!L)return;r("#server-info").innerHTML=Fe(L,l.baseUrl),r("#pool-section").innerHTML=Re(L,N);const e=r("#workspace-section");k&&k.enabled&&(k.workspaces.length>0||k.error)?(e.hidden=!1,e.innerHTML=B(k,O,z,q)):(e.hidden=!0,e.innerHTML=""),r("#keys-section").innerHTML=se(L,P),N&&(r("#traffic-section").innerHTML=Be(N,L));const t=Object.entries(N?.model_aliases??{}),s=t.map(([f,u])=>`${c(f)} → ${c(u)}`).join(" · "),n=t.length>0?`${t.length} aliases · ${s}`:"",a=`${l.baseUrl||""}/metrics`,i=N?.generated_at?N.generated_at.slice(11,19)+"Z":"—";r("#footer-section").innerHTML=`
    <div class="num" style="display:flex;justify-content:space-between;gap:var(--space-6);font-size:11.5px;color:var(--color-neutral-600);flex-wrap:wrap;line-height:1.9">
      <span>${n}</span>
      <span>snapshot ${c(i)} · <a href="#" id="open-raw-json-link">raw JSON</a> · <a href="${c(a)}" target="_blank" rel="noopener">/metrics</a></span>
    </div>
  `}function Ve(){const e=Date.now();for(const t of document.querySelectorAll("[data-countdown]"))t.textContent=re(t.dataset.countdown,e);for(const t of document.querySelectorAll("[data-retry]")){const s=Number(t.dataset.retry??0);Number.isFinite(s)&&s>0&&(t.dataset.retry=String(s-1),t.textContent=`${s-1}s`)}}async function le(){const e=r("#proxy-config-body");e.innerHTML='<p class="text-muted">Loading proxy configuration…</p>';try{if(C=await je(l.baseUrl,l.apiKey),e.innerHTML=he(C),r("#proxy-config-source").textContent=C.config_source==="none"?"no config file":C.config_source,r("#proxy-config-save").disabled=!C.editable,!C.editable){for(const t of e.querySelectorAll("input, select, button"))t.disabled=!0;e.insertAdjacentHTML("afterbegin",'<p class="text-muted cfg-note">No writable config file found. Set SWITCHBOARD_GO_CONFIG or create a config file, then reload.</p>')}}catch(t){e.innerHTML=`<p class="text-muted">Could not load proxy configuration: ${c(String(t))}</p>`,r("#proxy-config-save").disabled=!0}}function Ze(){const e=r("#proxy-config-dialog");e.open||(typeof e.showModal=="function"?e.showModal():e.setAttribute("open","")),le()}function fe(){const e=r("#proxy-config-dialog");typeof e.close=="function"?e.close():e.removeAttribute("open")}async function Xe(){if(!C)return;const e=r("#proxy-config-body"),t=He(C,e);if(t.error){x(t.error);return}if(!t.changed||!t.patch){b("No configuration changes to apply");return}try{const s=await Ee(l.baseUrl,l.apiKey,t.patch);C=s,e.innerHTML=he(s),r("#proxy-config-source").textContent=s.config_source==="none"?"no config file":s.config_source,x(null),b("Proxy configuration applied"),await A(!1)}catch(s){if(s instanceof _&&s.status===409){b("Configuration changed elsewhere — reloading"),await le();return}s instanceof _?x(`Configuration update failed: ${s.message}`):x(`Configuration update failed: ${String(s)}`)}}function ce(){const e=r("#settings-dialog");r("#base-url").value=l.baseUrl,r("#proxy-key").value=l.apiKey,r("#poll-interval").value=String(l.intervalMs),typeof e.showModal=="function"?e.showModal():e.setAttribute("open","")}function ee(){const e=r("#settings-dialog");typeof e.close=="function"?e.close():e.removeAttribute("open")}function et(){const e=r("#raw-json-dialog"),t=r("#raw-json-content");t.textContent=N?JSON.stringify(N,null,2):"No snapshot data available.",typeof e.showModal=="function"?e.showModal():e.setAttribute("open","")}function te(){const e=r("#raw-json-dialog");typeof e.close=="function"?e.close():e.removeAttribute("open")}async function tt(e,t){if(e){r("#dropdown-menu").hidden=!0,P=null;try{switch(e){case"reset-key":{const s=Number(t);if(!Number.isInteger(s))return;await Ie(l.baseUrl,l.apiKey,s),b(`Key ${s} reset`);break}case"reset-all":{if(!window.confirm("Reset all keys and quota limits?"))return;await Te(l.baseUrl,l.apiKey),b("All keys reset");break}case"reload":{if(!window.confirm("Reload configuration from disk?"))return;await Ke(l.baseUrl,l.apiKey),b("Configuration reloaded");break}case"validate":{const s=await Ae(l.baseUrl,l.apiKey),n=s.results.filter(a=>a.state==="exhausted").length;b(`Validated ${s.results.length} keys (${n} exhausted)`);break}case"proxy-config":{Ze();return}case"settings":{ce();return}default:return}await A(!1)}catch(s){s instanceof _&&s.status===401?x("Invalid proxy API key — update it in Settings."):b(`Action failed: ${String(s)}`)}}}function st(){r("#label-auto-on").addEventListener("click",e=>{e.preventDefault(),l.intervalMs<=0&&(l.intervalMs=3e4,localStorage.setItem(F,String(l.intervalMs))),J(),R(),A(!1)}),r("#label-auto-off").addEventListener("click",e=>{e.preventDefault(),l.intervalMs=0,localStorage.setItem(F,"0"),J(),R(),b("Auto-refresh paused (Hold)")}),r("#refresh-btn").addEventListener("click",()=>{A(!0)}),r("#menu-btn").addEventListener("click",e=>{e.stopPropagation();const t=r("#dropdown-menu");t.hidden=!t.hidden}),r("#settings-form").addEventListener("submit",e=>{e.preventDefault();const t=r("#base-url").value,s=r("#proxy-key").value,n=Number(r("#poll-interval").value);l={baseUrl:t,apiKey:s,intervalMs:Number.isFinite(n)&&n>=0?n:3e4},localStorage.setItem(me,l.baseUrl),localStorage.setItem(G,l.apiKey),localStorage.setItem(F,String(l.intervalMs)),ee(),J(),R(),b("Settings saved"),A(!1)}),r("#clear-key").addEventListener("click",()=>{localStorage.removeItem(G),l.apiKey=T,r("#proxy-key").value=T,b("Proxy key override cleared")}),r("#settings-close-btn").addEventListener("click",ee),r("#settings-backdrop").addEventListener("click",e=>{e.target===e.currentTarget&&ee()}),r("#proxy-config-form").addEventListener("submit",e=>{e.preventDefault(),Xe()}),r("#proxy-config-close-btn").addEventListener("click",fe),r("#proxy-config-refresh-btn").addEventListener("click",()=>{le()}),r("#proxy-config-backdrop").addEventListener("click",e=>{e.target===e.currentTarget&&fe()}),r("#raw-json-close-btn").addEventListener("click",te),r("#raw-json-done-btn").addEventListener("click",te),r("#raw-json-backdrop").addEventListener("click",e=>{e.target===e.currentTarget&&te()}),r("#raw-json-copy-btn").addEventListener("click",async()=>{const e=r("#raw-json-content").textContent??"";try{await navigator.clipboard.writeText(e),b("Copied JSON to clipboard")}catch{b("Failed to copy JSON")}}),document.addEventListener("click",e=>{const t=e.target;if(t.closest("#open-raw-json-link")){e.preventDefault(),et();return}const s=t.closest("[data-action]");if(s){const o=s.dataset.action,d=s.dataset.index;tt(o,d);return}const n=t.closest("[data-key-menu]");if(n){e.stopPropagation();const o=Number(n.dataset.keyMenu);P=P===o?null:o,L&&(r("#keys-section").innerHTML=se(L,P));return}const a=t.closest("[data-ws-id]");if(a){const o=a.dataset.wsId??null;o&&o!==O&&(O=o,k&&(r("#workspace-section").innerHTML=B(k,O,z,q)));return}const i=t.closest("[data-win-key]");if(i){const o=i.dataset.winKey;o&&o!==z&&(z=o,k&&(r("#workspace-section").innerHTML=B(k,O,z,q)));return}if(t.closest("#toggle-models-btn")){q=!q,k&&(r("#workspace-section").innerHTML=B(k,O,z,q));return}if(t.closest("[data-add-key]")){r("#proxy-config-body [data-key-list]")?.insertAdjacentHTML("beforeend",qe());return}if(t.closest("[data-remove-key]")){t.closest("[data-key-row]")?.remove();return}if(t.closest("[data-add-alias]")){r("#proxy-config-body [data-alias-list]")?.insertAdjacentHTML("beforeend",De());return}if(t.closest("[data-remove-alias]")){t.closest("[data-alias-row]")?.remove();return}const u=r("#dropdown-menu");!u.hidden&&!t.closest("#menu-btn")&&!t.closest("#dropdown-menu")&&(u.hidden=!0),P!==null&&!t.closest(".menu-container")&&(P=null,L&&(r("#keys-section").innerHTML=se(L,null)))}),document.addEventListener("visibilitychange",()=>{document.hidden?j!==void 0&&(window.clearInterval(j),j=void 0):(R(),A(!1))})}function nt(){J(),st(),window.setInterval(Ve,1e3),!l.apiKey&&T&&(l.apiKey=T),l.apiKey?A(!1):(ce(),x("Set your proxy API key in Settings to load usage data.")),R()}nt();
