//go:build windows

package app

func processAlive(pid int) (bool, error) {
	return pid > 0, nil
}
