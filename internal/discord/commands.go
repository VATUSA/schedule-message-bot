package discord

import "github.com/bwmarrin/discordgo"

// commands defines the slash commands the bot registers with Discord.
var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "schedule",
		Description: "Schedule a message to be sent to a channel at a future time (UTC/Zulu).",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "date",
				Description: "Date in UTC, YYYY-MM-DD (e.g. 2026-02-10)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "time",
				Description: "Time in 24h UTC/Zulu, HH:MM (e.g. 18:30)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "message",
				Description: "The message text to send",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionChannel,
				Name:        "channel",
				Description: "Channel to send the message in",
				Required:    true,
				ChannelTypes: []discordgo.ChannelType{
					discordgo.ChannelTypeGuildText,
					discordgo.ChannelTypeGuildNews,
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "image",
				Description: "Optional direct image URL to include",
				Required:    false,
			},
		},
	},
	{
		Name:        "cancel",
		Description: "Cancel a pending scheduled message by its ID.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "id",
				Description: "The schedule ID to cancel",
				Required:    true,
				MinValue:    ptr(1.0),
			},
		},
	},
	{
		Name:        "list",
		Description: "List all pending scheduled messages.",
	},
}

func ptr[T any](v T) *T { return &v }
