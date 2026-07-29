import{v as S,b as B,a as T,J as U}from"./element-plus-yOl673PD.js";import"./vendor-B4Ab_Ycs.js";import{B as N,o as W,m as A,M as h,P as r,a1 as E,e as w,a0 as _,X as D,Y as X,O as Y,aX as z,r as c,c as d,L as p,$ as H,aY as J}from"./vue-Daz41RbG.js";import{m as O,V as Z}from"./element-icons-CYYeWvxr.js";import{p as v}from"./plugin-qcorXJct.js";import{_ as F}from"./_plugin-vue_export-helper-DlAUqK2U.js";import"./index-tT6mM_1l.js";const G={class:"page"},K={class:"head"},Q={class:"title-block"},ee={class:"title"},te={class:"meta"},ne={class:"frame-shell"},se=["srcdoc"],ae=N({__name:"PluginView",setup(ie){const k=J(),x=z(),f=c(null),o=c(""),l=c(!1),g=c(null),a=d(()=>String(k.params.pluginId||"")),m=d(()=>String(k.params.viewId||"")),u=d(()=>{var t,e;return(((e=(t=f.value)==null?void 0:t.contributes)==null?void 0:e.views)||[]).find(n=>n.id===m.value)}),I=d(()=>{var t,e;return((t=u.value)==null?void 0:t.title)||((e=f.value)==null?void 0:e.name)||"Plugin View"});function y(t){const e=(t||"index.html").replace(/^\/+/,"");return e.startsWith("web/")?e.slice(4):e}function j(t){const n=y(t).split("/").filter(Boolean);if(!n.length)return`/api/v1/plugins/${encodeURIComponent(a.value)}/assets/`;const s=n[n.length-1]||"",$=(/\.[A-Za-z0-9]+$/.test(s)?n.slice(0,-1):n).map(V=>encodeURIComponent(V)).join("/"),M=$?`${$}/`:"";return`/api/v1/plugins/${encodeURIComponent(a.value)}/assets/${M}`}function R(){return`<script>
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
<\/script>`}function b(t,e){const n=/<head[^>]*>/i;return n.test(t)?t.replace(n,s=>`${s}
${e}`):`${e}
${t}`}function C(t,e){const n=`<base href="${e}">`,s=R(),i=/<base\b/i.test(t)?t.replace(/<base\b[^>]*>/i,n):b(t,n);return b(i,s)}async function P(t){var s;if(!((s=g.value)!=null&&s.contentWindow)||t.source!==g.value.contentWindow)return;const e=t.data||{};if(e.ptp!=="plugin-host"||!e.id)return;const n=t.source;try{const i=await v.bridge(a.value,String(e.method||""),e.params||{});n.postMessage({ptp:"plugin-host-result",id:e.id,ok:!0,result:i},"*")}catch(i){n.postMessage({ptp:"plugin-host-result",id:e.id,ok:!1,error:i.message||"Bridge failed"},"*")}}async function q(){l.value=!0;try{if(f.value=await v.get(a.value),!u.value){o.value="",B.warning("Plugin view not found");return}const[,t]=await Promise.all([v.assetTicket(a.value),v.asset(a.value,y(u.value.path))]);o.value=C(t,j(u.value.path))}catch(t){o.value="",B.error(t.message||"Load plugin view failed")}finally{l.value=!1}}return W(()=>{window.addEventListener("message",P),q()}),A(()=>window.removeEventListener("message",P)),(t,e)=>{const n=T,s=U,i=S;return p(),h("div",G,[r("div",K,[r("div",Q,[E(n,{icon:w(O),circle:"",onClick:e[0]||(e[0]=L=>w(x).push({name:"plugins"}))},null,8,["icon"]),r("div",null,[r("h2",ee,_(I.value),1),r("div",te,_(a.value)+" / "+_(m.value),1)])]),E(n,{icon:w(Z),loading:l.value,onClick:q},{default:D(()=>[...e[1]||(e[1]=[H("Refresh",-1)])]),_:1},8,["icon","loading"])]),X((p(),h("div",ne,[o.value?(p(),h("iframe",{key:`${a.value}:${m.value}:${o.value.length}`,ref_key:"frame",ref:g,class:"plugin-frame",srcdoc:o.value,sandbox:"allow-scripts allow-forms allow-popups allow-modals",referrerpolicy:"no-referrer"},null,8,se)):(p(),Y(s,{key:1,description:"No view"}))])),[[i,l.value]])])}}}),ve=F(ae,[["__scopeId","data-v-e79923b5"]]);export{ve as default};
