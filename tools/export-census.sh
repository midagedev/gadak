#!/usr/bin/env bash
# export-census — "which exported symbols in internal/ does nobody outside
# their own file actually use?" as one command.
#
#   tools/export-census.sh [--root <repo>] [-v|--verbose] [-h|--help]
#
# --verbose also prints the skipped table — every candidate an exposure
# filter spared, and which one — which is the first thing to read when the
# count looks wrong in either direction.
#
# Hand-built for one audit (GDK-1462, whose 65→0 findings were fixed in that
# round) and promoted so the next audit does not re-derive it (GDK-1485).
# The exposure filters are the substance — without them the raw answer is
# ~65 false positives on this tree:
#
#   - a reference anywhere counts, including docs, tests, TS and shell —
#     only same-package _test.go files count as in-file
#   - stdlib interface method names (String, Read, Close, …) are skipped
#   - a declaration block sibling that stays exported pins the block
#     (enum symmetry)
#   - a name whose unexported spelling is already a top-level name in the
#     package cannot be renamed
#   - a type named in the public shape of an export that itself survives is
#     public API even when nothing spells it (fixed point)
#   - a value of an exported enum type would make the enum unnameable
#
# Stdlib-only Go (four single-file parsers, `go run` per file, no module
# context needed) + a python token index and picker. No network.
#
# The intermediates live in a mktemp dir OUTSIDE the scanned tree, unlike the
# session probe this promotes: that one wrote its index.json into the repo's
# scratch/, which census.py then walked — every token in the tree appeared in
# its own previous output, so every run after the first reported 0 findings
# vacuously (measured 2026-09-10: the honest count on that day's tree was 31;
# the four Go parsers and the picker are byte-identical to the probe, so the
# difference is exactly this).
#
# Exit 0 = no over-exported symbols; exit 1 = findings (one line each:
# file, kind, name, the unexported spelling that is free); exit 2 = usage
# or a tree without internal/.
set -euo pipefail

VERBOSE=0
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
usage() { echo "usage: tools/export-census.sh [--root <repo>] [-v|--verbose]" >&2; exit 2; }
while [[ $# -gt 0 ]]; do
  case "$1" in
    -v|--verbose) VERBOSE=1; shift ;;
    -h|--help)
      sed -n '2,/^set -euo pipefail$/p' "$0" | sed '$d'
      exit 0 ;;
    --root)
      [[ -n "${2:-}" ]] || usage
      ROOT="$(cd "$2" && pwd)"
      shift 2 ;;
    *) usage ;;
  esac
done
[[ -d "$ROOT/internal" ]] || { echo "export-census: $ROOT has no internal/ — nothing to census" >&2; exit 2; }
command -v go >/dev/null || { echo "export-census: go not on PATH" >&2; exit 2; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/gadak-export-census.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# ── stage 1: the four parsers (verbatim from the census round) ────────────
cat > "$WORK/decls.go" <<'GO_DECLS'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type decl struct {
	Name  string
	Kind  string
	File  string
	Recv  string
	Group string
}

func main() {
	root := os.Args[1]
	var decls []decl
	filepath.Walk(filepath.Join(root, "internal"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				recv := ""
				if d.Recv != nil && len(d.Recv.List) > 0 {
					recv = types(d.Recv.List[0].Type)
				}
				decls = append(decls, decl{d.Name.Name, "func", rel, recv, ""})
			case *ast.GenDecl:
				grp := fmt.Sprintf("%s#%d", rel, fset.Position(d.Pos()).Line)
				if len(d.Specs) < 2 {
					grp = ""
				}
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							decls = append(decls, decl{s.Name.Name, "type", rel, "", grp})
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								k := "var"
								if d.Tok == token.CONST {
									k = "const"
								}
								decls = append(decls, decl{n.Name, k, rel, "", grp})
							}
						}
					}
				}
			}
		}
		return nil
	})
	for _, d := range decls {
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", d.Name, d.Kind, d.File, d.Recv, d.Group)
	}
}

func types(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return types(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return types(t.X)
	}
	return "?"
}
GO_DECLS

cat > "$WORK/expose.go" <<'GO_EXPOSE'
package main

// For every exported top-level decl in internal/, print the type names its
// public shape mentions: parameter/result types of funcs and methods, and the
// field types of exported struct fields. A candidate type that appears in the
// shape of an exported decl that survives is still public API even when its
// name is written nowhere else.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := os.Args[1]
	filepath.Walk(filepath.Join(root, "internal"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		emit := func(owner string, e ast.Expr) {
			for _, n := range idents(e) {
				fmt.Printf("%s\t%s\t%s\n", owner, rel, n)
			}
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				owner := d.Name.Name
				if d.Recv != nil && len(d.Recv.List) > 0 {
					for _, n := range idents(d.Recv.List[0].Type) {
						owner = n + "." + d.Name.Name
					}
				}
				for _, fl := range fields(d.Type.Params) {
					emit(owner, fl)
				}
				for _, fl := range fields(d.Type.Results) {
					emit(owner, fl)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					ts, ok := s.(*ast.TypeSpec)
					if !ok || !ts.Name.IsExported() {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						emit(ts.Name.Name, ts.Type)
						continue
					}
					for _, fl := range st.Fields.List {
						exp := len(fl.Names) == 0
						for _, n := range fl.Names {
							if n.IsExported() {
								exp = true
							}
						}
						if exp {
							emit(ts.Name.Name, fl.Type)
						}
					}
				}
			}
		}
		return nil
	})
}

func fields(fl *ast.FieldList) []ast.Expr {
	if fl == nil {
		return nil
	}
	var out []ast.Expr
	for _, f := range fl.List {
		out = append(out, f.Type)
	}
	return out
}

func idents(e ast.Expr) []string {
	var out []string
	ast.Inspect(e, func(n ast.Node) bool {
		if se, ok := n.(*ast.SelectorExpr); ok {
			_ = se
			return false // pkg.Type belongs to another package
		}
		if id, ok := n.(*ast.Ident); ok && id.IsExported() {
			out = append(out, id.Name)
		}
		return true
	})
	return out
}
GO_EXPOSE

cat > "$WORK/toplevel.go" <<'GO_TOPLEVEL'
package main

// Every top-level name declared in each internal/ package (tests included) —
// the only spellings a rename can actually collide with.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := os.Args[1]
	filepath.Walk(filepath.Join(root, "internal"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		pkg := filepath.Dir(rel)
		out := func(n string) { fmt.Printf("%s\t%s\n", pkg, n) }
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					out(d.Name.Name)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						out(s.Name.Name)
					case *ast.ValueSpec:
						for _, n := range s.Names {
							out(n.Name)
						}
					}
				}
			}
		}
		return nil
	})
}
GO_TOPLEVEL

cat > "$WORK/vtype.go" <<'GO_VTYPE'
package main

// name<TAB>file<TAB>declared type, for every top-level const/var in internal/.
// Inside a const block a spec with no type inherits the last one seen (iota).

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := os.Args[1]
	filepath.Walk(filepath.Join(root, "internal"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
				continue
			}
			last := ""
			for _, s := range gd.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				t := ""
				if vs.Type != nil {
					if id, ok := vs.Type.(*ast.Ident); ok {
						t = id.Name
					}
					last = t
				} else if gd.Tok == token.CONST {
					t = last
				}
				for _, n := range vs.Names {
					fmt.Printf("%s\t%s\t%s\n", n.Name, rel, t)
				}
			}
		}
		return nil
	})
}
GO_VTYPE

# ── stage 2: token index + picker ─────────────────────────────────────────
go run "$WORK/decls.go" "$ROOT"    > "$WORK/decls.tsv"
go run "$WORK/expose.go" "$ROOT"   > "$WORK/expose.tsv"
go run "$WORK/toplevel.go" "$ROOT" > "$WORK/toplevel.tsv"
go run "$WORK/vtype.go" "$ROOT"    > "$WORK/vtype.tsv"

python3 - "$WORK/index.json" "$ROOT" <<'PY_INDEX'
import os, re, sys, json, collections
out, root = sys.argv[1], sys.argv[2]
SKIPDIR={'.git','node_modules','.svelte-kit','dist','build','target','.tmp','coverage','test-results','playwright-report','gen'}
EXT={'.go','.md','.ts','.svelte','.js','.mjs','.json','.sh','.yml','.yaml','.tsx','.html','.txt'}
tok=re.compile(r'[A-Za-z_][A-Za-z0-9_]*')
index=collections.defaultdict(set)
for dp,dns,fns in os.walk(root):
    dns[:]=[d for d in dns if d not in SKIPDIR and not d.startswith('.')]
    for fn in fns:
        e=os.path.splitext(fn)[1]
        if e not in EXT: continue
        p=os.path.join(dp,fn)
        rel=os.path.relpath(p,root)
        try: s=open(p,encoding='utf-8',errors='ignore').read()
        except Exception: continue
        for m in set(tok.findall(s)):
            index[m].add(rel)
json.dump({k:sorted(v) for k,v in index.items()}, open(out,'w'))
print('indexed tokens:', len(index), file=sys.stderr)
PY_INDEX

python3 - "$WORK" <<'PY_PICK'
import json,os,sys,collections,re
S=sys.argv[1]
idx=json.load(open(S+'/index.json'))
rows=[l.rstrip('\n').split('\t') for l in open(S+'/decls.tsv')]
STDIFACE={'UnmarshalJSON','MarshalJSON','String','Error','ServeHTTP','Read','Write','Close','Len','Less','Swap','Unwrap','Is','As','Format','Scan','Value','MarshalText','UnmarshalText','Next','Seek'}
SPECIAL={'IDMCPClaude':'idMCPClaude'}
def outside(name,f):
    pkg=os.path.dirname(f); out=[]
    for r in idx.get(name,[]):
        if r==f: continue
        if r.endswith('_test.go') and os.path.dirname(r)==pkg: continue
        out.append(r)
    return out
cand={}
for name,kind,f,recv,grp in rows:
    if not outside(name,f): cand[(name,f)]=(kind,recv,grp)
groups=collections.defaultdict(lambda:[0,0])
for name,kind,f,recv,grp in rows:
    if grp: groups[grp][0 if (name,f) in cand else 1]+=1
pkgtop=collections.defaultdict(set)
for l in open(S+'/toplevel.tsv'):
    pkg,n=l.rstrip('\n').split('\t'); pkgtop[pkg].add(n)
exprows=[l.rstrip('\n').split('\t') for l in open(S+'/expose.tsv')]
def lower(n):
    if n in SPECIAL: return SPECIAL[n]
    m=re.match(r'^([A-Z]+)(?![a-z])',n)
    if m and len(m.group(1))>1: return m.group(1).lower()+n[len(m.group(1)):]
    return n[0].lower()+n[1:]
skipped={}
def drop(name,f,why):
    skipped[(name,f)]=why; del cand[(name,f)]
for (name,f),(kind,recv,grp) in list(cand.items()):
    if recv and name in STDIFACE: drop(name,f,'stdlib interface method')
for (name,f),(kind,recv,grp) in list(cand.items()):
    if grp and groups[grp][1]>0: drop(name,f,'decl-block sibling stays exported (enum symmetry)')
for (name,f),(kind,recv,grp) in list(cand.items()):
    new=lower(name)
    if new in pkgtop[os.path.dirname(f)]: drop(name,f,'name collision: %s is already a top-level name in the package'%new)
# exposure, to a fixed point: a candidate type named in the public shape of an
# export that itself survives is public API even when nothing else spells it.
while True:
    changed=False
    for owner,f,t in exprows:
        if (t,f) not in cand or cand[(t,f)][0]!='type': continue
        parts=owner.split('.')
        if any((p,f) in cand for p in parts): continue     # the owner goes away too
        if any((p,f) in skipped and 'interface' in skipped[(p,f)] for p in parts): pass
        drop(t,f,'public through '+owner+' @ '+f); changed=True
    if not changed: break
# an enum value whose named type stays exported would be unnameable from outside
vtype={}
for l in open(S+'/vtype.tsv'):
    n,f,t=l.rstrip('\n').split('\t'); vtype[(n,f)]=t
while True:
    changed=False
    for (name,f),(kind,recv,grp) in list(cand.items()):
        t=vtype.get((name,f),'')
        if t and t[:1].isupper() and (t,f) not in cand and (t,f) in {(n,ff) for n,k,ff,r,g in rows}:
            drop(name,f,'value of exported type '+t+' — unexporting it would make the enum unnameable'); changed=True
    if not changed: break
final=[(n,lower(n),k,f,r) for (n,f),(k,r,g) in sorted(cand.items(), key=lambda x:(x[0][1],x[0][0]))]
print('candidates: %d, skipped by the exposure filters: %d'%(len(final),len(skipped)), file=sys.stderr)
open(S+'/final.tsv','w').write(''.join('\t'.join(x)+'\n' for x in final))
open(S+'/skipped.tsv','w').write(''.join('%s\t%s\t%s\n'%(n,f,w) for (n,f),w in sorted(skipped.items(),key=lambda x:(x[0][1],x[0][0]))))
PY_PICK

# ── stage 3: verdict ───────────────────────────────────────────────────────
n=0
if [[ -s "$WORK/final.tsv" ]]; then
  while IFS=$'\t' read -r name lower kind file recv; do
    [[ -n "$name" ]] || continue
    n=$((n + 1))
    echo "export-census: $file: exported $kind $name${recv:+ (method on $recv)} has no reference outside its own file — $lower is free"
  done < "$WORK/final.tsv"
fi
if [[ "$VERBOSE" == 1 && -s "$WORK/skipped.tsv" ]]; then
  echo "-- skipped by the exposure filters (name, file, why):" >&2
  cat "$WORK/skipped.tsv" >&2
fi
echo "over-exported symbols in $ROOT/internal: $n"
if [[ "$n" -eq 0 ]]; then
  echo "export-census: clean"
else
  exit 1
fi
