(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const a of document.querySelectorAll('link[rel="modulepreload"]'))n(a);new MutationObserver(a=>{for(const r of a)if(r.type==="childList")for(const u of r.addedNodes)u.tagName==="LINK"&&u.rel==="modulepreload"&&n(u)}).observe(document,{childList:!0,subtree:!0});function s(a){const r={};return a.integrity&&(r.integrity=a.integrity),a.referrerPolicy&&(r.referrerPolicy=a.referrerPolicy),a.crossOrigin==="use-credentials"?r.credentials="include":a.crossOrigin==="anonymous"?r.credentials="omit":r.credentials="same-origin",r}function n(a){if(a.ep)return;a.ep=!0;const r=s(a);fetch(a.href,r)}})();class N extends Error{status;constructor(t,s){super(s),this.status=t}}function ee(e){return e.trim().replace(/\/+$/,"")}function te(e){return e?{Authorization:`Bearer ${e}`}:{}}async function J(e,t,s){const n=await fetch(`${ee(e)}${t}`,{headers:te(s)});if(n.status===401)throw new N(401,"Invalid or missing proxy API key");if(!n.ok)throw new N(n.status,`Request failed with status ${n.status}`);return await n.json()}async function U(e,t,s,n){const a=await fetch(`${ee(e)}${t}`,{method:"POST",headers:{...te(s),"Content-Type":"application/json"},body:n===void 0?void 0:JSON.stringify(n)});if(a.status===401)throw new N(401,"Invalid or missing proxy API key");if(!a.ok)throw new N(a.status,`Request failed with status ${a.status}`);return await a.json()}async function ge(e,t,s=!1){return J(e,s?"/usage?refresh=true":"/usage",t)}async function he(e){return J(e,"/dashboard/api/metrics.json","")}async function we(e,t){return U(e,"/admin/validate-keys",t)}async function $e(e,t,s){return U(e,"/admin/reset-key",t,{index:s})}async function be(e,t){return U(e,"/admin/reset-all-keys",t)}async function ke(e,t){await U(e,"/admin/reload",t)}async function xe(e,t){return J(e,"/admin/workspace-usage",t)}function c(e){return e.replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;").replace(/'/g,"&#39;")}function A(e){return e===void 0||Number.isNaN(e)?"0.0%":`${e.toFixed(1)}%`}function _(e){return e===void 0||Number.isNaN(e)?"$0.00":"$"+e.toFixed(2)}function Y(e,t=95){return e===void 0||Number.isNaN(e)?"ok":e>=t?"critical":e>=70?"warn":"ok"}function Q(e){const t=Math.max(0,Math.floor(e)),s=Math.floor(t/86400),n=Math.floor(t%86400/3600),a=Math.floor(t%3600/60),r=t%60;return s>0?`${s}d ${n}h ${a}m`:n>0?`${n}h ${a}m`:a>0?`${a}m ${r}s`:`${r}s`}function V(e,t=Date.now()){if(!e)return"—";const s=Date.parse(e);if(Number.isNaN(s))return"—";const n=(s-t)/1e3;return n<=0?"reset due":Q(n)}function _e(e,t=Date.now()){if(!e)return"never";const s=Date.parse(e);if(Number.isNaN(s))return"—";const n=(t-s)/1e3;return n<0||n<60?"just now":`${Q(n)} ago`}function X(e,t){return t<=0?"—":`${Math.round(e/t*1e3).toLocaleString("en-US")} ms`}function H(e,t,s,n,a=!1){const r=s?s.average_percent:t.percent,u=Math.min(100,Math.max(0,r)),v=Y(u,95),o=s?`${A(s.min_percent)} – ${A(s.max_percent)}`:"—",d=s?`${Math.round(s.total_remaining_percent)}%`:"—",f=n??t.resetsAt,y=f?V(f):"—",T=a?"color: var(--color-accent-400);":"color: var(--color-neutral-400);";return`
    <div>
      <div style="display:flex;align-items:baseline;gap:var(--space-2)">
        <span class="num big">${A(r)}</span>
        <span class="num nw" style="font-size:12px;color:var(--color-neutral-500)">avg used</span>
        <span class="kicker" style="margin-left:auto;font-size:10.5px;color:var(--color-neutral-500)">${c(e)}</span>
      </div>
      <div class="bar-track" style="margin:var(--space-4) 0 var(--space-3)">
        <div class="bar-fill ${v}" style="width:${Math.max(1,u)}%"></div>
      </div>
      <div class="num nw" style="font-size:12px;color:var(--color-neutral-500);display:flex;justify-content:space-between;gap:var(--space-3);flex-wrap:wrap">
        <span>min–max ${c(o)}</span>
        <span>remaining ${c(d)}</span>
      </div>
      <div class="num" style="font-size:13px;${T}margin-top:var(--space-3);white-space:nowrap">
        earliest reset ${f?`in <span data-countdown="${c(f)}">${c(y)}</span>`:"—"}
      </div>
    </div>
  `}function Me(e,t){const s=e.summary,n=s.pool_usage,a=s.total_keys,r=s.available_keys,u=s.exhausted_keys,v=s.active_sessions,o=t?t.key_exhaustions.reduce((f,y)=>f+y.count,0):0,d=t?t.key_switches.reduce((f,y)=>f+y.count,0):0;return`
    <div class="pool-header">
      <span class="kicker">Pool usage · averaged across ${a} ${a===1?"key":"keys"}</span>
      <span class="num" style="font-size:11.5px;color:var(--color-neutral-600);letter-spacing:.04em;white-space:nowrap">
        warn ≥ 70% · critical ≥ 95%
      </span>
    </div>

    <div class="pool">
      ${H("5-hour",e.rolling,n?.rolling,n?.rolling?.earliest_reset_at??e.rolling.resetsAt,!0)}
      ${H("Weekly",e.weekly,n?.weekly,n?.weekly?.earliest_reset_at??e.weekly.resetsAt,!1)}
      ${H("Monthly",e.monthly,n?.monthly,n?.monthly?.earliest_reset_at??e.monthly.resetsAt,!1)}
    </div>

    <div class="summary-line num">
      <span><span style="color:var(--color-neutral-100)">${a}</span> ${a===1?"key":"keys"} · ${r} available · ${u} exhausted</span>
      <span><span style="color:var(--color-neutral-100)">${v}</span> active sessions</span>
      <span><span style="color:var(--color-neutral-100)">${o}</span> exhaustions · ${d} switches</span>
      <span class="summary-legend">
        <span class="legend-item"><span class="legend-pill" style="background:var(--color-neutral-400)"></span>ok</span>
        <span class="legend-item"><span class="legend-pill" style="background:var(--color-accent-500)"></span>warn</span>
        <span class="legend-item"><span class="legend-pill" style="background:var(--color-accent-300)"></span>critical</span>
      </span>
    </div>
  `}function q(e,t,s){const n=Math.min(100,Math.max(0,t.percent)),a=Y(n,s),r=t.resetsAt,u=r?V(r):"—";return`
    <div style="flex:1">
      <div class="num klab">
        <span>${c(e)}</span>
        <span style="color:var(--color-neutral-300)">${A(n)}</span>
      </div>
      <div style="height:3px;background:rgba(243,242,242,.14);margin-top:7px;border-radius:1px;overflow:hidden">
        <div class="bar-fill ${a}" style="width:${Math.max(1,n)}%;height:3px"></div>
      </div>
      <div class="num" style="font-size:10.5px;color:var(--color-neutral-600);margin-top:6px">
        ${r?`<span data-countdown="${c(r)}">${c(u)}</span>`:"—"}
      </div>
    </div>
  `}function Se(e,t,s){const n=e.current,a=e.state==="available"?"tag-available":e.state==="exhausted"?"tag-exhausted":"tag-unknown",r=e.retry_after_seconds!==void 0?`<span style="color:var(--status-bad)" data-retry="${e.retry_after_seconds}">${e.retry_after_seconds}s</span>`:"—",u=e.eligible?`<span style="color:var(--color-accent-400);display:flex">
         <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="display:block">
           <path d="M20 6 9 17l-5-5"></path>
         </svg>
       </span>`:'<span style="color:var(--status-bad);display:flex;font-size:12px;line-height:1">✕</span>',v=n?`<span style="color:var(--color-accent-400);display:flex" title="current key">
         <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="display:block">
           <path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z"></path>
         </svg>
       </span>`:"",o=s===e.index;return`
    <div class="key-card ${n?"current":""}" title="${c(e.error??"")}">
      <div class="key-card-header">
        <span class="num" style="font-size:11.5px;color:var(--color-neutral-600);width:12px">${e.index}</span>
        ${v}
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
        <span style="display:inline-flex;align-items:center;gap:5px">eligible ${u}</span>
        <span>retry ${r}</span>
        <span title="${c(e.last_checked_at??"")}">checked ${c(_e(e.last_checked_at))}</span>
      </div>

      <div class="krow">
        ${q("5-HOUR",e.rolling,t)}
        ${q("WEEKLY",e.weekly,t)}
        ${q("MONTHLY",e.monthly,t)}
      </div>
    </div>
  `}function F(e,t=null){const s=e.summary.proactive_threshold_percent||95;return`
    <div>
      <div class="kicker" style="margin-bottom:var(--space-4)">Keys</div>
      ${e.keys.map(a=>Se(a,s,t)).join("")||'<p class="text-muted" style="font-size:13px;">No keys configured.</p>'}
    </div>
  `}function Le(e,t){return e?.keys.find(n=>n.index===t)?.key_hint??`key #${t}`}function Ne(e,t){const s=new Map;for(const o of e.http_requests){const d=`${o.method} ${o.endpoint}`;let f=s.get(d);f||(f={method:o.method,endpoint:o.endpoint,totalCount:0,errors:[]},s.set(d,f)),f.totalCount+=o.count,o.status!==200&&f.errors.push({status:o.status,count:o.count})}const n=new Map;for(const o of e.http_durations)n.set(`${o.method} ${o.endpoint}`,{sum:o.duration_seconds_sum,count:o.duration_seconds_count});const a=Array.from(s.values()).sort((o,d)=>d.totalCount-o.totalCount),r=[];for(const o of a){const d=n.get(`${o.method} ${o.endpoint}`),f=d&&d.count>0?` · ${X(d.sum,d.count)}`:"";r.push(`
      <div class="traffic-row">
        <span>${c(o.method)} ${c(o.endpoint)}</span>
        <span style="color:var(--color-neutral-200);white-space:nowrap">${o.totalCount}${f}</span>
      </div>
    `);for(const y of o.errors)r.push(`
        <div class="traffic-row">
          <span style="color:var(--color-accent-300);padding-left:12px">└ ${y.status}</span>
          <span style="color:var(--color-neutral-300)">${y.count}</span>
        </div>
      `)}const u=[...e.upstream_requests].sort((o,d)=>o.key_index-d.key_index||d.count-o.count);for(const o of u){const d=Le(t,o.key_index),f=o.duration_seconds_count>0?` · ${X(o.duration_seconds_sum,o.duration_seconds_count)}`:"",y=o.status!==200?` (${o.status})`:"";r.push(`
      <div class="traffic-row">
        <span>upstream · ${c(d)} pri ${o.priority}${y}</span>
        <span style="color:var(--color-neutral-200);white-space:nowrap">${o.count}${f}</span>
      </div>
    `)}return`
    <div>
      <div class="kicker" style="margin-bottom:var(--space-3)">Traffic · API routes</div>
      <div class="num traffic-lines">
        ${r.length>0?r.join(""):'<div class="text-muted">No requests recorded yet.</div>'}
      </div>
    </div>
  `}function Ie(e,t){const s=e.summary,a=(t||window.location.host||"127.0.0.1:8495").replace(/^https?:\/\//,""),r=s.routing_strategy||"session_sticky",u=s.proactive_threshold_percent||95;return`${c(a)} · ${c(r)} · proactive threshold ${u}%`}const Ce=["rolling","weekly","monthly"],Ke={rolling:"5-hour",weekly:"Weekly",monthly:"Monthly"};function O(e,t,s,n){if(!e.enabled)return"";if(e.error&&e.workspaces.length===0)return`
      <div>
        <div class="kicker" style="margin-bottom:var(--space-4)">Workspace</div>
        <div class="banner">Workspace telemetry error: ${c(e.error)}</div>
      </div>
    `;const a=e.workspaces;if(a.length===0)return`
      <div>
        <div class="kicker" style="margin-bottom:var(--space-4)">Workspace</div>
        <p class="text-muted" style="font-size:13px;">No workspace telemetry configured or recorded.</p>
      </div>
    `;const r=a.find(p=>p.id===t)??a[0],u=r.id,v=a.map(p=>{const h=p.id===u;return`
        <label class="segl num ${h?"active":""}" data-ws-id="${c(p.id)}">
          <input type="radio" name="ws-select" ${h?"checked":""} />
          ${c(p.name)}
        </label>
      `}).join(""),o=Ce.map(p=>{const h=p===s;return`
      <label class="segl num ${h?"active":""}" data-win-key="${p}">
        <input type="radio" name="win-select" ${h?"checked":""} />
        ${Ke[p]}
      </label>
    `}).join("");if(r.error)return`
      <div>
        <div class="ws-head-controls">
          <span class="kicker">Workspace</span>
          <div class="seg-group">${v}</div>
        </div>
        <div class="banner">Workspace ${c(r.name)}: ${c(r.error)}</div>
      </div>
    `;const d=r.windows[s];if(!d)return`
      <div>
        <div class="ws-head-controls">
          <span class="kicker">Workspace</span>
          <div class="seg-group">${v}</div>
          <div class="seg-group" style="margin-left:auto">${o}</div>
        </div>
        <p class="text-muted" style="font-size:13px;">No data recorded for this window.</p>
      </div>
    `;const f=_(d.usage_usd),y=_(d.limit_usd),T=d.usage_percent,ne=_(Math.max(0,d.limit_usd-d.usage_usd)),ae=Y(T,95),oe=Math.min(100,Math.max(.8,T)),re=Q(d.reset_in_sec),g=d.rows||[],ie=g.length>0?`${g.length} models charged`:"no model usage recorded",le=g.length&&Math.max(...g.map(p=>p.contribution_percent))||1,D=g.slice(8),ce=D.length>0,de=(n?g:g.slice(0,8)).map(p=>{const h=p.multiplier&&p.multiplier!==1?`${p.multiplier}×`:"",G=p.estimated?" (est.)":"",ye=Math.max(1.5,p.contribution_percent/le*100),me=_(Math.max(0,d.limit_usd-p.quota_cost));return`
        <div class="mrow data-row">
          <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-neutral-200)">
            ${c(p.name)}
            ${h?`<span class="num" style="font-size:11px;color:var(--color-accent-400);margin-left:4px;">${c(h)}</span>`:""}
            ${G?`<span class="num" style="font-size:11px;color:var(--color-neutral-600);margin-left:2px;">${c(G)}</span>`:""}
          </span>
          <span style="display:flex;align-items:center;gap:var(--space-2)">
            <span style="flex:1;height:3px;background:rgba(243,242,242,.14);display:block;overflow:hidden">
              <span style="display:block;height:3px;background:var(--color-accent-400);width:${ye}%"></span>
            </span>
            <span class="num" style="font-size:12.5px;color:var(--color-neutral-200);width:44px;text-align:right">
              ${A(p.contribution_percent)}
            </span>
          </span>
          <span class="num" style="text-align:right;color:var(--color-neutral-500)">${_(p.cost)}</span>
          <span class="num" style="text-align:right;color:var(--color-neutral-200)">${_(p.quota_cost)}</span>
          <span class="num" style="text-align:right;color:var(--color-accent-300)">${me}</span>
        </div>
      `}).join(""),ue=D.reduce((p,h)=>p+h.quota_cost,0),pe=n?"Show top 8 only":`Show ${D.length} more · ${_(ue)} quota cost`,fe=n?"M18 15l-6-6-6 6":"M6 9l6 6 6-6",ve=g.length===0?"No model usage recorded in this window.":`${n?g.length:Math.min(8,g.length)} of ${g.length} models shown.<br>Available = window limit − that model's quota cost.`;return`
    <div>
      <div class="ws-head-controls">
        <span class="kicker">Workspace</span>
        <div class="seg-group" id="ws-tabs-group">${v}</div>
        <div class="seg-group" id="win-tabs-group" style="margin-left:auto">${o}</div>
      </div>

      <div class="ws-big-stats">
        <span class="num ws-big-amount">${f}</span>
        <span class="num" style="font-size:13px;color:var(--color-neutral-500)">of ${y} · ${A(T)} used</span>
        <span class="num" style="margin-left:auto;font-size:13px;color:var(--color-accent-300);white-space:nowrap">${ne} available</span>
      </div>

      <div class="bar-track" style="margin:var(--space-3) 0 var(--space-3)">
        <div class="bar-fill ${ae}" style="width:${oe}%"></div>
      </div>

      <div class="num" style="font-size:12px;color:var(--color-neutral-500)">
        Resets in ${re} · status ${c(d.status||"ok")} · ${ie}
      </div>

      <div class="mrow header-row kicker">
        <span>Model</span>
        <span>Contribution</span>
        <span style="text-align:right">Cost</span>
        <span style="text-align:right">Quota cost</span>
        <span style="text-align:right">Available</span>
      </div>

      <div id="ws-model-rows">${de}</div>

      ${ce?`
        <button type="button" id="toggle-models-btn" class="num btn-outline-gold">
          <span>${c(pe)}</span>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display:block">
            <path d="${fe}"></path>
          </svg>
        </button>
      `:""}

      <div class="num" style="font-size:12px;color:var(--color-neutral-500);margin-top:var(--space-3);line-height:1.7">
        ${ve}
      </div>
    </div>
  `}const k=window.__SWB_CONFIG__?.apiKey??"",se="sb_base_url",z="sb_proxy_key",P="sb_interval_ms";function Ae(){const e=localStorage.getItem(P),t=e===null?3e4:Number(e);return{baseUrl:localStorage.getItem(se)??"",apiKey:localStorage.getItem(z)??"",intervalMs:Number.isFinite(t)&&t>=0?t:3e4}}let l=Ae(),L,j=!1,w=null,b=null,m=null,I=null,C="monthly",K=!1,M=null;const i=e=>{const t=document.querySelector(e);if(!t)throw new Error(`missing element ${e}`);return t};function S(e){const t=i("#error-banner");e?(t.textContent=e,t.hidden=!1):t.hidden=!0}function $(e){const t=i("#toast");t.textContent=e,t.classList.add("show"),window.setTimeout(()=>t.classList.remove("show"),3e3)}function W(){const e=l.intervalMs>0,t=i("#label-auto-on"),s=i("#label-auto-off"),n=i("#radio-auto-on"),a=i("#radio-auto-off"),r=i("#auto-label-text");if(e){t.classList.add("active"),s.classList.remove("active"),n.checked=!0,a.checked=!1;const u=Math.round(l.intervalMs/1e3);r.textContent=`Auto ${u}s`}else t.classList.remove("active"),s.classList.add("active"),n.checked=!1,a.checked=!0}function E(){L!==void 0&&(window.clearInterval(L),L=void 0),l.intervalMs>0&&l.apiKey&&(L=window.setInterval(()=>{x(!1)},l.intervalMs))}async function x(e,t=!1){if(j)return;if(!l.apiKey){S("Set your proxy API key in Settings to load usage data.");return}j=!0;const s=i("#refresh-btn");s.classList.add("loading");try{const[n,a,r]=await Promise.all([ge(l.baseUrl,l.apiKey,e),he(l.baseUrl),xe(l.baseUrl,l.apiKey)]);w=n,b=a,m=r,S(null);const v=new Date().toISOString().slice(11,19)+"Z";i("#updated-label").textContent=`updated ${v}`,Te()}catch(n){if(n instanceof N&&n.status===401){if(l.apiKey!==""&&l.apiKey!==k&&k&&!t)return localStorage.removeItem(z),l.apiKey=k,i("#proxy-key").value=k,j=!1,s.classList.remove("loading"),x(e,!0);S("Invalid proxy API key — update it in Settings."),Z()}else n instanceof N?S(`Proxy request failed: ${n.message}`):S(`Cannot reach proxy at ${l.baseUrl||window.location.origin}: ${String(n)}`)}finally{j=!1,s.classList.remove("loading")}}function Te(){if(!w)return;i("#server-info").innerHTML=Ie(w,l.baseUrl),i("#pool-section").innerHTML=Me(w,b);const e=i("#workspace-section");m&&m.enabled&&(m.workspaces.length>0||m.error)?(e.hidden=!1,e.innerHTML=O(m,I,C,K)):(e.hidden=!0,e.innerHTML=""),i("#keys-section").innerHTML=F(w,M),b&&(i("#traffic-section").innerHTML=Ne(b,w));const t=Object.entries(b?.model_aliases??{}),s=t.map(([u,v])=>`${c(u)} → ${c(v)}`).join(" · "),n=t.length>0?`${t.length} aliases · ${s}`:"",a=`${l.baseUrl||""}/metrics`,r=b?.generated_at?b.generated_at.slice(11,19)+"Z":"—";i("#footer-section").innerHTML=`
    <div class="num" style="display:flex;justify-content:space-between;gap:var(--space-6);font-size:11.5px;color:var(--color-neutral-600);flex-wrap:wrap;line-height:1.9">
      <span>${n}</span>
      <span>snapshot ${c(r)} · <a href="#" id="open-raw-json-link">raw JSON</a> · <a href="${c(a)}" target="_blank" rel="noopener">/metrics</a></span>
    </div>
  `}function Ee(){const e=Date.now();for(const t of document.querySelectorAll("[data-countdown]"))t.textContent=V(t.dataset.countdown,e);for(const t of document.querySelectorAll("[data-retry]")){const s=Number(t.dataset.retry??0);Number.isFinite(s)&&s>0&&(t.dataset.retry=String(s-1),t.textContent=`${s-1}s`)}}function Z(){const e=i("#settings-dialog");i("#base-url").value=l.baseUrl,i("#proxy-key").value=l.apiKey,i("#poll-interval").value=String(l.intervalMs),typeof e.showModal=="function"?e.showModal():e.setAttribute("open","")}function B(){const e=i("#settings-dialog");typeof e.close=="function"?e.close():e.removeAttribute("open")}function je(){const e=i("#raw-json-dialog"),t=i("#raw-json-content");t.textContent=b?JSON.stringify(b,null,2):"No snapshot data available.",typeof e.showModal=="function"?e.showModal():e.setAttribute("open","")}function R(){const e=i("#raw-json-dialog");typeof e.close=="function"?e.close():e.removeAttribute("open")}async function Oe(e,t){if(e){i("#dropdown-menu").hidden=!0,M=null;try{switch(e){case"reset-key":{const s=Number(t);if(!Number.isInteger(s))return;await $e(l.baseUrl,l.apiKey,s),$(`Key ${s} reset`);break}case"reset-all":{if(!window.confirm("Reset all keys and quota limits?"))return;await be(l.baseUrl,l.apiKey),$("All keys reset");break}case"reload":{if(!window.confirm("Reload configuration from disk?"))return;await ke(l.baseUrl,l.apiKey),$("Configuration reloaded");break}case"validate":{const s=await we(l.baseUrl,l.apiKey),n=s.results.filter(a=>a.state==="exhausted").length;$(`Validated ${s.results.length} keys (${n} exhausted)`);break}case"settings":{Z();return}default:return}await x(!1)}catch(s){s instanceof N&&s.status===401?S("Invalid proxy API key — update it in Settings."):$(`Action failed: ${String(s)}`)}}}function Pe(){i("#label-auto-on").addEventListener("click",e=>{e.preventDefault(),l.intervalMs<=0&&(l.intervalMs=3e4,localStorage.setItem(P,String(l.intervalMs))),W(),E(),x(!1)}),i("#label-auto-off").addEventListener("click",e=>{e.preventDefault(),l.intervalMs=0,localStorage.setItem(P,"0"),W(),E(),$("Auto-refresh paused (Hold)")}),i("#refresh-btn").addEventListener("click",()=>{x(!0)}),i("#menu-btn").addEventListener("click",e=>{e.stopPropagation();const t=i("#dropdown-menu");t.hidden=!t.hidden}),i("#settings-form").addEventListener("submit",e=>{e.preventDefault();const t=i("#base-url").value,s=i("#proxy-key").value,n=Number(i("#poll-interval").value);l={baseUrl:t,apiKey:s,intervalMs:Number.isFinite(n)&&n>=0?n:3e4},localStorage.setItem(se,l.baseUrl),localStorage.setItem(z,l.apiKey),localStorage.setItem(P,String(l.intervalMs)),B(),W(),E(),$("Settings saved"),x(!1)}),i("#clear-key").addEventListener("click",()=>{localStorage.removeItem(z),l.apiKey=k,i("#proxy-key").value=k,$("Proxy key override cleared")}),i("#settings-close-btn").addEventListener("click",B),i("#settings-backdrop").addEventListener("click",e=>{e.target===e.currentTarget&&B()}),i("#raw-json-close-btn").addEventListener("click",R),i("#raw-json-done-btn").addEventListener("click",R),i("#raw-json-backdrop").addEventListener("click",e=>{e.target===e.currentTarget&&R()}),i("#raw-json-copy-btn").addEventListener("click",async()=>{const e=i("#raw-json-content").textContent??"";try{await navigator.clipboard.writeText(e),$("Copied JSON to clipboard")}catch{$("Failed to copy JSON")}}),document.addEventListener("click",e=>{const t=e.target;if(t.closest("#open-raw-json-link")){e.preventDefault(),je();return}const s=t.closest("[data-action]");if(s){const o=s.dataset.action,d=s.dataset.index;Oe(o,d);return}const n=t.closest("[data-key-menu]");if(n){e.stopPropagation();const o=Number(n.dataset.keyMenu);M=M===o?null:o,w&&(i("#keys-section").innerHTML=F(w,M));return}const a=t.closest("[data-ws-id]");if(a){const o=a.dataset.wsId??null;o&&o!==I&&(I=o,m&&(i("#workspace-section").innerHTML=O(m,I,C,K)));return}const r=t.closest("[data-win-key]");if(r){const o=r.dataset.winKey;o&&o!==C&&(C=o,m&&(i("#workspace-section").innerHTML=O(m,I,C,K)));return}if(t.closest("#toggle-models-btn")){K=!K,m&&(i("#workspace-section").innerHTML=O(m,I,C,K));return}const v=i("#dropdown-menu");!v.hidden&&!t.closest("#menu-btn")&&!t.closest("#dropdown-menu")&&(v.hidden=!0),M!==null&&!t.closest(".menu-container")&&(M=null,w&&(i("#keys-section").innerHTML=F(w,null)))}),document.addEventListener("visibilitychange",()=>{document.hidden?L!==void 0&&(window.clearInterval(L),L=void 0):(E(),x(!1))})}function We(){W(),Pe(),window.setInterval(Ee,1e3),!l.apiKey&&k&&(l.apiKey=k),l.apiKey?x(!1):(Z(),S("Set your proxy API key in Settings to load usage data.")),E()}We();
