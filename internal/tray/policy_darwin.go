//go:build darwin

package tray

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// setupMainApp 把当前进程设为 Accessory（菜单栏 app，不显示 Dock 图标）。
static void setupMainApp(void) {
	[NSApplication sharedApplication];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

// setAppIcon 只设 applicationIconImage，不改激活策略。
// config window helper 在 webview 创建完 NSApp 后调用。
static void setAppIcon(const void *iconData, int iconLen) {
	if (iconData == NULL || iconLen <= 0) {
		return;
	}
	NSApplication *app = [NSApplication sharedApplication];
	NSData *nsData = [NSData dataWithBytes:iconData length:iconLen];
	NSImage *img = [[NSImage alloc] initWithData:nsData];
	if (img != nil) {
		[app setApplicationIconImage:img];
	}
}

// setFinderIcon 用 NSWorkspace 给指定文件设自定义 Finder 图标。
static int setFinderIcon(const char *filePath, const void *pngData, int pngLen) {
	NSString *path = [NSString stringWithUTF8String:filePath];
	NSData *data = [NSData dataWithBytes:pngData length:pngLen];
	NSImage *img = [[NSImage alloc] initWithData:data];
	if (img == nil) {
		return 0;
	}
	BOOL ok = [[NSWorkspace sharedWorkspace] setIcon:img forFile:path options:0];
	return ok ? 1 : 0;
}

// hideMinimizeAndZoomButtons 隐藏 NSWindow 的最小化和最大化按钮，只保留关闭按钮。
static void hideMinimizeAndZoomButtons(void *windowPtr) {
	NSWindow *window = (__bridge NSWindow *)windowPtr;
	if (window == nil) {
		return;
	}
	NSButton *miniaturizeButton = [window standardWindowButton:NSWindowMiniaturizeButton];
	if (miniaturizeButton != nil) {
		[miniaturizeButton setHidden:YES];
	}
	NSButton *zoomButton = [window standardWindowButton:NSWindowZoomButton];
	if (zoomButton != nil) {
		[zoomButton setHidden:YES];
	}
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// SetupMainApp 在 macOS 上把当前进程设为菜单栏 app（Accessory）。
// 必须在 tray.Run 之前调用，且只需调用一次。
func SetupMainApp() {
	C.setupMainApp()
}

// SetAppIcon 只设 applicationIconImage，不改激活策略。
// 用于 config window helper 在 webview 创建 NSApp 后设置图标。
func SetAppIcon(iconPNG []byte) {
	if len(iconPNG) == 0 {
		return
	}
	C.setAppIcon(unsafe.Pointer(&iconPNG[0]), C.int(len(iconPNG)))
}

// SetFinderIcon 给指定路径的文件设置 Finder 自定义图标（来自 PNG 字节）。
func SetFinderIcon(filePath string, pngBytes []byte) error {
	if len(pngBytes) == 0 {
		return fmt.Errorf("empty icon data")
	}
	cPath := C.CString(filePath)
	defer C.free(unsafe.Pointer(cPath))
	ok := C.setFinderIcon(cPath, unsafe.Pointer(&pngBytes[0]), C.int(len(pngBytes)))
	if ok == 0 {
		return fmt.Errorf("setFinderIcon failed")
	}
	return nil
}

// HideMinimizeAndZoomButtons 隐藏窗口的最小化和最大化按钮，只保留关闭按钮。
func HideMinimizeAndZoomButtons(window unsafe.Pointer) {
	C.hideMinimizeAndZoomButtons(window)
}
