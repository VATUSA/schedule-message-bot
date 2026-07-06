// Package discord wires the bot's slash commands to Discord and implements
// message delivery for the scheduler.
package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/vatusa/schedule-message-bot/internal/storage"
)

// Bot owns the Discord session and command handlers.
type Bot struct {
	session         *discordgo.Session
	store           *storage.Store
	guildID         string
	requiredRoleIDs map[string]struct{}
	log             *slog.Logger

	registered []*discordgo.ApplicationCommand
}

// New creates a Bot from a token. It does not connect; call Open for that.
func New(token, guildID string, requiredRoleIDs []string, store *storage.Store, log *slog.Logger) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	// Guilds intent is all we need: slash commands arrive as interactions and
	// we only ever send (never read) messages.
	session.Identify.Intents = discordgo.IntentsGuilds

	roleSet := make(map[string]struct{}, len(requiredRoleIDs))
	for _, id := range requiredRoleIDs {
		roleSet[id] = struct{}{}
	}

	b := &Bot{
		session:         session,
		store:           store,
		guildID:         guildID,
		requiredRoleIDs: roleSet,
		log:             log,
	}
	session.AddHandler(b.onInteraction)
	return b, nil
}

// Session exposes the underlying Discord session.
func (b *Bot) Session() *discordgo.Session { return b.session }

// Open connects to Discord and registers the bot's slash commands.
func (b *Bot) Open() error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	b.log.Info("connected to discord", "user", b.session.State.User.String())

	for _, c := range commands {
		created, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, b.guildID, c)
		if err != nil {
			return fmt.Errorf("register command %q: %w", c.Name, err)
		}
		b.registered = append(b.registered, created)
	}
	b.log.Info("registered slash commands", "count", len(b.registered), "guild", b.guildID)
	return nil
}

// Close disconnects from Discord.
func (b *Bot) Close() error {
	return b.session.Close()
}

// Send delivers a scheduled message to its channel. It satisfies
// scheduler.Sender.
func (b *Bot) Send(m storage.ScheduledMessage) error {
	content := m.Content
	if m.ImageURL != "" {
		// Discord automatically embeds a bare image URL on its own line.
		content += "\n" + m.ImageURL
	}
	if _, err := b.session.ChannelMessageSend(m.ChannelID, content); err != nil {
		return fmt.Errorf("send message to channel %s: %w", m.ChannelID, err)
	}
	return nil
}

func (b *Bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if !b.authorized(i) {
		b.respondEphemeral(i, "❌ You do not have permission to use this command.")
		return
	}

	switch i.ApplicationCommandData().Name {
	case "schedule":
		b.handleSchedule(s, i)
	case "cancel":
		b.handleCancel(i)
	case "list":
		b.handleList(i)
	}
}

// authorized reports whether the invoking member holds any of the required
// roles. When no roles are configured, all members are permitted.
func (b *Bot) authorized(i *discordgo.InteractionCreate) bool {
	if len(b.requiredRoleIDs) == 0 {
		return true
	}
	if i.Member == nil {
		return false
	}
	for _, role := range i.Member.Roles {
		if _, ok := b.requiredRoleIDs[role]; ok {
			return true
		}
	}
	return false
}

func (b *Bot) handleSchedule(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i.ApplicationCommandData().Options)

	date := opts["date"].StringValue()
	timeStr := opts["time"].StringValue()
	message := opts["message"].StringValue()
	channel := opts["channel"].ChannelValue(s)

	var imageURL string
	if opt, ok := opts["image"]; ok {
		imageURL = strings.TrimSpace(opt.StringValue())
	}

	sendAt, err := time.Parse("2006-01-02 15:04", date+" "+timeStr)
	if err != nil {
		b.respondEphemeral(i, "❌ Invalid date or time. Use date `YYYY-MM-DD` and time `HH:MM` (UTC).")
		return
	}
	if !sendAt.After(time.Now().UTC()) {
		b.respondEphemeral(i, "❌ That time is in the past. Schedule a time in the future (UTC).")
		return
	}

	id, err := b.store.Create(context.Background(), &storage.ScheduledMessage{
		ChannelID: channel.ID,
		GuildID:   i.GuildID,
		Content:   message,
		ImageURL:  imageURL,
		SendAt:    sendAt,
		CreatedBy: invokerID(i),
	})
	if err != nil {
		b.log.Error("create scheduled message", "error", err)
		b.respondEphemeral(i, "❌ Failed to schedule the message. Please try again.")
		return
	}

	b.respond(i, fmt.Sprintf(
		"✅ Message scheduled for **%s** in <#%s>\n🆔 **Schedule ID:** %d",
		formatZulu(sendAt), channel.ID, id))
}

func (b *Bot) handleCancel(i *discordgo.InteractionCreate) {
	id := i.ApplicationCommandData().Options[0].IntValue()

	cancelled, err := b.store.Cancel(context.Background(), id)
	if err != nil {
		b.log.Error("cancel scheduled message", "id", id, "error", err)
		b.respondEphemeral(i, "❌ Failed to cancel the message. Please try again.")
		return
	}
	if !cancelled {
		b.respondEphemeral(i, fmt.Sprintf("❌ No pending scheduled message found with ID **%d**.", id))
		return
	}
	b.respond(i, fmt.Sprintf("🛑 Scheduled message with ID **%d** has been cancelled.", id))
}

func (b *Bot) handleList(i *discordgo.InteractionCreate) {
	msgs, err := b.store.ListPending(context.Background(), i.GuildID)
	if err != nil {
		b.log.Error("list scheduled messages", "error", err)
		b.respondEphemeral(i, "❌ Failed to list scheduled messages. Please try again.")
		return
	}
	if len(msgs) == 0 {
		b.respondEphemeral(i, "📭 There are no pending scheduled messages.")
		return
	}

	var sb strings.Builder
	sb.WriteString("📅 **Pending Scheduled Messages:**\n\n")
	for _, m := range msgs {
		fmt.Fprintf(&sb, "🆔 **ID:** %d\n", m.ID)
		fmt.Fprintf(&sb, "⏰ **When:** %s\n", formatZulu(m.SendAt))
		fmt.Fprintf(&sb, "📢 **Channel:** <#%s>\n", m.ChannelID)
		fmt.Fprintf(&sb, "📝 **Message:** %s\n", truncate(m.Content, 200))
		if m.ImageURL != "" {
			fmt.Fprintf(&sb, "🖼 **Image:** %s\n", m.ImageURL)
		}
		sb.WriteString("\n")
	}

	b.respondEphemeral(i, truncate(sb.String(), 2000))
}

// optionMap indexes command options by name for convenient lookup.
func optionMap(opts []*discordgo.ApplicationCommandInteractionDataOption) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, o := range opts {
		m[o.Name] = o
	}
	return m
}

func (b *Bot) respond(i *discordgo.InteractionCreate, content string) {
	b.respondWith(i, content, 0)
}

func (b *Bot) respondEphemeral(i *discordgo.InteractionCreate, content string) {
	b.respondWith(i, content, discordgo.MessageFlagsEphemeral)
}

func (b *Bot) respondWith(i *discordgo.InteractionCreate, content string, flags discordgo.MessageFlags) {
	err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   flags,
		},
	})
	if err != nil {
		b.log.Error("respond to interaction", "error", err)
	}
}

// invokerID returns the ID of the user who invoked the interaction, whether in
// a guild (Member) or DM (User) context.
func invokerID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func formatZulu(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04") + "Z"
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	if limit <= 1 {
		return s[:limit]
	}
	return s[:limit-1] + "…"
}
