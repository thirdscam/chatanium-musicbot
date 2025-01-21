package main

import (
	"github.com/bwmarrin/discordgo"
	"github.com/thirdscam/chatanium/src/Util/Log"
)

type EmbedState struct {
	messageID    string
	Title        string
	ThumbnailUrl string
}

var metadatas = map[string]EmbedState{}

// SendStatusEmbed sends the status embed.
// if the embed is already created, must call SetStatusEmbed.
func SendStatusEmbed(s *discordgo.Session, channelID string, form EmbedState) string {
	embed := &discordgo.MessageEmbed{
		Title:       "Now Playing",
		Description: form.Title,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: form.ThumbnailUrl,
		},
		Color: 0x9f7fed,
	}

	m, err := s.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return ""
	}

	metadatas[m.ID] = EmbedState{
		messageID:    m.ID,
		Title:        form.Title,
		ThumbnailUrl: form.ThumbnailUrl,
	}

	return m.ID
}

// SetStatusEmbed sets the status embed.
// if the embed is not found, it will create a new one.
func SetStatusEmbed(s *discordgo.Session, channelID string, form EmbedState) string {
	embed := &discordgo.MessageEmbed{
		Title:       "Now Playing",
		Description: form.Title,
		Color:       0x9f7fed,
	}

	m, err := s.ChannelMessageEditEmbed(channelID, metadatas[channelID].messageID, embed)
	if err != nil {
		newMessageID := SendStatusEmbed(s, channelID, form)
		metadatas[channelID] = EmbedState{
			messageID:    newMessageID,
			Title:        form.Title,
			ThumbnailUrl: form.ThumbnailUrl,
		}
		return newMessageID
	}

	metadatas[m.ID] = EmbedState{
		messageID:    m.ID,
		Title:        form.Title,
		ThumbnailUrl: form.ThumbnailUrl,
	}

	return m.ID
}

func RemoveStatusEmbed(s *discordgo.Session, channelID string) {
	messageID := metadatas[channelID].messageID

	err := s.ChannelMessageDelete(channelID, messageID)
	if err != nil {
		Log.Warn.Printf("[MusicBot] Failed to remove embed: %v", err)
	}
}
