package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"discord-bot/src/handlers"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	godotenv.Load()

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_TOKEN environment variable not set")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("error creating Discord session: %v", err)
	}

	// Register event handlers
	dg.AddHandler(handlers.HandleMessageCreate)
	dg.AddHandler(handlers.HandleReactionAdd)
	dg.AddHandler(handlers.HandleReactionRemove)

	// Intents
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuildMessageReactions | discordgo.IntentsGuilds

	err = dg.Open()
	if err != nil {
		log.Fatalf("error opening connection: %v", err)
	}

	// Load previous sessions from disk
	if err := handlers.LoadSessions(); err != nil {
		log.Fatalf("error loading sessions: %v", err)
	}

	log.Println("Bot is now running. Press CTRL+C to exit.")

	// Wait for interrupt signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.Close()
}
