// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import "testing"

func TestAgentConfigBinding(t *testing.T) {
	validator := newBindValidator(t)
	valid := agentConfigReq{
		Enabled: true, Provider: "openai", BaseURL: "https://api.example.com/v1", Model: "model",
		Temperature: 0.3, MaxTokens: 2048, MaxIterations: 12, SystemPrompt: "system",
	}
	mustBind(t, validator, valid, false)

	cases := []agentConfigReq{
		{Provider: "unknown", BaseURL: valid.BaseURL, Model: valid.Model, MaxTokens: 1, MaxIterations: 1, SystemPrompt: "s"},
		{Provider: "openai", BaseURL: "not-a-url", Model: valid.Model, MaxTokens: 1, MaxIterations: 1, SystemPrompt: "s"},
		{Provider: "openai", BaseURL: valid.BaseURL, Model: "", MaxTokens: 1, MaxIterations: 1, SystemPrompt: "s"},
		{Provider: "openai", BaseURL: valid.BaseURL, Model: valid.Model, Temperature: 2.1, MaxTokens: 1, MaxIterations: 1, SystemPrompt: "s"},
		{Provider: "openai", BaseURL: valid.BaseURL, Model: valid.Model, MaxTokens: -1, MaxIterations: 1, SystemPrompt: "s"},
		{Provider: "openai", BaseURL: valid.BaseURL, Model: valid.Model, MaxTokens: 1, MaxIterations: 31, SystemPrompt: "s"},
		{Provider: "openai", BaseURL: valid.BaseURL, Model: valid.Model, MaxTokens: 1, MaxIterations: 1, SystemPrompt: ""},
	}
	for index, input := range cases {
		if err := validator.Struct(input); err == nil {
			t.Errorf("case %d should fail validation: %+v", index, input)
		}
	}
}

func TestAgentConfirmBinding(t *testing.T) {
	validator := newBindValidator(t)
	// 合法：tool_call_id 非空，approved 任意。
	mustBind(t, validator, agentConfirmReq{ToolCallID: "c1", Approved: true}, false)
	mustBind(t, validator, agentConfirmReq{ToolCallID: "c1", Approved: false}, false)
	// 非法：缺 tool_call_id（required）。
	mustBind(t, validator, agentConfirmReq{ToolCallID: ""}, true)
}
