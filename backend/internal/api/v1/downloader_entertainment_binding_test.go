// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import "testing"

func TestDownloaderEntertainmentActionBindings(t *testing.T) {
	v := newBindValidator(t)
	cases := []struct {
		name    string
		valid   any
		invalid any
	}{
		{"downloader tag mode", tagsReq{Mode: "add"}, tagsReq{Mode: "typo"}},
		{"downloader queue action", queueReq{Action: "bottom"}, queueReq{Action: "first"}},
		{"tracker removal target", trackersReq{Hash: "abc", URLs: []string{"https://tracker.invalid/announce"}}, trackersReq{}},
		{"category name", categoryEditReq{Name: "movies"}, categoryEditReq{}},
		{"farm section", farmItemReq{Section: "crop", ItemID: 1}, farmItemReq{Section: "tree", ItemID: 1}},
		{"sell key", sellReq{Key: "wheat"}, sellReq{}},
		{"blackjack action", blackjackReq{Action: "stand"}, blackjackReq{Action: "split"}},
		{"bigsmall chat", bsSendChatReq{Message: "hello"}, bsSendChatReq{}},
		{"bigsmall task", bsTaskReq{Action: "reward", TaskID: 1}, bsTaskReq{Action: "delete", TaskID: 1}},
		{"bigsmall item", bsItemReq{Tag: "refund"}, bsItemReq{}},
		{"bigsmall redpacket", bsRedpacketReq{Action: "claim"}, bsRedpacketReq{Action: "open"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mustBind(t, v, tc.valid, false)
			mustBind(t, v, tc.invalid, true)
		})
	}
}
