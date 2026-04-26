#!/bin/bash

# Script to clean Git history of sensitive tokens
# WARNING: This rewrites history - force push will be required

echo "Starting Git history cleaning..."

# First, create a backup branch just in case
git branch backup-before-clean

# Remove .env and .env.example from history (keep current state on disk)
echo "Removing .env and .env.example from history..."
git filter-branch --force --index-filter '
    git rm --cached --ignore-unmatch .env .env.example
' -- --all

# Clean reflog to make recovery harder
echo "Cleaning reflog..."
git reflog expire --expire=now --all
git gc --prune=now --aggressive

echo ""
echo "✅ History cleaning complete!"
echo ""
echo "IMPORTANT - Next steps:"
echo "1. Review the changes: git log --all --oneline"
echo "2. Force push: git push --force --all"
echo "3. Force push tags: git push --force --tags"
echo "4. On collaborators' machines: git pull --force"
echo ""
echo "Backup branch 'backup-before-clean' has been created in case of issues."
