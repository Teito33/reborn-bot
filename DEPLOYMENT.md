# Guide d'intégration des Secrets - GHCR & GitHub Secrets

## Situation actuelle

- **Repo**: GitLab (https://gitlab.com/teito33-group/reborn-bot)
- **Image Docker**: À pousser sur GitHub Container Registry (GHCR)
- **Secrets**: Token Discord nécessaire au runtime

---

## Option 1: Migrer vers GitHub (RECOMMANDÉ pour GHCR)

### Étapes:

#### 1. Créer un nouveau repo GitHub
```bash
# Sur github.com/new
# Nom: reborn-bot
# Public: Oui (pour GHCR gratuit)
```

#### 2. Pousser le code depuis GitLab
```bash
cd d:\Discord BOT\discord-bot

# Ajouter GitHub comme remote
git remote add github https://github.com/YOUR_USERNAME/reborn-bot.git

# Pousser tout
git push github main
git push github --all
git push github --tags
```

#### 3. Ajouter les Secrets GitHub
1. Va sur: **GitHub.com** → Ton repo → **Settings** → **Secrets and variables** → **Actions**
2. Clique **New repository secret**
3. Ajoute:
   - **Name**: `DISCORD_TOKEN`
   - **Value**: (Ton token Discord - place-le ici, ne le mets pas dans Git!)
4. Clique **Add secret**

#### 4. Le workflow utilise automatiquement le secret
Le fichier `.github/workflows/build-and-push.yml` build l'image et la pousse.

Pour utiliser le token au runtime:
```bash
# Somewhere in your deployment (docker-compose, Kubernetes, etc.)
docker run \
  -e DISCORD_TOKEN=${DISCORD_TOKEN} \
  ghcr.io/your-username/reborn-bot:latest
```

---

## Option 2: Rester sur GitLab + pousser vers GHCR

Si tu veux garder ton repo sur GitLab mais pousser vers GHCR:

### 1. Créer un Personal Access Token GitHub

GitHub:
1. Va sur **Settings** → **Developer settings** → **Personal access tokens** → **Tokens (classic)**
2. Clique **Generate new token**
3. Configure:
   - **Expiration**: 90 jours (ou plus)
   - **Scopes**: ✅ `write:packages` (pour GHCR)
4. Copie le token généré

### 2. Ajouter les variables GitLab CI/CD

GitLab:
1. Va sur ton repo → **Settings** → **CI/CD** → **Variables**
2. Ajoute:
   - **Name**: `GHCR_TOKEN`
   - **Value**: `ton_token_github_ici`
   - **Protect variable**: ✅ Coché

3. Ajoute un autre:
   - **Name**: `DISCORD_TOKEN`
   - **Value**: `MTQ6MTM2MjU0MzE0OTk3MzYwNQ.GW7fvi.gG3-_Kjq5yFFW2t2CEZNgtCYkAXrqv9VwTRWUE`
   - **Protect variable**: ✅ Coché

### 3. Créer un `.gitlab-ci.yml`

```yaml
stages:
  - build

build_and_push:
  stage: build
  image: docker:latest
  services:
    - docker:dind
  script:
    # Login to GHCR
    - echo "${GHCR_TOKEN}" | docker login ghcr.io -u "YOUR_GITHUB_USERNAME" --password-stdin
    
    # Build
    - docker build -t ghcr.io/YOUR_GITHUB_USERNAME/reborn-bot:latest .
    
    # Push
    - docker push ghcr.io/YOUR_GITHUB_USERNAME/reborn-bot:latest
  only:
    - main
```

---

## À faire APRÈS que GitLab supprime la protection des branches:

```bash
# Force push les changements nettoyés
git push --force --all
git push --force --tags
```

---

## Checklist de déploiement GHCR

- [ ] Secrets ajoutés (GitHub ou GitLab)
- [ ] Workflow configuré
- [ ] Image buildée et pushée vers GHCR
- [ ] Vérifier: `docker pull ghcr.io/YOUR_USERNAME/reborn-bot:latest`
- [ ] Test de lancement avec token:
  ```bash
  docker run -e DISCORD_TOKEN=token_ici ghcr.io/YOUR_USERNAME/reborn-bot:latest
  ```

---

## Sécurité - Important ⚠️

✅ **À faire:**
- Stocker secrets dans GitHub/GitLab (jamais en Git)
- Utiliser variables d'environnement au runtime
- Marquer secrets comme "Protected"

❌ **À NE PAS faire:**
- Mettre secrets dans Dockerfile
- Commiter .env
- Pusher sur des repos publics

---

## Déploiement sur Kubernetes (optionnel)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: discord-bot-secrets
type: Opaque
stringData:
  discord_token: "ton_token_ici"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: discord-bot
spec:
  replicas: 1
  selector:
    matchLabels:
      app: discord-bot
  template:
    metadata:
      labels:
        app: discord-bot
    spec:
      containers:
      - name: discord-bot
        image: ghcr.io/YOUR_USERNAME/reborn-bot:latest
        imagePullPolicy: Always
        env:
        - name: DISCORD_TOKEN
          valueFrom:
            secretKeyRef:
              name: discord-bot-secrets
              key: discord_token
```
