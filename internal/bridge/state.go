package bridge

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type receiverKey struct {
	id   string
	kind string // "open_id" or "chat_id"
}

// receiverState 维护单个接收方的状态
type receiverState struct {
	lastPane string
	ready    bool // baseline 是否已初始化，防止竞态发送完整 pane
	mu       sync.Mutex
	sendMu   sync.Mutex

	// 累积消息模式：所有内容追加到一条 markdown 消息中
	messageID  string          // 当前累积消息的 open_message_id
	contentBuf strings.Builder // 累积的 markdown 内容

	pendingTable      []string  // 等待更多流式行的 markdown 表格
	pendingTableSince time.Time // pendingTable 最近追加时间

	pendingCode      []string // 等待闭合的流式代码块
	pendingCodeLang  string
	pendingCodeSince time.Time
}

// bridgeMetrics 轻量级运行时指标
type bridgeMetrics struct {
	messagesReceived atomic.Uint64
	messagesSent     atomic.Uint64
	captures         atomic.Uint64
	diffHits         atomic.Uint64
	diffMisses       atomic.Uint64
}
