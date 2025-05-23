#!/usr/bin/env bash
set -euo pipefail

# Get the root directory of the repository
rootDir="$(git rev-parse --show-toplevel)"

ghBuildPath="$rootDir/bin/gh"

ghCLIArtifact="$rootDir/pkg/cmd/attestation/test/data/gh_2.60.1_windows_arm64.zip"

# Verify the gh CLI artifact
echo "Testing with package $ghCLIArtifact"
if ! $ghBuildPath attestation verify "$ghCLIArtifact" --digest-alg=sha256 --owner=cli; then
    echo "Failed to verify"
    exit 1
fi

# Try to verify when specifying a predicate type that does not match the attestation
if $ghBuildPath attestation verify "$ghCLIArtifact" --digest-alg=sha256 --owner=cli --predicate-type=my-custom-predicate-type; then
    echo "Verification should have failed"
    exit 1
fi
