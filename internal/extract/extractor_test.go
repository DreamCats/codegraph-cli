package extract_test

import (
	"github.com/DreamCats/codegraph-cli/internal/extract"
	"testing"
)

func TestExtractGoBasicSymbolsAndCalls(t *testing.T) {
	src := []byte(`package service

import (
	"fmt"
	alias "example.com/team/proj/pkg/util"
)

type Service struct { name string }
type Runner interface { Run() error }
type Handler = Service
const answer = 42
var Default = Service{}

func helper(x int) int { return utilAdd(x) }

func (s *Service) Serve() error {
	const localAnswer = 7
	var localDefault = Service{}
	fmt.Println(s.name)
	alias.Do()
	alias.Generic[int]()
	helper(1)
	return nil
}
`)
	res := extract.ExtractFile("svc/service.go", src)
	byQ := map[string]string{}
	for _, node := range res.Nodes {
		byQ[node.QualifiedName] = node.Kind
	}
	for q, kind := range map[string]string{
		"Service":       "struct",
		"Runner":        "interface",
		"Handler":       "type_alias",
		"answer":        "constant",
		"Default":       "variable",
		"localAnswer":   "constant",
		"localDefault":  "variable",
		"helper":        "function",
		"Service.Serve": "method",
	} {
		if byQ[q] != kind {
			t.Fatalf("%s kind = %q, want %q; nodes=%#v", q, byQ[q], kind, res.Nodes)
		}
	}
	targets := map[string]bool{}
	for _, edge := range res.Edges {
		if edge.Kind == "calls" {
			targets[edge.Target] = true
		}
	}
	for _, target := range []string{"name:utilAdd", "name:Println", "name:Do", "name:Generic", "name:helper"} {
		if !targets[target] {
			t.Fatalf("missing call target %s in %#v", target, res.Edges)
		}
	}
}

func TestExtractPythonAndTypeScriptShape(t *testing.T) {
	py := extract.ExtractFile("t.py", []byte(`import os
from pathlib import Path

class Foo:
    """foo class."""
    @staticmethod
    def util(): ...

async def fetch(url):
    return await get(url)
`))
	seenPy := map[string]bool{}
	for _, node := range py.Nodes {
		seenPy[node.Kind+":"+node.QualifiedName] = true
	}
	for _, key := range []string{"import:os", "import:pathlib.Path", "class:Foo", "method:Foo.util", "function:fetch"} {
		if !seenPy[key] {
			t.Fatalf("missing %s in %#v", key, py.Nodes)
		}
	}
	for _, node := range py.Nodes {
		if node.QualifiedName == "Foo" && node.EndLine <= node.StartLine {
			t.Fatalf("expected class end_line to span body: %#v", node)
		}
	}
	var pyContains bool
	for _, edge := range py.Edges {
		if edge.Kind == "contains" {
			pyContains = true
		}
	}
	if !pyContains {
		t.Fatalf("expected python contains edge: %#v", py.Edges)
	}

	ts := extract.ExtractFile("t.ts", []byte(`import { foo, bar as baz } from './util';
import * as ns from 'lib';
import def from 'mod';

export class Service {
  async serve(req: Request): Promise<string> { return helper(req.id); }
  static build(): Service { return new Service(); }
}
export function helper(id: number): string { return new Service().serve(new Request()); }
const arrow = (x: number) => x + 1;
export const arrow2 = async (x: number) => foo(x);
`))
	seenTS := map[string]bool{}
	for _, node := range ts.Nodes {
		seenTS[node.Kind+":"+node.QualifiedName] = true
	}
	for _, key := range []string{"class:Service", "method:Service.serve", "method:Service.build", "function:helper", "function:arrow", "function:arrow2"} {
		if !seenTS[key] {
			t.Fatalf("missing %s in %#v", key, ts.Nodes)
		}
	}
	imports := map[string]struct {
		module string
		symbol string
	}{}
	for _, node := range ts.Nodes {
		if node.Kind != "import" {
			continue
		}
		module, symbol := "", ""
		if node.ImportModule != nil {
			module = *node.ImportModule
		}
		if node.ImportSymbol != nil {
			symbol = *node.ImportSymbol
		}
		imports[node.Name] = struct {
			module string
			symbol string
		}{module: module, symbol: symbol}
	}
	for name, want := range map[string]struct {
		module string
		symbol string
	}{
		"foo": {"./util", "foo"},
		"baz": {"./util", "bar"},
		"ns":  {"lib", "*"},
		"def": {"mod", "default"},
	} {
		if imports[name] != want {
			t.Fatalf("import %s = %#v, want %#v; all=%#v", name, imports[name], want, imports)
		}
	}
	byQ := map[string]struct {
		exported bool
		async    bool
		static   bool
	}{}
	for _, node := range ts.Nodes {
		byQ[node.QualifiedName] = struct {
			exported bool
			async    bool
			static   bool
		}{node.IsExported, node.IsAsync, node.IsStatic}
	}
	if !byQ["Service"].exported || !byQ["helper"].exported || byQ["arrow"].exported || !byQ["arrow2"].exported {
		t.Fatalf("unexpected export flags: %#v", byQ)
	}
	if !byQ["Service.serve"].async || !byQ["Service.build"].static || !byQ["arrow2"].async {
		t.Fatalf("unexpected async/static flags: %#v", byQ)
	}
	for _, node := range ts.Nodes {
		if node.QualifiedName == "Service" && node.EndLine <= node.StartLine {
			t.Fatalf("expected TS class end_line to span body: %#v", node)
		}
	}
	targets := map[string]bool{}
	for _, edge := range ts.Edges {
		if edge.Kind == "calls" {
			targets[edge.Target] = true
		}
	}
	for _, target := range []string{"name:helper", "name:Service", "name:Request"} {
		if !targets[target] {
			t.Fatalf("missing TS call %s in %#v", target, ts.Edges)
		}
	}
}

func TestExtractJavaScriptAndTsx(t *testing.T) {
	js := extract.ExtractFile("app.js", []byte("import { helper as h } from './util';\nexport class Service {\n  run() { return h(); }\n}\nexport function main() { return new Service().run(); }\n"))
	seen := map[string]bool{}
	for _, node := range js.Nodes {
		seen[node.Kind+":"+node.Name] = true
	}
	for _, key := range []string{"class:Service", "method:run", "function:main", "import:h"} {
		if !seen[key] {
			t.Fatalf("missing %s in %#v", key, js.Nodes)
		}
	}
	var callsH bool
	for _, edge := range js.Edges {
		if edge.Kind == "calls" && edge.Target == "name:h" {
			callsH = true
		}
	}
	if !callsH {
		t.Fatalf("missing h call in %#v", js.Edges)
	}

	tsx := extract.ExtractFile("Button.tsx", []byte("export function Button() {\n  return <button onClick={() => track()}>Save</button>;\n}\nfunction track() {}\n"))
	var button, trackCall bool
	for _, node := range tsx.Nodes {
		if node.Kind == "function" && node.Name == "Button" {
			button = true
		}
	}
	for _, edge := range tsx.Edges {
		if edge.Kind == "calls" && edge.Target == "name:track" {
			trackCall = true
		}
	}
	if !button || !trackCall {
		t.Fatalf("button=%v trackCall=%v nodes=%#v edges=%#v", button, trackCall, tsx.Nodes, tsx.Edges)
	}
}
