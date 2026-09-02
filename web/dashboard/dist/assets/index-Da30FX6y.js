(function(){const e=document.createElement("link").relList;if(e&&e.supports&&e.supports("modulepreload"))return;for(const n of document.querySelectorAll('link[rel="modulepreload"]'))a(n);new MutationObserver(n=>{for(const l of n)if(l.type==="childList")for(const c of l.addedNodes)c.tagName==="LINK"&&c.rel==="modulepreload"&&a(c)}).observe(document,{childList:!0,subtree:!0});function s(n){const l={};return n.integrity&&(l.integrity=n.integrity),n.referrerPolicy&&(l.referrerPolicy=n.referrerPolicy),n.crossOrigin==="use-credentials"?l.credentials="include":n.crossOrigin==="anonymous"?l.credentials="omit":l.credentials="same-origin",l}function a(n){if(n.ep)return;n.ep=!0;const l=s(n);fetch(n.href,l)}})();class v extends Error{status;constructor(e,s){super(s),this.status=e}}function P(t){return t.trim().replace(/\/+$/,"")}function O(t){return t?{Authorization:`Bearer ${t}`}:{}}async function A(t,e,s){const a=await fetch(`${P(t)}${e}`,{headers:O(s)});if(a.status===401)throw new v(401,"Invalid or missing proxy API key");if(!a.ok)throw new v(a.status,`Request failed with status ${a.status}`);return await a.json()}async function M(t,e,s,a){const n=await fetch(`${P(t)}${e}`,{method:"POST",headers:{...O(s),"Content-Type":"application/json"},body:a===void 0?void 0:JSON.stringify(a)});if(n.status===401)throw new v(401,"Invalid or missing proxy API key");if(!n.ok)throw new v(n.status,`Request failed with status ${n.status}`);return await n.json()}async function H(t,e,s=!1){return A(t,s?"/usage?refresh=true":"/usage",e)}async function F(t){return A(t,"/dashboard/api/metrics.json","")}async function B(t,e){return M(t,"/admin/validate-keys",e)}async function J(t,e,s){return M(t,"/admin/reset-key",e,{index:s})}async function V(t,e){return M(t,"/admin/reset-all-keys",e)}async function z(t,e){await M(t,"/admin/reload",e)}async function G(t,e){return A(t,"/admin/workspace-usage",e)}function r(t){return t.replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;").replace(/'/g,"&#39;")}function $(t){return t===void 0||Number.isNaN(t)?"—":`${Math.round(t*10)/10}%`}function I(t,e){return t===void 0||Number.isNaN(t)?"ok":t>=e?"critical":t>=70?"warn":"ok"}function R(t){const e=Math.max(0,Math.floor(t)),s=Math.floor(e/86400),a=Math.floor(e%86400/3600),n=Math.floor(e%3600/60),l=e%60;return s>0?`${s}d ${a}h ${n}m`:a>0?`${a}h ${n}m`:n>0?`${n}m ${l}s`:`${l}s`}function C(t,e=Date.now()){if(!t)return"—";const s=Date.parse(t);if(Number.isNaN(s))return"—";const a=(s-e)/1e3;return a<=0?"reset due":`in ${R(a)}`}function Q(t,e=Date.now()){if(!t)return"never";const s=Date.parse(t);if(Number.isNaN(s))return"—";const a=(e-s)/1e3;return a<0||a<60?"just now":`${R(a)} ago`}function Y(t){if(!t)return"—";const e=Date.parse(t);return Number.isNaN(e)?"—":new Date(e).toLocaleTimeString()}function U(t,e){return e<=0?"—":`${Math.round(t/e*1e3)}ms`}function N(t,e,s,a){const n=I(e.percent,95),l=s?`${$(s.min_percent)} – ${$(s.max_percent)}`:"—",c=s?$(s.total_remaining_percent):"—",u=a??e.resetsAt?`<span class="muted" data-countdown="${r(a??e.resetsAt??"")}">${r(C(a??e.resetsAt))}</span>`:'<span class="muted">—</span>';return`
    <div class="pool-row">
      <div class="pool-head">
        <span class="pool-name">${r(t)}</span>
        <span class="pool-avg">${$(e.percent)} avg</span>
        ${u}
      </div>
      <div class="bar"><div class="bar-fill ${n}" style="width:${Math.min(100,Math.max(0,e.percent))}%"></div></div>
      <div class="pool-sub muted small">min–max ${r(l)} · remaining ${r(c)}</div>
    </div>
  `}function X(t){const e=t.summary.pool_usage;return`
    <h2>Pool usage</h2>
    ${N("Rolling",t.rolling,e?.rolling,e?.rolling?.earliest_reset_at)}
    ${N("Weekly",t.weekly,e?.weekly)}
    ${N("Monthly",t.monthly,e?.monthly)}
  `}function Z(t){return`<span class="chip ${t.state==="available"?"ok":t.state==="exhausted"?"bad":"unknown"}">${r(t.state)}</span>`}function K(t,e){const s=I(t.percent,e),a=t.resetsAt?`<span class="muted small" data-countdown="${r(t.resetsAt)}">${r(Y(t.resetsAt))}</span>`:"";return`
    <td class="win-cell">
      <div class="bar tiny"><div class="bar-fill ${s}" style="width:${Math.min(100,Math.max(0,t.percent))}%"></div></div>
      <span class="win-pct">${$(t.percent)}</span>
      ${a}
    </td>
  `}function tt(t){return t.retry_after_seconds!==void 0?`<td><span class="chip bad" data-retry="${t.retry_after_seconds}">${t.retry_after_seconds}s</span></td>`:'<td><span class="muted">—</span></td>'}function et(t){const e=t.summary.proactive_threshold_percent||95;return`
    <div class="section-head">
      <h2>Keys</h2>
      <div class="actions">
        <button class="btn" data-action="validate">Validate keys</button>
        <button class="btn" data-action="reset-all">Reset all</button>
        <button class="btn" data-action="reload">Reload config</button>
      </div>
    </div>
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>#</th><th>Key</th><th>Pri/W</th><th>State</th><th>Elig</th>
            <th>Rolling</th><th>Weekly</th><th>Monthly</th><th>Retry</th><th>Checked</th><th></th>
          </tr>
        </thead>
        <tbody>${t.keys.map(a=>`
      <tr class="${a.current?"current-row":""}" title="${r(a.error??"")}">
        <td>${a.index}${a.current?' <span class="star" title="current key">★</span>':""}</td>
        <td class="mono">${r(a.key_hint??"…")}</td>
        <td>${a.priority} / ${a.weight}</td>
        <td>${Z(a)}</td>
        <td>${a.eligible?'<span class="ok-text">✔</span>':'<span class="bad-text">✘</span>'}</td>
        ${K(a.rolling,e)}
        ${K(a.weekly,e)}
        ${K(a.monthly,e)}
        ${tt(a)}
        <td class="muted small" title="${r(a.last_checked_at??"")}">${r(Q(a.last_checked_at))}</td>
        <td><button class="btn tiny-btn" data-action="reset-key" data-index="${a.index}">Reset</button></td>
      </tr>
    `).join("")}</tbody>
      </table>
    </div>
  `}function E(t,e){return t?.keys.find(a=>a.index===e)?.key_hint??`key ${e}`}function st(t,e){const s=[...t.http_requests].sort((i,_)=>_.count-i.count),a=s.length?s.map(i=>`
        <tr>
          <td class="mono">${r(i.endpoint)}</td>
          <td>${r(i.method)}</td>
          <td>${i.status===200?'<span class="ok-text">200</span>':i.status>=500?'<span class="bad-text">'+i.status+"</span>":i.status}</td>
          <td>${i.count}</td>
        </tr>`).join(""):'<tr><td colspan="4" class="muted">No requests recorded yet</td></tr>',n=t.http_durations.length?t.http_durations.map(i=>`
        <tr>
          <td class="mono">${r(i.endpoint)}</td>
          <td>${r(i.method)}</td>
          <td>${r(U(i.duration_seconds_sum,i.duration_seconds_count))}</td>
          <td>${i.duration_seconds_count}</td>
        </tr>`).join(""):'<tr><td colspan="4" class="muted">No latency data yet</td></tr>',l=[...t.upstream_requests].sort((i,_)=>i.key_index-_.key_index||_.count-i.count),c=l.length?l.map(i=>`
        <tr>
          <td class="mono">${r(E(e,i.key_index))}</td>
          <td>${i.priority}</td>
          <td>${i.status===200?'<span class="ok-text">200</span>':i.status===429?'<span class="bad-text">429</span>':i.status}</td>
          <td>${i.count}</td>
          <td>${r(U(i.duration_seconds_sum,i.duration_seconds_count))}</td>
        </tr>`).join(""):'<tr><td colspan="5" class="muted">No upstream traffic yet</td></tr>',u=t.key_exhaustions.length?t.key_exhaustions.map(i=>`<tr><td class="mono">${r(E(e,i.key_index))}</td><td>${i.count}</td></tr>`).join(""):'<tr><td colspan="2" class="muted">None 🎉</td></tr>',W=t.key_switches.length?t.key_switches.map(i=>`<tr><td>${i.from_key} → ${i.to_key}</td><td>${r(i.reason)}</td><td>${i.count}</td></tr>`).join(""):'<tr><td colspan="3" class="muted">None</td></tr>';return`
    <h2>Metrics</h2>
    <div class="metrics-grid">
      <div class="metric-block">
        <h3>Requests by endpoint</h3>
        <table><thead><tr><th>Endpoint</th><th>Method</th><th>Status</th><th>Count</th></tr></thead><tbody>${a}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Latency (avg)</h3>
        <table><thead><tr><th>Endpoint</th><th>Method</th><th>Avg</th><th>Count</th></tr></thead><tbody>${n}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Upstream traffic per key</h3>
        <table><thead><tr><th>Key</th><th>Pri</th><th>Status</th><th>Count</th><th>Avg</th></tr></thead><tbody>${c}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Exhaustions</h3>
        <table><thead><tr><th>Key</th><th>429s</th></tr></thead><tbody>${u}</tbody></table>
        <h3>Key switches</h3>
        <table><thead><tr><th>Switch</th><th>Reason</th><th>Count</th></tr></thead><tbody>${W}</tbody></table>
      </div>
    </div>
    <details class="raw-metrics">
      <summary>Raw snapshot JSON</summary>
      <pre>${r(JSON.stringify(t,null,2))}</pre>
    </details>
  `}function at(t){const e=t.summary,s=t.rolling.resetsAt;return`
    <div class="card">
      <div class="card-value">${e.total_keys}</div>
      <div class="card-label">Keys</div>
      <div class="card-sub"><span class="chip ok">${e.available_keys} available</span> <span class="chip ${e.exhausted_keys>0?"bad":"muted-chip"}">${e.exhausted_keys} exhausted</span></div>
    </div>
    <div class="card">
      <div class="card-value">${e.active_sessions}</div>
      <div class="card-label">Active sessions</div>
    </div>
    <div class="card">
      <div class="card-value small-value">${r(e.routing_strategy)}</div>
      <div class="card-label">Routing strategy</div>
    </div>
    <div class="card">
      <div class="card-value">${$(e.proactive_threshold_percent)}</div>
      <div class="card-label">Proactive threshold</div>
    </div>
    <div class="card">
      <div class="card-value small-value" data-countdown="${r(s??"")}">${r(C(s))}</div>
      <div class="card-label">Next rolling reset</div>
    </div>
  `}const nt={rolling:"5-hour",weekly:"Weekly",monthly:"Monthly"};function k(t){return new Intl.NumberFormat("en-US",{style:"currency",currency:"USD",minimumFractionDigits:2,maximumFractionDigits:4}).format(t)}function rt(t,e){const s=Math.min(100,Math.max(0,t.usage_percent)),a=I(s,95),n=new Date(Date.now()+t.reset_in_sec*1e3).toISOString(),l=t.rows.map(u=>`<tr>
        <td>${r(u.name)}${u.estimated?' <span class="muted small">(est.)</span>':""}</td>
        <td class="mono">${k(u.cost)}</td>
        <td class="mono">${k(u.quota_cost)}</td>
        <td class="mono">${u.contribution_percent.toFixed(1)}%</td>
      </tr>`).join(""),c=l?`<table>
        <thead><tr><th>Model</th><th>Cost</th><th>Quota cost</th><th>Contribution</th></tr></thead>
        <tbody>${l}</tbody>
      </table>`:'<p class="muted small">No model usage recorded in this window.</p>';return`
    <div class="ws-window" data-component="ws-window">
      <div class="ws-window-head">
        <span>${r(e)}</span>
        <span class="mono">${t.usage_percent.toFixed(1)}% · ${k(t.usage_usd)} / ${k(t.limit_usd)}</span>
      </div>
      <div class="bar" role="progressbar" aria-valuenow="${s}" aria-valuemin="0" aria-valuemax="100"><div class="bar-fill ${a}" style="width:${s}%"></div></div>
      <div class="muted small">Resets in <span data-countdown="${r(n)}"></span></div>
      ${c}
    </div>`}function ot(t){return t.error?`<p class="muted small">Workspace ${r(t.name)}: ${r(t.error)}</p>`:["rolling","weekly","monthly"].map(e=>{const s=t.windows[e];return s?rt(s,nt[e]??e):""}).join("")}function j(t,e){if(!t.enabled)return"";const s=t.workspaces.map(c=>`<button class="ws-tab${c.id===e?" active":""}" data-ws-tab="${r(c.id)}">${r(c.name)}</button>`).join(""),a=t.workspaces.find(c=>c.id===e)??t.workspaces[0],n=a?ot(a):'<p class="muted small">No workspaces found for this account.</p>',l=t.error?`<p class="muted small">Last refresh error: ${r(t.error)}</p>`:"";return`
    <div class="ws-tabs">${s}</div>
    ${l}
    ${n}
    ${t.updated_at?`<div class="muted small">Updated ${r(t.updated_at)}</div>`:""}`}const p=window.__SWB_CONFIG__?.apiKey??"",T="sb_base_url",S="sb_proxy_key",D="sb_interval_ms";function it(){const t=localStorage.getItem(D),e=t===null?1e4:Number(t);return{baseUrl:localStorage.getItem(T)??"",apiKey:localStorage.getItem(S)??"",intervalMs:Number.isFinite(e)&&e>=0?e:1e4}}let o=it(),f,b=null,w=null,h=null,L=null;const d=t=>{const e=document.querySelector(t);if(!e)throw new Error(`missing element ${t}`);return e};function m(t){const e=d("#error-banner");t?(e.textContent=t,e.hidden=!1):e.hidden=!0}function g(t){let e=document.querySelector("#toast");e||(e=document.createElement("div"),e.id="toast",document.body.appendChild(e)),e.textContent=t,e.classList.add("show"),window.setTimeout(()=>e.classList.remove("show"),3e3)}function q(){const t=d("#base-url-label");t.textContent=o.baseUrl||window.location.origin,d("#interval").value=String(o.intervalMs)}function x(){f!==void 0&&(window.clearInterval(f),f=void 0),o.intervalMs>0&&o.apiKey&&(f=window.setInterval(()=>{y(!1)},o.intervalMs))}async function y(t,e=!1){if(!o.apiKey){m("Set your proxy API key in Settings to load usage data.");return}try{const[s,a,n]=await Promise.all([H(o.baseUrl,o.apiKey,t),F(o.baseUrl),G(o.baseUrl,o.apiKey)]);b=s,w=a,h=n,m(null),dt()}catch(s){if(s instanceof v&&s.status===401){if(o.apiKey!==""&&o.apiKey!==p&&p&&!e)return localStorage.removeItem(S),o.apiKey=p,d("#proxy-key").value=p,y(t,!0);m("Invalid proxy API key — update it in Settings."),d("#settings").hidden=!1}else s instanceof v?m(`Proxy request failed: ${s.message}`):m(`Cannot reach proxy at ${o.baseUrl||window.location.origin}: ${String(s)}`)}}function dt(){if(!b||!w)return;d("#summary").innerHTML=at(b),d("#pool").innerHTML=X(b),d("#keys").innerHTML=et(b),d("#metrics").innerHTML=st(w,b);const t=d("#workspace-usage");h&&h.enabled&&(h.workspaces.length>0||h.error)?(t.hidden=!1,t.innerHTML=j(h,L)):(t.hidden=!0,t.innerHTML="");const s=Object.entries(w.model_aliases??{}).map(([n,l])=>`<span class="alias-chip mono">${r(n)} → ${r(l)}</span>`).join(""),a=`${o.baseUrl||""}/metrics`;d("#footer").innerHTML=`
    ${s?`<div class="aliases">${s}</div>`:""}
    <div class="muted small">Snapshot ${r(w.generated_at)} · raw Prometheus: <a href="${r(a)}">/metrics</a></div>
  `}function lt(){const t=Date.now();for(const e of document.querySelectorAll("[data-countdown]"))e.textContent=C(e.dataset.countdown,t);for(const e of document.querySelectorAll("[data-retry]")){const s=Number(e.dataset.retry??0);Number.isFinite(s)&&s>0&&(e.dataset.retry=String(s-1),e.textContent=`${s-1}s`)}}function ct(){d("#settings-toggle").addEventListener("click",()=>{const t=d("#settings");t.hidden=!t.hidden}),d("#settings-form").addEventListener("submit",t=>{t.preventDefault(),o={baseUrl:d("#base-url").value,apiKey:d("#proxy-key").value,intervalMs:o.intervalMs},localStorage.setItem(T,o.baseUrl),localStorage.setItem(S,o.apiKey),d("#settings").hidden=!0,q(),x(),y(!1)}),d("#clear-key").addEventListener("click",()=>{localStorage.removeItem(S),o.apiKey=p,d("#proxy-key").value=p,m("Proxy key override cleared.")}),d("#interval").addEventListener("change",()=>{o.intervalMs=Number(d("#interval").value),localStorage.setItem(D,String(o.intervalMs)),x()}),d("#refresh").addEventListener("click",()=>{y(!0)}),document.addEventListener("click",t=>{const e=t.target.closest("[data-action]");e&&ut(e.dataset.action,e.dataset.index)}),document.addEventListener("visibilitychange",()=>{document.hidden?f!==void 0&&(window.clearInterval(f),f=void 0):(x(),y(!1))}),d("#workspace-usage").addEventListener("click",t=>{const e=t.target.closest("[data-ws-tab]");e&&(L=e.dataset.wsTab??null,h&&(d("#workspace-usage").innerHTML=j(h,L)))})}async function ut(t,e){if(t)try{switch(t){case"reset-key":{const s=Number(e);if(!Number.isInteger(s))return;await J(o.baseUrl,o.apiKey,s),g(`Key ${s} reset`);break}case"reset-all":{if(!window.confirm("Mark all keys as available?"))return;await V(o.baseUrl,o.apiKey),g("All keys reset");break}case"reload":{if(!window.confirm("Reload configuration from disk?"))return;await z(o.baseUrl,o.apiKey),g("Configuration reloaded");break}case"validate":{const s=await B(o.baseUrl,o.apiKey),a=s.results.filter(n=>n.state==="exhausted").length;g(`Validated ${s.results.length} keys (${a} exhausted)`);break}default:return}await y(!1)}catch(s){s instanceof v&&s.status===401?m("Invalid proxy API key — update it in Settings."):g(`Action failed: ${String(s)}`)}}function ht(){q(),d("#base-url").value=o.baseUrl,d("#proxy-key").value=o.apiKey,ct(),window.setInterval(lt,1e3),!o.apiKey&&p&&(o.apiKey=p),o.apiKey?y(!1):(d("#settings").hidden=!1,m("Set your proxy API key in Settings to load usage data.")),x()}ht();
