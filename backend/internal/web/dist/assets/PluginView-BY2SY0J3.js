import{E as U}from"./index-fy6uhZqU.js";import{p,E as D}from"./el-empty-C6CKBwb9.js";import{v as F}from"./directive-CFmq1GNu.js";import"./vendor-V8ft6Cc5.js";import"./index-DKrkGb6b.js";import{d as N,k as W,D as A,c as k,a as l,n as x,t as u,I as m,p as Q,ad as z,G,r as f,b as v,V as H,ab as Z,aa as J,o as g}from"./vue-DuijWcMQ.js";import{u as K}from"./i18n-CDvUOty8.js";import{a as O}from"./useUrlState-DyKHW1p0.js";import{u as X}from"./useFeedback-C-x_ZtEh.js";import{o as Y,r as ee}from"./element-icons-TA69fluO.js";import{_ as te}from"./_plugin-vue_export-helper-B83tab4Z.js";import"./index-B_fMZyY4.js";import"./typescript-Bp3YSIOJ.js";import"./event-CYgedeqI.js";import"./raf-D7m5YjE3.js";import"./index-DfbYzDTH.js";import"./index-CEcuuA6a.js";import"./validator-BHNDnmQs.js";import"./index-B4yEyK_U.js";import"./index-CBW8V0Hg.js";import"./index-DzRDZIQQ.js";const ne={class:"page"},ae={class:"head"},se={class:"title-block"},ie={class:"title"},oe={class:"meta"},re={class:"frame-shell"},le=["srcdoc"],ue=N({__name:"PluginView",setup(ce){const y=Z(),j=J(),{t:r}=K(),q=X(),h=O("tab",""),_=f(null),o=f(""),c=f(!1),w=f(null),s=v(()=>String(y.params.pluginId||"")),b=v(()=>String(y.params.viewId||"")),d=v(()=>{var e,t;return(((t=(e=_.value)==null?void 0:e.contributes)==null?void 0:t.views)||[]).find(n=>n.id===b.value)}),C=v(()=>{var e,t;return((e=d.value)==null?void 0:e.title)||((t=_.value)==null?void 0:t.name)||"Plugin View"});function P(e){const t=(e||"index.html").replace(/^\/+/,"");return t.startsWith("web/")?t.slice(4):t}function R(e){const n=P(e).split("/").filter(Boolean);if(!n.length)return`/api/v1/plugins/${encodeURIComponent(s.value)}/assets/`;const a=n[n.length-1]||"",I=(/\.[A-Za-z0-9]+$/.test(a)?n.slice(0,-1):n).map(T=>encodeURIComponent(T)).join("/"),M=I?`${I}/`:"";return`/api/v1/plugins/${encodeURIComponent(s.value)}/assets/${M}`}function S(){return`<script>
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
<\/script>`}function E(e,t){const n=/<head[^>]*>/i;return n.test(e)?e.replace(n,a=>`${a}
${t}`):`${t}
${e}`}function V(e,t){const n=`<base href="${t}">`,a=S(),i=/<base\b/i.test(e)?e.replace(/<base\b[^>]*>/i,n):E(e,n);return E(i,a)}async function B(e){var a;if(!((a=w.value)!=null&&a.contentWindow)||e.source!==w.value.contentWindow||e.origin!=="null")return;const t=e.data||{};if(t.ptp!=="plugin-host"||!t.id)return;const n=e.source;try{const i=await p.bridge(s.value,String(t.method||""),t.params||{});n.postMessage({ptp:"plugin-host-result",id:t.id,ok:!0,result:i},e.origin)}catch(i){n.postMessage({ptp:"plugin-host-result",id:t.id,ok:!1,error:i.message||"Bridge failed"},e.origin)}}async function $(){c.value=!0;try{if(_.value=await p.get(s.value),!d.value){o.value="",q.toast(r("apiErr.notFound"),"warning");return}const[,e]=await Promise.all([p.assetTicket(s.value),p.asset(s.value,P(d.value.path))]);o.value=V(e,R(d.value.path))}catch(e){o.value="",q.toast(e.message||r("common.loadFailed"),"error")}finally{c.value=!1}}W(()=>{window.addEventListener("message",B),h.value||(h.value="installed"),$()});function L(){j.push({name:"plugins",query:{tab:h.value}})}return A(()=>window.removeEventListener("message",B)),(e,t)=>{const n=U,a=D,i=F;return g(),k("div",ne,[l("div",ae,[l("div",se,[x(n,{icon:u(Y),circle:"","aria-label":u(r)("common.back"),onClick:L},null,8,["icon","aria-label"]),l("div",null,[l("h2",ie,m(C.value),1),l("div",oe,m(s.value)+" / "+m(b.value),1)])]),x(n,{icon:u(ee),loading:c.value,onClick:$},{default:Q(()=>[H(m(u(r)("common.refresh")),1)]),_:1},8,["icon","loading"])]),z((g(),k("div",re,[o.value?(g(),k("iframe",{key:`${s.value}:${b.value}:${o.value.length}`,ref_key:"frame",ref:w,class:"plugin-frame",srcdoc:o.value,sandbox:"allow-scripts allow-forms allow-popups allow-modals",referrerpolicy:"no-referrer"},null,8,le)):(g(),G(a,{key:1,description:u(r)("common.noData")},null,8,["description"]))])),[[i,c.value]])])}}}),Se=te(ue,[["__scopeId","data-v-50936b5c"]]);export{Se as default};
