package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/def4alt/kaptl/internal/bot"
	"github.com/def4alt/kaptl/internal/db"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Validate required env vars
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	// Connect to database
	ctx := context.Background()
	database, err := db.New(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()
	log.Println("✅ Connected to PostgreSQL")

	// Create and start bot
	b, err := bot.New(database)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("🛑 Shutting down...")
		b.Tele.Stop()
	}()

	log.Println("🤖 Kaptl bot is running...")
	b.Start()
}
