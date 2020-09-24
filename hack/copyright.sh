# WARNING: This will remove any new line characters in files that are touched.
# Be sure to attempt to build before pushing changes

SCRIPT_DIR="$(cd $(dirname $0) && pwd -P)"
BASE_DIR="${SCRIPT_DIR}/.."

for file in $(find $BASE_DIR -type f -name "*.go" -not -path "*/vendor/*");
    do echo "$file"; echo -e "// Copyright 2020 VMware, Inc.\n// SPDX-License-Identifier: Apache-2.0\n\n$(cat $file)" > $file;
done
