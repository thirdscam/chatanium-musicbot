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
				Name:        "query",
				Description: "Enter a query to search",
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

type state struct {
	queue []Provider.Music
	skip  chan bool
}

var musicQueue map[string]state = make(map[string]state)

var providers map[string]Provider.Interface = make(map[string]Provider.Interface)

func Start() {
	Log.Verbose.Println("[MusicBot] Initializing...")

	providers = Provider.GetProviders()

	for k, v := range providers {
		Log.Verbose.Printf("[MusicBot] Starting provider: %s", k)
		v.Start()
	}

	Log.Verbose.Println("[MusicBot] Initialized.")
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
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: util.Str2ptr("Invalid type!"),
		})
		return
	}

	channelID := getJoinedVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: util.Str2ptr("**Failed to join voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again."),
		})
		return
	}

	// Get the music
	m, err := provider.GetByQuery(query)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: util.Str2ptr("**Failed to query music.**\nPlease try again or input another query."),
		})
		return
	}

	// Join the voice channel
	dgv, err := s.ChannelVoiceJoin(i.GuildID, channelID, false, true)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: util.Str2ptr("**Failed to join voice channel.** (maybe not your fault)\nPlease try again."),
		})
		return
	}
	Log.Verbose.Printf("[MusicBot] Joined voice channel: %s", channelID)

	// Download the music
	if err := DownloadMusic(m.RawUrl, MusicID(m.Id)); err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: util.Str2ptr("**Failed to download music.**\nPlease try again."),
		})
		return
	}

	// Add the music to the queue
	if _, ok := musicQueue[channelID]; !ok {
		musicQueue[channelID] = state{queue: []Provider.Music{}}
	}
	musicQueue[channelID] = state{
		queue: append(musicQueue[channelID].queue, m),
		skip:  musicQueue[channelID].skip,
	}

	newRespMsg := new(string)
	*newRespMsg = fmt.Sprintf("**Added to queue!**\n-> **%s**", m.Title)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: newRespMsg,
	})

	// Play the music
	if len(musicQueue[channelID].queue) <= 1 {
		playMusic(s, dgv)
	}
}

func Dequeue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the index of the music to remove
	index := util.Str2Int64(i.ApplicationCommandData().Options[0].StringValue())
	Log.Verbose.Printf("[MusicBot] Dequeue: %d", index)

	if _, ok := musicQueue[i.ChannelID]; !ok {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Invalid index! (Queue Length: %d)", 0),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Check if the index is valid
	if index < 0 || index >= int64(len(musicQueue[i.ChannelID].queue)) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Invalid index! (Queue Length: %d)", len(musicQueue[i.ChannelID].queue)),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Send a message to the channel
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Removing: **#%d** - %s", index, musicQueue[i.ChannelID].queue[index].Title),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	// Remove the music from the queue
	queueState := musicQueue[i.ChannelID]
	queueState.queue = append(queueState.queue[:index], queueState.queue[index+1:]...)
	musicQueue[i.ChannelID] = queueState
}

func Queue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if _, ok := musicQueue[i.ChannelID]; !ok {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Queue is empty!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Check if the queue is empty
	if len(musicQueue[i.ChannelID].queue) == 0 {
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
	for i, music := range musicQueue[i.ChannelID].queue {
		queueMessage += fmt.Sprintf("**#%d** - %s\n", i, music.Title)
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

func playMusic(s *discordgo.Session, dgv *discordgo.VoiceConnection) {
	// Check if the queue is empty
	if len(musicQueue[dgv.ChannelID].queue) == 0 {
		err := dgv.Disconnect()
		if err != nil {
			Log.Warn.Printf("[MusicBot] Failed to disconnect from voice channel: %v", err)
		}

		RemoveStatusEmbed(s, dgv.ChannelID)
		return
	}

	// Set a message to the channel
	SetStatusEmbed(s, dgv.ChannelID, EmbedState{
		Title:        musicQueue[dgv.ChannelID].queue[0].Title,
		ThumbnailUrl: musicQueue[dgv.ChannelID].queue[0].RawUrl,
	})

	// Start playing the music
	Log.Verbose.Printf("[MusicBot] Started!")
	musicId := MusicID(musicQueue[dgv.ChannelID].queue[0].Id)
	PlayMusic(dgv, musicId)
	RemoveMusic(musicId)

	// Remove the first element from the queue
	queueState := musicQueue[dgv.ChannelID]
	queueState.queue = queueState.queue[1:]
	musicQueue[dgv.ChannelID] = queueState

	// Play the next song
	playMusic(s, dgv)
}

func getJoinedVoiceChannel(s *discordgo.Session, guildID, userID string) string {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		Log.Warn.Printf("[MusicBot] Failed to get guild: %v", err)
		return ""
	}

	for _, v := range guild.VoiceStates {
		Log.Verbose.Printf("[MusicBot] VC_STATE => C:%s, U:%s", v.ChannelID, v.UserID)
		if v.UserID != userID {
			continue
		}

		return v.ChannelID
	}

	return ""
}
