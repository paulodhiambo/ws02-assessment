// Package carpkg packages the MI integration modules into Carbon Application
// (.car) archives.
//
// Each module directory (common, account-balance, customer-proxy,
// loan-eligibility, loan-events) becomes one CAR. Artifacts are discovered by folder
// convention:
//
//	<module>/src/main/synapse-config/api/*.xml           -> synapse/api
//	<module>/src/main/synapse-config/sequences/*.xml     -> synapse/sequence
//	<module>/src/main/synapse-config/endpoints/*.xml     -> synapse/endpoint
//	<module>/src/main/synapse-config/local-entries/*.xml -> synapse/local-entry
//	<module>/src/main/synapse-config/templates/*.xml     -> synapse/template
//	<module>/src/main/synapse-config/inbound-endpoints/*.xml -> synapse/inbound-endpoint
//	<module>/src/main/dataservice/*.dbs                  -> service/dataservice
//	common/sequences/*.xml                               -> synapse/sequence
//
// The CAR layout is the one produced by WSO2 Integration Studio / the MI
// VS Code extension, so the archives hot-deploy into any MI 4.x carbonapps
// directory:
//
//	<Name>_<version>.car
//	  artifacts.xml
//	  <Artifact>_<version>/artifact.xml
//	  <Artifact>_<version>/<Artifact>.xml|.dbs
package carpkg

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const serverRole = "EnterpriseIntegrator"

// Module is one MI module and the CAR it becomes.
type Module struct {
	Dir     string // directory under mi/
	CarName string
}

// Modules in deployment order.
var Modules = []Module{
	{"common", "JamiiCommon"},
	{"account-balance", "JamiiAccountBalance"},
	{"customer-proxy", "JamiiCustomerProxy"},
	{"loan-eligibility", "JamiiLoanEligibility"},
	{"loan-events", "JamiiLoanEvents"},
}

// synapseFolders maps a folder to its artifact type, in packaging order.
var synapseFolders = []struct{ folder, typ string }{
	{"local-entries", "synapse/local-entry"},
	{"endpoints", "synapse/endpoint"},
	{"sequences", "synapse/sequence"},
	{"templates", "synapse/template"},
	{"api", "synapse/api"},
	// Inbound endpoints last: they start consuming as soon as they deploy.
	{"inbound-endpoints", "synapse/inbound-endpoint"},
}

// Artifact is one deployable file.
type Artifact struct {
	Name, Type, Path string
}

func (a Artifact) String() string { return a.Type + ":" + a.Name }

// artifactName reads the name (or, for local entries, the key) from the root element.
func artifactName(path, typ string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var root *xml.StartElement
	for root == nil {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("%s: malformed XML: %w", path, err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			root = &se
		}
	}
	// Walk the whole document so malformed XML anywhere fails the build.
	for {
		if _, err := dec.Token(); err == io.EOF {
			break
		} else if err != nil {
			return "", fmt.Errorf("%s: malformed XML: %w", path, err)
		}
	}
	attr := "name"
	if typ == "synapse/local-entry" {
		attr = "key"
	}
	for _, a := range root.Attr {
		if a.Name.Local == attr {
			return a.Value, nil
		}
	}
	return "", nil
}

func glob(dir, pattern string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	sort.Strings(matches)
	return matches
}

// Discover lists the artifacts of one module directory.
func Discover(moduleDir string) ([]Artifact, error) {
	var found []Artifact
	synapseRoot := filepath.Join(moduleDir, "src", "main", "synapse-config")
	if filepath.Base(moduleDir) == "common" {
		synapseRoot = moduleDir
	}
	add := func(path, typ string) error {
		name, err := artifactName(path, typ)
		if err != nil {
			return err
		}
		found = append(found, Artifact{Name: name, Type: typ, Path: path})
		return nil
	}
	for _, f := range synapseFolders {
		for _, p := range glob(filepath.Join(synapseRoot, f.folder), "*.xml") {
			if err := add(p, f.typ); err != nil {
				return nil, err
			}
		}
	}
	for _, p := range glob(filepath.Join(moduleDir, "src", "main", "dataservice"), "*.dbs") {
		if err := add(p, "service/dataservice"); err != nil {
			return nil, err
		}
	}
	return found, nil
}

// Validate reports artifacts MI would reject: missing names, names that do
// not match the file name, and duplicates across modules.
func Validate(modules map[string][]Artifact) []string {
	var errs []string
	seen := map[string]string{}
	keys := make([]string, 0, len(modules))
	for k := range modules {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, module := range keys {
		artifacts := modules[module]
		if len(artifacts) == 0 {
			errs = append(errs, module+": no artifacts found")
		}
		for _, a := range artifacts {
			stem := strings.TrimSuffix(filepath.Base(a.Path), filepath.Ext(a.Path))
			switch {
			case a.Name == "":
				errs = append(errs, a.Path+": missing name/key attribute")
				continue
			case a.Name != stem:
				errs = append(errs, fmt.Sprintf("%s: name '%s' does not match file name '%s'", a.Path, a.Name, stem))
			}
			if other, dup := seen[a.Name]; dup {
				errs = append(errs, fmt.Sprintf("%s: duplicate artifact name '%s' (also in %s)", a.Path, a.Name, other))
			}
			seen[a.Name] = a.Path
		}
	}
	return errs
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Package writes <out>/<carName>_<version>.car and returns its path.
func Package(carName string, artifacts []Artifact, version, out string) (string, error) {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	carPath := filepath.Join(out, carName+"_"+version+".car")
	f, err := os.Create(carPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	zw := zip.NewWriter(f)

	var deps strings.Builder
	for _, a := range artifacts {
		folder := a.Name + "_" + version
		// MI's extractor does not create parent directories itself, so each
		// artifact folder needs an explicit directory entry.
		if _, err := zw.Create(folder + "/"); err != nil {
			return "", err
		}
		meta := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
			`<artifact name="%s" groupId="ke.co.jamiisavings.mi" version="%s" type="%s" serverRole="%s"><file>%s</file></artifact>`,
			esc(a.Name), esc(version), a.Type, serverRole, esc(filepath.Base(a.Path)))
		if err := writeEntry(zw, folder+"/artifact.xml", []byte(meta)); err != nil {
			return "", err
		}
		data, err := os.ReadFile(a.Path)
		if err != nil {
			return "", err
		}
		if err := writeEntry(zw, folder+"/"+filepath.Base(a.Path), data); err != nil {
			return "", err
		}
		fmt.Fprintf(&deps, `<dependency artifact="%s" version="%s" include="true" serverRole="%s" />`, esc(a.Name), esc(version), serverRole)
	}
	index := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
		`<artifacts><artifact name="%s" version="%s" type="carbon/application">%s</artifact></artifacts>`,
		esc(carName), esc(version), deps.String())
	if err := writeEntry(zw, "artifacts.xml", []byte(index)); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return carPath, nil
}

func writeEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// Run discovers, validates and (unless validateOnly) packages every module
// under miRoot, logging progress to log.
func Run(miRoot, version, out string, validateOnly bool, log io.Writer) error {
	modules := map[string][]Artifact{}
	for _, m := range Modules {
		artifacts, err := Discover(filepath.Join(miRoot, m.Dir))
		if err != nil {
			return err
		}
		modules[m.Dir] = artifacts
	}
	if errs := Validate(modules); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(log, "[package-car] ERROR %s\n", e)
		}
		return errors.New("artifact validation failed")
	}
	for _, m := range Modules {
		fmt.Fprintf(log, "[package-car] %s: %d artifacts %v\n", m.Dir, len(modules[m.Dir]), modules[m.Dir])
	}
	if validateOnly {
		return nil
	}
	for _, m := range Modules {
		path, err := Package(m.CarName, modules[m.Dir], version, out)
		if err != nil {
			return err
		}
		fmt.Fprintf(log, "[package-car] built %s\n", path)
	}
	return nil
}
