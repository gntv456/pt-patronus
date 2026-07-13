package aiagent

import "github.com/hxpt/ptpatronus/internal/llm"

// maxMessages 上下文保留的最近消息数（不含 system），超过裁掉最早的。
const maxMessages = 30

// trimMessages 保留首条 system + 最近 N 条，避免上下文无限增长。
func trimMessages(msgs []llm.Message) []llm.Message {
	if len(msgs) <= maxMessages {
		return msgs
	}
	head := []llm.Message{}
	if len(msgs) > 0 && msgs[0].Role == llm.RoleSystem {
		head = []llm.Message{msgs[0]}
		msgs = msgs[1:]
	}
	if len(msgs) <= maxMessages {
		return append(head, msgs...)
	}
	return append(head, msgs[len(msgs)-maxMessages:]...)
}
