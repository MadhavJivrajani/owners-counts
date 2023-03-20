# owners-counts

This contains some very rough code to automate the process of getting unique reviewer and approver counts for a SIG/WG/Committee.

## Disclaimer

Much of this code is taken from existing tooling in [`k/community`](https://github.com/kubernetes/community/tree/master/generator) and [`k-sigs/maintainers`](https://github.com/kubernetes-sigs/maintainers).

Ideally, this should be part of `k-sigs/maintainers` and should be eventually PR'd in, once in better shape.

## Running The Tool
The tool on a high level, does the following:
- Clones all repos that a subproject by itself or parent repos whose sub-directories contain a subproject (like most `staging/src/k8s.io` dirs in `k/k`) into a temp dir.
- Goes through all `OWNERS` files and the `OWNERS_ALIAS` if any, and counts how many reviewers and approvers exist while de-duplicating them.

### Prerequisites

You will need to have a [GitHub personal access token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token) to run this tool.

Once you have one, please store it in an env var called `GITHUB_TOKEN`

```
export GITHUB_TOKEN=<your_token>
```

### Running It

```
go build .
```
and
```
./owners-counts <sig/wg/committee-name>
```
Please note that the sig/wg/committee name is how it appears in `sigs.yaml`.
For ex, for SIG API Machinery, the name would be `sig-api-machinery`.
