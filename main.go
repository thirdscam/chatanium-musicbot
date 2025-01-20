package main

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	Provider "github.com/thirdscam/chatanium-musicbot/provider"
	"github.com/thirdscam/chatanium-musicbot/util"
	"github.com/thirdscam/chatanium/src/Backends/Discord/Interface/Slash"
	"github.com/thirdscam/chatanium/src/Util/Log"
)

var MANIFEST_VERSION = 1

var (
	NAME       = "MusicBot"
	BACKEND    = "discord"
	VERSION    = "0.1.0"
	AUTHOR     = "ANTEGRAL"
	REPOSITORY = "github:thirdscam/chatanium-musicbot"
)

var DEFINE_SLASHCMD = Slash.Commands{
	{
		Name:        "play",
		Description: "Play music",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "provider",
				Description: "Enter a provider of music",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{
						Name:  "youtube",
						Value: "youtube",
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "keyword",
				Description: "Enter a keyword to search",
				Required:    true,
			},
		},
	}: Play,
	{
		Name:        "dequeue",
		Description: "Remove a music from queue",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "index",
				Description: "Enter a index of music",
				Required:    true,
			},
		},
	}: Dequeue,
	{
		Name:        "queue",
		Description: "Show queue",
	}: Queue,
}

var musicQueue map[string][]Provider.Music = make(map[string][]Provider.Music)

func Start() {
	Log.Verbose.Println("[MusicBot] Initializing...")
}

func Play(s *discordgo.Session, i *discordgo.InteractionCreate) {
	Log.Verbose.Printf("[MusicBot] Play command called by %s (C:%s, %s)", i.Member.User.Username, i.ChannelID, i.ApplicationCommandData().Options[1].StringValue())

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "**Adding song to queue...**\nThe first playback of the queue might take a while.",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	// Get the query
	queryType := i.ApplicationCommandData().Options[0].StringValue()
	query := i.ApplicationCommandData().Options[1].StringValue()

	// Get the provider
	var provider Provider.Interface
	switch queryType {
	case "youtube":
		provider = &Provider.Youtube{}
	default:
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Invalid type",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Get the music
	m, err := provider.GetByQuery(query)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Failed to get music",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Join the voice channel
	dgv, err := s.ChannelVoiceJoin(i.GuildID, i.ChannelID, false, true)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Failed to join voice channel. (or you're not in a voice channel)",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	Log.Verbose.Printf("[MusicBot] Joined voice channel: %s", i.ChannelID)

	// Download the music
	if err := DownloadMusic(m.RawUrl, MusicID(m.Id)); err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Failed to download music",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Add the music to the queue
	musicQueue[i.ChannelID] = append(musicQueue[i.ChannelID], m)

	newRespMsg := new(string)
	*newRespMsg = fmt.Sprintf("**Added to queue!**\n-> **%s**", m.Title)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: newRespMsg,
	})

	// Play the music
	if len(musicQueue[i.ChannelID]) <= 1 {
		playMusic(s, dgv, i.ChannelID)
	}
}

func Dequeue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the index of the music to remove
	index := util.Str2Int64(i.ApplicationCommandData().Options[0].StringValue())
	Log.Verbose.Printf("[MusicBot] Dequeue: %d", index)

	// Check if the index is valid
	if index < 0 || index >= int64(len(musicQueue[i.ChannelID])) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Invalid index! (Queue Length: %d)", len(musicQueue[i.ChannelID])),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Send a message to the channel
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Removing: **#%d** - %s", index, musicQueue[i.ChannelID][index].Title),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	// Remove the music from the queue
	musicQueue[i.ChannelID] = append(musicQueue[i.ChannelID][:index], musicQueue[i.ChannelID][index+1:]...)
}

func Queue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if the queue is empty
	if len(musicQueue[i.ChannelID]) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Queue is empty!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Create a message to send
	queueMessage := "Queue:\n"
	for i, music := range musicQueue[i.ChannelID] {
		queueMessage += fmt.Sprintf("**#%d** - %s\n", i+1, music.Title)
	}

	// Send a message to the channel
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: queueMessage,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func playMusic(s *discordgo.Session, dgv *discordgo.VoiceConnection, channel string) {
	// Check if the queue is empty
	if len(musicQueue[channel]) == 0 {
		err := dgv.Disconnect()
		if err != nil {
			Log.Warn.Printf("[MusicBot] Failed to disconnect from voice channel: %v", err)
		}
		return
	}

	// Send a message to the channel
	s.ChannelMessageSend(channel, fmt.Sprintf("Now playing\n-> **%s**", musicQueue[channel][0].Title))

	Log.Verbose.Printf("[MusicBot] Started!")

	// Start playing the music
	id := MusicID(musicQueue[channel][0].Id)
	PlayMusic(dgv, id)
	RemoveMusic(id)

	// Remove the first element from the queue
	musicQueue[channel] = musicQueue[channel][1:]

	// Play the next song
	playMusic(s, dgv, channel)
}
