//go:build !darwin

package tray

type Config struct {
	OnOpenConfig func()
	OnExit       func()
	OnMenuQuit   func()
}

// Run is a no-op on non-darwin platforms.
func Run(cfg Config) {}

// Stop is a no-op on non-darwin platforms.
func Stop() {}

// OpenConfig is a no-op on non-darwin platforms.
func OpenConfig() {}

// ensureMenuBarApp is a no-op on non-darwin platforms.
func ensureMenuBarApp() {}
