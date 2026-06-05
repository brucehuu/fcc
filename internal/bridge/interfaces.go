package bridge

import (
	"context"
	"time"
)

// Messenger 向飞书发送消息的抽象
// 实现：internal/bot.Bot
type Messenger interface {
	Start(ctx context.Context) error
	SendText(ctx context.Context, receiveIDType, receiveID, text string) error
	SendInteractiveTable(ctx context.Context, receiveIDType, receiveID, markdownTable string) error
	SendWelcome(ctx context.Context, targetName, text string) error
	CleanupOldImages(maxAge time.Duration) error
	Close()
}

// Terminal 与终端交互的抽象
// 实现：internal/terminal.TmuxSession
type Terminal interface {
	Start(command, workDir string) error
	SendKeys(text string) error
	SendSpecialKey(key string) error
	CaptureVisible(historyLines int) (string, error)
	WaitReady() error
	Kill() error
	HasSession() bool
	IsAvailable() bool
}
