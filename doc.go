// Package canvas is an independent HTML template engine for Go.
//
// Hot path: lex → AST → CompileFunc (native AOT closures) → pooled Writer.
// Zero framework dependencies. Used by ZATRANO V3 as its SSR engine; usable alone.
package canvas
