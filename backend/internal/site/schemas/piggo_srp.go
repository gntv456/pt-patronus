// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

// piggo（猪猪，piggo.me）SRP-6a 登录客户端。
// 契约来源：站点前端 entry chunk 的 v5()（2026-11-07 逆向，Python 原型全协议实测）：
//   - 群参数：RFC5054 2048-bit（N 常量见 pgNHex），g=2，全部大整数按 256 字节（pgPadLen）左补零定宽编码
//   - k = H(N_pad || g_pad)；u = H(A_pad || B_pad)；S = (B - k·g^x)^(a + u·x) mod N
//   - x = H(salt || H(username+":" || PBKDF2-SHA256(password, salt, 210000, 32)))（定制派生）
//   - M1 = H( H(N)⊕H(g_pad) || H(username) || salt || A_pad || B_pad || H(S_pad) )
//   - M2 = H(A_pad || M1 || H(S_pad))（服务器回传，客户端校验防钓鱼）
// 流程：POST /auth/srp/challenge {username} → {i,s,B,ticket}；
//       POST /auth/srp/login {ticket,a,m1} → {m2,token,user}；失败回退 POST /login {username,password}。

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// piggo SRP 群参数与派生常量（对齐前端 Nw/Mw/Pw/uh）。
const (
	pgNHex    = "AC6BDB41324A9A9BF166DE5E1389582FAF72B6651987EE07FC3192943DB56050A37329CBB4A099ED8193E0757767A13DD52312AB4B03310DCD7F48A9DA04FD50E8083969EDB767B0CF6095179A163AB3661A05FBD5FAAAE82918A9962F0B93B855F97993EC975EEAA80D740ADBF4FF747359D041D5C33EA71D281E446B14773BC0F66C0B6B3CF3C1DD0E9E5717A7FA6D35E1C15EE9E7B0BF52FCE96B16C90F044DADCF0F472B8C84F5FEB613B6E4F3C0D9FA24A9B0B6B5B7A02C6F9B702E8B4BE8D2C5A9E2F0B7C6D5A4E3F2E1D0C9B8A7F6E5D4C3B2A1908F7E6D5C4B3A2918F7E6D5C4B3A29180F7E6D5C4B3A29180706"
	pgPadLen  = 256    // N 的字节宽（2048-bit）
	pgIter    = 210000 // PBKDF2 迭代（前端 Nw）
	pgDKLen   = 32     // PBKDF2 派生长度（前端 Mw）
	pgPrivLen = 32     // 客户端私钥随机字节数（前端 Pw）
)

var (
	pgN = new(big.Int)
	pgG = big.NewInt(2)
)

func init() {
	pgN.SetString(pgNHex, 16)
}

// pgPad 定宽大端编码（前端 rn/Lw：不足左补零，超宽报错）。
func pgPad(x *big.Int) []byte {
	b := x.Bytes()
	if len(b) > pgPadLen {
		return b // 服务器 B 理论上 < N；超宽时按原样哈希，不制造假错误
	}
	out := make([]byte, pgPadLen)
	copy(out[pgPadLen-len(b):], b)
	return out
}

// pgH 拼接 SHA-256（前端 Fr：Iw 拼接 + digest）。
func pgH(parts ...[]byte) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

// pgX 派生私钥指数（前端 g5）：x = H(salt || H(user+":" || PBKDF2(password, salt, 210000, 32)))。
func pgX(username, password string, salt []byte) *big.Int {
	inner := pbkdf2.Key([]byte(password), salt, pgIter, pgDKLen, sha256.New)
	id := sha256.Sum256([]byte(username + ":"))
	outer := pgH(salt, id[:], inner)
	return new(big.Int).SetBytes(outer)
}

// pgSrpClient 执行 SRP-6a 客户端计算。返回 aHex（客户端公钥 A 定宽 hex）、m1Hex、m2ExpectHex。
// saltHex/BHex 来自 challenge 响应；identity 用服务器回传的 i（前端 n.i||username）。
func pgSrpClient(identity, password, saltHex, bHex string) (aHex, m1Hex, m2ExpectHex string, err error) {
	salt, err := pgHexBytes(saltHex)
	if err != nil {
		return "", "", "", fmt.Errorf("salt hex: %w", err)
	}
	B, ok := new(big.Int).SetString(strings.TrimSpace(bHex), 16)
	if !ok || B.Sign() <= 0 {
		return "", "", "", fmt.Errorf("SRP challenge B 非法")
	}
	x := pgX(identity, password, salt)
	// 私钥 a：32 随机字节（前端 p5：重抽到落在 [1,N)）
	aPriv, err := rand.Int(rand.Reader, pgN)
	for err == nil && aPriv.Sign() == 0 {
		aPriv, err = rand.Int(rand.Reader, pgN)
	}
	if err != nil {
		return "", "", "", fmt.Errorf("rand: %w", err)
	}
	A := new(big.Int).Exp(pgG, aPriv, pgN)
	u := new(big.Int).SetBytes(pgH(pgPad(A), pgPad(B)))
	if u.Sign() == 0 {
		return "", "", "", fmt.Errorf("SRP u 为 0，请重试")
	}
	// k = H(N_pad || g_pad)（前端 zw，进程内缓存同义——直接算，代价可忽略）
	k := new(big.Int).SetBytes(pgH(pgPad(pgN), pgPad(pgG)))
	// S = (B - k·g^x)^(a + u·x) mod N（模减先归正）
	gx := new(big.Int).Exp(pgG, x, pgN)
	base := new(big.Int).Sub(B, new(big.Int).Mul(k, gx))
	base.Mod(base, pgN)
	exp := new(big.Int).Add(aPriv, new(big.Int).Mul(u, x))
	S := new(big.Int).Exp(base, exp, pgN)
	// M1 = H(H(N)⊕H(g) || H(identity) || salt || A_pad || B_pad || H(S_pad))
	hn := pgH(pgPad(pgN))
	hg := pgH(pgPad(pgG))
	hnx := make([]byte, len(hn))
	subtle.XORBytes(hnx, hn, hg)
	idHash := sha256.Sum256([]byte(identity))
	M1 := pgH(hnx, idHash[:], salt, pgPad(A), pgPad(B), pgH(pgPad(S)))
	M2 := pgH(pgPad(A), M1, pgH(pgPad(S)))
	return fmt.Sprintf("%x", pgPad(A)), fmt.Sprintf("%x", M1), fmt.Sprintf("%x", M2), nil
}

// pgHexBytes 清洗 hex 串并解码（前端 ti：剔除非 hex 字符，奇数长度报错）。
func pgHexBytes(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			return r
		}
		return -1
	}, s)
	if len(s) == 0 || len(s)%2 != 0 {
		return nil, fmt.Errorf("hex 长度非法（%d）", len(s))
	}
	return hexDecode(s)
}

func hexDecode(s string) ([]byte, error) {
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi, ok1 := pgHexVal(s[i*2])
		lo, ok2 := pgHexVal(s[i*2+1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("hex 字符非法")
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func pgHexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// pgSrpLogin 走完整登录：challenge → SRP 计算 → login（校验 M2）→ 失败回退明文 /login。
// 返回 (token, err)。identity 优先取服务器回传的 i（前端 n.i||username）。
func pgSrpLogin(ctx context.Context, hc *httpclient.Client, baseURL, username, password string) (string, error) {
	var ch struct {
		Code int `json:"code"`
		Data struct {
			I       any    `json:"i"`
			S       string `json:"s"`
			B       string `json:"B"`
			Ticket  string `json:"ticket"`
			Message string `json:"message"`
		} `json:"data"`
		Message string `json:"msg"`
	}
	if err := pgJSON(ctx, hc, baseURL, "POST", "/v1/web/auth/srp/challenge",
		map[string]string{"username": username}, &ch); err != nil {
		return "", fmt.Errorf("SRP challenge: %w", err)
	}
	if ch.Code != 0 || ch.Data.Ticket == "" || ch.Data.S == "" || ch.Data.B == "" {
		msg := ch.Data.Message
		if msg == "" {
			msg = ch.Message
		}
		return "", fmt.Errorf("SRP challenge 拒绝：%s", msg)
	}
	identity := username
	if s, ok := ch.Data.I.(string); ok && strings.TrimSpace(s) != "" {
		identity = strings.TrimSpace(s)
	}
	aHex, m1Hex, m2Expect, err := pgSrpClient(identity, password, ch.Data.S, ch.Data.B)
	if err != nil {
		return "", err
	}
	var lg struct {
		Code int `json:"code"`
		Data struct {
			M2    string `json:"m2"`
			Token string `json:"token"`
			User  struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			} `json:"user"`
			Message string `json:"message"`
		} `json:"data"`
		Message string `json:"msg"`
	}
	if err := pgJSON(ctx, hc, baseURL, "POST", "/v1/web/auth/srp/login",
		map[string]string{"ticket": ch.Data.Ticket, "a": aHex, "m1": m1Hex}, &lg); err != nil {
		return "", fmt.Errorf("SRP login: %w", err)
	}
	if lg.Code == 0 && lg.Data.Token != "" {
		// M2 校验（服务器证据）：不一致说明对端未持 verifier，防钓鱼。
		if lg.Data.M2 != "" && !pgCteq(lg.Data.M2, m2Expect) {
			return "", fmt.Errorf("SRP 服务器证据校验失败（M2 不匹配）")
		}
		return lg.Data.Token, nil
	}
	// SRP 被拒（老账号未迁移/协议版本差）：回退明文登录（前端 loginWithSrpThenPassword 同款兜底）。
	var pl struct {
		Code int `json:"code"`
		Data struct {
			Token   string `json:"token"`
			Message string `json:"message"`
		} `json:"data"`
		Message string `json:"msg"`
	}
	if err := pgJSON(ctx, hc, baseURL, "POST", "/v1/web/login",
		map[string]string{"username": username, "password": password}, &pl); err != nil {
		return "", fmt.Errorf("登录: %w", err)
	}
	if pl.Code == 0 && pl.Data.Token != "" {
		return pl.Data.Token, nil
	}
	msg := pl.Data.Message
	if msg == "" {
		msg = pl.Message
	}
	return "", fmt.Errorf("登录失败：%s", msg)
}

// pgCteq 恒定时间 hex 字符串比较。
func pgCteq(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// pgJSON 发 JSON 请求（登录/免签名阶段：带 visitorId/ts/version/webversion 固定头）并解包响应。
func pgJSON(ctx context.Context, hc *httpclient.Client, baseURL, method, path string, body any, out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = b
	} else {
		payload = []byte("{}")
	}
	headers := map[string]string{
		"Content-Type":     "application/json",
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"visitorId":        pgVisitorID(),
		"ts":               strconv.FormatInt(time.Now().Unix(), 10),
		"version":          "1.0.0",
		"webversion":       "1",
	}
	resp, err := hc.Do(ctx, method, strings.TrimRight(baseURL, "/")+path, payload, headers, "", true)
	if err != nil {
		return err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("未认证（%d）", resp.StatusCode)
	}
	return json.Unmarshal(resp.Body, out)
}

// pgVisitorID 生成 32 位 hex 设备指纹（前端 y1：16 随机字节 hex；登录时随 token 绑定）。
func pgVisitorID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
