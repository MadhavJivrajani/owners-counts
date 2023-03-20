/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"k8s.io/enhancements/api"

	yaml "gopkg.in/yaml.v3"
)

const (
	regexRawGitHubURL = "https://raw.githubusercontent.com/(?P<org>[^/]+)/(?P<repo>[^/]+)/(?P<branch>[^/]+)/(?P<path>.*)"
	regexGitHubURL    = "https://github.com/(?P<org>[^/]+)/(?P<repo>[^/]+)/(blob|tree)/(?P<branch>[^/]+)/(?P<path>.*)"
)

// FoldedString is a string that will be serialized in FoldedStyle by go-yaml
type FoldedString string

// MarshalYAML customizes how FoldedStrings will be serialized by go-yaml
func (x FoldedString) MarshalYAML() (interface{}, error) {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Style: yaml.FoldedStyle,
		Value: string(x),
	}, nil
}

// Person represents an individual person holding a role in a group.
type Person struct {
	GitHub  string
	Name    string
	Company string `yaml:"company,omitempty"`
}

// Meeting represents a regular meeting for a group.
type Meeting struct {
	Description   string
	Day           string
	Time          string
	TZ            string
	Frequency     string
	URL           string `yaml:",omitempty"`
	ArchiveURL    string `yaml:"archive_url,omitempty"`
	RecordingsURL string `yaml:"recordings_url,omitempty"`
}

// Contact represents the various contact points for a group.
type Contact struct {
	Slack              string       `yaml:",omitempty"`
	MailingList        string       `yaml:"mailing_list,omitempty"`
	PrivateMailingList string       `yaml:"private_mailing_list,omitempty"`
	GithubTeams        []GithubTeam `yaml:"teams,omitempty"`
	Liaison            Person       `yaml:"liaison,omitempty"`
}

// GithubTeam represents a specific Github Team.
type GithubTeam struct {
	Name        string
	Description string `yaml:",omitempty"`
}

// Subproject represents a specific subproject owned by the group
type Subproject struct {
	Name        string
	Description string   `yaml:",omitempty"`
	Contact     *Contact `yaml:",omitempty"`
	Owners      []string
	Meetings    []Meeting `yaml:",omitempty"`
}

// LeadershipGroup represents the different groups of leaders within a group
type LeadershipGroup struct {
	Chairs         []Person
	TechnicalLeads []Person `yaml:"tech_leads,omitempty"`
	EmeritusLeads  []Person `yaml:"emeritus_leads,omitempty"`
}

// PrefixToPersonMap returns a map of prefix to persons, useful for iteration over all persons
func (g *LeadershipGroup) PrefixToPersonMap() map[string][]Person {
	return map[string][]Person{
		"chair":         g.Chairs,
		"tech_lead":     g.TechnicalLeads,
		"emeritus_lead": g.EmeritusLeads,
	}
}

// Owners returns a sorted and de-duped list of owners for a LeadershipGroup
func (g *LeadershipGroup) Owners() []Person {
	o := append(g.Chairs, g.TechnicalLeads...)

	// Sort
	sort.Slice(o, func(i, j int) bool {
		return o[i].GitHub < o[j].GitHub
	})

	// De-dupe
	seen := make(map[string]struct{}, len(o))
	i := 0
	for _, p := range o {
		if _, ok := seen[p.GitHub]; ok {
			continue
		}
		seen[p.GitHub] = struct{}{}
		o[i] = p
		i++
	}
	return o[:i]
}

// Group represents either a Special Interest Group (SIG) or a Working Group (WG)
type Group struct {
	Dir              string
	Prefix           string `yaml:",omitempty"`
	Name             string
	MissionStatement FoldedString `yaml:"mission_statement,omitempty"`
	CharterLink      string       `yaml:"charter_link,omitempty"`
	ReportingWGs     []WGName     `yaml:"-"` // populated by Context#Complete()
	StakeholderSIGs  []SIGName    `yaml:"stakeholder_sigs,omitempty"`
	Label            string
	Leadership       LeadershipGroup `yaml:"leadership"`
	Meetings         []Meeting
	Contact          Contact
	Subprojects      []Subproject              `yaml:",omitempty"`
	KEPs             map[string][]api.Proposal `yaml:",omitempty"`
}

type WGName string

func (n WGName) DirName() string {
	return DirName("wg", string(n))
}

type SIGName string

func (n SIGName) DirName() string {
	return DirName("sig", string(n))
}

// DirName returns the directory that a group's documentation will be
// generated into. It is composed of a prefix (sig for SIGs and wg for WGs),
// and a formatted version of the group's name (in kebab case).
func (g *Group) DirName(prefix string) string {
	return DirName(prefix, g.Name)
}

func DirName(prefix, name string) string {
	return fmt.Sprintf("%s-%s", prefix, strings.ToLower(strings.Replace(name, " ", "-", -1)))
}

// LabelName returns the expected label for a given group
func (g *Group) LabelName(prefix string) string {
	return strings.Replace(g.DirName(prefix), fmt.Sprintf("%s-", prefix), "", 1)
}

// Context is the context for the sigs.yaml file.
type Context struct {
	Sigs          []Group
	WorkingGroups []Group
	UserGroups    []Group
	Committees    []Group
}

// Complete populates derived portions of the Context struct
func (c *Context) Complete() {
	// Copy working group names into ReportingWGs list of their stakeholder sigs
	for _, wg := range c.WorkingGroups {
		for _, stakeholderSIG := range wg.StakeholderSIGs {
			for i, sig := range c.Sigs {
				if sig.Name == string(stakeholderSIG) {
					c.Sigs[i].ReportingWGs = append(c.Sigs[i].ReportingWGs, WGName(wg.Name))
				}
			}
		}
	}
}

func sshCloneURL(url string) string {
	re := regexp.MustCompile(regexRawGitHubURL)
	mat := re.FindStringSubmatchIndex(url)
	if mat == nil {
		return url
	}
	result := re.ExpandString([]byte{}, "git@github.com:${org}/${repo}.git", url, mat)
	return string(result)
}

func getPath(url string) string {
	re := regexp.MustCompile(regexRawGitHubURL)
	mat := re.FindStringSubmatchIndex(url)
	if mat == nil {
		return url
	}
	return string(url[mat[8]:])
}

func getRepo(url string) string {
	re := regexp.MustCompile(regexRawGitHubURL)
	mat := re.FindStringSubmatchIndex(url)
	if mat == nil {
		return url
	}
	return strings.Split(string(url[mat[4]:]), string(os.PathSeparator))[0]
}

// readSigsYaml decodes yaml stored in a file at path into the
// specified yaml.Node
func readYaml(path string, data interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	return decoder.Decode(data)
}
