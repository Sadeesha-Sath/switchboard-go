(function(){const e=document.createElement("link").relList;if(e&&e.supports&&e.supports("modulepreload"))return;for(const r of document.querySelectorAll('link[rel="modulepreload"]'))a(r);new MutationObserver(r=>{for(const d of r)if(d.type==="childList")for(const u of d.addedNodes)u.tagName==="LINK"&&u.rel==="modulepreload"&&a(u)}).observe(document,{childList:!0,subtree:!0});function s(r){const d={};return r.integrity&&(d.integrity=r.integrity),r.referrerPolicy&&(d.referrerPolicy=r.referrerPolicy),r.crossOrigin==="use-credentials"?d.credentials="include":r.crossOrigin==="anonymous"?d.credentials="omit":d.credentials="same-origin",d}function a(r){if(r.ep)return;r.ep=!0;const d=s(r);fetch(r.href,d)}})();class f extends Error{status;constructor(e,s){super(s),this.status=e}}function L(t){return t.trim().replace(/\/+$/,"")}function K(t){return t?{Authorization:`Bearer ${t}`}:{}}async function I(t,e,s){const a=await fetch(`${L(t)}${e}`,{headers:K(s)});if(a.status===401)throw new f(401,"Invalid or missing proxy API key");if(!a.ok)throw new f(a.status,`Request failed with status ${a.status}`);return await a.json()}async function w(t,e,s,a){const r=await fetch(`${L(t)}${e}`,{method:"POST",headers:{...K(s),"Content-Type":"application/json"},body:a===void 0?void 0:JSON.stringify(a)});if(r.status===401)throw new f(401,"Invalid or missing proxy API key");if(!r.ok)throw new f(r.status,`Request failed with status ${r.status}`);return await r.json()}async function q(t,e,s=!1){return I(t,s?"/usage?refresh=true":"/usage",e)}async function O(t){return I(t,"/dashboard/api/metrics.json","")}async function T(t,e){return w(t,"/admin/validate-keys",e)}async function D(t,e,s){return w(t,"/admin/reset-key",e,{index:s})}async function H(t,e){return w(t,"/admin/reset-all-keys",e)}async function J(t,e){await w(t,"/admin/reload",e)}function i(t){return t.replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;").replace(/'/g,"&#39;")}function p(t){return t===void 0||Number.isNaN(t)?"—":`${Math.round(t*10)/10}%`}function P(t,e){return t===void 0||Number.isNaN(t)?"ok":t>=e?"critical":t>=70?"warn":"ok"}function C(t){const e=Math.max(0,Math.floor(t)),s=Math.floor(e/3600),a=Math.floor(e%3600/60),r=e%60;return s>0?`${s}h ${a}m`:a>0?`${a}m ${r}s`:`${r}s`}function N(t,e=Date.now()){if(!t)return"—";const s=Date.parse(t);if(Number.isNaN(s))return"—";const a=(s-e)/1e3;return a<=0?"reset due":`in ${C(a)}`}function B(t,e=Date.now()){if(!t)return"never";const s=Date.parse(t);if(Number.isNaN(s))return"—";const a=(e-s)/1e3;return a<0||a<60?"just now":`${C(a)} ago`}function F(t){if(!t)return"—";const e=Date.parse(t);return Number.isNaN(e)?"—":new Date(e).toLocaleTimeString()}function M(t,e){return e<=0?"—":`${Math.round(t/e*1e3)}ms`}function k(t,e,s,a){const r=P(e.percent,95),d=s?`${p(s.min_percent)} – ${p(s.max_percent)}`:"—",u=s?p(s.total_remaining_percent):"—",_=a??e.resetsAt?`<span class="muted" data-countdown="${i(a??e.resetsAt??"")}">${i(N(a??e.resetsAt))}</span>`:'<span class="muted">—</span>';return`
    <div class="pool-row">
      <div class="pool-head">
        <span class="pool-name">${i(t)}</span>
        <span class="pool-avg">${p(e.percent)} avg</span>
        ${_}
      </div>
      <div class="bar"><div class="bar-fill ${r}" style="width:${Math.min(100,Math.max(0,e.percent))}%"></div></div>
      <div class="pool-sub muted small">min–max ${i(d)} · remaining ${i(u)}</div>
    </div>
  `}function V(t){const e=t.summary.pool_usage;return`
    <h2>Pool usage</h2>
    ${k("Rolling",t.rolling,e?.rolling,e?.rolling?.earliest_reset_at)}
    ${k("Weekly",t.weekly,e?.weekly)}
    ${k("Monthly",t.monthly,e?.monthly)}
  `}function W(t){return`<span class="chip ${t.state==="available"?"ok":t.state==="exhausted"?"bad":"unknown"}">${i(t.state)}</span>`}function x(t,e){const s=P(t.percent,e),a=t.resetsAt?`<span class="muted small" data-countdown="${i(t.resetsAt)}">${i(F(t.resetsAt))}</span>`:"";return`
    <td class="win-cell">
      <div class="bar tiny"><div class="bar-fill ${s}" style="width:${Math.min(100,Math.max(0,t.percent))}%"></div></div>
      <span class="win-pct">${p(t.percent)}</span>
      ${a}
    </td>
  `}function z(t){return t.retry_after_seconds!==void 0?`<td><span class="chip bad" data-retry="${t.retry_after_seconds}">${t.retry_after_seconds}s</span></td>`:'<td><span class="muted">—</span></td>'}function Y(t){const e=t.summary.proactive_threshold_percent||95;return`
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
      <tr class="${a.current?"current-row":""}" title="${i(a.error??"")}">
        <td>${a.index}${a.current?' <span class="star" title="current key">★</span>':""}</td>
        <td class="mono">${i(a.key_hint??"…")}</td>
        <td>${a.priority} / ${a.weight}</td>
        <td>${W(a)}</td>
        <td>${a.eligible?'<span class="ok-text">✔</span>':'<span class="bad-text">✘</span>'}</td>
        ${x(a.rolling,e)}
        ${x(a.weekly,e)}
        ${x(a.monthly,e)}
        ${z(a)}
        <td class="muted small" title="${i(a.last_checked_at??"")}">${i(B(a.last_checked_at))}</td>
        <td><button class="btn tiny-btn" data-action="reset-key" data-index="${a.index}">Reset</button></td>
      </tr>
    `).join("")}</tbody>
      </table>
    </div>
  `}function A(t,e){return t?.keys.find(a=>a.index===e)?.key_hint??`key ${e}`}function G(t,e){const s=[...t.http_requests].sort((n,$)=>$.count-n.count),a=s.length?s.map(n=>`
        <tr>
          <td class="mono">${i(n.endpoint)}</td>
          <td>${i(n.method)}</td>
          <td>${n.status===200?'<span class="ok-text">200</span>':n.status>=500?'<span class="bad-text">'+n.status+"</span>":n.status}</td>
          <td>${n.count}</td>
        </tr>`).join(""):'<tr><td colspan="4" class="muted">No requests recorded yet</td></tr>',r=t.http_durations.length?t.http_durations.map(n=>`
        <tr>
          <td class="mono">${i(n.endpoint)}</td>
          <td>${i(n.method)}</td>
          <td>${i(M(n.duration_seconds_sum,n.duration_seconds_count))}</td>
          <td>${n.duration_seconds_count}</td>
        </tr>`).join(""):'<tr><td colspan="4" class="muted">No latency data yet</td></tr>',d=[...t.upstream_requests].sort((n,$)=>n.key_index-$.key_index||$.count-n.count),u=d.length?d.map(n=>`
        <tr>
          <td class="mono">${i(A(e,n.key_index))}</td>
          <td>${n.priority}</td>
          <td>${n.status===200?'<span class="ok-text">200</span>':n.status===429?'<span class="bad-text">429</span>':n.status}</td>
          <td>${n.count}</td>
          <td>${i(M(n.duration_seconds_sum,n.duration_seconds_count))}</td>
        </tr>`).join(""):'<tr><td colspan="5" class="muted">No upstream traffic yet</td></tr>',_=t.key_exhaustions.length?t.key_exhaustions.map(n=>`<tr><td class="mono">${i(A(e,n.key_index))}</td><td>${n.count}</td></tr>`).join(""):'<tr><td colspan="2" class="muted">None 🎉</td></tr>',j=t.key_switches.length?t.key_switches.map(n=>`<tr><td>${n.from_key} → ${n.to_key}</td><td>${i(n.reason)}</td><td>${n.count}</td></tr>`).join(""):'<tr><td colspan="3" class="muted">None</td></tr>';return`
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
        <table><thead><tr><th>Key</th><th>Pri</th><th>Status</th><th>Count</th><th>Avg</th></tr></thead><tbody>${u}</tbody></table>
      </div>
      <div class="metric-block">
        <h3>Exhaustions</h3>
        <table><thead><tr><th>Key</th><th>429s</th></tr></thead><tbody>${_}</tbody></table>
        <h3>Key switches</h3>
        <table><thead><tr><th>Switch</th><th>Reason</th><th>Count</th></tr></thead><tbody>${j}</tbody></table>
      </div>
    </div>
    <details class="raw-metrics">
      <summary>Raw snapshot JSON</summary>
      <pre>${i(JSON.stringify(t,null,2))}</pre>
    </details>
  `}function Q(t){const e=t.summary,s=t.rolling.resetsAt;return`
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
      <div class="card-value small-value">${i(e.routing_strategy)}</div>
      <div class="card-label">Routing strategy</div>
    </div>
    <div class="card">
      <div class="card-value">${p(e.proactive_threshold_percent)}</div>
      <div class="card-label">Proactive threshold</div>
    </div>
    <div class="card">
      <div class="card-value small-value" data-countdown="${i(s??"")}">${i(N(s))}</div>
      <div class="card-label">Next rolling reset</div>
    </div>
  `}const E="sb_base_url",S="sb_proxy_key",R="sb_interval_ms";function X(){const t=localStorage.getItem(R),e=t===null?1e4:Number(t);return{baseUrl:localStorage.getItem(E)??"",apiKey:localStorage.getItem(S)??"",intervalMs:Number.isFinite(e)&&e>=0?e:1e4}}let o=X(),h,y=null,b=null;const l=t=>{const e=document.querySelector(t);if(!e)throw new Error(`missing element ${t}`);return e};function c(t){const e=l("#error-banner");t?(e.textContent=t,e.hidden=!1):e.hidden=!0}function v(t){let e=document.querySelector("#toast");e||(e=document.createElement("div"),e.id="toast",document.body.appendChild(e)),e.textContent=t,e.classList.add("show"),window.setTimeout(()=>e.classList.remove("show"),3e3)}function U(){const t=l("#base-url-label");t.textContent=o.baseUrl||window.location.origin,l("#interval").value=String(o.intervalMs)}function g(){h!==void 0&&(window.clearInterval(h),h=void 0),o.intervalMs>0&&o.apiKey&&(h=window.setInterval(()=>{m(!1)},o.intervalMs))}async function m(t){if(!o.apiKey){c("Set your proxy API key in Settings to load usage data.");return}try{const[e,s]=await Promise.all([q(o.baseUrl,o.apiKey,t),O(o.baseUrl)]);y=e,b=s,c(null),Z()}catch(e){e instanceof f&&e.status===401?(c("Invalid proxy API key — update it in Settings."),l("#settings").hidden=!1):e instanceof f?c(`Proxy request failed: ${e.message}`):c(`Cannot reach proxy at ${o.baseUrl||window.location.origin}: ${String(e)}`)}}function Z(){if(!y||!b)return;l("#summary").innerHTML=Q(y),l("#pool").innerHTML=V(y),l("#keys").innerHTML=Y(y),l("#metrics").innerHTML=G(b,y);const e=Object.entries(b.model_aliases??{}).map(([a,r])=>`<span class="alias-chip mono">${i(a)} → ${i(r)}</span>`).join(""),s=`${o.baseUrl||""}/metrics`;l("#footer").innerHTML=`
    ${e?`<div class="aliases">${e}</div>`:""}
    <div class="muted small">Snapshot ${i(b.generated_at)} · raw Prometheus: <a href="${i(s)}">/metrics</a></div>
  `}function tt(){const t=Date.now();for(const e of document.querySelectorAll("[data-countdown]"))e.textContent=N(e.dataset.countdown,t);for(const e of document.querySelectorAll("[data-retry]")){const s=Number(e.dataset.retry??0);Number.isFinite(s)&&s>0&&(e.dataset.retry=String(s-1),e.textContent=`${s-1}s`)}}function et(){l("#settings-toggle").addEventListener("click",()=>{const t=l("#settings");t.hidden=!t.hidden}),l("#settings-form").addEventListener("submit",t=>{t.preventDefault(),o={baseUrl:l("#base-url").value,apiKey:l("#proxy-key").value,intervalMs:o.intervalMs},localStorage.setItem(E,o.baseUrl),localStorage.setItem(S,o.apiKey),l("#settings").hidden=!0,U(),g(),m(!1)}),l("#clear-key").addEventListener("click",()=>{localStorage.removeItem(S),o.apiKey="",l("#proxy-key").value="",c("Proxy key cleared.")}),l("#interval").addEventListener("change",()=>{o.intervalMs=Number(l("#interval").value),localStorage.setItem(R,String(o.intervalMs)),g()}),l("#refresh").addEventListener("click",()=>{m(!0)}),document.addEventListener("click",t=>{const e=t.target.closest("[data-action]");e&&st(e.dataset.action,e.dataset.index)}),document.addEventListener("visibilitychange",()=>{document.hidden?h!==void 0&&(window.clearInterval(h),h=void 0):(g(),m(!1))})}async function st(t,e){if(t)try{switch(t){case"reset-key":{const s=Number(e);if(!Number.isInteger(s))return;await D(o.baseUrl,o.apiKey,s),v(`Key ${s} reset`);break}case"reset-all":{if(!window.confirm("Mark all keys as available?"))return;await H(o.baseUrl,o.apiKey),v("All keys reset");break}case"reload":{if(!window.confirm("Reload configuration from disk?"))return;await J(o.baseUrl,o.apiKey),v("Configuration reloaded");break}case"validate":{const s=await T(o.baseUrl,o.apiKey),a=s.results.filter(r=>r.state==="exhausted").length;v(`Validated ${s.results.length} keys (${a} exhausted)`);break}default:return}await m(!1)}catch(s){s instanceof f&&s.status===401?c("Invalid proxy API key — update it in Settings."):v(`Action failed: ${String(s)}`)}}function at(){U(),l("#base-url").value=o.baseUrl,l("#proxy-key").value=o.apiKey,et(),window.setInterval(tt,1e3),o.apiKey?m(!1):(l("#settings").hidden=!1,c("Set your proxy API key in Settings to load usage data.")),g()}at();
