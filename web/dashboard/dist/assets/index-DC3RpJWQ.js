(function(){const e=document.createElement("link").relList;if(e&&e.supports&&e.supports("modulepreload"))return;for(const r of document.querySelectorAll('link[rel="modulepreload"]'))a(r);new MutationObserver(r=>{for(const d of r)if(d.type==="childList")for(const h of d.addedNodes)h.tagName==="LINK"&&h.rel==="modulepreload"&&a(h)}).observe(document,{childList:!0,subtree:!0});function s(r){const d={};return r.integrity&&(d.integrity=r.integrity),r.referrerPolicy&&(d.referrerPolicy=r.referrerPolicy),r.crossOrigin==="use-credentials"?d.credentials="include":r.crossOrigin==="anonymous"?d.credentials="omit":d.credentials="same-origin",d}function a(r){if(r.ep)return;r.ep=!0;const d=s(r);fetch(r.href,d)}})();class p extends Error{status;constructor(e,s){super(s),this.status=e}}function L(t){return t.trim().replace(/\/+$/,"")}function I(t){return t?{Authorization:`Bearer ${t}`}:{}}async function C(t,e,s){const a=await fetch(`${L(t)}${e}`,{headers:I(s)});if(a.status===401)throw new p(401,"Invalid or missing proxy API key");if(!a.ok)throw new p(a.status,`Request failed with status ${a.status}`);return await a.json()}async function k(t,e,s,a){const r=await fetch(`${L(t)}${e}`,{method:"POST",headers:{...I(s),"Content-Type":"application/json"},body:a===void 0?void 0:JSON.stringify(a)});if(r.status===401)throw new p(401,"Invalid or missing proxy API key");if(!r.ok)throw new p(r.status,`Request failed with status ${r.status}`);return await r.json()}async function q(t,e,s=!1){return C(t,s?"/usage?refresh=true":"/usage",e)}async function T(t){return C(t,"/dashboard/api/metrics.json","")}async function D(t,e){return k(t,"/admin/validate-keys",e)}async function H(t,e,s){return k(t,"/admin/reset-key",e,{index:s})}async function B(t,e){return k(t,"/admin/reset-all-keys",e)}async function F(t,e){await k(t,"/admin/reload",e)}function o(t){return t.replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;").replace(/'/g,"&#39;")}function v(t){return t===void 0||Number.isNaN(t)?"—":`${Math.round(t*10)/10}%`}function P(t,e){return t===void 0||Number.isNaN(t)?"ok":t>=e?"critical":t>=70?"warn":"ok"}function E(t){const e=Math.max(0,Math.floor(t)),s=Math.floor(e/86400),a=Math.floor(e%86400/3600),r=Math.floor(e%3600/60),d=e%60;return s>0?`${s}d ${a}h ${r}m`:a>0?`${a}h ${r}m`:r>0?`${r}m ${d}s`:`${d}s`}function M(t,e=Date.now()){if(!t)return"—";const s=Date.parse(t);if(Number.isNaN(s))return"—";const a=(s-e)/1e3;return a<=0?"reset due":`in ${E(a)}`}function J(t,e=Date.now()){if(!t)return"never";const s=Date.parse(t);if(Number.isNaN(s))return"—";const a=(e-s)/1e3;return a<0||a<60?"just now":`${E(a)} ago`}function W(t){if(!t)return"—";const e=Date.parse(t);return Number.isNaN(e)?"—":new Date(e).toLocaleTimeString()}function K(t,e){return e<=0?"—":`${Math.round(t/e*1e3)}ms`}function S(t,e,s,a){const r=P(e.percent,95),d=s?`${v(s.min_percent)} – ${v(s.max_percent)}`:"—",h=s?v(s.total_remaining_percent):"—",x=a??e.resetsAt?`<span class="muted" data-countdown="${o(a??e.resetsAt??"")}">${o(M(a??e.resetsAt))}</span>`:'<span class="muted">—</span>';return`
    <div class="pool-row">
      <div class="pool-head">
        <span class="pool-name">${o(t)}</span>
        <span class="pool-avg">${v(e.percent)} avg</span>
        ${x}
      </div>
      <div class="bar"><div class="bar-fill ${r}" style="width:${Math.min(100,Math.max(0,e.percent))}%"></div></div>
      <div class="pool-sub muted small">min–max ${o(d)} · remaining ${o(h)}</div>
    </div>
  `}function V(t){const e=t.summary.pool_usage;return`
    <h2>Pool usage</h2>
    ${S("Rolling",t.rolling,e?.rolling,e?.rolling?.earliest_reset_at)}
    ${S("Weekly",t.weekly,e?.weekly)}
    ${S("Monthly",t.monthly,e?.monthly)}
  `}function z(t){return`<span class="chip ${t.state==="available"?"ok":t.state==="exhausted"?"bad":"unknown"}">${o(t.state)}</span>`}function N(t,e){const s=P(t.percent,e),a=t.resetsAt?`<span class="muted small" data-countdown="${o(t.resetsAt)}">${o(W(t.resetsAt))}</span>`:"";return`
    <td class="win-cell">
      <div class="bar tiny"><div class="bar-fill ${s}" style="width:${Math.min(100,Math.max(0,t.percent))}%"></div></div>
      <span class="win-pct">${v(t.percent)}</span>
      ${a}
    </td>
  `}function G(t){return t.retry_after_seconds!==void 0?`<td><span class="chip bad" data-retry="${t.retry_after_seconds}">${t.retry_after_seconds}s</span></td>`:'<td><span class="muted">—</span></td>'}function Y(t){const e=t.summary.proactive_threshold_percent||95;return`
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
      <tr class="${a.current?"current-row":""}" title="${o(a.error??"")}">
        <td>${a.index}${a.current?' <span class="star" title="current key">★</span>':""}</td>
        <td class="mono">${o(a.key_hint??"…")}</td>
        <td>${a.priority} / ${a.weight}</td>
        <td>${z(a)}</td>
        <td>${a.eligible?'<span class="ok-text">✔</span>':'<span class="bad-text">✘</span>'}</td>
        ${N(a.rolling,e)}
        ${N(a.weekly,e)}
        ${N(a.monthly,e)}
        ${G(a)}
        <td class="muted small" title="${o(a.last_checked_at??"")}">${o(J(a.last_checked_at))}</td>
        <td><button class="btn tiny-btn" data-action="reset-key" data-index="${a.index}">Reset</button></td>
      </tr>
    `).join("")}</tbody>
      </table>
    </div>
  `}function A(t,e){return t?.keys.find(a=>a.index===e)?.key_hint??`key ${e}`}function Q(t,e){const s=[...t.http_requests].sort((n,g)=>g.count-n.count),a=s.length?s.map(n=>`
        <tr>
          <td class="mono">${o(n.endpoint)}</td>
          <td>${o(n.method)}</td>
          <td>${n.status===200?'<span class="ok-text">200</span>':n.status>=500?'<span class="bad-text">'+n.status+"</span>":n.status}</td>
          <td>${n.count}</td>
        </tr>`).join(""):'<tr><td colspan="4" class="muted">No requests recorded yet</td></tr>',r=t.http_durations.length?t.http_durations.map(n=>`
        <tr>
          <td class="mono">${o(n.endpoint)}</td>
          <td>${o(n.method)}</td>
          <td>${o(K(n.duration_seconds_sum,n.duration_seconds_count))}</td>
          <td>${n.duration_seconds_count}</td>
        </tr>`).join(""):'<tr><td colspan="4" class="muted">No latency data yet</td></tr>',d=[...t.upstream_requests].sort((n,g)=>n.key_index-g.key_index||g.count-n.count),h=d.length?d.map(n=>`
        <tr>
          <td class="mono">${o(A(e,n.key_index))}</td>
          <td>${n.priority}</td>
          <td>${n.status===200?'<span class="ok-text">200</span>':n.status===429?'<span class="bad-text">429</span>':n.status}</td>
          <td>${n.count}</td>
          <td>${o(K(n.duration_seconds_sum,n.duration_seconds_count))}</td>
        </tr>`).join(""):'<tr><td colspan="5" class="muted">No upstream traffic yet</td></tr>',x=t.key_exhaustions.length?t.key_exhaustions.map(n=>`<tr><td class="mono">${o(A(e,n.key_index))}</td><td>${n.count}</td></tr>`).join(""):'<tr><td colspan="2" class="muted">None 🎉</td></tr>',j=t.key_switches.length?t.key_switches.map(n=>`<tr><td>${n.from_key} → ${n.to_key}</td><td>${o(n.reason)}</td><td>${n.count}</td></tr>`).join(""):'<tr><td colspan="3" class="muted">None</td></tr>';return`
    <h2>Metrics</h2>
    <div class="metrics-grid">
      <div class="metric-block">
        <h3>Requests by endpoint</h3>
        <table><thead><tr><th>Endpoint</th><th>Method</th><th>Status</th><th>Count</th></tr></thead><tbody>${a}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Latency (avg)</h3>
        <table><thead><tr><th>Endpoint</th><th>Method</th><th>Avg</th><th>Count</th></tr></thead><tbody>${r}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Upstream traffic per key</h3>
        <table><thead><tr><th>Key</th><th>Pri</th><th>Status</th><th>Count</th><th>Avg</th></tr></thead><tbody>${h}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Exhaustions</h3>
        <table><thead><tr><th>Key</th><th>429s</th></tr></thead><tbody>${x}</tbody></table>
        <h3>Key switches</h3>
        <table><thead><tr><th>Switch</th><th>Reason</th><th>Count</th></tr></thead><tbody>${j}</tbody></table>
      </div>
    </div>
    <details class="raw-metrics">
      <summary>Raw snapshot JSON</summary>
      <pre>${o(JSON.stringify(t,null,2))}</pre>
    </details>
  `}function X(t){const e=t.summary,s=t.rolling.resetsAt;return`
    <div class="card">
      <div class="card-value">${e.total_keys}</div>
      <div class="card-label">Keys</div>
      <div class="card-sub"><span class="chip ok">${e.available_keys} avail</span> <span class="chip ${e.exhausted_keys>0?"bad":"muted-chip"}">${e.exhausted_keys} exhausted</span></div>
    </div>
    <div class="card">
      <div class="card-value">${e.active_sessions}</div>
      <div class="card-label">Active sessions</div>
    </div>
    <div class="card">
      <div class="card-value small-value">${o(e.routing_strategy)}</div>
      <div class="card-label">Routing strategy</div>
    </div>
    <div class="card">
      <div class="card-value">${v(e.proactive_threshold_percent)}</div>
      <div class="card-label">Proactive threshold</div>
    </div>
    <div class="card">
      <div class="card-value small-value" data-countdown="${o(s??"")}">${o(M(s))}</div>
      <div class="card-label">Next rolling reset</div>
    </div>
  `}const u=window.__SWB_CONFIG__?.apiKey??"",R="sb_base_url",_="sb_proxy_key",O="sb_interval_ms";function Z(){const t=localStorage.getItem(O),e=t===null?1e4:Number(t);return{baseUrl:localStorage.getItem(R)??"",apiKey:localStorage.getItem(_)??"",intervalMs:Number.isFinite(e)&&e>=0?e:1e4}}let i=Z(),y,m=null,$=null;const l=t=>{const e=document.querySelector(t);if(!e)throw new Error(`missing element ${t}`);return e};function c(t){const e=l("#error-banner");t?(e.textContent=t,e.hidden=!1):e.hidden=!0}function b(t){let e=document.querySelector("#toast");e||(e=document.createElement("div"),e.id="toast",document.body.appendChild(e)),e.textContent=t,e.classList.add("show"),window.setTimeout(()=>e.classList.remove("show"),3e3)}function U(){const t=l("#base-url-label");t.textContent=i.baseUrl||window.location.origin,l("#interval").value=String(i.intervalMs)}function w(){y!==void 0&&(window.clearInterval(y),y=void 0),i.intervalMs>0&&i.apiKey&&(y=window.setInterval(()=>{f(!1)},i.intervalMs))}async function f(t,e=!1){if(!i.apiKey){c("Set your proxy API key in Settings to load usage data.");return}try{const[s,a]=await Promise.all([q(i.baseUrl,i.apiKey,t),T(i.baseUrl)]);m=s,$=a,c(null),tt()}catch(s){if(s instanceof p&&s.status===401){if(i.apiKey!==""&&i.apiKey!==u&&u&&!e)return localStorage.removeItem(_),i.apiKey=u,l("#proxy-key").value=u,f(t,!0);c("Invalid proxy API key — update it in Settings."),l("#settings").hidden=!1}else s instanceof p?c(`Proxy request failed: ${s.message}`):c(`Cannot reach proxy at ${i.baseUrl||window.location.origin}: ${String(s)}`)}}function tt(){if(!m||!$)return;l("#summary").innerHTML=X(m),l("#pool").innerHTML=V(m),l("#keys").innerHTML=Y(m),l("#metrics").innerHTML=Q($,m);const e=Object.entries($.model_aliases??{}).map(([a,r])=>`<span class="alias-chip mono">${o(a)} → ${o(r)}</span>`).join(""),s=`${i.baseUrl||""}/metrics`;l("#footer").innerHTML=`
    ${e?`<div class="aliases">${e}</div>`:""}
    <div class="muted small">Snapshot ${o($.generated_at)} · raw Prometheus: <a href="${o(s)}">/metrics</a></div>
  `}function et(){const t=Date.now();for(const e of document.querySelectorAll("[data-countdown]"))e.textContent=M(e.dataset.countdown,t);for(const e of document.querySelectorAll("[data-retry]")){const s=Number(e.dataset.retry??0);Number.isFinite(s)&&s>0&&(e.dataset.retry=String(s-1),e.textContent=`${s-1}s`)}}function st(){l("#settings-toggle").addEventListener("click",()=>{const t=l("#settings");t.hidden=!t.hidden}),l("#settings-form").addEventListener("submit",t=>{t.preventDefault(),i={baseUrl:l("#base-url").value,apiKey:l("#proxy-key").value,intervalMs:i.intervalMs},localStorage.setItem(R,i.baseUrl),localStorage.setItem(_,i.apiKey),l("#settings").hidden=!0,U(),w(),f(!1)}),l("#clear-key").addEventListener("click",()=>{localStorage.removeItem(_),i.apiKey=u,l("#proxy-key").value=u,c("Proxy key override cleared.")}),l("#interval").addEventListener("change",()=>{i.intervalMs=Number(l("#interval").value),localStorage.setItem(O,String(i.intervalMs)),w()}),l("#refresh").addEventListener("click",()=>{f(!0)}),document.addEventListener("click",t=>{const e=t.target.closest("[data-action]");e&&at(e.dataset.action,e.dataset.index)}),document.addEventListener("visibilitychange",()=>{document.hidden?y!==void 0&&(window.clearInterval(y),y=void 0):(w(),f(!1))})}async function at(t,e){if(t)try{switch(t){case"reset-key":{const s=Number(e);if(!Number.isInteger(s))return;await H(i.baseUrl,i.apiKey,s),b(`Key ${s} reset`);break}case"reset-all":{if(!window.confirm("Mark all keys as available?"))return;await B(i.baseUrl,i.apiKey),b("All keys reset");break}case"reload":{if(!window.confirm("Reload configuration from disk?"))return;await F(i.baseUrl,i.apiKey),b("Configuration reloaded");break}case"validate":{const s=await D(i.baseUrl,i.apiKey),a=s.results.filter(r=>r.state==="exhausted").length;b(`Validated ${s.results.length} keys (${a} exhausted)`);break}default:return}await f(!1)}catch(s){s instanceof p&&s.status===401?c("Invalid proxy API key — update it in Settings."):b(`Action failed: ${String(s)}`)}}function nt(){U(),l("#base-url").value=i.baseUrl,l("#proxy-key").value=i.apiKey,st(),window.setInterval(et,1e3),!i.apiKey&&u&&(i.apiKey=u),i.apiKey?f(!1):(l("#settings").hidden=!1,c("Set your proxy API key in Settings to load usage data.")),w()}nt();
