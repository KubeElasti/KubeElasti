#!/usr/bin/env bash
# Create a GitHub-verified commit via the createCommitOnBranch GraphQL mutation.
# Commits made this way are signed by GitHub and show as verified.
#
# Usage:
#   verified-commit.sh <commit-message-headline> <file> [<file> ...]
#
# Required env:
#   GH_TOKEN              - token with contents:write
#   GITHUB_REPOSITORY     - owner/repo
#   COMMIT_BRANCH         - branch to commit to (e.g. PR head ref)
#
# Optional env:
#   COMMIT_MESSAGE_BODY   - commit message body (e.g. Signed-off-by line)

set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <commit-message-headline> <file> [<file> ...]" >&2
  exit 1
fi

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "GH_TOKEN is required" >&2
  exit 1
fi

if [[ -z "${GITHUB_REPOSITORY:-}" ]]; then
  echo "GITHUB_REPOSITORY is required" >&2
  exit 1
fi

if [[ -z "${COMMIT_BRANCH:-}" ]]; then
  echo "COMMIT_BRANCH is required" >&2
  exit 1
fi

headline="$1"
shift

additions='[]'
for path in "$@"; do
  if [[ ! -f "$path" ]]; then
    echo "skipping missing file: $path" >&2
    continue
  fi
  additions="$(jq -c -n --argjson additions "$additions" --arg path "$path" --arg contents "$(base64 -w0 "$path")" \
    '$additions + [{path: $path, contents: $contents}]')"
done

if [[ "$(jq 'length' <<<"$additions")" -eq 0 ]]; then
  echo "No files to commit"
  exit 0
fi

expected_head_oid="$(git rev-parse HEAD)"

message_json="$(jq -n --arg headline "$headline" --arg body "${COMMIT_MESSAGE_BODY:-}" '
  if $body == "" then
    {headline: $headline}
  else
    {headline: $headline, body: $body}
  end
')"

payload="$(jq -n \
  --arg repo "$GITHUB_REPOSITORY" \
  --arg branch "$COMMIT_BRANCH" \
  --arg head "$expected_head_oid" \
  --argjson message "$message_json" \
  --argjson additions "$additions" \
  '{
    query: "mutation($input: CreateCommitOnBranchInput!) { createCommitOnBranch(input: $input) { commit { oid url } } }",
    variables: {
      input: {
        branch: {
          repositoryNameWithOwner: $repo,
          branchName: $branch
        },
        message: $message,
        fileChanges: { additions: $additions },
        expectedHeadOid: $head
      }
    }
  }')"

echo "Creating verified commit on ${COMMIT_BRANCH} (expected HEAD ${expected_head_oid})"
response="$(gh api graphql --input - <<<"$payload")"

if jq -e '.errors' >/dev/null <<<"$response"; then
  echo "createCommitOnBranch failed:" >&2
  echo "$response" | jq '.' >&2
  exit 1
fi

echo "$response" | jq -r '"Created verified commit \(.data.createCommitOnBranch.commit.oid) (\(.data.createCommitOnBranch.commit.url))"'
