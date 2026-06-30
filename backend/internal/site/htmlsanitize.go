package site

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// SanitizeHTML 清洗来自站点详情页的不可信 HTML，防存储型 XSS（Web 端 v-html 渲染简介）。
// 移除脚本/样式/嵌入等危险标签；剥离 on* 事件属性；中和 javascript:/vbscript:/data:text/html 协议。
// 保留排版标签（p/img/a/br/table/...），前端仍能渲染富文本简介。goquery 解析极宽松，失败时返回空。
func SanitizeHTML(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(s))
	if err != nil {
		return ""
	}
	doc.Find("script,style,iframe,frame,object,embed,applet,form,meta,link,base,svg,math,noscript").Remove()
	doc.Find("*").Each(func(_ int, sel *goquery.Selection) {
		if len(sel.Nodes) == 0 {
			return
		}
		n := sel.Nodes[0]
		if n.Type != xhtml.ElementNode {
			return
		}
		kept := n.Attr[:0]
		for _, a := range n.Attr {
			key := strings.ToLower(a.Key)
			val := strings.TrimSpace(a.Val)
			if strings.HasPrefix(key, "on") { // 事件处理器 onclick/onerror/...
				continue
			}
			if isSanitizeURLAttr(key) && isDangerousURL(val) {
				continue
			}
			kept = append(kept, a)
		}
		n.Attr = kept
	})
	out, _ := doc.Find("body").Html()
	return out
}

func isSanitizeURLAttr(key string) bool {
	switch key {
	case "href", "src", "action", "formaction", "background", "poster", "data", "xlink:href", "srcset":
		return true
	}
	return false
}

// isDangerousURL 判定可能执行脚本的 URL 协议（先剥空白/控制字符防绕过）。
func isDangerousURL(u string) bool {
	compact := strings.Map(func(r rune) rune {
		if r <= 0x20 {
			return -1
		}
		return r
	}, u)
	low := strings.ToLower(compact)
	return strings.HasPrefix(low, "javascript:") ||
		strings.HasPrefix(low, "vbscript:") ||
		strings.HasPrefix(low, "data:text/html")
}
