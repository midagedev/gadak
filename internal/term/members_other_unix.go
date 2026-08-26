//go:build !windows && !darwin && !linux

package term

func controllingTty(int) (int32, bool) { return 0, false }
func listPidsOnTty(int32) []int        { return nil }

func processStamp(int) (uint64, bool) { return 0, false }
