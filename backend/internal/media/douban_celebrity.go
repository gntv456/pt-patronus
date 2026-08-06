package media


import (
	"context"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func (d *Douban) Celebrity(ctx context.Context, personageID string) (*CelebrityProfile, error) {
	var raw struct {
		Name      string `json:"name"`
		LatinName string `json:"latin_title"`
		Cover     struct {
			Large struct {
				URL string `json:"url"`
			} `json:"large"`
		} `json:"cover"`
		FollowedCount int    `json:"followed_count"`
		Desc          string `json:"desc"`
		URL           string `json:"url"`
		Extra         struct {
			Info      [][2]string `json:"info"`
			ShortInfo string      `json:"short_info"`
		} `json:"extra"`
		Modules []struct {
			Type    string `json:"type"`
			Payload struct {
				Collections []struct {
					Works []struct {
						Roles   []string `json:"roles"`
						Subject struct {
							Title string `json:"title"`
							URL   string `json:"url"`
						} `json:"subject"`
					} `json:"works"`
				} `json:"collections"`
			} `json:"payload"`
		} `json:"modules"`
	}
	if err := d.get(ctx, "/elessar/subject/"+personageID, nil, &raw); err != nil {
		return nil, err
	}
	p := &CelebrityProfile{
		Name: raw.Name, LatinName: raw.LatinName, Avatar: raw.Cover.Large.URL,
		ShortInfo: raw.Extra.ShortInfo, Info: raw.Extra.Info,
		FollowedCount: raw.FollowedCount, DoubanURL: raw.URL,
	}
	// desc 是 HTML 多段 <p>，提纯文本段落；avatar 原样（personage 路径不过 doubanPosterURL）。
	if raw.Desc != "" {
		if doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw.Desc)); err == nil {
			doc.Find("p").Each(func(_ int, s *goquery.Selection) {
				if t := strings.TrimSpace(s.Text()); t != "" {
					p.Bio = append(p.Bio, t)
				}
			})
			if len(p.Bio) == 0 { // 无 <p> 标签时取整体文本兜底
				if t := strings.TrimSpace(doc.Text()); t != "" {
					p.Bio = []string{t}
				}
			}
		}
	}
	// 参演作品：work_collections 下各 collection 的 works（最多 20 个，跳过无标题）。
	for _, m := range raw.Modules {
		if m.Type != "work_collections" {
			continue
		}
		for _, col := range m.Payload.Collections {
			for _, w := range col.Works {
				if w.Subject.Title == "" || len(p.Works) >= 20 {
					break
				}
				p.Works = append(p.Works, CelebrityWork{
					Title: w.Subject.Title, Role: strings.Join(w.Roles, " / "), URL: w.Subject.URL,
				})
			}
		}
		break
	}
	return p, nil
}

// doubanPhotoFile 匹配豆瓣 photo 文件名（p 开头 + 数字）。
var doubanPhotoFile = regexp.MustCompile(`(p\d+)\.jpg`)

// doubanPosterURL 把豆瓣 cover_url 规整成稳定的竖版海报地址。
//
// frodo 返回的 cover_url 多为 qnmob3 的 large 横裁图（带 imageView2 .../h/120），
// 且防盗链下无 Referer 直连只回 1x1 占位图。改成 img9 的 m_ratio_poster（2:3 海报），
// 再由前端经 /media/image 反代（带 movie.douban.com Referer）取真图。
// 非 photo 封面（如 subject 的 s*.jpg）无法换算，原样返回。
func doubanPosterURL(cover string) string {
	if cover == "" {
		return ""
	}
	m := doubanPhotoFile.FindStringSubmatch(cover)
	if m == nil {
		return cover
	}
	return "https://img9.doubanio.com/view/photo/m_ratio_poster/public/" + m[1] + ".jpg"
}

// doubanType 把 frodo 的 type 映射到 MediaType（只取影视，过滤 book/music 等）。
func doubanType(t string) MediaType {
	switch strings.ToLower(t) {
	case "movie":
		return TypeMovie
	case "tv":
		return TypeTV
	}
	return ""
}

