package main

import (
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

type OwnersInfo struct {
	Filters           map[string]FiltersInfo `json:"filters,omitempty"`
	Approvers         []string               `json:"approvers,omitempty"`
	Reviewers         []string               `json:"reviewers,omitempty"`
	RequiredReviewers []string               `json:"required_reviewers,omitempty"`
	Labels            []string               `json:"labels,omitempty"`
	EmeritusApprovers []string               `json:"emeritus_approvers,omitempty"`
	EmeritusReviewers []string               `json:"emeritus_reviewers,omitempty"`
	Options           DirOptions             `json:"options,omitempty"`
}

type DirOptions struct {
	NoParentOwners bool `json:"no_parent_owners,omitempty"`
}

type FiltersInfo struct {
	Approvers         []string `json:"approvers,omitempty"`
	Reviewers         []string `json:"reviewers,omitempty"`
	Labels            []string `json:"labels,omitempty"`
	EmeritusApprovers []string `json:"emeritus_approvers,omitempty"`
	EmeritusReviewers []string `json:"emeritus_reviewers,omitempty"`
	RequiredReviewers []string `json:"required_reviewers,omitempty"`
}

type Aliases struct {
	RepoAliases map[string][]string `json:"aliases,omitempty"`
}

func GetOwnerAliases(filename string) (*Aliases, error) {
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	config := &Aliases{}
	err = yaml.UnmarshalStrict(yamlFile, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func GetOwnersInfo(file string) (*OwnersInfo, error) {
	filename, _ := filepath.Abs(file)
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	config, err := GetOwnersInfoFromBytes(bytes)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func GetOwnersInfoFromBytes(bytes []byte) (*OwnersInfo, error) {
	config := &OwnersInfo{}
	err := yaml.UnmarshalStrict(bytes, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func GetOwnersAliasesFile(root string) (string, error) {
	var err error
	aliasPath, _ := filepath.Abs(filepath.Join(root, "OWNERS_ALIASES"))
	if _, err = os.Stat(aliasPath); err == nil {
		return aliasPath, nil
	}
	return "", err
}

func GetOwnerFiles(root string) ([]string, error) {
	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "OWNERS" && !strings.Contains(path, "vendor") {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}
