//go:build !darwin

package tray

type Config struct {
	Version      string
	Icon         []byte
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

// SetupMainApp is a no-op on non-darwin platforms.
func SetupMainApp() {}

// SetAppIcon is a no-op on non-darwin platforms.
func SetAppIcon(_ []byte) {}

// SetFinderIcon is a no-op on non-darwin platforms.
func SetFinderIcon(_ string, _ []byte) error { return nil }

// AddIconPadding is a no-op on non-darwin platforms.
func AddIconPadding(src []byte, _ int) ([]byte, bool) { return src, false }

// ApplyRoundedCorners is a no-op on non-darwin platforms.
func ApplyRoundedCorners(src []byte) ([]byte, bool) { return src, false }

// RunConfigWindow is a no-op on non-darwin platforms.
func RunConfigWindow(_ []byte, _ bool, _ string) {}
