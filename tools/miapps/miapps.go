// Package miapps interprets the MI management API's GET /applications
// response for scripts/deploy-mi.sh.
package miapps

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type app struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type applications struct {
	ActiveList []app            `json:"activeList"`
	FaultyList *json.RawMessage `json:"faultyList"`
}

func parse(r io.Reader) (applications, error) {
	var a applications
	if err := json.NewDecoder(r).Decode(&a); err != nil {
		return a, fmt.Errorf("invalid /applications response: %w", err)
	}
	return a, nil
}

// split turns "Name:version" into its parts.
func split(expected string) (string, string) {
	name, version, _ := strings.Cut(expected, ":")
	return name, version
}

// State reports the deployment state of the expected "Name:version" apps:
// "faulty:<names>" if MI lists any of them as faulty, "pending:<apps>" if
// some are not active yet, otherwise "ok".
func State(r io.Reader, expected []string) (string, error) {
	a, err := parse(r)
	if err != nil {
		return "", err
	}
	active := map[string]bool{}
	for _, x := range a.ActiveList {
		active[x.Name+":"+x.Version] = true
	}
	faultyRaw := ""
	if a.FaultyList != nil {
		faultyRaw = string(*a.FaultyList)
	}
	var faulty, pending []string
	for _, e := range expected {
		name, _ := split(e)
		if strings.Contains(faultyRaw, name) {
			faulty = append(faulty, name)
		}
		if !active[e] {
			pending = append(pending, e)
		}
	}
	switch {
	case len(faulty) > 0:
		return "faulty:" + strings.Join(faulty, ","), nil
	case len(pending) > 0:
		return "pending:" + strings.Join(pending, ","), nil
	}
	return "ok", nil
}

// Undeployed reports whether none of the expected apps (any version) is active.
func Undeployed(r io.Reader, expected []string) (bool, error) {
	a, err := parse(r)
	if err != nil {
		return false, err
	}
	names := map[string]bool{}
	for _, x := range a.ActiveList {
		names[x.Name] = true
	}
	for _, e := range expected {
		if name, _ := split(e); names[name] {
			return false, nil
		}
	}
	return true, nil
}

// Active lists the active apps as "Name:version, ...".
func Active(r io.Reader) (string, error) {
	a, err := parse(r)
	if err != nil {
		return "", err
	}
	out := make([]string, 0, len(a.ActiveList))
	for _, x := range a.ActiveList {
		out = append(out, x.Name+":"+x.Version)
	}
	return strings.Join(out, ", "), nil
}
