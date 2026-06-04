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

// SetupMainApp is a no-op on non-darwin platforms.
func SetupMainApp() {}

// SetAppIcon is a no-op on non-darwin platforms.
func SetAppIcon(_ []byte) {}

// SetFinderIcon is a no-op on non-darwin platforms.
func SetFinderIcon(_ string, _ []byte) error { return nil }
