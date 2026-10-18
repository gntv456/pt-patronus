package chain

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/iyuu"
)

// multiProvider 按下载器 id 返回各自的做种列表（现网形态：NAS + 台式机各挂一台 qB）。
type multiProvider struct{ byID map[uint]*fakeDL }

func (p multiProvider) Get(_ context.Context, id uint) (reseedClient, error) { return p.byID[id], nil }

// TestReseedIsPerDownloader 同一颗种子被两台下载器做着时，辅种候选必须**每台一条**。
// 回归：候选去重只按 (info_hash, candidate_site)（唯一索引同名），不含 downloader_id，
// 于是第二台扫描时撞上第一台的行就被当成「进行中」跳过 ⇒
// ② 那台的候选永远不出现，而它的自动辅种设置在 automation 里是**按下载器逐台**跑的
// （ListAutoReseedSettings → for st → Reseed(st.DownloaderID)），看起来「开关已开」实则整台空转；
// 另外辅种要复用**本机**的 save_path，两台的物理路径本来就不同，一条全局候选也存不下两套路径。
func TestReseedIsPerDownloader(t *testing.T) {
	db := newReseedDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	cfgOp := oper.NewSystemConfigOper(db)
	if _, err := siteOp.Create(oper.SiteInput{
		Name: "ourbits", Platform: "nexusphp", BaseURL: "https://ourbits.org", Passkey: "PK",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cfgOp.Set(IYUUTokenKey, "TOKEN"); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("service") {
		case "App.Api.Hash":
			io.WriteString(w, `{"ret":200,"data":[{"hash":"H1","torrent":[{"sid":7,"torrent_id":42,"info_hash":"H1"}]}]}`)
		case "App.Api.Sites":
			io.WriteString(w, `{"ret":200,"data":{"sites":[{"id":7,"site":"ourbits","base_url":"ourbits.org","download_page":"download.php?id={}","is_https":1}]}}`)
		default:
			http.Error(w, "unknown", 400)
		}
	}))
	defer ts.Close()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}

	dl1 := &fakeDL{torrents: []downloader.Torrent{{Hash: "H1", Name: "src", SavePath: "/nas1/seed"}}}
	dl2 := &fakeDL{torrents: []downloader.Torrent{{Hash: "H1", Name: "src", SavePath: "/nas2/seed"}}}
	ch := NewKeepSeedChain(NewBase(module.New(), event.New()), nil, iyuu.New(hc, ts.URL),
		multiProvider{byID: map[uint]*fakeDL{1: dl1, 2: dl2}}, nil, siteOp, cfgOp, db, nil)
	ctx := context.Background()

	first, err := ch.Reseed(ctx, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("下载器1 应建 1 条候选，got n=%d err=%v", len(first), err)
	}

	second, err := ch.Reseed(ctx, 2)
	if err != nil {
		t.Fatalf("下载器2 Reseed: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("两台各挂同一颗做种时，第二台也必须有自己的候选（它的保存路径是 /nas2/seed），got %d 条", len(second))
	}
	if second[0].DownloaderID != 2 || second[0].SavePath != "/nas2/seed" {
		t.Errorf("第二台候选应绑自己的下载器与路径，got %+v", second[0])
	}

	// 正控：同一台再扫仍不许重复生成（去重语义不能被放宽成「每次都建新行」）。
	again, err := ch.Reseed(ctx, 2)
	if err != nil || len(again) != 0 {
		t.Errorf("同一下载器重复扫描应去重，got n=%d err=%v", len(again), err)
	}

	// 正控：候选总数 = 2（每台一条），且互不覆盖。
	var n int64
	db.Model(&models.CrossSeedTask{}).Count(&n)
	if n != 2 {
		t.Errorf("库里应有 2 条候选（1 台一条），got %d", n)
	}
}

// TestUpsertCrossSeedTaskIsPerDownloader 本地辅种/Torznab 补候选那一路（upsertCrossSeedTask）
// 与 IYUU 路共用同一个去重形状，也必须按下载器分开；顺带钉住两条既有语义没被改坏：
// 同机重复不新建、用户跳过的候选恢复成 pending 但不交给调用方（自动化不得推翻显式否决）。
func TestUpsertCrossSeedTaskIsPerDownloader(t *testing.T) {
	db := newReseedDB(t)
	ch := NewKeepSeedChain(NewBase(module.New(), event.New()), nil, nil, nil, nil, nil,
		oper.NewSystemConfigOper(db), db, nil)

	first, ok := ch.upsertCrossSeedTask(1, 0, "H1", "src", "ourbits", "u1", "/nas1/seed", 80)
	if !ok || first.SavePath != "/nas1/seed" {
		t.Fatalf("第一台应新建候选，got ok=%v %+v", ok, first)
	}
	second, ok := ch.upsertCrossSeedTask(2, 0, "H1", "src", "ourbits", "u2", "/nas2/seed", 70)
	if !ok {
		t.Fatalf("第二台撞上第一台的行就被当成「进行中」——那台的候选永远不会出现")
	}
	if second.DownloaderID != 2 || second.SavePath != "/nas2/seed" {
		t.Errorf("第二台候选应绑自己，got %+v", second)
	}

	// 正控：同一台重复扫描不新建（放宽成「无去重」是另一种坏）。
	if _, ok := ch.upsertCrossSeedTask(2, 0, "H1", "src", "ourbits", "u2", "/nas2/seed", 70); ok {
		t.Error("同下载器同 hash 同候选站不该再判成新增")
	}

	// 正控：skipped 的候选被重新发现时恢复 pending，但不交给自动化（既有语义）。
	if e := db.Model(&models.CrossSeedTask{}).Where("downloader_id = ?", 1).
		Update("status", "skipped").Error; e != nil {
		t.Fatal(e)
	}
	if _, ok := ch.upsertCrossSeedTask(1, 0, "H1", "src", "ourbits", "u1", "/nas1/seed", 80); ok {
		t.Error("用户跳过过的候选不许交给 AutoReseedAdd")
	}
	var status string
	db.Model(&models.CrossSeedTask{}).Where("downloader_id = ?", 1).Select("status").Scan(&status)
	if status != "pending" {
		t.Errorf("重新发现的 skipped 行应恢复成 pending（让用户再决定一次），got %q", status)
	}
}
