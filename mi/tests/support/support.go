// Package support loads MI artifacts (Synapse XML, data services) as a
// generic element tree and offers the small set of queries the artifact
// tests need. Element names are matched by local name, ignoring namespaces.
package support

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Modules in the order they are packaged.
var Modules = []string{"common", "account-balance", "customer-proxy", "loan-eligibility"}

// ErrorEnvelopeKeys are the JSON keys every error response must contain.
var ErrorEnvelopeKeys = []string{"error", "code", "message", "status", "requestId", "correlationId", "timestamp"}

// Node is one XML element.
type Node struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []*Node    `xml:",any"`
	Text     string     `xml:",chardata"`
}

// Name is the element's local name.
func (n *Node) Name() string { return n.XMLName.Local }

// Attr returns the attribute value, or "" if it is absent.
func (n *Node) Attr(name string) string {
	v, _ := n.Lookup(name)
	return v
}

// Lookup returns the attribute value and whether it is present.
func (n *Node) Lookup(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// All returns n and every descendant element named name, in document order.
func (n *Node) All(name string) []*Node {
	var out []*Node
	var walk func(*Node)
	walk = func(e *Node) {
		if e.Name() == name {
			out = append(out, e)
		}
		for _, c := range e.Children {
			walk(c)
		}
	}
	walk(n)
	return out
}

// First returns the first descendant named name whose attributes include
// every key=value pair in attrs, or nil.
func (n *Node) First(name string, attrs ...string) *Node {
	for _, e := range n.All(name) {
		if e.has(attrs) {
			return e
		}
	}
	return nil
}

// Child returns the first direct child named name, or nil.
func (n *Node) Child(name string, attrs ...string) *Node {
	for _, c := range n.Children {
		if c.Name() == name && c.has(attrs) {
			return c
		}
	}
	return nil
}

func (n *Node) has(attrs []string) bool {
	for i := 0; i+1 < len(attrs); i += 2 {
		if n.Attr(attrs[i]) != attrs[i+1] {
			return false
		}
	}
	return true
}

// PropertiesSet returns the literal values assigned to property name anywhere under n.
func (n *Node) PropertiesSet(name string) []string {
	var out []string
	for _, p := range n.All("property") {
		if v, ok := p.Lookup("value"); ok && p.Attr("name") == name {
			out = append(out, v)
		}
	}
	return out
}

// SequenceKeys returns the keys of every <sequence key="..."/> call under n.
func (n *Node) SequenceKeys() []string {
	var out []string
	for _, s := range n.All("sequence") {
		if k := s.Attr("key"); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// PayloadFormats returns the text of every payloadFactory <format> under n.
func (n *Node) PayloadFormats() []string {
	var out []string
	for _, f := range n.All("format") {
		if t := strings.TrimSpace(f.Text); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// MIRoot is the mi/ directory.
func MIRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// SynapseDir is where a module keeps its Synapse artifacts.
func SynapseDir(module string) string {
	if module == "common" {
		return filepath.Join(MIRoot(), "common")
	}
	return filepath.Join(MIRoot(), module, "src", "main", "synapse-config")
}

// Parse reads an XML file.
func Parse(t testing.TB, path string) *Node {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var n Node
	if err := xml.Unmarshal(data, &n); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return &n
}

// Load reads <module>/.../<folder>/<name>.xml.
func Load(t testing.TB, module, folder, name string) *Node {
	t.Helper()
	return Parse(t, filepath.Join(SynapseDir(module), folder, name+".xml"))
}

// Artifact is one Synapse XML file.
type Artifact struct {
	File string
	Root *Node
}

// AllArtifacts returns every Synapse XML file across all modules.
func AllArtifacts(t testing.TB) []Artifact {
	t.Helper()
	var out []Artifact
	for _, m := range Modules {
		var paths []string
		_ = filepath.WalkDir(SynapseDir(m), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".xml") {
				paths = append(paths, p)
			}
			return nil
		})
		sort.Strings(paths)
		for _, p := range paths {
			out = append(out, Artifact{File: filepath.Base(p), Root: Parse(t, p)})
		}
	}
	if len(out) == 0 {
		t.Fatal("no artifacts found")
	}
	return out
}

// JSONFormat renders a payloadFactory JSON format as a parsed object by
// filling the $n placeholders ("$1" -> "x", bare $1 -> 0).
func JSONFormat(t testing.TB, format string) map[string]any {
	t.Helper()
	filled := format
	for i := 20; i >= 1; i-- {
		p := "$" + strconv.Itoa(i)
		filled = strings.ReplaceAll(filled, `"`+p+`"`, `"x"`)
		filled = strings.ReplaceAll(filled, p, "0")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(filled), &out); err != nil {
		t.Fatalf("format is not valid JSON once filled: %v\n%s", err, filled)
	}
	return out
}

// Keys returns the sorted keys of a JSON object.
func Keys(v any) []string {
	m, _ := v.(map[string]any)
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Set returns the sorted, de-duplicated values.
func Set(values ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// Contains reports whether values contains v.
func Contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
