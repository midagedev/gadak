#!/usr/bin/env bash
# The e2e suite's shell, dressed as the Linux CI runner's.
#
# Why this exists: on 2026-08-30 five terminal e2e died on GitHub Actions and
# nowhere else. Two things about that runner's shell were never running on
# anyone's machine, and each broke a different test:
#
#   1. Its prompt is 24 columns wide (`runner@runnervmgx7h7:~$ `), so every
#      command a spec types starts 24 columns further right than it does
#      locally and the longer ones wrap. xterm stores a wrapped line as a
#      separate buffer row, so a test that read the buffer row by row saw the
#      command cut in half.
#   2. Ubuntu's stock /etc/skel/.bashrc puts the window title in PS1 for any
#      xterm-ish TERM — `\[\e]0;\u@\h: \w\a\]` — and that OSC string is
#      terminated by BEL. Sessions start with TERM=xterm-256color, so on that
#      runner *every prompt* emits a 0x07. Anything counting bare 0x07 bytes as
#      "this shell rang for a person" is therefore permanently true there.
#
# `GADAK_E2E_SHELL=e2e/ci-shell.sh` makes e2e/serve.sh point the pane's
# sessions here (settings block terminal.shell, GDK-896), so that environment
# is one command away from a local run:
#
#   npm run test:e2e:wide-prompt
#
# The rc file below is Ubuntu's default prompt block, copied rather than
# paraphrased: the reproduction has to be the measurement, not an impression
# of it. `runnervmgx7h7` is the hostname from the failing run.
set -u

# macOS ships bash 3.2 patched to print a three-line "the default interactive
# shell is now zsh" banner on every interactive start. The CI runner's bash
# prints nothing, and those three lines are not cosmetic: they push everything
# the shell prints three rows further down, which is exactly the axis one of
# the failing tests turned out to be sensitive to. Silence it so the local
# buffer has the same shape as the runner's.
export BASH_SILENCE_DEPRECATION_WARNING=1

# A fixed path, not a mktemp: this script execs, so no trap of ours would ever
# run to clean one up. e2e/.tmp is gitignored and is where the suite already
# keeps its scratch.
RC="$(cd "$(dirname "$0")" && pwd)/.tmp/ci-shell-rc"
mkdir -p "$(dirname "$RC")"

cat >"$RC" <<'RCEOF'
# Ubuntu /etc/skel/.bashrc, prompt block only.
PS1='\[\033[01;32m\]runner@runnervmgx7h7\[\033[00m\]:\[\033[01;34m\]~\[\033[00m\]\$ '
case "$TERM" in
xterm*|rxvt*)
    # The OSC-8-style window title. Its terminator is a BEL.
    PS1="\[\e]0;runner@runnervmgx7h7: ~\a\]$PS1"
    ;;
esac
RCEOF

exec /bin/bash --noprofile --rcfile "$RC" "$@"
