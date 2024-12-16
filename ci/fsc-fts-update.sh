#!/bin/bash

# Variables
REPO_URL="git@github.com:hyperledger-labs/fabric-token-sdk.git"
DEPENDENCY="github.com/hyperledger-labs/fabric-smart-client"
REPO_DIR="repo"

# Get the last commit hash of the current branch
VERSION=$(git log -n 1 --pretty=format:"%H")
BRANCH_NAME=f-fsc-$VERSION

echo "Branch FTS to: $BRANCH_NAME"
echo "Last FSC commit hash: $VERSION"

echo "Clone the repository"
git clone $REPO_URL $REPO_DIR
cd $REPO_DIR

echo "Create a new branch"
git checkout -b $BRANCH_NAME

echo "Update the dependency"
go get $DEPENDENCY@$VERSION

echo "Commit the changes"
git add go.mod go.sum
git commit -m "Update $DEPENDENCY to $VERSION"

echo "Push the branch"
git push origin $BRANCH_NAME

echo "Create a pull request (requires GitHub CLI)"
gh pr create --title "Update $DEPENDENCY to $VERSION" --body "This PR updates $DEPENDENCY to version $VERSION."

echo "Cleanup: Remove the cloned repository"
cd ..
rm -rf $REPO_DIR

echo "Cleanup completed. The local repository has been removed."
