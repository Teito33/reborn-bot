# Secrets Management Guide

This document explains how to safely manage secrets in this project, especially when deploying on GitHub Container Registry.

## Local Development

### Setting up `.env` file

1. Copy the example file:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` and add your actual tokens and credentials:
   ```env
   DISCORD_TOKEN=your_actual_token_here
   ```

3. **IMPORTANT:** Never commit `.env` - it's in `.gitignore` by default

## Variables Required

- **`DISCORD_TOKEN`** (Required)
  - Your Discord bot token
  - Get it from [Discord Developer Portal](https://discord.com/developers/applications)
  - Never share this token

## Docker Deployment

When running with Docker:

```bash
docker run -e DISCORD_TOKEN=your_token_here discord-bot:latest
```

## GitHub Container Registry Deployment

### Using GitHub Actions with Secrets

1. Add your secrets to GitHub:
   - Go to Repository → Settings → Secrets and variables → Actions
   - Add new repository secret: `DISCORD_TOKEN`

2. Use in workflow file (`.github/workflows/deploy.yml`):
   ```yaml
   env:
     DISCORD_TOKEN: ${{ secrets.DISCORD_TOKEN }}
   ```

### Important Security Rules

✅ **DO:**
- Store secrets in GitHub repository secrets
- Use environment variables at runtime
- Keep `.env` file local only
- Rotate tokens if accidentally exposed

❌ **DON'T:**
- Commit secrets to the repository
- Store secrets in Dockerfile
- Push `.env` files to any branch
- Share tokens in issues or pull requests

## Data Files

The following files are excluded from Git and contain session/user data:
- `boosts_data.json`
- `CACHED`
- `ERROR`
- `*.tar`

These are automatically generates at runtime and should not be committed.

## Token Rotation

If your token is accidentally exposed:
1. Immediately regenerate it in Discord Developer Portal
2. Update all deployment configurations
3. Check git history to ensure it wasn't committed
