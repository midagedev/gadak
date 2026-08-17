//go:build !unix

package integrations

import "os/exec"

func setProbeProcAttr(*exec.Cmd) {}

func killProbe(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
