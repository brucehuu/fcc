//go:build darwin

package tray

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// setAccessoryActivationPolicy 把当前进程标记为 menu-bar-only 应用。
// 不设这个的话，裸 CLI 二进制在 macOS 上默认是 NSApplicationActivationPolicyProhibited，
// 菜单栏 status item 不会显示。
static void setAccessoryActivationPolicy(void) {
	[NSApplication sharedApplication];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}
*/
import "C"

func ensureMenuBarApp() {
	C.setAccessoryActivationPolicy()
}
