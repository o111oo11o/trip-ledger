package main

import (
	"embed"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/o111oo11o/trip-ledger/internal/bot"
	"github.com/o111oo11o/trip-ledger/internal/service"
	"github.com/o111oo11o/trip-ledger/internal/store"
	"github.com/o111oo11o/trip-ledger/pkg/currency"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "trip-ledger.db"
	}

	exchangeAPIKey := os.Getenv("EXCHANGE_RATE_API_KEY")
	if exchangeAPIKey == "" {
		log.Fatal("EXCHANGE_RATE_API_KEY environment variable is required")
	}

	// Initialize database.
	db, err := store.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	// Run migrations.
	migrationSQL, err := migrationsFS.ReadFile("migrations/001_init.up.sql")
	if err != nil {
		log.Fatalf("Failed to read migration: %v", err)
	}
	if err := db.Migrate(string(migrationSQL)); err != nil {
		log.Printf("Failed to run migrations: %v", err)
	}

	// Initialize stores.
	groupStore := store.NewGroupStore(db)
	memberStore := store.NewMemberStore(db)
	txStore := store.NewTransactionStore(db)

	// Initialize currency client.
	currClient := currency.NewClient(exchangeAPIKey)

	// Initialize service.
	svc := service.New(groupStore, memberStore, txStore, currClient)

	// Initialize Telegram bot.
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("Failed to create Telegram bot: %v", err)
	}
	log.Printf("Authorized on account %s", api.Self.UserName)

	// Start bot.
	b := bot.New(api, svc)
	b.Run()
}
