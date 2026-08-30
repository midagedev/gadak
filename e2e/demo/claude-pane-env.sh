# PTY environment for a live Claude Code session in a gadak pane (GDK-1159).
# Sourced by the serve so the pane inherits it. Sister of the tapes' own env;
# the four lines below are what the pilot needed on top of it:
#   unset SHELL            -> /bin/sh, so no zsh + starship prompt on camera
#   PROMPT                 -> zsh reads PROMPT, not PS1, if SHELL survives
#   unset NODE_EXTRA_CA_CERTS -> kills two mkcert SSL warnings at claude boot
#   GADAK_HOME             -> the writable standalone home, not the frozen demo
# The fixture repo lives at $HOME (the PTY cwd) and $HOME is trusted in
# .claude.json, so no "do you trust this folder" dialog eats the prompt.

# Path comes from the caller so no machine's checkout is baked in here.
: "${GADAK_REPO:?set GADAK_REPO to the repo root before sourcing}"
. "$GADAK_REPO/tools/tapes/.tmp/env-claude-drive.sh"
unset SHELL                      # /bin/sh: no zsh, no starship prompt
export PROMPT='$ '
unset NODE_EXTRA_CA_CERTS        # kills the mkcert SSL warning pair on boot
export GADAK_HOME=/private/tmp/gadak-hero-desk/home
