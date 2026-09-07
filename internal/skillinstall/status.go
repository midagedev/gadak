package skillinstall

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/clitool"
)

// The four words every caller uses for "what is at this destination". They are
// the wire format of `gadak doctor --json`, so the strings are contract.
const (
	StatusMissing   = "missing"   // nothing there
	StatusIdentical = "identical" // byte-equal to the embedded skill
	StatusStale     = "stale"     // gadak wrote it, and it has fallen behind
	StatusConflict  = "conflict"  // someone else's file, or gadak's after an edit
)

// DestStatus classifies dest relative to content.
//
//	missing    nothing at dest
//	identical  byte-equal to content — nothing to do
//	stale      gadak wrote it and it has since fallen behind: either it still
//	           matches the receipt gadak left beside it, or its digest is one of
//	           the copies gadak shipped before receipts existed
//	conflict   anything else — someone else's file, or gadak's file after a hand
//	           edit. Only --force replaces it.
//
// Identity is the content hash, never mtime: `brew upgrade` rewrites timestamps
// and `git checkout` restores them, so neither says who wrote the bytes.
func DestStatus(dest string, content []byte) (status string, existing []byte, err error) {
	fi, err := os.Lstat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return StatusMissing, nil, nil
		}
		return "", nil, fmt.Errorf("inspect %s: %w", clitool.TildeHome(dest), err)
	}
	if fi.IsDir() {
		return "", nil, fmt.Errorf("%s is a directory — remove it or choose another --dir", clitool.TildeHome(dest))
	}
	existing, err = os.ReadFile(dest)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", clitool.TildeHome(dest), err)
	}
	if bytes.Equal(existing, content) {
		return StatusIdentical, existing, nil
	}
	if IsOurs(filepath.Dir(dest), existing) {
		return StatusStale, existing, nil
	}
	return StatusConflict, existing, nil
}

// ---------------------------------------------------------------------------
// Provenance: telling gadak's own copy from the user's file (GDK-92)
//
// Before this, "differs from the embedded skill" was read as "the user edited
// it", so every `brew upgrade` turned the documented one-liner red. The two
// cases are told apart by content hash:
//
//   1. a receipt gadak leaves beside SKILL.md on every install. If the file
//      still hashes to what the receipt says gadak wrote, gadak wrote it and
//      nobody has touched it since. This is the mechanism going forward — it
//      needs no per-release maintenance.
//   2. LegacyDigests, for the installs that predate the receipt.
//
// A file that matches neither is the user's, and the refusal stands.
// ---------------------------------------------------------------------------

// ReceiptName sits next to SKILL.md. It is a disposable cache, not data:
// delete it and the worst that happens is the next upgrade asks for --force.
const ReceiptName = ".gadak-skill.json"

// Receipt records what gadak last wrote at a destination. Only SHA256 is
// compared; the other fields are there so a human who opens the file can tell
// what put it there and when.
type Receipt struct {
	SHA256       string `json:"sha256"`
	GadakVersion string `json:"gadak_version"`
	InstalledAt  string `json:"installed_at"`
	// Source and Revision say which *kind* of binary wrote the copy
	// (GDK-1531): a cut release, or one built from a checkout, and in the
	// second case the short git hash it came from. Before these fields a
	// working-tree SKILL.md installed by a `go run` build was indistinguishable
	// from a shipped one — `gadak doctor` called it "current" and nobody could
	// see that what the agent loaded had never been reviewed.
	//
	// Both are omitempty: a receipt written before this release has neither,
	// and SourceWord backfills the first from GadakVersion.
	Source   string `json:"source,omitempty"`
	Revision string `json:"revision,omitempty"`
}

// SourceWord is the receipt's provenance, backfilled for the receipts that
// predate the field: those recorded a version, and the version already says
// whether the binary was a release. "" only for a receipt with neither.
func (r Receipt) SourceWord() string {
	if r.Source != "" {
		return r.Source
	}
	if r.GadakVersion == "" {
		return ""
	}
	return SourceFor(r.GadakVersion)
}

// LegacyDigests are the SHA-256 digests of every skills/gadak/SKILL.md gadak
// shipped *before* installs started leaving a receipt.
//
// This set is FROZEN — it is the backfill for pre-receipt installs only, and it
// must not grow. Every release from this one on writes ReceiptName, so its own
// body is recognised by the receipt rather than by a new entry here. (An
// append-only table that a release could forget to update is exactly the bug
// that round was closing, so there is nothing to append to.)
//
// Derived 2026-08-16 from `git log --follow -- skills/gadak/SKILL.md`: seven
// revisions, of which the newest is the current embed and is deliberately
// absent — that one classifies as "identical", not "stale". The two oldest
// lived at skills/scry/SKILL.md, before the rename to gadak.
var LegacyDigests = map[string]string{
	"37c489c4475984c1a9c33852828640c4833dda7f20d939063af6f944ccd40565": "79a70f3",
	"a00da5247df29926d88d4948f1ba16e36ea1c9cda1eb8728a2a9cc2d2ff1b594": "3d7a65b",
	"be5be92dfc76faed5a330dd905895efcbc6a433fc782fa566c6c1653956e9a32": "c7628ef",
	"5a6ca6f702ade9f91fa740f80b1600fe781508c82b17dfbee242b5f506d9b3ab": "eed711e",
	"5a6d63ae45af97344c0b91052ef59abbc763de815e277aa6a12bd2d8981f06fd": "1096106",
	"1f7000999eaebdeade1995b97373a083a3cc9f02673a8798020f463e3b7d27d8": "f2b8d94",
}

// IsOurs reports whether gadak wrote these exact bytes.
func IsOurs(dir string, existing []byte) bool {
	digest := Digest(existing)
	if r, ok := ReadReceipt(dir); ok && r.SHA256 == digest {
		return true
	}
	_, ok := LegacyDigests[digest]
	return ok
}

// Digest is the content hash the whole provenance story is keyed on.
func Digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// ReadReceipt returns the receipt in dir. A missing, unreadable, corrupt or
// digest-less receipt is simply "no receipt" — it degrades to the legacy digest
// table and, failing that, to the refusal.
func ReadReceipt(dir string) (Receipt, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, ReceiptName))
	if err != nil {
		return Receipt{}, false
	}
	var r Receipt
	if err := json.Unmarshal(raw, &r); err != nil {
		return Receipt{}, false
	}
	if r.SHA256 == "" {
		return Receipt{}, false
	}
	return r, true
}

// WriteReceipt records digest as what gadak just wrote into dir. version is the
// binary's version string, kept for the human who opens the file — and, through
// SourceFor, the single input to the provenance word. The caller never decides
// "is this a dev build": it passes its version and this file answers, so the
// receipt and the auto-sync gate can never disagree (GDK-1531).
func WriteReceipt(dir, digest, version string) error {
	raw, err := json.MarshalIndent(Receipt{
		SHA256:       digest,
		GadakVersion: version,
		InstalledAt:  time.Now().UTC().Format(time.RFC3339),
		Source:       SourceFor(version),
		Revision:     BuildRevision(),
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ReceiptName), append(raw, '\n'), 0o644)
}

// FrontmatterName returns the `name:` value of a leading YAML frontmatter
// block, or "". It is used only to sharpen a refusal message, never as a
// licence to overwrite: a user who edits gadak's own skill keeps `name: gadak`
// in it, so trusting that line would delete exactly the edits the refusal
// exists to protect.
func FrontmatterName(content []byte) string {
	v, _ := frontmatterField(content, "name")
	return v
}

// FrontmatterDescription returns the `description:` value of the leading YAML
// frontmatter, folded to the single line a host actually loads.
//
// This is the field with a hard budget. Every host keeps `name` + `description`
// resident in the system prompt and reads the body only when the skill fires,
// and Codex truncates the description at 1024 characters — measured, not
// documented — so a description that runs long is silently cut mid-sentence.
// The parse is deliberately small: gadak's own frontmatter is the
// only input, and pulling in a YAML dependency for two fields would make
// desktop/'s separate module a gate on this file.
func FrontmatterDescription(content []byte) (string, bool) {
	return frontmatterField(content, "description")
}

// frontmatterField reads one scalar out of a leading `---` block. It handles a
// plain `key: value` and a folded `key: >` / literal `key: |` block, which is
// how a long description is written so the file stays readable.
//
// The key must start the line: a `description:` whose text happens to contain
// "name:" must not answer a request for `name`.
func frontmatterField(content []byte, key string) (string, bool) {
	const fence = "---\n"
	s := string(content)
	if !strings.HasPrefix(s, fence) {
		return "", false
	}
	body := s[len(fence):]
	end := strings.Index(body, "\n---")
	if end < 0 {
		return "", false
	}
	lines := strings.Split(body[:end], "\n")
	for i, line := range lines {
		rest, ok := strings.CutPrefix(line, key+":")
		if !ok {
			continue
		}
		v := strings.TrimSpace(rest)
		switch v {
		case ">", ">-", ">+":
			return foldBlock(lines[i+1:], true), true
		case "|", "|-", "|+":
			return foldBlock(lines[i+1:], false), true
		}
		return v, true
	}
	return "", false
}

// foldBlock joins the indented continuation lines of a block scalar the way a
// YAML reader would: a folded block (`>`) becomes one line with single spaces,
// a blank line inside it becomes a real newline, and a literal block (`|`)
// keeps its line breaks. The block ends at the first line that is not indented.
func foldBlock(lines []string, folded bool) string {
	kept := []string{}
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(kept) > 0 {
				kept = append(kept, "")
			}
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, " "))
		if n == 0 || (indent >= 0 && n < indent) {
			break // dedented: the block ended
		}
		if indent < 0 {
			indent = n
		}
		kept = append(kept, line[indent:])
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	if !folded {
		return strings.Join(kept, "\n")
	}
	var b strings.Builder
	for i, l := range kept {
		switch {
		case l == "":
			b.WriteString("\n")
		case i > 0 && kept[i-1] != "":
			b.WriteString(" ")
		}
		b.WriteString(l)
	}
	return b.String()
}
