#!/bin/bash
set -eo pipefail

function set_globals {
    pipeline=$1
    TARGET="${TARGET:-denver}"
    FLY_URL="https://concourse.cf-denver.com"
}

function set_pipeline {
    project_dir="$(git rev-parse --show-toplevel)"
    pipeline_file="$project_dir/ci/ci.yaml"

    echo setting pipeline for "$pipeline_name"

    fly -t denver set-pipeline -p azure \
        -c  ${pipeline_file} \
        -l <(lpass show 'Shared-Loggregator (Pivotal Only)/pipeline-secrets.yml' --notes)
}

function sync_fly {
    if ! fly -t ${TARGET} status; then
      fly -t ${TARGET} login -b -c ${FLY_URL} --team-name loggregator
    fi
    fly -t ${TARGET} sync
}

function set_pipelines {
    if [[ "$pipeline" = all ]]; then
        for pipeline_path in $(find pipelines -maxdepth 1 -type f); do
          pipeline_file=$(basename ${pipeline_path})
          set_pipeline "${pipeline_file%%.*}"
        done
        exit 0
    fi

    set_pipeline "$pipeline"
}

function print_usage {
    echo "usage: $0 <pipeline | all>"
}

function main {
    set_globals $1
    sync_fly
    set_pipelines
}

lpass ls 1>/dev/null
main $1 $2
