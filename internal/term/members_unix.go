//go:build !windows

package term

func sessionMembers(shellPID int) []int {
	dev, ok := controllingTty(shellPID)
	if !ok {
		return nil
	}
	return pidsOnTty(dev)
}

// pidsOnTty is the device-keyed walk. A zero or -1 device is "no
// controlling terminal" and must yield an empty set — using it as a key
// would collect every daemon.
func pidsOnTty(dev int32) []int {
	if dev == 0 || dev == -1 {
		return nil
	}
	return listPidsOnTty(dev)
}
