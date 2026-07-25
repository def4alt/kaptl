package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

type apiCall struct {
	Path        string
	Text        string
	ReplyMarkup json.RawMessage
}

type telegramRecorder struct {
	mu    sync.Mutex
	calls []apiCall
}

func newTestTelegramServer(t *testing.T) (*httptest.Server, *telegramRecorder) {
	t.Helper()
	recorder := &telegramRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"test","username":"test_bot","can_join_groups":true,"can_read_all_group_messages":true,"supports_inline_queries":true}}`))
		case strings.Contains(r.URL.Path, "/sendMessage"), strings.Contains(r.URL.Path, "/editMessageText"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":303330553},"from":{"id":1,"is_bot":true,"first_name":"test","username":"test_bot"}}}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	t.Cleanup(server.Close)
	return server, recorder
}

func (r *telegramRecorder) record(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	_ = req.ParseMultipartForm(1 << 20)
	_ = req.ParseForm()

	call := apiCall{Path: req.URL.Path, Text: req.Form.Get("text")}
	if raw := req.Form.Get("reply_markup"); raw != "" {
		call.ReplyMarkup = json.RawMessage(raw)
	}
	if len(body) > 0 {
		var payload struct {
			Text        string          `json:"text"`
			ReplyMarkup json.RawMessage `json:"reply_markup"`
		}
		if json.Unmarshal(body, &payload) == nil {
			if call.Text == "" {
				call.Text = payload.Text
			}
			if len(call.ReplyMarkup) == 0 {
				call.ReplyMarkup = payload.ReplyMarkup
			}
		}
	}
	if len(call.ReplyMarkup) > 0 && call.ReplyMarkup[0] == '"' {
		var encoded string
		if json.Unmarshal(call.ReplyMarkup, &encoded) == nil {
			call.ReplyMarkup = json.RawMessage(encoded)
		}
	}

	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
}

func (r *telegramRecorder) reset() {
	r.mu.Lock()
	r.calls = nil
	r.mu.Unlock()
}

func (r *telegramRecorder) callCount(pathSuffix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, call := range r.calls {
		if strings.HasSuffix(call.Path, pathSuffix) {
			count++
		}
	}
	return count
}

func (r *telegramRecorder) lastText(t *testing.T, pathSuffix string) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.calls) - 1; i >= 0; i-- {
		call := r.calls[i]
		if strings.HasSuffix(call.Path, pathSuffix) && call.Text != "" {
			return call.Text
		}
	}
	t.Fatalf("no API call ending in %q with text; calls: %+v", pathSuffix, r.calls)
	return ""
}

func (r *telegramRecorder) callbackData(t *testing.T, label string) string {
	t.Helper()
	type button struct {
		Text         string `json:"text"`
		CallbackData string `json:"callback_data"`
	}
	type markup struct {
		InlineKeyboard [][]button `json:"inline_keyboard"`
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.calls) - 1; i >= 0; i-- {
		if len(r.calls[i].ReplyMarkup) == 0 {
			continue
		}
		var keyboard markup
		if json.Unmarshal(r.calls[i].ReplyMarkup, &keyboard) != nil {
			continue
		}
		for _, row := range keyboard.InlineKeyboard {
			for _, btn := range row {
				if btn.Text == label && btn.CallbackData != "" {
					return btn.CallbackData
				}
			}
		}
	}
	t.Fatalf("button %q not found in emitted Telegram markup; calls: %+v", label, r.calls)
	return ""
}

func testBotWithRecorder(t *testing.T) (*Bot, *memStore, *telegramRecorder) {
	t.Helper()
	server, recorder := newTestTelegramServer(t)
	store := newMemStore()
	store.users[303330553] = &models.User{TelegramID: 303330553, FirstName: "Test"}

	pref := tele.Settings{
		Token:       "test-token",
		Poller:      &tele.LongPoller{Timeout: 0},
		URL:         server.URL,
		Synchronous: true,
		OnError: func(err error, _ tele.Context) {
			t.Errorf("Telebot handler/API error: %v", err)
		},
	}
	tb, err := tele.NewBot(pref)
	if err != nil {
		t.Fatalf("create bot: %v", err)
	}

	b := &Bot{Tele: tb, Store: store, States: make(map[int64]*userState)}
	RegisterHandlers(b.Tele, b)
	recorder.reset() // Ignore NewBot's getMe request.
	return b, store, recorder
}

// testBot creates a bot wired to an in-memory store for testing.
func testBot(t *testing.T) (*Bot, *memStore) {
	t.Helper()
	b, store, _ := testBotWithRecorder(t)
	return b, store
}

// update creates a synthetic Telegram update from a text message.
func textUpdate(text string) tele.Update {
	return tele.Update{
		Message: &tele.Message{
			Text:   text,
			Sender: &tele.User{ID: 303330553, Username: "test", FirstName: "Test"},
			Chat:   &tele.Chat{ID: 303330553, Type: tele.ChatPrivate},
		},
	}
}

// callbackUpdate creates a synthetic callback query from dynamic inline button data.
func callbackUpdate(data string) tele.Update {
	return tele.Update{
		Callback: &tele.Callback{
			ID:     "test-callback-id",
			Data:   data,
			Sender: &tele.User{ID: 303330553, Username: "test", FirstName: "Test"},
			Message: &tele.Message{
				ID:   1,
				Chat: &tele.Chat{ID: 303330553, Type: tele.ChatPrivate},
			},
		},
	}
}

// staticCb creates a callback for a static tele.Btn (Unique-based routing).
// telebot internally prefixes these with \f in the callback data.
func staticCb(unique string) tele.Update {
	return tele.Update{
		Callback: &tele.Callback{
			ID:     "test-callback-id",
			Data:   "\f" + unique,
			Sender: &tele.User{ID: 303330553, Username: "test", FirstName: "Test"},
			Message: &tele.Message{
				ID:   1,
				Chat: &tele.Chat{ID: 303330553, Type: tele.ChatPrivate},
			},
		},
	}
}

func processUpdate(b *Bot, u tele.Update) {
	b.Tele.ProcessUpdate(u)
}

func clickButton(t *testing.T, b *Bot, recorder *telegramRecorder, label string) {
	t.Helper()
	processUpdate(b, callbackUpdate(recorder.callbackData(t, label)))
}

// ─── Tests ────────────────────────────────────────────────

func TestStart(t *testing.T) {
	b, store := testBot(t)
	processUpdate(b, textUpdate("/start"))

	// User should exist
	if _, ok := store.users[303330553]; !ok {
		t.Error("user not created")
	}
}

func TestCatAdd(t *testing.T) {
	b, store := testBot(t)

	processUpdate(b, textUpdate("/cat add 🍞 Groceries"))

	cats, _ := store.GetCategories(nil, 303330553)
	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	if cats[0].Name != "Groceries" {
		t.Errorf("expected 'Groceries', got '%s'", cats[0].Name)
	}
	if cats[0].Emoji != "🍞" {
		t.Errorf("expected '🍞', got '%s'", cats[0].Emoji)
	}
}

func TestCatAddNoArgs(t *testing.T) {
	b, _ := testBot(t)

	processUpdate(b, textUpdate("/cat add"))

	// Should return usage without panicking. Hard to assert output without mock API.
	// At minimum, verify no crash.
}

func TestCatList(t *testing.T) {
	b, store := testBot(t)

	store.CreateCategory(nil, 303330553, "Food", "🍞", nil)
	store.CreateCategory(nil, 303330553, "Transport", "🚗", nil)

	processUpdate(b, textUpdate("/cat list"))

	cats, _ := store.GetCategories(nil, 303330553)
	if len(cats) != 2 {
		t.Errorf("expected 2 categories, got %d", len(cats))
	}
}

func TestCatRemove(t *testing.T) {
	b, store := testBot(t)

	store.CreateCategory(nil, 303330553, "Food", "🍞", nil)
	processUpdate(b, textUpdate("/cat rm Food"))

	cats, _ := store.GetCategories(nil, 303330553)
	if len(cats) != 0 {
		t.Errorf("expected 0 categories after delete, got %d", len(cats))
	}
}

func TestCatRemoveNotFound(t *testing.T) {
	b, _ := testBot(t)
	processUpdate(b, textUpdate("/cat rm NonExistent"))
	// Should not panic
}

func TestAccAdd(t *testing.T) {
	b, store := testBot(t)

	processUpdate(b, textUpdate("/acc add 💳 Mono EUR"))

	accs, _ := store.GetAccounts(nil, 303330553)
	if len(accs) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accs))
	}
	if accs[0].Name != "Mono" {
		t.Errorf("expected 'Mono', got '%s'", accs[0].Name)
	}
	if accs[0].Emoji != "💳" {
		t.Errorf("expected '💳', got '%s'", accs[0].Emoji)
	}
	if accs[0].Currency != "EUR" {
		t.Errorf("expected 'EUR', got '%s'", accs[0].Currency)
	}
}

func TestAccAddDefaults(t *testing.T) {
	b, store := testBot(t)

	processUpdate(b, textUpdate("/acc add Cash"))

	accs, _ := store.GetAccounts(nil, 303330553)
	if len(accs) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accs))
	}
	if accs[0].Emoji != "💳" {
		t.Errorf("expected default 💳 emoji, got '%s'", accs[0].Emoji)
	}
	if accs[0].Currency != "EUR" {
		t.Errorf("expected default EUR, got '%s'", accs[0].Currency)
	}
}

func TestBudgetSet(t *testing.T) {
	b, store := testBot(t)

	store.CreateCategory(nil, 303330553, "Groceries", "🍞", nil)
	processUpdate(b, textUpdate("/budget set Groceries 5000"))

	summary, _ := store.GetBudgetSummary(nil, 303330553, 0)
	if len(summary) != 1 {
		t.Fatalf("expected 1 category in summary, got %d", len(summary))
	}
	if summary[0].Budget != 5000 {
		t.Errorf("expected budget 5000, got %.0f", summary[0].Budget)
	}
}

func TestExpenseWizard(t *testing.T) {
	b, store := testBot(t)

	store.CreateCategory(nil, 303330553, "Food", "🍞", nil)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 0)

	// Tap "➕ Expense" → pick category "Food" (callback: cat|1)
	processUpdate(b, staticCb("cat|1"))
	// Type amount
	processUpdate(b, textUpdate("42.50"))
	// Pick account "Mono" (callback: acc|1)
	processUpdate(b, staticCb("acc|1"))

	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txs))
	}
	if txs[0].Amount != 42.50 {
		t.Errorf("expected 42.50, got %.2f", txs[0].Amount)
	}
	if txs[0].Type != "expense" {
		t.Errorf("expected 'expense', got '%s'", txs[0].Type)
	}
}

func TestHandleText(t *testing.T) {
	b, _ := testBot(t)
	// Sending bare text like a URL should trigger handleText and return a message
	// This shouldn't panic
	processUpdate(b, textUpdate("https://google.com"))
}

func TestHandleTextInWizard(t *testing.T) {
	b, store := testBot(t)
	store.CreateCategory(nil, 303330553, "Food", "🍞", nil)

	// Start expense wizard by picking category
	processUpdate(b, staticCb("cat|1"))
	// Enter an invalid amount (not a number)
	processUpdate(b, textUpdate("not-a-number"))
	// Should show error, not crash
}

func TestHandleCallbackUnknown(t *testing.T) {
	b, _ := testBot(t)
	processUpdate(b, staticCb("unknown|data"))
	// Should not panic — defer c.Respond() handles it
}

func TestBudgetCommandWithoutCategoriesShowsGuidance(t *testing.T) {
	b, _, recorder := testBotWithRecorder(t)
	processUpdate(b, textUpdate("/budget"))
	assertContains(t, recorder.lastText(t, "/sendMessage"), "No categories yet")
}

func TestHandleBudgetSetNotFound(t *testing.T) {
	b, _ := testBot(t)
	processUpdate(b, textUpdate("/budget set NonExistent 5000"))
	// Should show "Category not found"
}

func TestWizardCancel(t *testing.T) {
	b, store := testBot(t)
	store.CreateCategory(nil, 303330553, "Food", "🍞", nil)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 0)

	// Start expense wizard
	processUpdate(b, staticCb("cat|1"))
	// Type amount
	processUpdate(b, textUpdate("42.50"))
	// Cancel instead of picking account
	processUpdate(b, staticCb("cancel"))

	// No transaction should be created
	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 0 {
		t.Errorf("expected 0 transactions after cancel, got %d", len(txs))
	}
}

func TestRecentEmpty(t *testing.T) {
	b, _ := testBot(t)
	processUpdate(b, textUpdate("/menu"))
	// Trigger the 📋 Recent button by simulating callback
	processUpdate(b, staticCb("recent"))
	// Should show "No transactions yet!"
}

func TestIncomeWizard(t *testing.T) {
	b, store := testBot(t)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 0)

	// Tap 💵 Income → type amount
	processUpdate(b, staticCb("add_income"))
	processUpdate(b, textUpdate("1500"))
	// Pick account
	processUpdate(b, staticCb("acc|1"))

	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 1 {
		t.Fatalf("expected 1 income transaction, got %d", len(txs))
	}
	if txs[0].Type != "income" {
		t.Errorf("expected 'income', got '%s'", txs[0].Type)
	}
	if txs[0].Amount != 1500 {
		t.Errorf("expected 1500, got %.2f", txs[0].Amount)
	}
}

func TestSummaryWithData(t *testing.T) {
	b, store := testBot(t)
	store.CreateCategory(nil, 303330553, "Food", "🍞", nil)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 0)

	store.SetBudget(nil, 303330553, 1, 0, 1, 5000)
	store.CreateTransaction(nil, 303330553, 1, intPtr(1), "expense", 2500, nil, "lunch")

	processUpdate(b, staticCb("summary"))

	summary, _ := store.GetBudgetSummary(nil, 303330553, 0)
	if len(summary) != 1 {
		t.Fatalf("expected 1 category in summary, got %d", len(summary))
	}
	if summary[0].Spent != 2500 {
		t.Errorf("expected spent 2500, got %.0f", summary[0].Spent)
	}
	if summary[0].Remaining != 2500 {
		t.Errorf("expected remaining 2500, got %.0f", summary[0].Remaining)
	}
}

func TestDuplicateCategory(t *testing.T) {
	b, store := testBot(t)
	store.CreateCategory(nil, 303330553, "Food", "🍞", nil)
	processUpdate(b, textUpdate("/cat add 🍕 Food"))
	// memStore returns error for duplicate, handler sends error message
}

// ─── Helpers ──────────────────────────────────────────────

func intPtr(i int64) *int64 { return &i }

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected '%s' to contain '%s'", haystack, needle)
	}
}

func TestE2E(t *testing.T) {
	b, store := testBot(t)

	// 1. Create an account
	t.Log("Step 1: Create account")
	processUpdate(b, textUpdate("/acc add 💳 Mono EUR"))

	accs, _ := store.GetAccounts(nil, 303330553)
	if len(accs) != 1 || accs[0].Name != "Mono" || accs[0].Currency != "EUR" {
		t.Fatalf("account creation failed: %+v", accs)
	}
	accID := accs[0].ID
	t.Logf("  ✅ Account: %s %s (%s)", accs[0].Emoji, accs[0].Name, accs[0].Currency)

	// 2. Create a category
	t.Log("Step 2: Create category")
	processUpdate(b, textUpdate("/cat add 🍞 Groceries"))

	cats, _ := store.GetCategories(nil, 303330553)
	if len(cats) != 1 || cats[0].Name != "Groceries" {
		t.Fatalf("category creation failed: %+v", cats)
	}
	catID := cats[0].ID
	t.Logf("  ✅ Category: %s %s (id=%d)", cats[0].Emoji, cats[0].Name, catID)

	// 3. Set budget first
	t.Log("Step 3: Set budget")
	processUpdate(b, textUpdate("/budget set Groceries 3000"))

	budgets, _ := store.GetBudgets(nil, 303330553)
	found := false
	for _, bd := range budgets {
		if bd.CategoryID == catID && bd.Amount == 3000 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("budget not found: %+v", budgets)
	}
	t.Logf("  ✅ Budget set: Groceries = 3000/month")

	// 4. Add income
	t.Log("Step 4: Add income")
	processUpdate(b, staticCb("add_income"))
	processUpdate(b, textUpdate("5000"))
	processUpdate(b, staticCb(fmt.Sprintf("acc|%d", accID)))

	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 1 || txs[0].Type != "income" || txs[0].Amount != 5000 {
		t.Fatalf("income failed: %+v", txs)
	}
	t.Logf("  ✅ Income: +%.2f on %s", txs[0].Amount, txs[0].AccountName)

	// 5. Add expenses
	t.Log("Step 5: Add expenses")
	processUpdate(b, staticCb(fmt.Sprintf("cat|%d", catID)))
	processUpdate(b, textUpdate("42.50"))
	processUpdate(b, staticCb(fmt.Sprintf("acc|%d", accID)))

	processUpdate(b, staticCb(fmt.Sprintf("cat|%d", catID)))
	processUpdate(b, textUpdate("18.90"))
	processUpdate(b, staticCb(fmt.Sprintf("acc|%d", accID)))

	txs, _ = store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 3 {
		t.Fatalf("expected 3 transactions, got %d", len(txs))
	}
	totalExp := 0.0
	for _, tx := range txs {
		if tx.Type == "expense" {
			totalExp += tx.Amount
		}
	}
	t.Logf("  ✅ Expenses logged: %d transactions, total spend: %.2f", len(txs)-1, totalExp)

	// 6. View summary
	t.Log("Step 6: View summary")
	processUpdate(b, staticCb("summary"))

	summary, _ := store.GetBudgetSummary(nil, 303330553, 0)
	if len(summary) != 1 {
		t.Fatalf("expected 1 category in summary, got %d", len(summary))
	}
	s := summary[0]
	t.Logf("  ✅ Summary: %s %s — spent: %.2f / budget: %.0f / remaining: %.0f",
		s.Emoji, s.Name, s.Spent, s.Budget, s.Remaining)

	if s.Spent != totalExp {
		t.Errorf("spent mismatch: expected %.2f, got %.2f", totalExp, s.Spent)
	}
	if s.Budget != 3000 {
		t.Errorf("budget mismatch: expected 3000, got %.0f", s.Budget)
	}
	if s.Remaining != 3000-totalExp {
		t.Errorf("remaining mismatch: expected %.2f, got %.2f", 3000-totalExp, s.Remaining)
	}

	// 7. View budgets
	t.Log("Step 7: View budgets")
	processUpdate(b, staticCb("budgets"))

	// 8. View recent transactions
	t.Log("Step 8: Recent transactions")
	processUpdate(b, staticCb("recent"))

	// 9. View accounts with updated balance
	t.Log("Step 9: Account balance")
	processUpdate(b, staticCb("accounts"))
	accs, _ = store.GetAccounts(nil, 303330553)
	t.Logf("  ✅ Final balance: %s %s = %.2f %s", accs[0].Emoji, accs[0].Name, accs[0].Balance, accs[0].Currency)

	expectedBalance := 5000.0 - totalExp
	if accs[0].Balance != expectedBalance {
		t.Errorf("balance mismatch: expected %.2f, got %.2f", expectedBalance, accs[0].Balance)
	}
}

func TestMoveCommand(t *testing.T) {
	b, store := testBot(t)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 1000)
	store.CreateAccount(nil, 303330553, "Cash", "💵", "EUR", 500)

	processUpdate(b, textUpdate("/move 200 from Mono to Cash"))

	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txs))
	}
	if txs[0].Type != "transfer" {
		t.Errorf("expected 'transfer', got '%s'", txs[0].Type)
	}
	if txs[0].Amount != 200 {
		t.Errorf("expected 200, got %.2f", txs[0].Amount)
	}
}

func TestMoveCommandInvalid(t *testing.T) {
	b, store := testBot(t)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 1000)

	// Missing args
	processUpdate(b, textUpdate("/move"))
	// Wrong format
	processUpdate(b, textUpdate("/move 500"))
	// Negative amount
	processUpdate(b, textUpdate("/move -100 from Mono to Cash"))
	// Source = dest
	store.CreateAccount(nil, 303330553, "Cash", "💵", "EUR", 500)
	processUpdate(b, textUpdate("/move 100 from Mono to Mono"))

	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 0 {
		t.Errorf("expected 0 transfers, got %d", len(txs))
	}
}

func TestMoveInteractive(t *testing.T) {
	b, store := testBot(t)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 1000)
	store.CreateAccount(nil, 303330553, "Cash", "💵", "EUR", 500)

	// Tap 🔀 Move → pick source (Mono, id=1) → pick dest (Cash, id=2) → enter amount
	processUpdate(b, staticCb("move"))
	processUpdate(b, staticCb("acc|1")) // source
	processUpdate(b, staticCb("acc|2")) // destination
	processUpdate(b, textUpdate("300")) // amount

	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 1 || txs[0].Type != "transfer" || txs[0].Amount != 300 {
		t.Fatalf("transfer failed: %+v", txs)
	}
}

func TestMoveInteractiveCancel(t *testing.T) {
	b, store := testBot(t)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 1000)
	store.CreateAccount(nil, 303330553, "Cash", "💵", "EUR", 500)

	// Tap 🔀 Move → pick source → cancel
	processUpdate(b, staticCb("move"))
	processUpdate(b, staticCb("acc|1"))
	processUpdate(b, staticCb("cancel"))

	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 0 {
		t.Errorf("expected 0 transfers after cancel, got %d", len(txs))
	}
}

func TestMoveNeedsTwoAccounts(t *testing.T) {
	b, store := testBot(t)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 1000)

	// Should warn that 2 accounts are needed
	processUpdate(b, staticCb("move"))
	// Nothing crashes, message sent
}

func TestMoveE2E(t *testing.T) {
	b, store := testBot(t)
	store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 1000)
	store.CreateAccount(nil, 303330553, "Cash", "💵", "USD", 500)

	// Interactive move: Mono → Cash, 400
	processUpdate(b, staticCb("move"))
	processUpdate(b, staticCb("acc|1"))
	processUpdate(b, staticCb("acc|2"))
	processUpdate(b, textUpdate("400"))

	// Check balances
	accs, _ := store.GetAccounts(nil, 303330553)
	var mono, cash *models.Account
	for i, a := range accs {
		switch a.Name {
		case "Mono":
			mono = &accs[i]
		case "Cash":
			cash = &accs[i]
		}
	}

	if mono.Balance != 600 {
		t.Errorf("Mono balance: expected 600, got %.2f", mono.Balance)
	}
	if cash.Balance != 900 {
		t.Errorf("Cash balance: expected 900, got %.2f", cash.Balance)
	}

	// Also test command: Cash → Mono, 200
	processUpdate(b, textUpdate("/move 200 from Cash to Mono"))
	accs, _ = store.GetAccounts(nil, 303330553)
	for i, a := range accs {
		switch a.Name {
		case "Mono":
			mono = &accs[i]
		case "Cash":
			cash = &accs[i]
		}
	}

	if mono.Balance != 800 {
		t.Errorf("Mono balance after cmd: expected 800, got %.2f", mono.Balance)
	}
	if cash.Balance != 700 {
		t.Errorf("Cash balance after cmd: expected 700, got %.2f", cash.Balance)
	}
}

func TestBudgetInteractive(t *testing.T) {
	b, store := testBot(t)
	store.CreateCategory(nil, 303330553, "Groceries", "🍞", nil)

	processUpdate(b, staticCb("budgets"))
	processUpdate(b, staticCb("budget|1"))
	processUpdate(b, textUpdate("3000"))
	processUpdate(b, staticCb("intv|monthly")) // pick interval

	budgets, _ := store.GetBudgets(nil, 303330553)
	if len(budgets) != 1 || budgets[0].Amount != 3000 {
		t.Fatalf("budget not set via interactive: %+v", budgets)
	}
}

func TestManageNavigationSendsNewMessagesAndAcknowledgesCallbacks(t *testing.T) {
	b, _, recorder := testBotWithRecorder(t)

	// /menu sends one message
	processUpdate(b, textUpdate("/menu"))

	// Button taps edit the existing message + acknowledge callback
	clickButton(t, b, recorder, "⚙️ Manage")
	assertContains(t, recorder.lastText(t, "/editMessageText"), "Manage")

	clickButton(t, b, recorder, "🏷️ Categories")
	assertContains(t, recorder.lastText(t, "/editMessageText"), "/cat add")

	clickButton(t, b, recorder, "◀ Back")
	assertContains(t, recorder.lastText(t, "/editMessageText"), "Manage")

	clickButton(t, b, recorder, "◀ Back")
	assertContains(t, recorder.lastText(t, "/editMessageText"), "Kaptl")

	// 1 /menu send + 4 button edits
	if got := recorder.callCount("/sendMessage"); got != 1 {
		t.Fatalf("expected 1 sendMessage (/menu), got %d", got)
	}
	if got := recorder.callCount("/answerCallbackQuery"); got != 4 {
		t.Fatalf("expected 4 acknowledged callbacks, got %d", got)
	}
	if got := recorder.callCount("/editMessageText"); got != 4 {
		t.Fatalf("expected 4 edits (callback navigation), got %d", got)
	}
}

func TestExpenseBackChangesCategoryBeforeSaving(t *testing.T) {
	b, store, recorder := testBotWithRecorder(t)
	food, _ := store.CreateCategory(nil, 303330553, "Food", "🍞", nil)
	travel, _ := store.CreateCategory(nil, 303330553, "Travel", "🚗", nil)
	account, _ := store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 0)

	processUpdate(b, textUpdate("/menu"))
	clickButton(t, b, recorder, "➖ Expense")
	clickButton(t, b, recorder, "🍞 Food")
	processUpdate(b, textUpdate("42.50"))
	clickButton(t, b, recorder, "◀ Back")
	assertContains(t, recorder.lastText(t, "/editMessageText"), "Pick a category")

	clickButton(t, b, recorder, "🚗 Travel")
	processUpdate(b, textUpdate("42.50"))
	clickButton(t, b, recorder, "💳 Mono")

	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 1 {
		t.Fatalf("expected one saved expense, got %d", len(txs))
	}
	tx := txs[0]
	if tx.Type != "expense" || tx.Amount != 42.50 || tx.AccountID != account.ID || tx.CategoryID == nil || *tx.CategoryID != travel.ID {
		t.Fatalf("expense saved with wrong values: %+v", tx)
	}
	if tx.CategoryID != nil && *tx.CategoryID == food.ID {
		t.Fatalf("Back did not replace the original category: %+v", tx)
	}
}

func TestMoveBackChangesSourceBeforeSaving(t *testing.T) {
	b, store, recorder := testBotWithRecorder(t)
	mono, _ := store.CreateAccount(nil, 303330553, "Mono", "💳", "EUR", 1000)
	cash, _ := store.CreateAccount(nil, 303330553, "Cash", "💵", "EUR", 500)

	processUpdate(b, textUpdate("/menu"))
	clickButton(t, b, recorder, "🔀 Move")
	clickButton(t, b, recorder, "💳 Mono")
	clickButton(t, b, recorder, "◀ Back")
	assertContains(t, recorder.lastText(t, "/editMessageText"), "From: —")

	clickButton(t, b, recorder, "💵 Cash")
	clickButton(t, b, recorder, "💳 Mono")
	processUpdate(b, textUpdate("125"))

	txs, _ := store.GetRecentTransactions(nil, 303330553, 10)
	if len(txs) != 1 {
		t.Fatalf("expected one transfer, got %d", len(txs))
	}
	tx := txs[0]
	if tx.Type != "transfer" || tx.Amount != 125 || tx.AccountID != cash.ID || tx.TransferAccountID == nil || *tx.TransferAccountID != mono.ID {
		t.Fatalf("transfer saved with wrong values: %+v", tx)
	}
	accounts, _ := store.GetAccounts(nil, 303330553)
	for _, account := range accounts {
		switch account.ID {
		case cash.ID:
			if account.Balance != 375 {
				t.Fatalf("source balance = %.2f, want 375", account.Balance)
			}
		case mono.ID:
			if account.Balance != 1125 {
				t.Fatalf("destination balance = %.2f, want 1125", account.Balance)
			}
		}
	}
}
