// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// CameraCollector 采集摄像头：protocol=onvif 取设备信息/码流地址；protocol=rtsp 仅探活。
type CameraCollector struct{}

// NewCameraCollector 构造。
func NewCameraCollector() CameraCollector { return CameraCollector{} }

func (CameraCollector) String() string { return "camera" }

// Probe 探活。
func (CameraCollector) Probe(ctx context.Context, t *models.MonitorTarget, creds Credentials) (bool, error) {
	switch ResolveProtocol(t) {
	case "rtsp":
		if t.RTSPURL == "" {
			return false, fmt.Errorf("rtsp url is required")
		}
		return ProbeRTSP(ctx, t.RTSPURL)
	default: // onvif
		_, err := newOnvifDevice(t, creds).GetDeviceInformation(ctx)
		return err == nil, err
	}
}

// Collect 采集。ONVIF 取设备信息 + 码流地址（回填 RTSPURL 由 service 处理）。
func (CameraCollector) Collect(ctx context.Context, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	m := Metrics{Online: true, Extra: map[string]any{}}
	if t.RTSPURL != "" {
		m.Extra["rtsp_url"] = t.RTSPURL
	}
	switch ResolveProtocol(t) {
	case "onvif":
		dev := newOnvifDevice(t, creds)
		info, err := dev.GetDeviceInformation(ctx)
		if err != nil {
			m.Online = false
			return m, nil
		}
		m.DeviceInfo = info
		if os := strings.TrimSpace(info.Manufacturer + " " + info.Model); os != "" {
			m.OS = os
		}
		if uri, err := dev.GetStreamUri(ctx); err == nil && uri != "" {
			m.Extra["stream_uri"] = uri
			if t.RTSPURL == "" {
				m.Extra["rtsp_url"] = uri
			}
		}
	case "rtsp":
		if ok, _ := ProbeRTSP(ctx, t.RTSPURL); !ok {
			m.Online = false
		}
	}
	return m, nil
}
