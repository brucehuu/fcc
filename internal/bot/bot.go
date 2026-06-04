package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"feishu-connect/internal/log"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type Bot struct {
	client     *lark.Client
	wsClient   *larkws.Client
	botOpenID  string
	botIDMu    sync.RWMutex
	onMessage  func(chatType, openID, chatID, text string)
	maxRetries int
	closeOnce  sync.Once
}

type TextContent struct {
	Text string `json:"text"`
}

type ImageContent struct {
	ImageKey string `json:"image_key"`
}

type botInfoResp struct {
	Code int `json:"code"`
	Data struct {
		Bot struct {
			OpenID string `json:"open_id"`
		} `json:"bot"`
	} `json:"data"`
}

func New(appID, appSecret string, onMessage func(chatType, openID, chatID, text string), maxRetries int) *Bot {
	client := lark.NewClient(appID, appSecret)

	b := &Bot{
		client:     client,
		onMessage:  onMessage,
		maxRetries: maxRetries,
	}

	d := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(b.handleEvent)

	b.wsClient = larkws.NewClient(appID, appSecret,
		larkws.WithEventHandler(d),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	return b
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (b *Bot) handleEvent(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event.Event == nil || event.Event.Message == nil {
		return nil
	}
	msg := event.Event.Message

	chatType := ptrStr(msg.ChatType)
	if chatType != "p2p" && chatType != "group" {
		return nil
	}

	msgType := ptrStr(msg.MessageType)
	if msgType != "text" && msgType != "image" {
		return nil
	}

	sender := event.Event.Sender
	openID := ""
	b.botIDMu.RLock()
	botID := b.botOpenID
	b.botIDMu.RUnlock()
	if sender == nil {
		// 无法确定发送者身份，保守过滤（避免 bot 自己消息回显）
		if botID != "" {
			return nil
		}
	} else {
		if senderType := ptrStr(sender.SenderType); senderType != "" && senderType != "user" {
			return nil
		}
		if sender.SenderId != nil {
			openID = ptrStr(sender.SenderId.OpenId)
		}
		if botID != "" && openID == botID {
			return nil
		}
	}

	text, err := b.parseMessage(ctx, msgType, ptrStr(msg.Content), msg.MessageId)
	if err != nil || text == "" {
		return nil
	}

	chatID := ptrStr(msg.ChatId)

	log.Infof("[bot] %s msg from %s: %q", chatType, openID, log.Truncate(text, 80))
	b.onMessage(chatType, openID, chatID, text)
	return nil
}

// parseMessage 根据消息类型解析出文本内容
func (b *Bot) parseMessage(ctx context.Context, msgType, msgContent string, msgID *string) (string, error) {
	switch msgType {
	case "text":
		var content TextContent
		if err := json.Unmarshal([]byte(msgContent), &content); err != nil {
			return "", err
		}
		return strings.TrimSpace(content.Text), nil
	case "image":
		var content ImageContent
		if err := json.Unmarshal([]byte(msgContent), &content); err != nil {
			return "", err
		}
		if content.ImageKey == "" {
			return "", nil
		}
		id := ""
		if msgID != nil {
			id = *msgID
		}
		imagePath, err := b.downloadImage(ctx, id, content.ImageKey)
		if err != nil {
			log.Warnf("[bot] download image failed: %v", err)
			return "[用户发送了一张图片，但下载失败]", nil
		}
		return fmt.Sprintf("[用户发送了一张图片: %s]", imagePath), nil
	default:
		return "", nil
	}
}

func (b *Bot) downloadImage(ctx context.Context, messageID, imageKey string) (string, error) {
	if messageID == "" || imageKey == "" {
		return "", fmt.Errorf("message_id or image_key is empty")
	}

	path := fmt.Sprintf("/open-apis/im/v1/messages/%s/resources/%s?type=image", messageID, imageKey)
	resp, err := b.client.Get(ctx, path, nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return "", fmt.Errorf("download image failed: %w", err)
	}

	const maxImageSize = 10 * 1024 * 1024 // 10MB
	if len(resp.RawBody) > maxImageSize {
		return "", fmt.Errorf("image too large: %d bytes > %d bytes", len(resp.RawBody), maxImageSize)
	}

	// 保存图片到本地
	dir := "log/images"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create image dir failed: %w", err)
	}

	safeKey := filepath.Base(imageKey)
	filename := filepath.Join(dir, safeKey+".png")
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("create image file failed: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(resp.RawBody); err != nil {
		return "", fmt.Errorf("write image file failed: %w", err)
	}

	return fmt.Sprintf("[图片已保存: %s]", filepath.Base(filename)), nil
}

// CleanupOldImages 清理 N 天前的图片，防止磁盘无限增长
// 由 main 启动时和定期调用
func (b *Bot) CleanupOldImages(maxAge time.Duration) error {
	dir := "log/images"
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		log.Infof("[bot] cleaned up %d old images (older than %s)", removed, maxAge)
	}
	return nil
}

func (b *Bot) fetchBotOpenID(ctx context.Context) error {
	resp, err := b.client.Get(ctx, "/open-apis/bot/v3/info", nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return fmt.Errorf("failed to fetch bot info: %w", err)
	}

	var result botInfoResp
	if err := json.Unmarshal(resp.RawBody, &result); err != nil {
		return fmt.Errorf("failed to parse bot info: %w", err)
	}
	if result.Code != 0 {
		return fmt.Errorf("bot info returned code=%d", result.Code)
	}

	b.botIDMu.Lock()
	b.botOpenID = result.Data.Bot.OpenID
	b.botIDMu.Unlock()
	log.Infof("[bot] bot open_id: %s", result.Data.Bot.OpenID)
	return nil
}

func (b *Bot) Start(ctx context.Context) error {
	log.Info("[bot] connecting to feishu websocket...")
	// 获取 bot open_id，失败时重试 3 次，避免 WebSocket 连接后因未识别自身消息而回显
	for attempt := 0; attempt < 3; attempt++ {
		if err := b.fetchBotOpenID(ctx); err != nil {
			log.Warnf("[bot] warning: failed to fetch bot open_id (attempt %d): %v", attempt+1, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		} else {
			return b.wsClient.Start(ctx)
		}
	}
	return fmt.Errorf("failed to fetch bot open_id after 3 attempts")
}

func (b *Bot) SendText(ctx context.Context, receiveIDType, receiveID, text string) error {
	return b.sendWithRetry(ctx, "text", func(ctx context.Context) error {
		content := TextContent{Text: text}
		contentJSON, err := json.Marshal(content)
		if err != nil {
			return err
		}

		req := larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(receiveIDType).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(receiveID).
				MsgType("text").
				Content(string(contentJSON)).
				Build()).
			Build()

		resp, err := b.client.Im.V1.Message.Create(ctx, req)
		if err != nil {
			return fmt.Errorf("send message failed: %w", err)
		}
		if !resp.Success() {
			return fmt.Errorf("send message failed: code=%d, msg=%s", resp.Code, resp.Msg)
		}
		return nil
	})
}

// sendWithRetry 带指数退避的重试包装器
func (b *Bot) sendWithRetry(ctx context.Context, label string, fn func(ctx context.Context) error) error {
	var lastErr error
	backoff := 500 * time.Millisecond

	for attempt := 0; attempt < b.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		// 不重试上下文取消
		if ctx.Err() != nil {
			return err
		}
		if attempt < b.maxRetries-1 {
			log.Warnf("[bot] %s attempt %d failed: %v, retrying in %s", label, attempt+1, err, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
	return fmt.Errorf("%s failed after %d attempts: %w", label, b.maxRetries, lastErr)
}

// SendInteractiveTable 使用 interactive 卡片消息发送表格，通过 column_set 布局模拟表格
func (b *Bot) SendInteractiveTable(ctx context.Context, receiveIDType, receiveID, markdownTable string) error {
	card, err := buildTableCard(markdownTable)
	if err != nil {
		return err
	}
	contentJSON, err := json.Marshal(card)
	if err != nil {
		return err
	}
	return b.sendWithRetry(ctx, "interactive_table", func(ctx context.Context) error {
		req := larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(receiveIDType).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(receiveID).
				MsgType("interactive").
				Content(string(contentJSON)).
				Build()).
			Build()
		resp, err := b.client.Im.V1.Message.Create(ctx, req)
		if err != nil {
			return fmt.Errorf("send interactive table failed: %w", err)
		}
		if !resp.Success() {
			return fmt.Errorf("send interactive table failed: code=%d, msg=%s", resp.Code, resp.Msg)
		}
		return nil
	})
}

// buildTableCard 把 Markdown 表格转成飞书 interactive 卡片结构
func buildTableCard(markdownTable string) (map[string]interface{}, error) {
	lines := strings.Split(markdownTable, "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("table too short")
	}

	headers := parseMarkdownTableCells(lines[0])
	if len(headers) == 0 {
		return nil, fmt.Errorf("empty table header")
	}
	colCount := len(headers)

	elements := []map[string]interface{}{
		buildTableRow(headers, "grey"),
	}

	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || isSeparatorLine(line) {
			continue
		}
		cells := parseMarkdownTableCells(line)
		if len(cells) == 0 {
			continue
		}
		// 补齐或截断列数
		if len(cells) < colCount {
			for len(cells) < colCount {
				cells = append(cells, "")
			}
		} else if len(cells) > colCount {
			cells = cells[:colCount]
		}
		elements = append(elements, buildTableRow(cells, "default"))
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"elements": elements,
	}, nil
}

// buildTableRow 把一行单元格转成 column_set 元素
func buildTableRow(cells []string, bgStyle string) map[string]interface{} {
	cols := make([]map[string]interface{}, len(cells))
	for i, c := range cells {
		cols[i] = map[string]interface{}{
			"tag":   "column",
			"width": "weighted",
			"weight": 1,
			"elements": []map[string]interface{}{
				{
					"tag": "div",
					"text": map[string]interface{}{
						"tag":     "lark_md",
						"content": c,
					},
				},
			},
			"vertical_align": "center",
		}
	}
	return map[string]interface{}{
		"tag":                "column_set",
		"flex_mode":          "none",
		"background_style":   bgStyle,
		"horizontal_spacing": "small",
		"columns":            cols,
	}
}

func parseMarkdownTableCells(line string) []string {
	parts := strings.Split(line, "|")
	var cells []string
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" && (i == 0 || i == len(parts)-1) {
			continue
		}
		cells = append(cells, part)
	}
	return cells
}

// separatorCellRe 匹配合法的 separator 单元格：
// 由可选的前后冒号 + 至少 3 个连字符 + 可选的前后冒号组成
// 例如: ---, :---, ---:, :---:
var separatorCellRe = regexp.MustCompile(`^:?-{3,}:?$`)

func isSeparatorLine(line string) bool {
	cells := parseMarkdownTableCells(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !separatorCellRe.MatchString(strings.TrimSpace(cell)) {
			return false
		}
	}
	return true
}

func (b *Bot) Close() {
	b.closeOnce.Do(func() {
		b.wsClient.Close()
	})
}
