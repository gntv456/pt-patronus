package site

import (
	"encoding/base64"
	"testing"
)

// TestExtractUserIDFromCookie c_secure_pass：base64 payload 内 user_id 正则。
func TestExtractUserIDFromCookie(t *testing.T) {
	// payload = base64({"user_id":"12345"}) + ".suffix"
	payload := b64(`{"user_id":"12345","passkey":"abc"}`) + ".suffix"
	cookie := "c_secure_pass=" + payload + "; c_secure_uid=ignored"
	if got := ExtractUserIDFromCookie(cookie); got != "12345" {
		t.Errorf("c_secure_pass uid: got %q want 12345", got)
	}
	// c_secure_uid 纯数字回退
	if got := ExtractUserIDFromCookie("c_secure_uid=999"); got != "999" {
		t.Errorf("c_secure_uid uid: got %q want 999", got)
	}
}

// TestParseInviteStatus info_block 邀请链接 + takeinvite 表单 + 魔力。
func TestParseInviteStatus(t *testing.T) {
	html := `<html><body>
		<div id="info_block"><a href="invite.php">邀请[发送]: 5 (2)</a></div>
		<form action="takeinvite.php"><input type="text" name="email"><input type="submit"></form>
		<div>魔力: 1,234.5</div>
	</body></html>`
	doc := docFromHTML(t, html)
	st := ParseInviteStatus(doc)
	if !st.CanInvite {
		t.Error("canInvite: want true")
	}
	if st.PermanentCount != 5 {
		t.Errorf("permanent: got %d want 5", st.PermanentCount)
	}
	if st.TemporaryCount != 2 {
		t.Errorf("temporary: got %d want 2", st.TemporaryCount)
	}
	if st.Bonus < 1234.4 || st.Bonus > 1234.6 {
		t.Errorf("bonus: got %f want ~1234.5", st.Bonus)
	}
}

// TestParseInviteStatus_NoPerm 显式无权限覆盖。
func TestParseInviteStatus_NoPerm(t *testing.T) {
	html := `<html><body><div>您目前没有足够的邀请名额</div></body></html>`
	st := ParseInviteStatus(docFromHTML(t, html))
	if st.CanInvite {
		t.Error("canInvite: want false (no-perm override)")
	}
}

// TestParseInvitees 被邀请人：username + 流量 + 分享率 + 禁用属性检测。
func TestParseInvitees(t *testing.T) {
	html := `<html><body><table>
		<tr><td>用户名</td><td>上传</td><td>下载</td><td>分享率</td><td>状态</td></tr>
		<tr>
			<td><a href="userdetails.php?id=10">alice</a></td>
			<td>1.5 TB</td>
			<td>500 GB</td>
			<td>3.07</td>
			<td>正常</td>
		</tr>
		<tr class="rowbanned">
			<td><a href="userdetails.php?id=11">bob</a></td>
			<td>2 GB</td>
			<td>4 GB</td>
			<td>0.5</td>
			<td><img alt="disabled"></td>
		</tr>
	</table></body></html>`
	got := ParseInvitees(docFromHTML(t, html))
	if len(got) != 2 {
		t.Fatalf("invitees: got %d want 2", len(got))
	}
	if got[0].Username != "alice" {
		t.Errorf("invitee0 name: got %q want alice", got[0].Username)
	}
	if got[0].Uploaded == 0 || got[0].Downloaded == 0 {
		t.Errorf("invitee0 traffic not parsed: up=%d down=%d", got[0].Uploaded, got[0].Downloaded)
	}
	if got[0].Ratio != "3.07" {
		t.Errorf("invitee0 ratio: got %q want 3.07", got[0].Ratio)
	}
	if got[1].Status != "disabled" || got[1].Enabled {
		t.Errorf("invitee1 should be disabled: status=%q enabled=%v", got[1].Status, got[1].Enabled)
	}
	if !got[1].IsDead() {
		t.Error("invitee1 IsDead: want true")
	}
	if !got[1].IsLowRatio() {
		t.Error("invitee1 IsLowRatio (0.5): want true")
	}
}

// TestInviteeComputed 计算字段：IsLowRatio 需 downloaded>0；HasNoTrafficData。
func TestInviteeComputed(t *testing.T) {
	pure := InviteeInfo{Uploaded: 100, Downloaded: 0, Ratio: "0"}
	if pure.IsLowRatio() {
		t.Error("pure seeder should not be low-ratio")
	}
	if pure.HasNoTrafficData() {
		t.Error("with upload > 0, should not be no-traffic-data")
	}
	empty := InviteeInfo{}
	if !empty.HasNoTrafficData() {
		t.Error("empty invitee should be no-traffic-data")
	}
}

// TestParseInvitees_Dedup 重复用户名去重 + UserID 解析（排除"自己"的前置）。
func TestParseInvitees_Dedup(t *testing.T) {
	html := `<html><body><table>
		<tr><td>用户名</td><td>上传</td><td>下载</td><td>分享率</td><td>状态</td></tr>
		<tr><td><a href="userdetails.php?id=10">alice</a></td><td>1 GB</td><td>1 GB</td><td>1.0</td><td>正常</td></tr>
		<tr><td><a href="userdetails.php?id=10">alice</a></td><td>2 GB</td><td>2 GB</td><td>1.0</td><td>正常</td></tr>
		<tr><td><a href="userdetails.php?id=20">bob</a></td><td>3 GB</td><td>3 GB</td><td>1.0</td><td>正常</td></tr>
	</table></body></html>`
	got := ParseInvitees(docFromHTML(t, html))
	if len(got) != 2 {
		t.Fatalf("dedup: got %d want 2 (alice+bob)", len(got))
	}
	if got[0].Username != "alice" || got[0].UserID != "10" {
		t.Errorf("alice: name=%q id=%q want alice/10", got[0].Username, got[0].UserID)
	}
	if got[1].Username != "bob" || got[1].UserID != "20" {
		t.Errorf("bob: name=%q id=%q want bob/20", got[1].Username, got[1].UserID)
	}
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
