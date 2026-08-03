package monitor

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// onvifDevice 封装对一台 ONVIF 摄像头的 SOAP 调用（device_service 端点；多数设备自动路由 action）。
type onvifDevice struct {
	endpoint string
	username string
	password string
	client   *http.Client
}

func newOnvifDevice(t *models.MonitorTarget, creds Credentials) *onvifDevice {
	endpoint := normalizeBase(t.Host)
	if endpoint == "" {
		endpoint = "http://" + strings.TrimSpace(t.Host)
	}
	// 未带路径则补 device_service 默认端点。
	if idx := strings.Index(endpoint, "://"); idx >= 0 {
		rest := endpoint[idx+3:]
		if !strings.Contains(rest, "/") {
			endpoint += "/onvif/device_service"
		}
	}
	return &onvifDevice{
		endpoint: endpoint,
		username: creds.Username,
		password: creds.Password,
		client:   &http.Client{Timeout: nasTimeout(t)},
	}
}

// GetDeviceInformation 取厂商/型号/固件/序列号（同时作为探活）。
func (d *onvifDevice) GetDeviceInformation(ctx context.Context) (*DeviceInfo, error) {
	body := `<tds:GetDeviceInformation xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`
	resp, err := d.call(ctx, body)
	if err != nil {
		return nil, err
	}
	if strings.Contains(resp, "Fault") {
		return nil, fmt.Errorf("onvif fault")
	}
	return &DeviceInfo{
		Manufacturer: extractTag(resp, "Manufacturer"),
		Model:        extractTag(resp, "Model"),
		Firmware:     extractTag(resp, "FirmwareVersion"),
		Serial:       extractTag(resp, "SerialNumber"),
		Hardware:     extractTag(resp, "HardwareId"),
	}, nil
}

// GetStreamUri 尝试获取 RTSP 拉流地址（用 Profile_1 token；失败返回空）。
func (d *onvifDevice) GetStreamUri(ctx context.Context) (string, error) {
	body := `<trt:GetStreamUri xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
	<trt:StreamSetup><tt:Stream xmlns:tt="http://www.onvif.org/ver10/schema">RTP-Unicast</tt:Stream>
	<tt:Transport><tt:Protocol>RTSP</tt:Protocol></tt:Transport></trt:StreamSetup>
	<trt:ProfileToken>Profile_1</trt:ProfileToken></trt:GetStreamUri>`
	resp, err := d.call(ctx, body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(extractTag(resp, "Uri")), nil
}

// call 发送带 WS-Security UsernameToken 的 SOAP envelope。
func (d *onvifDevice) call(ctx context.Context, body string) (string, error) {
	envelope, err := d.wrap(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader([]byte(envelope)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// ONVIF 设备 SOAP 响应正常 <64KB；限制 1MB 防恶意/故障设备回超大 body 撑爆内存
	// （采集是 cron 高频路径，无上限会逐次累积）。LimitReader 读到 1MB+1 即止。
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}

// wrap 生成 SOAP envelope + WS-Security digest 鉴权。
func (d *onvifDevice) wrap(body string) (string, error) {
	created := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	digest := sha1Digest(nonce, created, d.password)
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" s:mustUnderstand="1">
      <wsse:UsernameToken>
        <wsse:Username>%s</wsse:Username>
        <wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password>
        <wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</wsse:Nonce>
        <wsu:Created xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">%s</wsu:Created>
      </wsse:UsernameToken>
    </wsse:Security>
  </s:Header>
  <s:Body>%s</s:Body>
</s:Envelope>`, d.username, digest, nonceB64, created, body), nil
}

// sha1Digest = base64(sha1(nonce + created + password))。
func sha1Digest(nonce []byte, created, password string) string {
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// extractTag 宽松提取 SOAP 响应里的标签内容（忽略命名空间前缀，如 tds:Manufacturer）。
func extractTag(xmlStr, localName string) string {
	re := regexp.MustCompile(`<[A-Za-z0-9]*:?` + regexp.QuoteMeta(localName) + `[^>]*>([^<]*)<`)
	m := re.FindStringSubmatch(xmlStr)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
