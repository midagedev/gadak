//go:build darwin

package term

import "golang.org/x/sys/unix"

func controllingTty(pid int) (int32, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, false
	}
	return kp.Eproc.Tdev, true
}

func listPidsOnTty(dev int32) []int {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil
	}
	out := make([]int, 0, 4)
	for i := range kps {
		if kps[i].Eproc.Tdev != dev {
			continue
		}
		p := int(kps[i].Proc.P_pid)
		if p <= 0 {
			continue
		}
		out = append(out, p)
	}
	return out
}

// processStamp identifies this incarnation of pid, so a pid recycled
// between the SIGHUP and the SIGKILL is not signalled in its successor's
// place.
func processStamp(pid int) (uint64, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, false
	}
	t := kp.Proc.P_starttime
	return uint64(t.Sec)<<20 | uint64(uint32(t.Usec)), true
}
