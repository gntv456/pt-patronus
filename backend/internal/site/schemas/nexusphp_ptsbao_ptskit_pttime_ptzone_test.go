package schemas

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNexusPHPFetchPTSBaoPTSKITPTTimePTZoneUserInfoFromMigratedDefinitions(t *testing.T) {
	tests := []struct {
		id           string
		uid          string
		name         string
		level        string
		joinDate     string
		uploadedTiB  int64
		downloadGiB  int64
		ratio        float64
		bonusLabel   string
		bonus        float64
		bonusPerHour float64
		seeding      int
		seedingTiB   float64
		uploads      int
		listPath     string
		textLevel    bool
	}{
		{
			id:           "ptsbao",
			uid:          "1201",
			name:         "ptsbao-user",
			level:        "Power User",
			joinDate:     "2024-01-02",
			uploadedTiB:  2,
			downloadGiB:  512,
			ratio:        4,
			bonusLabel:   "魔力值",
			bonus:        12345.5,
			bonusPerHour: 21.5,
			seeding:      8,
			seedingTiB:   2.25,
			uploads:      3,
			listPath:     "/getusertorrentlistajax.php",
		},
		{
			id:           "ptskit",
			uid:          "1202",
			name:         "ptskit-user",
			level:        "Elite User",
			joinDate:     "2023-03-04",
			uploadedTiB:  3,
			downloadGiB:  768,
			ratio:        4.5,
			bonusLabel:   "魔力值",
			bonus:        23456.75,
			bonusPerHour: 32.25,
			seeding:      9,
			seedingTiB:   3.5,
			uploads:      4,
			listPath:     "/getusertorrentlistajax.php",
		},
		{
			id:           "pttime",
			uid:          "1203",
			name:         "pttime-user",
			level:        "小学",
			joinDate:     "2022-05-06",
			uploadedTiB:  4,
			downloadGiB:  1024,
			ratio:        4,
			bonusLabel:   "魔力值",
			bonus:        34567.25,
			bonusPerHour: 43.75,
			seeding:      10,
			seedingTiB:   4.75,
			uploads:      5,
			listPath:     "/getusertorrentlist.php",
			textLevel:    true,
		},
		{
			id:           "ptzone",
			uid:          "1204",
			name:         "ptzone-user",
			level:        "Crazy User",
			joinDate:     "2021-07-08",
			uploadedTiB:  5,
			downloadGiB:  1280,
			ratio:        4,
			bonusLabel:   "魔力值",
			bonus:        45678.5,
			bonusPerHour: 54.25,
			seeding:      11,
			seedingTiB:   5.5,
			uploads:      6,
			listPath:     "/getusertorrentlistajax.php",
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.php":
					_, _ = w.Write([]byte(indexHTML(tc.uid, tc.name, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus, tc.seeding, false)))
				case "/userdetails.php":
					if r.URL.Query().Get("id") != tc.uid {
						t.Fatalf("detail id = %q, want %s", r.URL.Query().Get("id"), tc.uid)
					}
					if tc.textLevel {
						_, _ = w.Write([]byte(pttimeDetailHTML(tc.level, tc.joinDate, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus)))
						return
					}
					_, _ = w.Write([]byte(detailHTML(tc.level, tc.joinDate, tc.uploadedTiB, tc.downloadGiB, tc.ratio, tc.bonusLabel, tc.bonus)))
				case "/mybonus.php":
					_, _ = w.Write([]byte(fmt.Sprintf(`<div>你当前每小时能获取 %.2f 个%s</div>`, tc.bonusPerHour, tc.bonusLabel)))
				case tc.listPath:
					if r.URL.Query().Get("userid") != tc.uid {
						t.Fatalf("list userid = %q, want %s", r.URL.Query().Get("userid"), tc.uid)
					}
					switch r.URL.Query().Get("type") {
					case "seeding":
						// PTTime 的 getusertorrentlist.php 用 #outer>span 聚合格式（非标准 ajax），
						// 子元素序：h1 / span(用户名) / span(共有<b>N</b>条记录) / span(资源总大小：XB) / ...
						// 对应 PT-depiler #outer > span:nth-child(3)（做种数）/ :nth-child(4)（做种体积）。
						// 其余 NexusPHP 站走标准 ajax「N | SIZE」汇总。
						if tc.id == "pttime" {
							_, _ = w.Write([]byte(fmt.Sprintf(
								`<div id="outer"><h1>正在做种</h1>`+
									`<span><b>%s:</b></span>`+
									`<span>共有<b>%d</b>条记录</span>`+
									`<span>资源总大小：%.2f TiB</span>`+
									`<span>上传总量：0B</span>`+
									`<span>下载总大小：0B</span>`+
									`<p>1 - %d</p>`+
									`<table><tbody><tr><th>类型</th></tr><tr><td>种子</td></tr></tbody></table></div>`,
								tc.name, tc.seeding, tc.seedingTiB, tc.seeding)))
						} else {
							_, _ = w.Write([]byte(fmt.Sprintf(`<div>%d | %.2f TiB</div>`, tc.seeding, tc.seedingTiB)))
						}
					case "uploaded":
						_, _ = w.Write([]byte(fmt.Sprintf(`<b>%d</b> records`, tc.uploads)))
					default:
						t.Fatalf("unexpected list type %q", r.URL.Query().Get("type"))
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			u := fetchTestUserInfo(t, tc.id, srv.URL)
			assertCoreStats(t, u, tc.uid, tc.name, tc.level, tc.joinDate, tc.uploadedTiB*(1<<40), tc.downloadGiB*(1<<30), tc.ratio, tc.bonus)
			if u.BonusPerHour != tc.bonusPerHour || u.Seeding != tc.seeding || u.Uploads != tc.uploads {
				t.Fatalf("stats = bph %g seeding %d uploads %d", u.BonusPerHour, u.Seeding, u.Uploads)
			}
			if want := int64(tc.seedingTiB * (1 << 40)); u.SeedingSize != want {
				t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
			}
		})
	}
}

// pttimeDetailHTML 生成 PTTime 风格的 userdetails.php：统计聚合在「核心数据」行的
// td.rowfollow 里（PTTime 无独立「传输」/「魔力」行），等级是 <b> 文本（非 img title）。
// 形态对齐真实 PTTime 页面（校准自实测抓取）。
func pttimeDetailHTML(level, joinDate string, uploadedTiB, downloadGiB int64, ratio float64, bonusLabel string, bonus float64) string {
	return fmt.Sprintf(`
<table>
  <tr><td class="rowhead">用户等级</td><td><b alt="%s" title="%s">%s</b></td></tr>
  <tr><td class="rowhead">加入日期</td><td>%s 09:10:11</td></tr>
  <tr><td class="rowhead">核心数据</td><td width="95%%" class="rowfollow" valign="top" align="left">分享率: <font color="">%.3g</font>&nbsp;&nbsp;&nbsp;&nbsp;上传量: %d.00 TB&nbsp;&nbsp;&nbsp;&nbsp;下载量: %d GB&nbsp;&nbsp;&nbsp;&nbsp;%s: %.2f</td></tr>
</table>`, level, level, level, joinDate, ratio, uploadedTiB, downloadGiB, bonusLabel, bonus)
}
