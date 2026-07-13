// Command pttime_dump 抓 PTTime 真实页面并对照 PT-depiler 选择器命中情况，
// 用于精确诊断「PTTime 数据显示不正确」的根因。
//
// 安全：cookie 仅从环境变量读，绝不落盘；HTML 输出到系统临时目录（os.TempDir），
// 不进 repo。诊断完可删。
//
// 运行（在 backend/ 下）：
//
//	PTTIME_COOKIE='c_dp=...; ...' PTTIME_UID=12345 go run ./tools/pttime_dump
//	PTTIME_COOKIE='...' PTTIME_UID=12345 PTTIME_BASE=https://pttime.xxx go run ./tools/pttime_dump
//
// 可选：PTTIME_OUT 指定输出目录（默认 $TMPDIR/pttime_live）。
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

func main() {
	cookie := os.Getenv("PTTIME_COOKIE")
	if cookie == "" {
		if cf := os.Getenv("PTTIME_COOKIE_FILE"); cf != "" {
			b, err := os.ReadFile(cf)
			if err != nil {
				fmt.Println("read cookie file:", err)
				os.Exit(1)
			}
			cookie = strings.TrimSpace(string(b))
		}
	}
	if cookie == "" {
		fmt.Println("PTTIME_COOKIE env (或 PTTIME_COOKIE_FILE 指向文件) required")
		os.Exit(1)
	}
	uid := os.Getenv("PTTIME_UID")
	if uid == "" {
		fmt.Println("PTTIME_UID env required (your numeric user id)")
		os.Exit(1)
	}
	base := strings.TrimRight(envOr("PTTIME_BASE", "https://www.pttime.org"), "/")
	out := envOr("PTTIME_OUT", filepath.Join(os.TempDir(), "pttime_live"))
	if err := os.MkdirAll(out, 0o755); err != nil {
		fmt.Println("mkdir out:", err)
		os.Exit(1)
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Proxy:           http.ProxyFromEnvironment,
		},
	}
	ctx := context.Background()

	pages := []struct {
		name string
		url  string
	}{
		{"index", base + "/index.php"},
		{"userdetails", base + "/userdetails.php?id=" + uid},
		{"seeding", base + "/getusertorrentlist.php?userid=" + uid + "&type=seeding"},
		{"seeding_ajax", base + "/getusertorrentlistajax.php?userid=" + uid + "&type=seeding"},
		{"mybonus", base + "/mybonus.php"},
		{"messages", base + "/messages.php"},
	}

	for _, p := range pages {
		fmt.Printf("\n========== %s ==========\n", p.name)
		body, status, err := fetch(ctx, client, p.url, cookie)
		if err != nil {
			fmt.Printf("FETCH ERROR: %v\n", err)
			continue
		}
		cf := strings.Contains(strings.ToLower(string(body)), "just a moment") || strings.Contains(strings.ToLower(string(body)), "cf-browser-verification")
		fmt.Printf("status=%d bytes=%d cloudflare=%v url=%s\n", status, len(body), cf, p.url)
		fname := filepath.Join(out, p.name+".html")
		if err := os.WriteFile(fname, body, 0o644); err != nil {
			fmt.Println("write:", err)
			continue
		}
		fmt.Printf("saved: %s\n", fname)
		diagnose(p.name, body)
	}
	fmt.Printf("\n输出目录: %s\n", out)
}

func fetch(ctx context.Context, c *http.Client, url, cookie string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, resp.StatusCode, err
	}
	return buf.Bytes(), resp.StatusCode, nil
}

func diagnose(name string, body []byte) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		fmt.Println("parse html:", err)
		return
	}
	switch name {
	case "index":
		diagnoseIndex(doc)
	case "userdetails":
		diagnoseUserDetails(doc)
	case "seeding", "seeding_ajax":
		diagnoseSeeding(name, doc, body)
	case "mybonus":
		diagnoseMyBonus(doc)
	case "messages":
		diagnoseMessages(doc)
	}
}

func diagnoseIndex(doc *goquery.Document) {
	fmt.Println("--- index.php: #info_block ---")
	ib := doc.Find("#info_block")
	if ib.Length() == 0 {
		fmt.Println("  #info_block NOT FOUND")
		return
	}
	fmt.Printf("  #info_block bytes=%d\n", len(ib.Text()))
	// PT-depiler uploaded/downloaded/ratio 用的锚点
	for _, label := range []string{"上传量", "上傳量", "Uploaded", "下载量", "下載量", "Downloaded", "分享率", "Ratio", "魔力", "Karma", "当前活动", "当前做种", "Active"} {
		if strings.Contains(ib.Text(), label) {
			fmt.Printf("  命中label[%s]: 是\n", label)
		}
	}
	// color_* 锚点
	for _, cls := range []string{".color_uploaded", ".color_downloaded", ".color_ratio", ".color_bonus", ".color_active"} {
		fmt.Printf("  锚点 %s -> 命中 %d 个\n", cls, ib.Find(cls).Length())
	}
	// font.color_* 父节点文本样例（前 3 个）
	ib.Find("font[class*='color_'], span[class*='color_']").Each(func(i int, s *goquery.Selection) {
		if i >= 6 {
			return
		}
		cls, _ := s.Attr("class")
		fmt.Printf("  [%d] class=%s 父文本=%q\n", i, cls, trunc(s.Parent().Text(), 90))
	})
	// 打印 #info_block outerHTML 片段（前 1200 字符），含结构
	html, _ := ib.Html()
	fmt.Printf("  #info_block innerHTML(截断):\n%s\n", trunc(html, 1200))

	// 公告区诊断（ParseAnnouncements 的 5 级级联选择器 + PTTime 候选容器）
	fmt.Println("--- index.php: 公告区 ---")
	for _, sel := range []string{
		`li.index-news-item`, `.index-news-item`,
		`div[id^="ka"]`, `p[id^="ka"]`,
		`#announcement`, `#ka1`, `.announcement`,
		`.siteinfo`, `#siteinfo`, `table.sitewarn`,
		`div.text.infinity`, `.infobox`, `.notice`,
	} {
		s := doc.Find(sel)
		if s.Length() > 0 {
			fmt.Printf("  %-28s -> 命中 %d，首块=%q\n", sel, s.Length(), trunc(s.First().Text(), 120))
		}
	}
}

func diagnoseUserDetails(doc *goquery.Document) {
	fmt.Println("--- userdetails.php: rowhead 表 ---")
	count := 0
	doc.Find("td.rowhead, td[class*='rowhead']").Each(func(_ int, s *goquery.Selection) {
		if count >= 25 {
			return
		}
		label := strings.TrimSpace(s.Text())
		next := s.Next()
		nextHTML, _ := next.Html()
		fmt.Printf("  rowhead[%q] -> next td text=%q html=%q\n", trunc(label, 20), trunc(next.Text(), 60), trunc(nextHTML, 120))
		count++
	})
	// 等级行：PT-depiler 用 td.rowhead:contains('等级') + td > b（attr title）
	fmt.Println("--- 等级行 ---")
	for _, sel := range []string{`td.rowhead:contains('等级') + td > b`, `td.rowhead:contains('等级') + td > img`, `td.rowhead:contains('Class') + td > b`, `td.rowhead:contains('Class') + td > img`} {
		s := doc.Find(sel)
		if s.Length() > 0 {
			title, _ := s.Attr("title")
			fmt.Printf("  %s -> 命中 text=%q title=%q\n", sel, trunc(s.Text(), 30), title)
		}
	}
}

func diagnoseSeeding(name string, doc *goquery.Document, body []byte) {
	fmt.Printf("--- %s: 做种汇总 ---\n", name)
	// 标准 nexusphp 汇总 div「N | SIZE」
	bodyStr := string(body)
	if strings.Contains(bodyStr, " | ") {
		// 找含 ' | ' 的行
		for _, line := range strings.Split(bodyStr, "\n") {
			if strings.Contains(line, " | ") && (strings.Contains(line, "TiB") || strings.Contains(line, "GiB") || strings.Contains(line, "TB") || strings.Contains(line, "GB")) {
				fmt.Printf("  含 'N | SIZE' 行: %q\n", trunc(strings.TrimSpace(line), 120))
			}
		}
	} else {
		fmt.Println("  页面无 ' | ' 汇总格式")
	}
	// PT-depiler PTTime 专用：#outer 直接子 span
	outer := doc.Find("#outer")
	if outer.Length() == 0 {
		fmt.Println("  #outer NOT FOUND")
	} else {
		fmt.Printf("  #outer 命中，直接子元素 %d 个:\n", outer.Children().Length())
		outer.Children().Each(func(i int, s *goquery.Selection) {
			if i >= 8 {
				return
			}
			// lastChild 文本：goquery 无直接 API，取最后一个子节点的文本
			lastChildText := lastChildText(s)
			fmt.Printf("    child[%d] tag=%s text=%q lastChild=%q\n", i, goquery.NodeName(s), trunc(s.Text(), 70), trunc(lastChildText, 40))
			// span 内 b 子节点
			if b := s.Find("b"); b.Length() > 0 {
				fmt.Printf("      -> 内含 <b>: %q\n", trunc(b.Text(), 30))
			}
		})
	}
	// 表格行数
	fmt.Printf("  table tr 数=%d (table:last tr=%d)\n", doc.Find("table tr").Length(), doc.Find("table").Last().Find("tr").Length())
	// <b>N</b>条记录
	if i := strings.Index(bodyStr, "条记录"); i >= 0 {
		fmt.Printf("  含「条记录」: ...%q...\n", trunc(bodyStr[max(0, i-40):i+10], 80))
	}
}

func diagnoseMyBonus(doc *goquery.Document) {
	fmt.Println("--- mybonus.php: 每小时积分 ---")
	for _, sel := range []string{`div:contains("你当前每小时能获取")`, `div:contains("你當前每小時能獲取")`, `div:contains("currently getting")`, `#outer td[rowspan]`} {
		s := doc.Find(sel)
		if s.Length() > 0 {
			fmt.Printf("  %s -> 命中 %d 个，最后文本=%q\n", sel, s.Length(), trunc(s.Last().Text(), 90))
		}
	}
}

func diagnoseMessages(doc *goquery.Document) {
	fmt.Println("--- messages.php: 私信列表 ---")
	rows := doc.Find(`table tr:has(td.rowfollow)`)
	fmt.Printf("  table tr:has(td.rowfollow) = %d\n", rows.Length())
	rows.Each(func(i int, tr *goquery.Selection) {
		if i >= 3 {
			return
		}
		direct := tr.Children().Filter("td.rowfollow").Length()
		vm := tr.Find(`a[href*="viewmessage"]`).Length()
		fmt.Printf("  row[%d] 直接子td.rowfollow=%d viewmessage=%d 文本=%q\n", i, direct, vm, trunc(tr.Text(), 90))
	})
	fmt.Printf("  a[href*=viewmessage]=%d  a[href*=viewpm]=%d\n",
		doc.Find(`a[href*="viewmessage"]`).Length(), doc.Find(`a[href*="viewpm"]`).Length())
	allRows := doc.Find("table tr")
	fmt.Printf("  全部 table tr=%d，前 4 行样例:\n", allRows.Length())
	allRows.Each(func(i int, tr *goquery.Selection) {
		if i >= 4 {
			return
		}
		cells := tr.Find("td")
		cellTexts := []string{}
		cells.Each(func(j int, td *goquery.Selection) {
			if j < 6 {
				cellTexts = append(cellTexts, trunc(strings.TrimSpace(td.Text()), 25))
			}
		})
		fmt.Printf("    tr[%d] td数=%d [%s]\n", i, cells.Length(), strings.Join(cellTexts, " | "))
	})
}

func lastChildText(s *goquery.Selection) string {
	if s.Length() == 0 {
		return ""
	}
	node := s.Get(0)
	if node.LastChild != nil {
		return strings.TrimSpace(node.LastChild.Data)
	}
	return ""
}

func trunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
