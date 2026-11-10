// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"time"
)

func mpParseXiaomiStatus(data map[string]any) map[string]any {
	wan, _ := data["wan"].(map[string]any)
	cpu, _ := data["cpu"].(map[string]any)
	mem, _ := data["mem"].(map[string]any)
	count, _ := data["count"].(map[string]any)
	online := pluginInt(count["online"], 0)
	return map[string]any{
		"router_name":     firstNonEmpty(pluginString(data["displayName"]), pluginString(data["routername"]), "Xiaomi Router"),
		"hardware":        pluginAnyString(data["hardware"]),
		"online_count":    online,
		"upload":          wan["upload"],
		"download":        wan["download"],
		"upspeed":         wan["upspeed"],
		"downspeed":       wan["downspeed"],
		"upload_human":    mpFormatBytes(mpFloat(wan["upload"])),
		"download_human":  mpFormatBytes(mpFloat(wan["download"])),
		"upspeed_human":   mpFormatSpeed(mpFloat(wan["upspeed"])),
		"downspeed_human": mpFormatSpeed(mpFloat(wan["downspeed"])),
		"cpu_load":        cpu["load"],
		"cpu_temp":        data["temperature"],
		"mem_usage":       mem["usage"],
		"uptime":          data["upTime"],
		"uptime_human":    mpUptime(pluginInt(data["upTime"], 0)),
	}
}

func mpRunSpeedtest(ctx context.Context, cfg, input map[string]any) (map[string]any, error) {
	downloadURL := strings.TrimSpace(pluginString(input["download_url"]))
	if downloadURL == "" {
		downloadURL = pluginString(cfg["download_url"])
	}
	uploadURL := strings.TrimSpace(pluginString(input["upload_url"]))
	if uploadURL == "" {
		uploadURL = pluginString(cfg["upload_url"])
	}
	timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 20)) * time.Second
	allowPrivate := asBool(cfg["allow_private_network"])
	downloadBytes := pluginInt(cfg["download_bytes"], 10000000)
	if downloadBytes <= 0 || downloadBytes > 100000000 {
		downloadBytes = 10000000
	}
	uploadBytes := pluginInt(cfg["upload_bytes"], 2000000)
	if uploadBytes < 0 || uploadBytes > 20000000 {
		uploadBytes = 2000000
	}
	out := map[string]any{}
	if err := mpValidateHTTPURL(ctx, downloadURL, allowPrivate); err != nil {
		return nil, err
	}
	start := time.Now()
	status, _, body, err := mpHTTPRequest(ctx, http.MethodGet, downloadURL, nil, nil, "", timeout)
	elapsed := time.Since(start)
	if err != nil {
		return nil, err
	}
	if len(body) > downloadBytes {
		body = body[:downloadBytes]
	}
	out["download_status"] = status
	out["download_bytes"] = len(body)
	out["download_seconds"] = elapsed.Seconds()
	out["download_mbps"] = mpMbps(len(body), elapsed)
	if uploadURL != "" && uploadBytes > 0 {
		if err := mpValidateHTTPURL(ctx, uploadURL, allowPrivate); err != nil {
			return nil, err
		}
		payload := bytes.Repeat([]byte("p"), uploadBytes)
		start = time.Now()
		status, _, _, err = mpHTTPRequest(ctx, http.MethodPost, uploadURL, bytes.NewReader(payload), map[string]string{"Content-Type": "application/octet-stream"}, "", timeout)
		elapsed = time.Since(start)
		if err != nil {
			out["upload_error"] = err.Error()
		} else {
			out["upload_status"] = status
			out["upload_bytes"] = uploadBytes
			out["upload_seconds"] = elapsed.Seconds()
			out["upload_mbps"] = mpMbps(uploadBytes, elapsed)
		}
	}
	out["tested_at"] = time.Now().Format(time.RFC3339)
	return out, nil
}
