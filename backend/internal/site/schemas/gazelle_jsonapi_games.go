package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/site"
)

func (g *GazelleJSONAPI) gazelleGamesRequest(ctx context.Context, baseURL, token, request string, params map[string]string) (json.RawMessage, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("gazellegames api token required")
	}
	q := url.Values{}
	q.Set("request", request)
	for k, v := range params {
		q.Set(k, v)
	}
	target := joinURL(baseURL, "api.php") + "?" + q.Encode()
	resp, err := g.hc.GetWithHeaders(ctx, target, "", map[string]string{
		"X-API-Key": token,
		"Accept":    "application/json",
	}, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	var env gazelleEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("parse gazellegames response: %w", err)
	}
	if env.Status != "success" {
		if isGazelleAuthError(env.Error) {
			return nil, site.ErrNotLoggedIn
		}
		return nil, fmt.Errorf("gazellegames %s failed: %s", request, env.Error)
	}
	return env.Response, nil
}

func (g *GazelleJSONAPI) getGazelleGamesAuthKey(ctx context.Context, baseURL, token string) (gazelleAuthKey, error) {
	cacheKey := baseURL + "|gazellegames"
	if v, ok := g.keys.Load(cacheKey); ok {
		return v.(gazelleAuthKey), nil
	}
	data, err := g.gazelleGamesRequest(ctx, baseURL, token, "quick_user", nil)
	if err != nil {
		return gazelleAuthKey{}, err
	}
	var idx struct {
		Authkey string `json:"authkey"`
		Passkey string `json:"passkey"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return gazelleAuthKey{}, fmt.Errorf("parse gazellegames quick_user: %w", err)
	}
	k := gazelleAuthKey{authkey: idx.Authkey, passkey: idx.Passkey}
	g.keys.Store(cacheKey, k)
	return k, nil
}

type gazelleGamesGroup struct {
	Artists []struct {
		Name string `json:"name"`
	} `json:"Artists"`
	Year     string                         `json:"Year"`
	Torrents map[string]gazelleGamesTorrent `json:"Torrents"`
}

type gazelleGamesTorrent struct {
	ID            any    `json:"ID"`
	GroupID       any    `json:"GroupID"`
	CategoryID    any    `json:"CategoryID"`
	Language      string `json:"Language"`
	Format        string `json:"Format"`
	Encoding      string `json:"Encoding"`
	Region        string `json:"Region"`
	Remastered    string `json:"Remastered"`
	RemasterYear  string `json:"RemasterYear"`
	RemasterTitle string `json:"RemasterTitle"`
	Scene         string `json:"Scene"`
	ReleaseTitle  string `json:"ReleaseTitle"`
	GameDOXType   string `json:"GameDOXType"`
	Size          any    `json:"Size"`
	Time          string `json:"Time"`
	Miscellaneous string `json:"Miscellaneous"`
	FreeTorrent   string `json:"FreeTorrent"`
	Seeders       any    `json:"Seeders"`
	Leechers      any    `json:"Leechers"`
	Snatched      any    `json:"Snatched"`
}

var gazelleGamesCategoryMap = map[int64]string{
	1: "Games",
	2: "Applications",
	3: "E-Books",
	4: "OST",
}

func (g *GazelleJSONAPI) searchGazelleGames(ctx context.Context, baseURL, token, keyword string, page int) ([]site.Torrent, error) {
	params := map[string]string{
		"search_type": "torrents",
		"order_by":    "time",
		"order_way":   "desc",
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		params["searchstr"] = strings.ReplaceAll(kw, ".", " ")
	}
	if page > 1 {
		params["page"] = strconv.Itoa(page)
	}
	data, err := g.gazelleGamesRequest(ctx, baseURL, token, "search", params)
	if err != nil {
		return nil, err
	}
	var groups map[string]gazelleGamesGroup
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, fmt.Errorf("parse gazellegames search: %w", err)
	}
	k, kerr := g.getGazelleGamesAuthKey(ctx, baseURL, token)
	out := []site.Torrent{}
	for _, group := range groups {
		for _, tor := range group.Torrents {
			id := fmt.Sprint(tor.ID)
			groupID := fmt.Sprint(tor.GroupID)
			out = append(out, site.Torrent{
				ID:          id,
				Title:       strings.TrimSpace(tor.ReleaseTitle),
				SubTitle:    gazelleGamesSubtitle(group, tor),
				DetailURL:   fmt.Sprintf("%s/torrents.php?id=%s&torrentid=%s", strings.TrimRight(baseURL, "/"), groupID, id),
				DownloadURL: gazelleDL(baseURL, id, k, kerr),
				Size:        formatMTBytes(toInt64(tor.Size)),
				Seeders:     int(toInt64(tor.Seeders)),
				Leechers:    int(toInt64(tor.Leechers)),
				Completed:   int(toInt64(tor.Snatched)),
				Time:        site.ApplyFilters(tor.Time, []site.Filter{{Name: "parse_time"}}),
				Category:    gazelleGamesCategory(group, tor),
				Free:        tor.FreeTorrent == "1",
			})
		}
	}
	return out, nil
}

func gazelleGamesSubtitle(group gazelleGamesGroup, tor gazelleGamesTorrent) string {
	var parts []string
	relYear := ""
	if tor.Miscellaneous != "GameDOX" {
		switch {
		case tor.RemasterYear != "" && tor.RemasterYear != "0":
			relYear = tor.RemasterYear
		case group.Year != "" && group.Year != "0":
			relYear = group.Year
		case tor.Remastered == "1":
			relYear = "Unknown Release(s)"
		}
	}
	edition := tor.RemasterTitle
	if tor.Miscellaneous == "GameDOX" {
		edition = tor.GameDOXType
	}
	for _, v := range []string{relYear, edition, tor.Language, tor.Region, tor.Format, tor.Encoding, tor.Miscellaneous} {
		if s := strings.TrimSpace(v); s != "" {
			parts = append(parts, s)
		}
	}
	if toInt64(tor.Scene) > 0 {
		parts = append(parts, "Scene")
	}
	return strings.Join(parts, " / ")
}

func gazelleGamesCategory(group gazelleGamesGroup, tor gazelleGamesTorrent) string {
	if len(group.Artists) > 0 && strings.TrimSpace(group.Artists[0].Name) != "" {
		return strings.TrimSpace(group.Artists[0].Name)
	}
	if cat := gazelleGamesCategoryMap[toInt64(tor.CategoryID)]; cat != "" {
		return cat
	}
	return fmt.Sprint(tor.CategoryID)
}

// FetchUserInfo 两阶段：action=index（id/基础统计）+ action=user&id=N（joinDate/seeding/uploads，best-effort）。
func (g *GazelleJSONAPI) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {
	if def != nil && def.ID == "gazellegames" {
		return g.fetchGazelleGamesUserInfo(ctx, baseURL, cookie)
	}
	data, err := g.gazelleRequest(ctx, baseURL, cookie, "index", nil)
	if err != nil {
		return nil, err
	}
	var idx gazelleIndexResp
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse gazelle index: %w", err)
	}
	out := &site.UserInfo{
		ID:                  strconv.FormatInt(idx.ID, 10),
		Name:                idx.Username,
		Level:               gazelleClass(idx.Userstats.Class),
		Uploaded:            toInt64(idx.Userstats.Uploaded),
		Downloaded:          toInt64(idx.Userstats.Downloaded),
		Ratio:               toFloat64(idx.Userstats.Ratio),
		Bonus:               toFloat64(idx.Userstats.BonusPoints),
		BonusPerHour:        toFloat64(idx.Userstats.BonusPointsPerHour),
		SeedingBonusPerHour: toFloat64(idx.Userstats.SeedingBonusPointsPerHour),
		Seeding:             int(toInt64(idx.Userstats.SeedingCount)),
		Leeching:            int(toInt64(idx.Userstats.LeechingCount)),
		SeedingSize:         toInt64(idx.Userstats.SeedingSize),
	}
	if out.BonusPerHour == 0 {
		out.BonusPerHour = toFloat64(idx.Userstats.SeedingBonusPointsPerHour)
	}
	if idx.Userstats.JoinedDate != "" {
		out.JoinDate = idx.Userstats.JoinedDate
	}
	if idx.Userstats.LastAccess != "" {
		out.LastAccessAt = idx.Userstats.LastAccess
	}
	if def != nil && len(def.UserInfo.Fields) > 0 {
		site.ParseJSONUserInfoFields(data, def.UserInfo.Fields, out)
	}
	if idx.ID > 0 {
		if udata, e := g.gazelleRequest(ctx, baseURL, cookie, "user",
			map[string]string{"id": strconv.FormatInt(idx.ID, 10)}); e == nil {
			var u gazelleUserResp
			if json.Unmarshal(udata, &u) == nil {
				if out.JoinDate == "" && u.Stats.JoinedDate != "" {
					out.JoinDate = u.Stats.JoinedDate
				}
				if out.LastAccessAt == "" && u.Stats.LastAccess != "" {
					out.LastAccessAt = u.Stats.LastAccess
				}
				if s := toInt64(u.Community.Seeding); out.Seeding == 0 && s > 0 {
					out.Seeding = int(s)
				}
				if n := toInt64(u.Community.Uploaded); n > 0 {
					out.Uploads = int(n)
				}
			}
		}
	}
	applyStaticUserInfoFields(def, out)
	if def != nil && len(def.UserInfo.Pages) > 0 {
		if err := fetchUserInfoPages(ctx, &BaseHTMLAdapter{hc: g.hc}, def, baseURL, cookie, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func applyStaticUserInfoFields(def *site.SiteMetadata, u *site.UserInfo) {
	if def == nil || u == nil || len(def.UserInfo.Fields) == 0 {
		return
	}
	fields := map[string]site.ElementQuery{}
	for key, q := range def.UserInfo.Fields {
		if strings.TrimSpace(q.Text) != "" && q.Selector == "" && len(q.Selectors) == 0 && q.Data == "" {
			fields[key] = q
		}
	}
	if len(fields) == 0 {
		return
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<html></html>"))
	if err != nil {
		return
	}
	site.ParseUserInfoFields(doc, fields, u)
}

func (g *GazelleJSONAPI) fetchGazelleGamesUserInfo(ctx context.Context, baseURL, token string) (*site.UserInfo, error) {
	data, err := g.gazelleGamesRequest(ctx, baseURL, token, "quick_user", nil)
	if err != nil {
		return nil, err
	}
	var idx struct {
		Username  string `json:"username"`
		ID        int64  `json:"id"`
		Authkey   string `json:"authkey"`
		Passkey   string `json:"passkey"`
		Userstats struct {
			Uploaded   any `json:"uploaded"`
			Downloaded any `json:"downloaded"`
			Ratio      any `json:"ratio"`
			Class      any `json:"class"`
		} `json:"userstats"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse gazellegames quick_user: %w", err)
	}
	if idx.Authkey != "" || idx.Passkey != "" {
		g.keys.Store(baseURL+"|gazellegames", gazelleAuthKey{authkey: idx.Authkey, passkey: idx.Passkey})
	}
	out := &site.UserInfo{
		ID:         strconv.FormatInt(idx.ID, 10),
		Name:       idx.Username,
		Level:      gazelleClass(idx.Userstats.Class),
		Uploaded:   toInt64(idx.Userstats.Uploaded),
		Downloaded: toInt64(idx.Userstats.Downloaded),
		Ratio:      toFloat64(idx.Userstats.Ratio),
	}
	if idx.ID > 0 {
		if udata, e := g.gazelleGamesRequest(ctx, baseURL, token, "user", map[string]string{"id": out.ID}); e == nil {
			var u struct {
				Stats struct {
					JoinedDate string `json:"joinedDate"`
					Gold       any    `json:"gold"`
				} `json:"stats"`
				Community struct {
					Seeding    any `json:"seeding"`
					Uploaded   any `json:"uploaded"`
					HourlyGold any `json:"hourlyGold"`
					SeedSize   any `json:"seedSize"`
				} `json:"community"`
			}
			if json.Unmarshal(udata, &u) == nil {
				out.JoinDate = u.Stats.JoinedDate
				out.Seeding = int(toInt64(u.Community.Seeding))
				out.Uploads = int(toInt64(u.Community.Uploaded))
				out.Bonus = toFloat64(u.Stats.Gold)
				out.BonusPerHour = toFloat64(u.Community.HourlyGold)
				out.SeedingSize = toInt64(u.Community.SeedSize)
			}
		}
	}
	return out, nil
}
