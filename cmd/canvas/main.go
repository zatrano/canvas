// Command canvas is the Canvas CLI (gen, …).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zatrano/canvas/ast"
	"github.com/zatrano/canvas/gen"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "gen":
		if err := cmdGen(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `canvas — Canvas template tooling

Usage:
  canvas gen -pkg NAME -func NAME [-out FILE] TEMPLATE.html

Commands:
  gen   Emit a typed Go stream stub from an AST-lowerable template
`)
}

func cmdGen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	pkg := fs.String("pkg", "main", "generated package name")
	fn := fs.String("func", "StreamPage", "generated function name")
	out := fs.String("out", "", "output .go path (default stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("canvas gen: need exactly one TEMPLATE.html")
	}
	raw, err := os.ReadFile(rest[0])
	if err != nil {
		return err
	}
	doc, err := ast.ParseSource(string(raw))
	if err != nil {
		return err
	}
	src, err := gen.EmitListPage(*pkg, *fn, doc)
	if err != nil {
		return err
	}
	if *out == "" {
		fmt.Print(src)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o750); err != nil {
		return err
	}
	if !strings.HasSuffix(*out, ".go") {
		return fmt.Errorf("canvas gen: -out must end with .go")
	}
	return os.WriteFile(*out, []byte(src), 0o600)
}
