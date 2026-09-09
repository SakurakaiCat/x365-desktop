package sysproxy

// Manager provides cross-platform system proxy control.
type Manager interface {
	SetSOCKS5(host string, port int) error
	Clear() error
	IsEnabled() bool
}

// New returns the platform-appropriate Manager.
func New() Manager {
	return newManager()
}
