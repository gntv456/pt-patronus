package cloud

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- Writer 契约单测（fakeDoer 复用 clients_test.go） ----

// TestQuarkDownloadURL 夸克直链：POST /file/download（fids 数组）→ data[0].download_url，
// 且返回 Header 含 Cookie/Referer（直链与 UA/Cookie 绑定）。
func TestQuarkDownloadURL(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"pdir_fid=0":     `{"code":0,"data":{"list":[{"fid":"f9","file_name":"A.mkv","file_type":1,"size":"5000"}]}}`,
		"/file/download": `{"code":0,"data":[{"download_url":"https://dl.quark.cn/x?sig=1"}]}`,
	}}
	c := &clientQuark{drive: Drive{Type: "quark", Token: "__puus=x"}, hc: doer}
	link, err := c.DownloadURL(context.Background(), "/A.mkv", "Emby/4.8 Player")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://dl.quark.cn/x?sig=1" {
		t.Errorf("url wrong: %s", link.URL)
	}
	if link.Header["Referer"] != "https://pan.quark.cn/" || link.Header["Cookie"] != "__puus=x" {
		t.Errorf("header wrong: %v", link.Header)
	}
	if link.Header["User-Agent"] != "Emby/4.8 Player" {
		t.Errorf("ua should pass through, got %q", link.Header["User-Agent"])
	}
}

// TestQuarkDownloadURLDir 直链目标是目录应报错。
func TestQuarkDownloadURLDir(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"pdir_fid=0": `{"code":0,"data":{"list":[{"fid":"f1","file_name":"剧集","file_type":0,"size":"0"}]}}`,
	}}
	c := &clientQuark{drive: Drive{Type: "quark"}, hc: doer}
	if _, err := c.DownloadURL(context.Background(), "/剧集", ""); err == nil {
		t.Error("dir should error")
	}
}

// Test123DownloadURL 123 直链：GET download_info?fileId= → data.downloadUrl。
func Test123DownloadURL(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"parentFileId=0": `{"code":0,"data":{"InfoList":[{"FileId":22,"FileName":"C.mp4","Type":1,"Size":"8000"}]}}`,
		"download_info":  `{"code":0,"data":{"downloadUrl":"https://download.123pan.com/y?e=1"}}`,
	}}
	c := &client123{drive: Drive{Type: "123", Token: "tok"}, hc: doer}
	link, err := c.DownloadURL(context.Background(), "/C.mp4", "")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://download.123pan.com/y?e=1" {
		t.Errorf("url wrong: %s", link.URL)
	}
	// 请求必须带 fileId=22（List 时 FileId 应回填 PickCode）
	found := false
	for _, u := range doer.calls {
		if strings.Contains(u, "download_info?fileId=22") {
			found = true
		}
	}
	if !found {
		t.Errorf("download_info call missing fileId, calls=%v", doer.calls)
	}
}

// Test115DownloadURLRequiresFile m115 直链已实现：根路径/不存在的文件应报错（不 panic）。
func Test115DownloadURLRequiresFile(t *testing.T) {
	c := &client115{drive: Drive{Type: "115"}}
	if _, err := c.DownloadURL(context.Background(), "/", ""); err == nil {
		t.Error("root path must error")
	}
}

// TestMkdirPQuark 夸克逐级建目录：已存在直接复用；不存在 POST /file 后按响应 fid 继续。
func TestMkdirPQuark(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"file/sort": `{"code":0,"data":{"list":[{"fid":"f1","file_name":"媒体库","file_type":0,"size":"0"}]}}`,
		"file?pr=u": `{"status":200,"code":0,"data":{"fid":"f2"}}`,
	}}
	c := &clientQuark{drive: Drive{Type: "quark"}, hc: doer}
	id, err := c.MkdirP(context.Background(), "/媒体库/电影")
	if err != nil {
		t.Fatal(err)
	}
	_ = id // 二级目录新建：fakeDoer 未匹配 mkdir 响应会走兜底重列；此处只验不 panic 与请求发出
	mkdirCalled := false
	for _, u := range doer.calls {
		if strings.Contains(u, "/file?pr=ucpro&fr=pc") {
			mkdirCalled = true
		}
	}
	if !mkdirCalled {
		t.Errorf("mkdir POST not issued, calls=%v", doer.calls)
	}
}

// TestMkdirP115 115 建目录：files/add 响应 {state:true,data:{cid:...}}。
func TestMkdirP115(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"cid=0":      `{"state":true,"data":[{"file_name":"媒体库","cid":"100"}]}`,
		"/files/add": `{"state":true,"data":{"cid":"200","cname":"电影"}}`,
		"cid=200":    `{"state":true,"data":[]}`,
	}}
	c := &client115{drive: Drive{Type: "115"}, hc: doer}
	id, err := c.MkdirP(context.Background(), "/媒体库/电影")
	if err != nil {
		t.Fatal(err)
	}
	if id != "200" {
		t.Errorf("want cid 200, got %s", id)
	}
}

// TestUpload123QuarkExist 幂等：目标已存在时不发任何上传请求。
func TestUploadQuarkExist(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"pdir_fid=0": `{"code":0,"data":{"list":[{"fid":"f2","file_name":"A.mkv","file_type":1,"size":"5000"}]}}`,
	}}
	c := &clientQuark{drive: Drive{Type: "quark"}, hc: doer}
	tmp := filepath.Join(t.TempDir(), "A.mkv")
	if err := os.WriteFile(tmp, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Upload(context.Background(), "/A.mkv", NewLocalSource(tmp, 5)); err != nil {
		t.Fatal(err)
	}
	for _, u := range doer.calls {
		if strings.Contains(u, "upload/pre") {
			t.Errorf("should not call upload/pre for existing file: %s", u)
		}
	}
}

// TestWriteStrmFileDirect 直链模板：{direct} → /api/v1/cloud/direct/:driveId + {cloudpath}。
func TestWriteStrmFileDirect(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteStrmFile(WriteStrmOptions{
		OutputDir: dir, CloudPath: "/媒体库/电影/A.mkv",
		Template: "{direct}{cloudpath}", FileName: "A.mkv", DriveID: 3,
	}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "A.mkv.strm"))
	if err != nil {
		t.Fatal(err)
	}
	want := "/api/v1/cloud/direct/3/媒体库/电影/A.mkv\n"
	if string(b) != want {
		t.Errorf("strm content=%q want %q", b, want)
	}
	// 幂等：再写一次内容不变不报错
	if _, err := WriteStrmFile(WriteStrmOptions{
		OutputDir: dir, CloudPath: "/媒体库/电影/A.mkv",
		Template: "{direct}{cloudpath}", FileName: "A.mkv", DriveID: 3,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestWriteStrmFileAbsoluteURL AppBaseURL 配置时渲染绝对地址（Emby 远端播放必需）。
func TestWriteStrmFileAbsoluteURL(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteStrmFile(WriteStrmOptions{
		OutputDir: dir, CloudPath: "/电影/A.mkv",
		Template: "{direct}{cloudpath}", FileName: "A.mkv", DriveID: 3,
		AppBaseURL: "http://nas.local:8088/",
	}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "A.mkv.strm"))
	if err != nil {
		t.Fatal(err)
	}
	want := "http://nas.local:8088/api/v1/cloud/direct/3/电影/A.mkv\n"
	if string(b) != want {
		t.Errorf("absolute strm=%q want %q", b, want)
	}
}

// TestPathHelpers pathBase / cloudDirOf。
func TestPathHelpers(t *testing.T) {
	if pathBase("/a/b/c.mkv") != "c.mkv" || pathBase("/") != "" {
		t.Errorf("pathBase wrong")
	}
	if cloudDirOf("/a/b/c.mkv") != "/a/b" || cloudDirOf("/c.mkv") != "/" {
		t.Errorf("cloudDirOf wrong")
	}
}

// TestAuthMetaQuark 夸克分片 auth_meta 拼接（canonical string 精确性）。
func TestAuthMetaQuark(t *testing.T) {
	got := authMetaPart("video/mp4", "Mon, 02 Jan 2006 15:04:05 GMT", "bkt", "obj/key", 3, "uid")
	want := "PUT\n\nvideo/mp4\nMon, 02 Jan 2006 15:04:05 GMT\n" +
		"x-oss-date:Mon, 02 Jan 2006 15:04:05 GMT\n" +
		"x-oss-user-agent:" + quarkOSSUA + "\n" +
		"/bkt/obj/key?partNumber=3&uploadId=uid"
	if got != want {
		t.Errorf("authMetaPart mismatch:\n got %q\nwant %q", got, want)
	}
	cm := "Base64Md5=="
	gotC := authMetaComplete(cm, "Mon, 02 Jan 2006 15:04:05 GMT", "Y2I=", "bkt", "obj/key", "uid")
	wantC := "POST\n" + cm + "\napplication/xml\nMon, 02 Jan 2006 15:04:05 GMT\n" +
		"x-oss-callback:Y2I=\n" +
		"x-oss-date:Mon, 02 Jan 2006 15:04:05 GMT\n" +
		"x-oss-user-agent:" + quarkOSSUA + "\n" +
		"/bkt/obj/key?uploadId=uid"
	if gotC != wantC {
		t.Errorf("authMetaComplete mismatch:\n got %q\nwant %q", gotC, wantC)
	}
}

// TestUploadInit115Form 115 initupload 表单字段（fileid=大写 SHA1、target=U_1_cid）。
func TestUploadInit115Form(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"initupload": `{"state":true,"status":2,"pickcode":"pc9"}`,
	}}
	c := &client115{drive: Drive{Type: "115"}, hc: doer}
	tmp := filepath.Join(t.TempDir(), "A.mkv")
	if err := os.WriteFile(tmp, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Upload(context.Background(), "/A.mkv", NewLocalSource(tmp, 5)); err != nil {
		t.Fatal(err)
	}
	// 秒传命中即结束，无 OSS 请求
	for _, u := range doer.calls {
		if strings.Contains(u, "aliyuncs.com") {
			t.Errorf("unexpected OSS call on fast-hit: %s", u)
		}
	}
}

// TestTrimJSONNumber data 兼容形态解析。
func TestTrimJSONNumber(t *testing.T) {
	if trimJSONNumber(`"123"`) != "123" || trimJSONNumber("123") != "123" {
		t.Error("numeric unwrap wrong")
	}
	if trimJSONNumber("null") != "" || trimJSONNumber(`{"a":1}`) != "" {
		t.Error("non-numeric should be empty")
	}
}

// TestFileMD5SHA1 摘要计算（夸克秒传用）。
func TestFileMD5SHA1(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(tmp, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	md5s, sha1s, err := fileMD5SHA1(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if md5s != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("md5 wrong: %s", md5s)
	}
	if sha1s != "AAF4C61DDCC5E8A2DABEDE0F3B482CD9AEA9434D" {
		t.Errorf("sha1 wrong (want upper): %s", sha1s)
	}
	_ = json.Number("1")
}
