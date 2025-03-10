#!/bin/bash

# The script checks whether a runner with a specific label is busy

OWNER_REPO=$1
CHECK_LABELS=$2

if [[ -z $OWNER_REPO ]]; then
    cat <<EOF 1>&2
Usage: $0 OWNER/REPO CHECK_LABELS

example: $0 anyproto/test-concurrency "self-hosted ubuntu-latest"
EOF
    exit 1
fi

EXIT_CODE=0

# get current runners id
# gh api repos/anyproto/test-concurrency/actions/runs --jq '.workflow_runs[] | select(.status!="completed") | {id, name, status, created_at, html_url}'
for RUN_ID in $(gh api repos/${OWNER_REPO}/actions/runs --jq '.workflow_runs[] | select(.status!="completed") | .id'); do
    # get runner_name
    LABELS=$(gh api repos/${OWNER_REPO}/actions/runs/${RUN_ID}/jobs --jq '[.jobs[].labels[]] | unique | .[]')
    for CHECK_LABEL in $CHECK_LABELS; do
        if echo "$LABELS" | grep -q "$CHECK_LABEL"; then
            echo "A run='$RUN_ID' is executing on a runner with LABEL='$CHECK_LABEL' in the repository='$OWNER_REPO'" 1>&2
            EXIT_CODE=$(( EXIT_CODE + 1 ))
        else
            continue
        fi
    done
done

exit $EXIT_CODE
