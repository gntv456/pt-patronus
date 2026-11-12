// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func plSendGroupMessage(ctx context.Context, input map[string]any, timeout time.Duration, allowPrivate bool) (map[string]any, error) {
	baseURL := pluginString(input["base_url"])
	cookie := pluginString(input["cookie"])
	message := pluginString(input["message"])
	endpoint := pluginString(input["endpoint"])
	if endpoint == "" {
		endpoint = "/shoutbox.php"
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(cookie) == "" || strings.TrimSpace(message) == "" {
		return nil, errors.New("站点地址、Cookie、消息不能为空")
	}
	target, err := plJoinURL(baseURL, endpoint)
	if err != nil {
		return nil, err
	}
	if err := plValidateHTTPURL(ctx, target, allowPrivate); err != nil {
		return nil, err
	}
	headers := pluginParseHeaders(pluginString(input["headers"]))
	headers["Referer"] = strings.TrimRight(baseURL, "/") + "/"
	mode := pluginString(input["mode"])
	field := pluginString(input["message_field"])
	if field == "" {
		field = "shbox_text"
	}
	var status int
	var h http.Header
	var body []byte
	switch mode {
	case "json_post":
		payload, _ := json.Marshal(map[string]string{field: message})
		headers["Content-Type"] = "application/json"
		status, h, body, err = plHTTPRequest(ctx, http.MethodPost, target, bytes.NewReader(payload), headers, cookie, timeout)
	case "form_post":
		form := url.Values{}
		form.Set(field, message)
		headers["Content-Type"] = "application/x-www-form-urlencoded"
		status, h, body, err = plHTTPRequest(ctx, http.MethodPost, target, strings.NewReader(form.Encode()), headers, cookie, timeout)
	default:
		u, _ := url.Parse(target)
		q := u.Query()
		q.Set(field, message)
		q.Set("sent", "yes")
		q.Set("shout", "我喊")
		q.Set("type", "shoutbox")
		u.RawQuery = q.Encode()
		status, h, body, err = plHTTPRequest(ctx, http.MethodGet, u.String(), nil, headers, cookie, timeout)
	}
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"status_code":  status,
		"content_type": h.Get("Content-Type"),
		"body_snippet": truncate(string(body), 1600),
	}
	if asBool(input["fetch_feedback"]) {
		feedback, _ := plFetchGroupMessages(ctx, map[string]any{
			"base_url": baseURL,
			"cookie":   cookie,
			"endpoint": endpoint,
			"headers":  pluginString(input["headers"]),
		}, timeout, allowPrivate)
		out["feedback"] = feedback["lines"]
	}
	return out, nil
}

func plFetchGroupMessages(ctx context.Context, input map[string]any, timeout time.Duration, allowPrivate bool) (map[string]any, error) {
	target, err := plJoinURL(pluginString(input["base_url"]), pluginString(input["endpoint"]))
	if err != nil {
		return nil, err
	}
	if err := plValidateHTTPURL(ctx, target, allowPrivate); err != nil {
		return nil, err
	}
	headers := pluginParseHeaders(pluginString(input["headers"]))
	status, h, body, err := plHTTPRequest(ctx, http.MethodGet, target, nil, headers, pluginString(input["cookie"]), timeout)
	if err != nil {
		return nil, err
	}
	text := plHTMLToText(string(body))
	return map[string]any{
		"status_code":  status,
		"content_type": h.Get("Content-Type"),
		"lines":        plTopLines(text, 20),
		"body_snippet": truncate(text, 1600),
	}, nil
}
