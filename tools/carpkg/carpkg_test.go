package carpkg

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func miRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "mi")
}

func TestRealModulesValidateAndPackage(t *testing.T) {
	out := t.TempDir()
	if err := Run(miRoot(), "9.9.9", out, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, m := range Modules {
		car := filepath.Join(out, m.CarName+"_9.9.9.car")
		zr, err := zip.OpenReader(car)
		if err != nil {
			t.Fatalf("%s: %v", m.CarName, err)
		}
		entries := map[string]*zip.File{}
		for _, f := range zr.File {
			entries[f.Name] = f
		}
		index := read(t, entries["artifacts.xml"])
		if !strings.Contains(index, `name="`+m.CarName+`" version="9.9.9" type="carbon/application"`) {
			t.Errorf("%s: bad artifacts.xml: %s", m.CarName, index)
		}
		artifacts, _ := Discover(filepath.Join(miRoot(), m.Dir))
		for _, a := range artifacts {
			folder := a.Name + "_9.9.9/"
			if entries[folder] == nil {
				t.Errorf("%s: missing directory entry %s (MI's extractor needs it)", m.CarName, folder)
			}
			meta := read(t, entries[folder+"artifact.xml"])
			if !strings.Contains(meta, `type="`+a.Type+`"`) || !strings.Contains(meta, "<file>"+filepath.Base(a.Path)+"</file>") {
				t.Errorf("%s: bad artifact.xml: %s", a.Name, meta)
			}
			if entries[folder+filepath.Base(a.Path)] == nil {
				t.Errorf("%s: artifact file missing", a.Name)
			}
			if !strings.Contains(index, `<dependency artifact="`+a.Name+`"`) {
				t.Errorf("%s: not listed as a dependency", a.Name)
			}
		}
		zr.Close()
	}
}

func read(t *testing.T, f *zip.File) string {
	t.Helper()
	if f == nil {
		t.Fatal("entry missing")
	}
	rc, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	return string(b)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidationCatchesNameMismatchAndDuplicates(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a/src/main/synapse-config/sequences/one.xml"), `<sequence name="other"/>`)
	writeFile(t, filepath.Join(root, "b/src/main/synapse-config/sequences/dup.xml"), `<sequence name="dup"/>`)
	writeFile(t, filepath.Join(root, "c/src/main/synapse-config/sequences/dup.xml"), `<sequence name="dup"/>`)
	modules := map[string][]Artifact{}
	for _, m := range []string{"a", "b", "c"} {
		artifacts, err := Discover(filepath.Join(root, m))
		if err != nil {
			t.Fatal(err)
		}
		modules[m] = artifacts
	}
	errs := strings.Join(Validate(modules), "\n")
	if !strings.Contains(errs, "does not match file name 'one'") || !strings.Contains(errs, "duplicate artifact name 'dup'") {
		t.Errorf("unexpected validation result:\n%s", errs)
	}
}

func TestMalformedXMLFailsDiscovery(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "m/src/main/synapse-config/api/Bad.xml"), `<api name="Bad"><resource></api>`)
	if _, err := Discover(filepath.Join(root, "m")); err == nil || !strings.Contains(err.Error(), "malformed XML") {
		t.Fatalf("want malformed XML error, got %v", err)
	}
}

func TestLocalEntriesUseKey(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "m/src/main/synapse-config/local-entries/Schema.xml"), `<localEntry key="Schema"><![CDATA[{}]]></localEntry>`)
	artifacts, err := Discover(filepath.Join(root, "m"))
	if err != nil || len(artifacts) != 1 || artifacts[0].Name != "Schema" || artifacts[0].Type != "synapse/local-entry" {
		t.Fatalf("got %v, %v", artifacts, err)
	}
}
