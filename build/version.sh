#!/usr/bin/env bash

# Copyright 2020 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit -o nounset -o pipefail

if [ -z "$(git tag --list)" ]; then
    if [ -n "${VERSION_FORCE_TAG_FETCH:-}" ]; then
        >&2 echo "fetching git tags"
        git fetch --tags --all 1> /dev/null 2> /dev/null
        >&2 echo "unshallowing git repository"
        git fetch --unshallow 1> /dev/null 2> /dev/null
    else
        >&2 echo "failed to fetch git tags"
        exit 1
    fi
fi

git_tag=$(git describe --tags)

# This dirty check also takes into consideration new files not staged for
# commit.
git_dirty=$([[ -z "$(git status --short)" ]] || echo "-dirty")

# Use the git tag, removing the leading 'v' if it exists, appended with -dirty
# if there are changes in the tree.
echo "${git_tag/#v/}${git_dirty}"
