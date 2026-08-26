//go:build linux

package term

import (
	"os"
	"strconv"
)

func controllingTty(pid int) (int32, bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	_, tty, ok := parseProcStat(string(b))
	return tty, ok
}

func listPidsOnTty(dev int32) []int {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	out := make([]int, 0, 4)
	for _, e := range ents {
		name := e.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		b, err := os.ReadFile("/proc/" + name + "/stat")
		if err != nil {
			continue
		}
		pid, tty, ok := parseProcStat(string(b))
		if !ok || tty != dev || pid <= 0 {
			continue
		}
		out = append(out, pid)
	}
	return out
}

// processStamp identifies this incarnation of pid, so a pid recycled
// between the SIGHUP and the SIGKILL is not signalled in its successor's
// place.
func processStamp(pid int) (uint64, bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	return parseProcStartTime(string(b))
}
