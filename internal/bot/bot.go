package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/o111oo11o/trip-ledger/internal/model"
	"github.com/o111oo11o/trip-ledger/internal/service"
)

// Bot wraps the Telegram bot API and routes commands to the service layer.
type Bot struct {
	api     *tgbotapi.BotAPI
	service *service.LedgerService
}

// New creates a new Bot.
func New(api *tgbotapi.BotAPI, svc *service.LedgerService) *Bot {
	return &Bot{api: api, service: svc}
}

// Run starts the bot and listens for updates.
func (b *Bot) Run() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		if !update.Message.IsCommand() {
			continue
		}
		go b.handleCommand(update.Message)
	}
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	ctx := context.Background()

	// Ensure group and sender exist.
	group, err := b.service.EnsureGroup(ctx, msg.Chat.ID)
	if err != nil {
		log.Printf("error ensuring group: %v", err)
		b.reply(msg, "Internal error. Please try again.")
		return
	}

	sender, err := b.service.EnsureMember(ctx, group.ID, int64(msg.From.ID), msg.From.UserName, msg.From.FirstName)
	if err != nil {
		log.Printf("error ensuring member: %v", err)
		b.reply(msg, "Internal error. Please try again.")
		return
	}

	switch msg.Command() {
	case "expense":
		b.handleExpense(ctx, msg, group, sender)
	case "split":
		b.handleSplit(ctx, msg, group, sender)
	case "splitexcept":
		b.handleSplitExcept(ctx, msg, group, sender)
	case "partial":
		b.handlePartial(ctx, msg, group, sender)
	case "lend":
		b.handleLend(ctx, msg, group, sender)
	case "repay":
		b.handleRepay(ctx, msg, group, sender)
	case "debts":
		b.handleDebts(ctx, msg, group, sender)
	case "spending":
		b.handleSpending(ctx, msg, group, sender)
	case "setcurrency":
		b.handleSetCurrency(ctx, msg, group)
	case "cancel":
		b.handleCancel(ctx, msg, group)
	case "start":
		b.reply(msg, "Welcome to Trip Ledger! I track shared expenses for your group.\nUse /help to see available commands.")
	case "help":
		b.reply(msg, helpText)
	}
}

// payerAmountCurrencyDateDesc holds the parsed output of parsePayerArgs.
type payerAmountCurrencyDateDesc struct {
	payer       *model.Member
	amountCents int64
	currency    string
	date        time.Time
	description string
}

// parsePayerArgs parses the common argument pattern:
// [@user] amount [currency] [date] description
// used by both /expense and /split.
// Returns (result, consumed_idx, ok). On failure it replies to the message and returns ok=false.
func (b *Bot) parsePayerArgs(ctx context.Context, msg *tgbotapi.Message, group *model.Group, sender *model.Member, args []string) (payerAmountCurrencyDateDesc, bool) {
	var out payerAmountCurrencyDateDesc
	out.payer = sender
	idx := 0

	if idx < len(args) && isUsername(args[idx]) {
		m, err := b.service.ResolveMember(ctx, group.ID, stripAt(args[idx]))
		if err != nil {
			b.reply(msg, fmt.Sprintf("Unknown user: %s", args[idx]))
			return out, false
		}
		out.payer = m
		idx++
	}

	if idx >= len(args) {
		b.reply(msg, "Missing amount.")
		return out, false
	}
	amountCents, err := parseAmount(args[idx])
	if err != nil {
		b.reply(msg, fmt.Sprintf("Invalid amount: %s", args[idx]))
		return out, false
	}
	out.amountCents = amountCents
	idx++

	out.currency = group.DefaultCurrency
	if idx < len(args) && isCurrency(args[idx]) {
		out.currency = strings.ToUpper(args[idx])
		idx++
	}

	out.date = time.Now()
	if idx < len(args) && isDate(args[idx]) {
		d, _ := parseDate(args[idx])
		out.date = d
		idx++
	}

	out.description = strings.Join(args[idx:], " ")
	if out.description == "" {
		b.reply(msg, "Missing description.")
		return out, false
	}

	return out, true
}

// handleExpense: /expense [@user] amount [currency] [date] description
func (b *Bot) handleExpense(ctx context.Context, msg *tgbotapi.Message, group *model.Group, sender *model.Member) {
	args := tokenize(msg.Text)
	if len(args) < 2 {
		b.reply(msg, "Usage: /expense [@user] amount [currency] [date] description")
		return
	}

	p, ok := b.parsePayerArgs(ctx, msg, group, sender, args)
	if !ok {
		return
	}

	tx, err := b.service.RecordExpense(ctx, group.ID, p.payer.ID, p.amountCents, p.currency, p.date, p.description)
	if err != nil {
		log.Printf("error recording expense: %v", err)
		b.reply(msg, "Failed to record expense. Please try again.")
		return
	}

	reply := fmt.Sprintf("Recorded personal expense for %s: %s %s — %s",
		p.payer.DisplayName(), service.FormatMoney(p.amountCents), p.currency, p.description)
	b.replyAndLink(msg, reply, tx.ID)
}

// handleSplit: /split [@user] amount [currency] [date] description
func (b *Bot) handleSplit(ctx context.Context, msg *tgbotapi.Message, group *model.Group, sender *model.Member) {
	args := tokenize(msg.Text)
	if len(args) < 2 {
		b.reply(msg, "Usage: /split [@user] amount [currency] [date] description")
		return
	}

	p, ok := b.parsePayerArgs(ctx, msg, group, sender, args)
	if !ok {
		return
	}

	tx, err := b.service.RecordSplitEqual(ctx, group.ID, p.payer.ID, p.amountCents, p.currency, p.date, p.description)
	if err != nil {
		log.Printf("error recording split: %v", err)
		b.reply(msg, "Failed to record split expense. Please try again.")
		return
	}

	reply := fmt.Sprintf("Split equally: %s paid %s %s — %s",
		p.payer.DisplayName(), service.FormatMoney(p.amountCents), p.currency, p.description)
	b.replyAndLink(msg, reply, tx.ID)
}

// handleSplitExcept: /splitexcept @user1:item_cost [@user2:item_cost ...] amount [currency] description
func (b *Bot) handleSplitExcept(ctx context.Context, msg *tgbotapi.Message, group *model.Group, sender *model.Member) {
	args := tokenize(msg.Text)
	if len(args) < 2 {
		b.reply(msg, "Usage: /splitexcept @user1:item_cost [@user2:item_cost ...] amount [currency] description")
		return
	}

	idx := 0
	var lineItems []service.LineItem

	// Parse @user:amount pairs.
	for idx < len(args) && strings.Contains(args[idx], ":") && isUsername(strings.Split(args[idx], ":")[0]) {
		parts := strings.SplitN(args[idx], ":", 2)
		username := stripAt(parts[0])
		liAmount, err := parseAmount(parts[1])
		if err != nil {
			b.reply(msg, fmt.Sprintf("Invalid amount for @%s: %s", username, parts[1]))
			return
		}
		m, err := b.service.ResolveMember(ctx, group.ID, username)
		if err != nil {
			b.reply(msg, fmt.Sprintf("Unknown user: @%s", username))
			return
		}
		lineItems = append(lineItems, service.LineItem{
			MemberID:    m.ID,
			AmountCents: liAmount,
			Description: fmt.Sprintf("personal item for @%s", username),
		})
		idx++
	}

	if idx >= len(args) {
		b.reply(msg, "Missing total amount.")
		return
	}
	totalCents, err := parseAmount(args[idx])
	if err != nil {
		b.reply(msg, fmt.Sprintf("Invalid amount: %s", args[idx]))
		return
	}
	idx++

	curr, desc, _ := parseCurrencyAndDesc(args, idx, group.DefaultCurrency, "split except")

	tx, err := b.service.RecordSplitExcept(ctx, group.ID, sender.ID, totalCents, curr, desc, lineItems)
	if err != nil {
		log.Printf("error recording split except: %v", err)
		b.reply(msg, "Failed to record expense. Please try again.")
		return
	}

	reply := fmt.Sprintf("Split except: %s paid %s %s — %s",
		sender.DisplayName(), service.FormatMoney(totalCents), curr, desc)
	b.replyAndLink(msg, reply, tx.ID)
}

// handlePartial: /partial @user1 @user2 [...] amount [currency] description
func (b *Bot) handlePartial(ctx context.Context, msg *tgbotapi.Message, group *model.Group, sender *model.Member) {
	args := tokenize(msg.Text)
	if len(args) < 3 {
		b.reply(msg, "Usage: /partial @user1 @user2 [...] amount [currency] description")
		return
	}

	idx := 0
	var participantIDs []int64

	for idx < len(args) && isUsername(args[idx]) {
		m, err := b.service.ResolveMember(ctx, group.ID, stripAt(args[idx]))
		if err != nil {
			b.reply(msg, fmt.Sprintf("Unknown user: %s", args[idx]))
			return
		}
		participantIDs = append(participantIDs, m.ID)
		idx++
	}

	if len(participantIDs) == 0 {
		b.reply(msg, "At least one participant is required.")
		return
	}

	if idx >= len(args) {
		b.reply(msg, "Missing amount.")
		return
	}
	amountCents, err := parseAmount(args[idx])
	if err != nil {
		b.reply(msg, fmt.Sprintf("Invalid amount: %s", args[idx]))
		return
	}
	idx++

	curr, desc, _ := parseCurrencyAndDesc(args, idx, group.DefaultCurrency, "partial split")

	tx, err := b.service.RecordSplitPartial(ctx, group.ID, sender.ID, amountCents, curr, time.Now(), desc, participantIDs)
	if err != nil {
		log.Printf("error recording partial: %v", err)
		b.reply(msg, "Failed to record expense. Please try again.")
		return
	}

	reply := fmt.Sprintf("Partial split: %s paid %s %s — %s",
		sender.DisplayName(), service.FormatMoney(amountCents), curr, desc)
	b.replyAndLink(msg, reply, tx.ID)
}

// resolveTwoUsers resolves @from and @to from the first two args.
// Returns (from, to, ok). On failure it replies to the message and returns ok=false.
func (b *Bot) resolveTwoUsers(ctx context.Context, msg *tgbotapi.Message, group *model.Group, args []string, usage string) (from, to *model.Member, ok bool) {
	if len(args) < 2 || !isUsername(args[0]) || !isUsername(args[1]) {
		b.reply(msg, usage)
		return nil, nil, false
	}
	var err error
	from, err = b.service.ResolveMember(ctx, group.ID, stripAt(args[0]))
	if err != nil {
		b.reply(msg, fmt.Sprintf("Unknown user: %s", args[0]))
		return nil, nil, false
	}
	to, err = b.service.ResolveMember(ctx, group.ID, stripAt(args[1]))
	if err != nil {
		b.reply(msg, fmt.Sprintf("Unknown user: %s", args[1]))
		return nil, nil, false
	}
	return from, to, true
}

// handleLend: /lend @from @to amount [currency] [description]
func (b *Bot) handleLend(ctx context.Context, msg *tgbotapi.Message, group *model.Group, _ *model.Member) {
	const usage = "Usage: /lend @from @to amount [currency] [description]"
	args := tokenize(msg.Text)
	if len(args) < 3 {
		b.reply(msg, usage)
		return
	}

	from, to, ok := b.resolveTwoUsers(ctx, msg, group, args, usage)
	if !ok {
		return
	}

	amountCents, err := parseAmount(args[2])
	if err != nil {
		b.reply(msg, fmt.Sprintf("Invalid amount: %s", args[2]))
		return
	}

	curr, desc, _ := parseCurrencyAndDesc(args, 3, group.DefaultCurrency, "loan")

	tx, err := b.service.RecordLend(ctx, group.ID, from.ID, to.ID, amountCents, curr, desc)
	if err != nil {
		log.Printf("error recording lend: %v", err)
		b.reply(msg, "Failed to record loan. Please try again.")
		return
	}

	reply := fmt.Sprintf("Loan recorded: %s lent %s %s to %s — %s",
		from.DisplayName(), service.FormatMoney(amountCents), curr, to.DisplayName(), desc)
	b.replyAndLink(msg, reply, tx.ID)
}

// handleRepay: /repay @from @to [amount] [currency]
func (b *Bot) handleRepay(ctx context.Context, msg *tgbotapi.Message, group *model.Group, _ *model.Member) {
	const usage = "Usage: /repay @from @to [amount] [currency]"
	args := tokenize(msg.Text)
	if len(args) < 2 {
		b.reply(msg, usage)
		return
	}

	from, to, ok := b.resolveTwoUsers(ctx, msg, group, args, usage)
	if !ok {
		return
	}

	var amountCents int64
	curr := group.DefaultCurrency
	idx := 2
	if idx < len(args) && isAmount(args[idx]) {
		amountCents, _ = parseAmount(args[idx])
		idx++
		if idx < len(args) && isCurrency(args[idx]) {
			curr = strings.ToUpper(args[idx])
		}
	}

	tx, err := b.service.RecordRepay(ctx, group.ID, from.ID, to.ID, amountCents, curr)
	if err != nil {
		b.reply(msg, fmt.Sprintf("Failed to record repayment: %v", err))
		return
	}

	displayAmount := service.FormatMoney(tx.OriginalAmount)
	reply := fmt.Sprintf("Repayment recorded: %s repaid %s %s to %s",
		from.DisplayName(), displayAmount, tx.OriginalCurrency, to.DisplayName())
	b.replyAndLink(msg, reply, tx.ID)
}

// handleDebts: /debts [@user]
func (b *Bot) handleDebts(ctx context.Context, msg *tgbotapi.Message, group *model.Group, sender *model.Member) {
	args := tokenize(msg.Text)

	filterID := &sender.ID
	if len(args) > 0 && isUsername(args[0]) {
		m, err := b.service.ResolveMember(ctx, group.ID, stripAt(args[0]))
		if err != nil {
			b.reply(msg, fmt.Sprintf("Unknown user: %s", args[0]))
			return
		}
		filterID = &m.ID
	}

	debts, err := b.service.GetDebts(ctx, group.ID, filterID)
	if err != nil {
		log.Printf("error getting debts: %v", err)
		b.reply(msg, "Failed to fetch debts.")
		return
	}

	if len(debts) == 0 {
		b.reply(msg, "No outstanding debts!")
		return
	}

	var sb strings.Builder
	sb.WriteString("Outstanding debts:\n")
	for _, d := range debts {
		displayCents, err := b.service.ConvertFromUSD(d.AmountCents, group.DefaultCurrency)
		if err != nil {
			displayCents = d.AmountCents
		}
		sb.WriteString(fmt.Sprintf("  %s owes %s → %s %s\n",
			d.FromMember.DisplayName(),
			d.ToMember.DisplayName(),
			service.FormatMoney(displayCents),
			group.DefaultCurrency))
	}
	b.reply(msg, sb.String())
}

// handleSpending: /spending [@user] [from_date] [to_date] [currency]
func (b *Bot) handleSpending(ctx context.Context, msg *tgbotapi.Message, group *model.Group, sender *model.Member) {
	args := tokenize(msg.Text)
	idx := 0

	member := sender
	if idx < len(args) && isUsername(args[idx]) {
		m, err := b.service.ResolveMember(ctx, group.ID, stripAt(args[idx]))
		if err != nil {
			b.reply(msg, fmt.Sprintf("Unknown user: %s", args[idx]))
			return
		}
		member = m
		idx++
	}

	from := time.Now().AddDate(0, -1, 0) // default: last 30 days
	to := time.Now()
	if idx < len(args) && isDate(args[idx]) {
		d, _ := parseDate(args[idx])
		from = d
		idx++
	}
	if idx < len(args) && isDate(args[idx]) {
		d, _ := parseDate(args[idx])
		to = d
		idx++
	}

	displayCurr := group.DefaultCurrency
	if idx < len(args) && isCurrency(args[idx]) {
		displayCurr = strings.ToUpper(args[idx])
	}

	summary, err := b.service.GetSpending(ctx, group.ID, member.ID, from, to)
	if err != nil {
		log.Printf("error getting spending: %v", err)
		b.reply(msg, "Failed to fetch spending.")
		return
	}

	if len(summary.Entries) == 0 {
		b.reply(msg, fmt.Sprintf("No spending recorded for %s in this period.", member.DisplayName()))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Spending for %s (%s to %s):\n",
		member.DisplayName(), from.Format("02-Jan"), to.Format("02-Jan")))
	for _, e := range summary.Entries {
		displayCents, err := b.service.ConvertFromUSD(e.AmountCents, displayCurr)
		if err != nil {
			displayCents = e.AmountCents
		}
		sb.WriteString(fmt.Sprintf("  %s: %s %s — %s\n",
			e.Date.Format("02-Jan"), service.FormatMoney(displayCents), displayCurr, e.Description))
	}
	totalDisplay, err := b.service.ConvertFromUSD(summary.TotalCents, displayCurr)
	if err != nil {
		totalDisplay = summary.TotalCents
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %s %s", service.FormatMoney(totalDisplay), displayCurr))
	b.reply(msg, sb.String())
}

// handleSetCurrency: /setcurrency currency_code
func (b *Bot) handleSetCurrency(ctx context.Context, msg *tgbotapi.Message, group *model.Group) {
	args := tokenize(msg.Text)
	if len(args) < 1 {
		b.reply(msg, "Usage: /setcurrency USD")
		return
	}
	code := strings.ToUpper(args[0])
	if err := b.service.SetCurrency(ctx, group.ID, code); err != nil {
		b.reply(msg, fmt.Sprintf("Failed to set currency: %v", err))
		return
	}
	b.reply(msg, fmt.Sprintf("Default currency set to %s", code))
}

// handleCancel: /cancel (must be a reply to a transaction confirmation message)
func (b *Bot) handleCancel(ctx context.Context, msg *tgbotapi.Message, group *model.Group) {
	if msg.ReplyToMessage == nil {
		b.reply(msg, "Reply to a transaction confirmation message to cancel it.")
		return
	}

	err := b.service.CancelTransaction(ctx, group.ID, int64(msg.ReplyToMessage.MessageID))
	if err != nil {
		b.reply(msg, fmt.Sprintf("Cannot cancel: %v", err))
		return
	}
	b.reply(msg, "Transaction cancelled.")
}

// reply sends a text message reply.
func (b *Bot) reply(msg *tgbotapi.Message, text string) {
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID
	if _, err := b.api.Send(reply); err != nil {
		log.Printf("error sending reply: %v", err)
	}
}

// replyAndLink sends a reply and links the bot's response message to the transaction.
func (b *Bot) replyAndLink(msg *tgbotapi.Message, text string, txID int64) {
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID
	sent, err := b.api.Send(reply)
	if err != nil {
		log.Printf("error sending reply: %v", err)
		return
	}
	if err := b.service.SetTransactionMessageID(context.Background(), txID, int64(sent.MessageID)); err != nil {
		log.Printf("error linking message to transaction: %v", err)
	}
}

const helpText = `Trip Ledger Commands:

/expense [@user] amount [currency] [date] description
  Record a personal expense

/split [@user] amount [currency] [date] description
  Split equally among all group members

/splitexcept @user1:cost [@user2:cost ...] amount [currency] description
  Split after subtracting personal items

/partial @user1 @user2 [...] amount [currency] description
  Split among specific members only

/lend @from @to amount [currency] [description]
  Record a loan

/repay @from @to [amount] [currency]
  Record a repayment (omit amount for full repay)

/debts [@user]
  Show debt summary

/spending [@user] [from_date] [to_date] [currency]
  Show spending breakdown

/setcurrency code
  Set group default currency (e.g., EUR)

/cancel
  Reply to a transaction message to cancel it`
