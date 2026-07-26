package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/def4alt/kaptl/internal/bot"
	"github.com/def4alt/kaptl/internal/db"
	"github.com/def4alt/kaptl/internal/fx/nbu"
	"github.com/def4alt/kaptl/internal/reporting"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Validate required env vars
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	// Connect to database
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
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

	workerID, err := os.Hostname()
	if err != nil || workerID == "" {
		workerID = "kaptl"
	}
	provider := nbu.NewClient(
		"https://bank.gov.ua/NBUStatService/v1/statdirectory/exchange",
		&http.Client{Timeout: 15 * time.Second},
	)
	valuationWorker := reporting.NewWorker(database, provider, workerID)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		valuationWorker.Run(ctx)
	}()

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		log.Println("🛑 Shutting down...")
		b.Tele.Stop()
	}()

	log.Println("🤖 Kaptl bot is running...")
	b.Start()
	stop()
	<-workerDone
}
