import{v as V,g as B,a as T,H as U}from"./element-plus-CeOYDNnd.js";import"./vendor-BZE60_y4.js";import{B as N,o as W,m as A,M as h,P as r,a1 as E,e as w,a0 as _,X as D,Y as H,O as X,aX as Y,r as c,c as d,L as p,$ as z,aY as O}from"./vue-CLd9ntt3.js";import{m as Z,W as F}from"./element-icons-CIf8YPR5.js";import{p as v}from"./plugin-bxX6fVSa.js";import{_ as G}from"./_plugin-vue_export-helper-DlAUqK2U.js";import"./index-yCCszYWk.js";const J={class:"page"},K={class:"head"},Q={class:"title-block"},ee={class:"title"},te={class:"meta"},ne={class:"frame-shell"},se=["srcdoc"],ae=N({__name:"PluginView",setup(ie){const k=O(),x=Y(),f=c(null),o=c(""),l=c(!1),g=c(null),a=d(()=>String(k.params.pluginId||"")),m=d(()=>String(k.params.viewId||"")),u=d(()=>{var e,t;return(((t=(e=f.value)==null?void 0:e.contributes)==null?void 0:t.views)||[]).find(n=>n.id===m.value)}),I=d(()=>{var e,t;return((e=u.value)==null?void 0:e.title)||((t=f.value)==null?void 0:t.name)||"Plugin View"});function y(e){const t=(e||"index.html").replace(/^\/+/,"");return t.startsWith("web/")?t.slice(4):t}function j(e){const n=y(e).split("/").filter(Boolean);if(!n.length)return`/api/v1/plugins/${encodeURIComponent(a.value)}/assets/`;const s=n[n.length-1]||"",$=(/\.[A-Za-z0-9]+$/.test(s)?n.slice(0,-1):n).map(S=>encodeURIComponent(S)).join("/"),M=$?`${$}/`:"";return`/api/v1/plugins/${encodeURIComponent(a.value)}/assets/${M}`}function R(){return`<script>
(function () {
  if (window.PTPatronus) return;
  var seq = 0;
  var pending = {};
  window.addEventListener('message', function (event) {
    var data = event.data || {};
    if (data.ptp !== 'plugin-host-result' || !data.id || !pending[data.id]) return;
    var task = pending[data.id];
    delete pending[data.id];
    if (data.ok) task.resolve(data.result || {});
    else task.reject(new Error(data.error || 'Plugin bridge failed'));
  });
  function request(method, params) {
    var id = String(++seq);
    window.parent.postMessage({ ptp: 'plugin-host', id: id, method: method, params: params || {} }, '*');
    return new Promise(function (resolve, reject) {
      pending[id] = { resolve: resolve, reject: reject };
      setTimeout(function () {
        if (!pending[id]) return;
        delete pending[id];
        reject(new Error('Plugin bridge timeout'));
      }, 30000);
    });
  }
  window.PTPatronus = {
    request: request,
    capabilities: function () { return request('capabilities'); },
    config: {
      get: function () { return request('config.get').then(function (result) { return result.config || {}; }); }
    },
    log: function (level, event, message, data) { return request('log.write', { level: level, event: event, message: message, data: data }); },
    notice: function (title, body, level) { return request('notice.send', { title: title, body: body, level: level || 'info' }); },
    publish: function (type, data) { return request('event.publish', { type: type, data: data }); },
    kv: {
      get: function (key) { return request('kv.get', { key: key }); },
      set: function (key, value) { return request('kv.set', { key: key, value: value }); },
      delete: function (key) { return request('kv.delete', { key: key }); }
    },
    sites: {
      list: function () { return request('sites.list').then(function (result) { return result.sites || []; }); }
    },
    action: function (name, input) { return request('action.invoke', { action: name, input: input || {} }); }
  };
})();
<\/script>`}function b(e,t){const n=/<head[^>]*>/i;return n.test(e)?e.replace(n,s=>`${s}
${t}`):`${t}
${e}`}function C(e,t){const n=`<base href="${t}">`,s=R(),i=/<base\b/i.test(e)?e.replace(/<base\b[^>]*>/i,n):b(e,n);return b(i,s)}async function P(e){var s;if(!((s=g.value)!=null&&s.contentWindow)||e.source!==g.value.contentWindow)return;const t=e.data||{};if(t.ptp!=="plugin-host"||!t.id)return;const n=e.source;try{const i=await v.bridge(a.value,String(t.method||""),t.params||{});n.postMessage({ptp:"plugin-host-result",id:t.id,ok:!0,result:i},e.origin)}catch(i){n.postMessage({ptp:"plugin-host-result",id:t.id,ok:!1,error:i.message||"Bridge failed"},e.origin)}}async function q(){l.value=!0;try{if(f.value=await v.get(a.value),!u.value){o.value="",B.warning("Plugin view not found");return}const[,e]=await Promise.all([v.assetTicket(a.value),v.asset(a.value,y(u.value.path))]);o.value=C(e,j(u.value.path))}catch(e){o.value="",B.error(e.message||"Load plugin view failed")}finally{l.value=!1}}return W(()=>{window.addEventListener("message",P),q()}),A(()=>window.removeEventListener("message",P)),(e,t)=>{const n=T,s=U,i=V;return p(),h("div",J,[r("div",K,[r("div",Q,[E(n,{icon:w(Z),circle:"",onClick:t[0]||(t[0]=L=>w(x).push({name:"plugins"}))},null,8,["icon"]),r("div",null,[r("h2",ee,_(I.value),1),r("div",te,_(a.value)+" / "+_(m.value),1)])]),E(n,{icon:w(F),loading:l.value,onClick:q},{default:D(()=>[...t[1]||(t[1]=[z("Refresh",-1)])]),_:1},8,["icon","loading"])]),H((p(),h("div",ne,[o.value?(p(),h("iframe",{key:`${a.value}:${m.value}:${o.value.length}`,ref_key:"frame",ref:g,class:"plugin-frame",srcdoc:o.value,sandbox:"allow-scripts allow-forms allow-popups allow-modals",referrerpolicy:"no-referrer"},null,8,se)):(p(),X(s,{key:1,description:"No view"}))])),[[i,l.value]])])}}}),ve=G(ae,[["__scopeId","data-v-4257558d"]]);export{ve as default};
