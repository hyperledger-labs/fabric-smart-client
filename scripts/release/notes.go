/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

const blobURL = "https://github.com/hyperledger-labs/fabric-smart-client/blob/"

// stepsDirs hold the golden steps files, <dialect>-<store>.steps, of every FSC store.
var stepsDirs = []string{
	"platform/view/services/storage/driver/sql/testdata/steps",
	"platform/fabric/services/db/driver/sql/testdata/steps",
}

// guideParts returns the opening paragraphs of the upgrade guide at path, between its title and
// its first heading, and its "# Checklist" section with every heading demoted one level. Lines
// inside fenced code blocks are never headings.
func guideParts(path string) (intro, checklist string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed reading the upgrade guide")
	}
	const (
		beforeTitle = iota
		inIntro
		inChecklist
		elsewhere
	)
	var introLines, checkLines []string
	section := beforeTitle
	fenced := false
	for line := range strings.Lines(string(raw)) {
		line = strings.TrimRight(line, "\n")
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			fenced = !fenced
		}
		heading := !fenced && strings.HasPrefix(line, "#")
		switch {
		case heading && section == beforeTitle:
			section = inIntro
			continue
		case heading && line == "# Checklist":
			section = inChecklist
			continue
		case heading && (section == inIntro || strings.HasPrefix(line, "# ")):
			section = elsewhere
		}
		switch section {
		case inIntro:
			introLines = append(introLines, line)
		case inChecklist:
			if heading {
				line = "#" + line
			}
			checkLines = append(checkLines, line)
		}
	}
	intro = strings.TrimSpace(strings.Join(introLines, "\n"))
	checklist = strings.TrimSpace(strings.Join(checkLines, "\n"))
	if intro == "" {
		return "", "", errors.Errorf("%s: no paragraph between the title and the first heading", path)
	}
	if checklist == "" {
		return "", "", errors.Errorf(`%s: no "# Checklist" section`, path)
	}
	return intro, checklist, nil
}

var mdLink = regexp.MustCompile(`\]\(([^)\s]*)\)`)

// absLinks makes the relative links of md, quoted from guide, point at the files at the tag
// version, so they work outside the repository.
func absLinks(md, guide, version string) string {
	return mdLink.ReplaceAllStringFunc(md, func(m string) string {
		target := mdLink.FindStringSubmatch(m)[1]
		if strings.Contains(target, "://") {
			return m
		}
		file, anchor, hasAnchor := strings.Cut(target, "#")
		if file == "" {
			file = guide
		} else {
			file = path.Join(path.Dir(guide), file)
		}
		if hasAnchor {
			anchor = "#" + anchor
		}
		return "](" + blobURL + version + "/" + file + anchor + ")"
	})
}

// unwrap joins the hard-wrapped lines of md's paragraphs and list items, since GitHub renders a
// newline in a release body as a line break. Fenced code, headings and blank lines stay; a line
// starting a list item, or indented as one, starts a new line.
func unwrap(md string) string {
	var out []string
	fenced, join := false, false
	for line := range strings.Lines(md) {
		line = strings.TrimRight(line, "\n")
		trimmed := strings.TrimSpace(line)
		fence := strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
		item := strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || orderedItem.MatchString(trimmed)
		switch {
		case fence || fenced:
			out = append(out, line)
			join = false
			if fence {
				fenced = !fenced
			}
		case trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "|"):
			out = append(out, line)
			join = false
		case join && !item:
			out[len(out)-1] += " " + trimmed
		default:
			out = append(out, line)
			join = true
		}
	}
	return strings.Join(out, "\n")
}

var orderedItem = regexp.MustCompile(`^\d+\. `)

// stepRow is one schema step added since the previous release, in every dialect of its store.
type stepRow struct {
	Store    string
	Version  int
	Name     string
	Contract bool
}

// addedSteps returns the steps of stores whose version is not in the golden steps file of their
// store at tag prev. A store without a file at prev is new, so all its steps are. A row is a
// contract step if it is one in any dialect.
func addedSteps(dir string, prev semver, stores []Store) ([]stepRow, error) {
	rows := map[[2]string]*stepRow{}
	for _, s := range stores {
		released, err := releasedSteps(dir, prev, s.Dialect+"-"+s.Type+".steps")
		if err != nil {
			return nil, err
		}
		for _, st := range s.Steps {
			if released[st.Version] {
				continue
			}
			key := [2]string{s.Type, fmt.Sprint(st.Version)}
			if rows[key] == nil {
				rows[key] = &stepRow{Store: s.Type, Version: st.Version, Name: st.Name}
			}
			rows[key].Contract = rows[key].Contract || st.Contract
		}
	}
	out := make([]stepRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b stepRow) int {
		return cmp.Or(strings.Compare(a.Store, b.Store), cmp.Compare(a.Version, b.Version))
	})
	return out, nil
}

// releasedSteps returns the step versions in the golden steps file name at tag prev; none if
// prev has no such file.
func releasedSteps(dir string, prev semver, name string) (map[int]bool, error) {
	versions := map[int]bool{}
	for _, d := range stepsDirs {
		out, _, err := showAt(dir, prev, d+"/"+name)
		if err != nil {
			return nil, err
		}
		for line := range strings.Lines(string(out)) {
			var v int
			if _, err := fmt.Sscanf(line, "%d ", &v); err == nil {
				versions[v] = true
			}
		}
	}
	return versions, nil
}

// testedNames are the rows of the "Tested with" table, in order.
var testedNames = []struct{ key, name string }{
	{"go", "Go"},
	{"fabric", "Hyperledger Fabric"},
	{"fabric_x_committer", "Fabric-x committer"},
	{"fabric_x_tools", "Fabric-x tools"},
	{"postgres", "PostgreSQL"},
}

// notes returns the body of the GitHub release of d: the upgrade guide's opening paragraphs and
// checklist, the release data as tables, and the generated changelog unchanged.
func notes(dir string, d Data, changelog string) (string, error) {
	v, ok := parseVersion(d.Version)
	if !ok {
		return "", errors.Errorf("invalid version [%s]", d.Version)
	}
	if d.Upgrade.Mode != "stop-all" {
		return "", errors.Errorf("upgrade mode %q has no release notes text", d.Upgrade.Mode)
	}
	intro, checklist, err := guideParts(filepath.Join(dir, filepath.FromSlash(d.Upgrade.Guide)))
	if err != nil {
		return "", err
	}
	tags, err := releaseTags(dir)
	if err != nil {
		return "", err
	}
	prev, err := previousRelease(tags, v)
	if err != nil {
		return "", err
	}
	steps, err := addedSteps(dir, prev, d.Stores)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString(unwrap(absLinks(intro, d.Upgrade.Guide, d.Version)) + "\n\n")
	b.WriteString("## Checklist\n\n" + unwrap(absLinks(checklist, d.Upgrade.Guide, d.Version)) + "\n\n")
	fmt.Fprintf(&b, "Full guide: %s%s/%s\n\n", blobURL, d.Version, d.Upgrade.Guide)
	b.WriteString("## Upgrade\n\nStop-all upgrade: every node of a network stops before any node runs " + d.Version + ".\n\n")
	b.WriteString("## Tested with\n\n| Component | Version |\n|---|---|\n")
	for _, t := range testedNames {
		fmt.Fprintf(&b, "| %s | %s |\n", t.name, d.TestedWith[t.key])
	}
	if len(steps) > 0 {
		b.WriteString("\n## Schema steps\n\n| Store | Step | Name | Rollback |\n|---|---|---|---|\n")
		for _, s := range steps {
			rollback := "free"
			if s.Contract {
				rollback = "restore from backup"
			}
			fmt.Fprintf(&b, "| %s | %d | %s | %s |\n", s.Store, s.Version, s.Name, rollback)
		}
	}
	if len(d.Components) > 0 {
		b.WriteString("\n## Protocol versions\n\n| Component | Min | Max |\n|---|---|---|\n")
		for _, c := range d.Components {
			fmt.Fprintf(&b, "| %s | %d | %d |\n", c.Name, c.Min, c.Max)
		}
	}
	b.WriteString("\n" + strings.TrimSpace(changelog) + "\n")
	return b.String(), nil
}
