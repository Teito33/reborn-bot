# Stage 1: Build
FROM golang:1.21 AS builder

WORKDIR /app

# Copier les fichiers de dépendances
COPY go.mod go.sum ./

# Télécharger les dépendances
RUN go mod download

# Copier le code source
COPY src/ ./src/

# Builder l'application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o discord-bot ./src/main.go

# Stage 2: Runtime
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copier le binaire depuis le stage de build
COPY --from=builder /app/discord-bot .

# Variable d'environnement pour le token Discord
ENV DISCORD_TOKEN=""

# Lancer l'application
ENTRYPOINT ["./discord-bot"]
