package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

var ghToken string

func main() {
	var ctx Context
	err := readYaml("sigs.yaml", &ctx)
	if err != nil {
		log.Fatal(err)
	}
	ghToken = os.Getenv("GITHUB_TOKEN")
	if len(ghToken) == 0 {
		log.Fatal("please set GITHUB_TOKEN env variable with your personal access token")
	}
	if len(os.Args) < 2 {
		log.Fatal("please provide name of a sig/wg/commitee")
	}
	group := os.Args[1]
	if !validateGroup(group) {
		log.Fatal("invalid group name format, for ex, for sig xy-z, group name = sig-xy-z")
	}

	owners := getOwnersRoots(ctx, group)
	oc := &ownerCounter{
		ownerPaths:  map[string]string{},
		reviewers:   sets.Set[string]{},
		approvers:   sets.Set[string]{},
		ghUserCache: map[string]bool{},
	}
	tmpPath, err := cloneRepos(oc.getReposToClone(owners))
	if len(tmpPath) > 0 {
		defer os.RemoveAll(tmpPath)
	}
	if err != nil {
		log.Fatal(err)
	}
	err = oc.getCounts(tmpPath)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Reviewers:", oc.reviewers.Len())
	fmt.Println("Approvers:", oc.approvers.Len())
}

func cloneRepos(toClone map[string]string) (string, error) {
	path, err := os.MkdirTemp("", "ownerscount")
	if err != nil {
		return path, err
	}
	log.Println("cloning in", path)

	err = os.Chdir(path)
	if err != nil {
		return path, err
	}

	for cloneURL, pathName := range toClone {
		log.Println(">>> cloning", cloneURL, "at", pathName)
		err = exec.Command("git", "clone", cloneURL, pathName, "--depth", "1").Run()
		if err != nil {
			log.Println("!!!! error cloning", cloneURL, "at", pathName)
			return path, err
		}
	}

	err = os.Chdir("..")
	if err != nil {
		return path, err
	}

	return path, nil
}

func getOwnersRoots(ctx Context, group string) []string {
	owners := []string{}
	for _, sig := range ctx.Sigs {
		if sig.Dir != group {
			continue
		}
		for _, subproject := range sig.Subprojects {
			owners = append(owners, subproject.Owners...)
		}
	}
	return owners
}

type ownerCounter struct {
	// owner URL -> repo path mapping
	ownerPaths           map[string]string
	reviewers, approvers sets.Set[string]
	ghUserCache          map[string]bool
}

func (oc *ownerCounter) getReposToClone(owners []string) map[string]string {
	toClone := map[string]string{}
	for _, owner := range owners {
		qualifiedRepoName := getRepoName(owner)
		oc.ownerPaths[owner] = qualifiedRepoName
		toClone[sshCloneURL(owner)] = qualifiedRepoName
	}

	return toClone
}

func (oc *ownerCounter) getCounts(path string) error {
	if err := os.Chdir(path); err != nil {
		return err
	}

	for url, repoPath := range oc.ownerPaths {
		if err := os.Chdir(repoPath); err != nil {
			log.Println("!!!! cant process dir", repoPath, err)
			// Go back to the tmp dir for the next repo
			if err := os.Chdir(".."); err != nil {
				return err
			}
			continue
		}
		rootPath := getPath(url)
		var root string
		if strings.EqualFold(rootPath, "OWNERS") {
			root = "."
		} else {
			root = filepath.Dir(rootPath)
		}
		_, err := os.Lstat(root)
		if err != nil {
			log.Println("!!!! cant process dir", root, err)
			// Go back to the tmp dir for the next repo
			if err := os.Chdir(".."); err != nil {
				return err
			}
			continue
		}
		owners, err := GetOwnerFiles(root)
		if err != nil {
			return fmt.Errorf("cannot get owners files %w", err)
		}
		var aliasRoot string
		if subprojectInSubDir(url) {
			aliasRoot = "."
		} else {
			aliasRoot = root
		}
		aliasesPath, err := GetOwnersAliasesFile(aliasRoot)
		if err != nil {
			log.Println("!!!! OWNERS_ALIASES does not exist", err, aliasRoot)
		}
		alias := &Aliases{RepoAliases: map[string][]string{}}
		if len(aliasesPath) > 0 {
			alias, err = GetOwnerAliases(aliasesPath)
			if err != nil {
				log.Println("!!!! err reading OWNERS_ALIASES", err)
			}
		}
		kubeAlias := &Aliases{RepoAliases: map[string][]string{}}
		if isRepoKube(url) {
			kubeAlias = alias
		} else {
			kubeAliasPath, _ := GetOwnersAliasesFile(filepath.Join("..", "kubernetes-org-kubernetes"))
			kubeAlias, _ = GetOwnerAliases(kubeAliasPath)
		}
		log.Println(">>> Processing repo", repoPath)
		for _, ownerFile := range owners {
			info, err := GetOwnersInfo(ownerFile)
			if err != nil {
				log.Println("!!!! err reading file", ownerFile, err)
				continue
			}
			reviewers := sets.
				NewString(info.Reviewers...).Union(sets.NewString(info.RequiredReviewers...)).List()
			oc.computeCount(reviewers, info.Approvers, alias, kubeAlias)
		}
		// Go back to the tmp dir for the next repo
		if err := os.Chdir(".."); err != nil {
			return err
		}
	}
	return nil
}

func (oc *ownerCounter) computeCount(reviewers, approvers []string, alias, kubeAlias *Aliases) {
	for _, entity := range reviewers {
		aliases, ok := alias.RepoAliases[entity]
		if !ok {
			// Maybe it exists in kubeAlias because of the
			// staging magic.
			if kubeAlias != nil {
				aliases, ok = kubeAlias.RepoAliases[entity]
				if !ok {
					// We assume that if the value isn't an alias, it is
					// a username. Check if it is a valid GH user.
					if oc.checkIfValidGHUser(entity) {
						oc.reviewers.Insert(entity)
					} else {
						log.Println("!!!! invalid entity", entity)
					}
					continue
				}
			}
		}
		oc.reviewers.Insert(aliases...)
	}

	for _, entity := range approvers {
		aliases, ok := alias.RepoAliases[entity]
		if !ok {
			// Maybe it exists in kubeAlias because of the
			// staging magic.
			if kubeAlias != nil {
				aliases, ok = kubeAlias.RepoAliases[entity]
				if !ok {
					// We assume that if the value isn't an alias, it is
					// a username. Check if it is a valid GH user.
					if oc.checkIfValidGHUser(entity) {
						oc.approvers.Insert(entity)
					} else {
						log.Println("!!!! invalid entity", entity)
					}
					continue
				}
			}
		}
		oc.approvers.Insert(aliases...)
	}
}

func (oc *ownerCounter) checkIfValidGHUser(user string) bool {
	// check if we have already queried github for this user.
	if isValid, ok := oc.ghUserCache[user]; ok {
		return isValid
	}
	client := &http.Client{}
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/users/%s", user), nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", ghToken))
	resp, err := client.Do(req)

	// cache query result for this user.
	oc.ghUserCache[user] = err == nil && resp.StatusCode == http.StatusOK
	return oc.ghUserCache[user]
}

func isRepoKube(path string) bool {
	return strings.Contains(path, "kubernetes/kubernetes")
}

func validateGroup(group string) bool {
	switch {
	case strings.HasPrefix(group, "sig-"):
		return true
	case strings.HasPrefix(group, "wg-"):
		return true
	case strings.HasPrefix(group, "committee-"):
		return true
	}
	return false
}
