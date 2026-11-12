import{E as S}from"./index-CtJFA4ZJ.js";import{p as c,E as T}from"./plugin-CS9BWi-Y.js";import{v as U}from"./directive-D7R1rTE3.js";import{E as $}from"./index-DjMiBqbK.js";import"./vendor-V8ft6Cc5.js";import"./index-Bseh3SVE.js";import{d as N,k as D,D as W,c as h,a as o,n as B,t as w,I as _,p as A,ad as z,G,aa as H,r as d,b as p,V as Z,ab as F,o as f}from"./vue-DkUKxTKa.js";import{q as J,r as K}from"./element-icons-CrEjNL5F.js";import{_ as O}from"./_plugin-vue_export-helper-C067dVrV.js";import"./typescript-Bp3YSIOJ.js";import"./event-J8ZR2Gtu.js";import"./browser-BBS9qPDd.js";import"./i18n-CXRcXEAs.js";const Q={class:"page"},X={class:"head"},Y={class:"title-block"},ee={class:"title"},te={class:"meta"},ne={class:"frame-shell"},se=["srcdoc"],ae=N({__name:"PluginView",setup(ie){const k=F(),x=H(),v=d(null),r=d(""),l=d(!1),g=d(null),a=p(()=>String(k.params.pluginId||"")),m=p(()=>String(k.params.viewId||"")),u=p(()=>{var e,t;return(((t=(e=v.value)==null?void 0:e.contributes)==null?void 0:t.views)||[]).find(n=>n.id===m.value)}),I=p(()=>{var e,t;return((e=u.value)==null?void 0:e.title)||((t=v.value)==null?void 0:t.name)||"Plugin View"});function y(e){const t=(e||"index.html").replace(/^\/+/,"");return t.startsWith("web/")?t.slice(4):t}function j(e){const n=y(e).split("/").filter(Boolean);if(!n.length)return`/api/v1/plugins/${encodeURIComponent(a.value)}/assets/`;const s=n[n.length-1]||"",E=(/\.[A-Za-z0-9]+$/.test(s)?n.slice(0,-1):n).map(M=>encodeURIComponent(M)).join("/"),L=E?`${E}/`:"";return`/api/v1/plugins/${encodeURIComponent(a.value)}/assets/${L}`}function R(){return`<script>
(function () {
  if (window.PTPatronus) return;
  var seq = 0;
  var pending = {};
  window.addEventListener('message', function (event) {
    if (event.origin !== 'null') return;
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
${e}`}function C(e,t){const n=`<base href="${t}">`,s=R(),i=/<base\b/i.test(e)?e.replace(/<base\b[^>]*>/i,n):b(e,n);return b(i,s)}async function q(e){var s;if(!((s=g.value)!=null&&s.contentWindow)||e.source!==g.value.contentWindow||e.origin!=="null")return;const t=e.data||{};if(t.ptp!=="plugin-host"||!t.id)return;const n=e.source;try{const i=await c.bridge(a.value,String(t.method||""),t.params||{});n.postMessage({ptp:"plugin-host-result",id:t.id,ok:!0,result:i},e.origin)}catch(i){n.postMessage({ptp:"plugin-host-result",id:t.id,ok:!1,error:i.message||"Bridge failed"},e.origin)}}async function P(){l.value=!0;try{if(v.value=await c.get(a.value),!u.value){r.value="",$.warning("Plugin view not found");return}const[,e]=await Promise.all([c.assetTicket(a.value),c.asset(a.value,y(u.value.path))]);r.value=C(e,j(u.value.path))}catch(e){r.value="",$.error(e.message||"Load plugin view failed")}finally{l.value=!1}}return D(()=>{window.addEventListener("message",q),P()}),W(()=>window.removeEventListener("message",q)),(e,t)=>{const n=S,s=T,i=U;return f(),h("div",Q,[o("div",X,[o("div",Y,[B(n,{icon:w(J),circle:"",onClick:t[0]||(t[0]=V=>w(x).push({name:"plugins"}))},null,8,["icon"]),o("div",null,[o("h2",ee,_(I.value),1),o("div",te,_(a.value)+" / "+_(m.value),1)])]),B(n,{icon:w(K),loading:l.value,onClick:P},{default:A(()=>[...t[1]||(t[1]=[Z("Refresh",-1)])]),_:1},8,["icon","loading"])]),z((f(),h("div",ne,[r.value?(f(),h("iframe",{key:`${a.value}:${m.value}:${r.value.length}`,ref_key:"frame",ref:g,class:"plugin-frame",srcdoc:r.value,sandbox:"allow-scripts allow-forms allow-popups allow-modals",referrerpolicy:"no-referrer"},null,8,se)):(f(),G(s,{key:1,description:"No view"}))])),[[i,l.value]])])}}}),_e=O(ae,[["__scopeId","data-v-dc17e22a"]]);export{_e as default};
