//go:build !windows
// +build !windows

package dokan

// api
func DriverVersion() (uint32, error) {
	return 0, ErrFailedToLoadDokan
}
func Version() (uint32, error) {
	return 0, ErrFailedToLoadDokan
}
func Init() error {
	return ErrFailedToLoadDokan
}
func Shutdown() error {
	return ErrFailedToLoadDokan
}
func MountPoints() ([]*MountPointInfo, error) {
	return nil, ErrFailedToLoadDokan
}
func CreateFileSystem(options *DokanOptions, operations *DokanOperations) (MountHandle, error) {
	return 0, ErrFailedToLoadDokan
}
func closeHandle(handle MountHandle) error {
	return ErrFailedToLoadDokan
}
func UTF16FromString(s string) ([]uint16, error) {
	return nil, ErrFailedToLoadDokan
}
func UTF16PtrFromString(s string) (*uint16, error) {
	return nil, ErrFailedToLoadDokan
}

// notify
func NotifyCreate(instance MountHandle, filePath string, isDirectory bool) error {
	return ErrFailedToLoadDokan
}
func NotifyDelete(instance MountHandle, filePath string, isDirectory bool) error {
	return ErrFailedToLoadDokan
}
func NotifyRename(instance MountHandle, oldPath, newPath string, isDirectory bool) error {
	return ErrFailedToLoadDokan
}
func NotifyUpdate(instance MountHandle, filePath string) error {
	return ErrFailedToLoadDokan
}
func NotifyXAttrUpdate(instance MountHandle, filePath string) error {
	return ErrFailedToLoadDokan
}

// disk
func initDokanOperations() *DokanOperations {
	return nil
}
