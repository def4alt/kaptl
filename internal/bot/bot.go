package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

const dbTimeout = 5 * time.Second

// Bot carries runtime state for the telegram bot.
type Bot struct {
	Tele   *tele.Bot
	Store  models.Store
	mu     sync.Mutex
	States map[int64]*userState
}

func New(store models.Store) (*Bot, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN not set")
	}

	allowedID, err := strconv.ParseInt(os.Getenv("ALLOWED_TELEGRAM_ID"), 10, 64)
	if err != nil && os.Getenv("ALLOWED_TELEGRAM_ID") != "" {
		log.Printf("invalid ALLOWED_TELEGRAM_ID, auth disabled: %v", err)
		allowedID = 0
	}

	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
		ParseMode: tele.ModeMarkdownV2,
	}

	tb, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}

	b := &Bot{
		Tele:   tb,
		Store:  store,
		States: make(map[int64]*userState),
	}

	if allowedID != 0 {
		authMiddleware(tb, allowedID)
	}
	userMiddleware(tb, b)
	RegisterHandlers(tb, b)
	registerCommands(tb)
	return b, nil
}

func (b *Bot) Start() {
	log.Println("🤖 Bot starting...")
	b.Tele.Start()
}

// ─── Middleware ────────────────────────────────────────────

func authMiddleware(tb *tele.Bot, allowedID int64) {
	tb.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			if c.Sender().ID != allowedID {
				return c.Send("⛔ This bot is private.")
			}
			return next(c)
		}
	})
}

func userMiddleware(tb *tele.Bot, b *Bot) {
	tb.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
			defer cancel()
			s := c.Sender()
			_, err := b.Store.GetOrCreateUser(ctx, s.ID, s.Username, s.FirstName, s.LanguageCode)
			if err != nil {
				log.Printf("get or create user: %v", err)
			}
			return next(c)
		}
	})
}

// ─── Commands registration ─────────────────────────────────

func registerCommands(tb *tele.Bot) {
	cmds := []tele.Command{
		{Text: "start", Description: "show main menu"},
		{Text: "help", Description: "show help"},
		{Text: "cat", Description: "/cat add 🍞 Name | /cat rm Name | /cat list"},
		{Text: "acc", Description: "/acc add 💳 Name [currency] | /acc list"},
		{Text: "budget", Description: "/budget set Name amount [weekly|monthly|quarterly]"},
		{Text: "move", Description: "/move amount from Account to Account"},
		{Text: "group", Description: "/group add 📁 Name | /group rm Name | /group"},
	}
	if err := tb.SetCommands(cmds); err != nil {
		log.Printf("register commands: %v", err)
	}
}

// ─── State management ──────────────────────────────────────

func (b *Bot) stateFor(uid int64) *userState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.States[uid]
}

func (b *Bot) setState(uid int64, s *userState) {
	b.mu.Lock()
	b.States[uid] = s
	b.mu.Unlock()
}

func (b *Bot) clearState(uid int64) {
	b.mu.Lock()
	delete(b.States, uid)
	b.mu.Unlock()
}
