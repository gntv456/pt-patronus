function s(r){if(typeof r!="string")return"";const t=r.trim();return t===""?"":t.startsWith("/")||t.startsWith("#")||t.startsWith("?")||/^(https?|mailto):/i.test(t)?t:""}export{s};
