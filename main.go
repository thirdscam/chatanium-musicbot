package main

import (
	"errors"
	"fmt"
	"time"

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
		Name:        "remove",
		Description: "Remove a music from playlist",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "index",
				Description: "Enter a index of music",
				Required:    true,
			},
		},
	}: Remove,
	{
		Name:        "list",
		Description: "Show playlist",
	}: List,
	{
		Name:        "pause",
		Description: "Pause/Resume music",
	}: Pause,
	{
		Name:        "skip",
		Description: "Skip music",
	}: Skip,
	{
		Name:        "loop",
		Description: "Loop music",
	}: Loop,
}

// The providers of the music (youtube, etc.)
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
	util.EphemeralResponse(s, i, "**Adding song to queue...**\nIf you enter a playlist, it might take a while for the entire contents to import.\n(The first song will automatically play when it's ready.)")

	// Get the query
	queryType := i.ApplicationCommandData().Options[0].StringValue()
	query := i.ApplicationCommandData().Options[1].StringValue()

	// Get the provider
	var provider Provider.Interface
	switch queryType {
	case "youtube":
		provider = &Provider.Youtube{}
	default:
		util.EditResponse(s, i, "**Invalid provider name!**\nPlease input a valid provider name.")
		return
	}

	channelID := getChannelIdByUser(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EditResponse(s, i, "**Failed to join voice channel.**\nPlease rejoin the voice channel and try again. (or you're not in a voice channel)")
		return
	}

	// Get the music
	m, err := provider.GetMusic(query)
	if err != nil {
		Log.Verbose.Printf("[MusicBot] Failed to query music: %s", err)
		util.EditResponse(s, i, "**Failed to query music.**\nPlease try again or input another query.")
		return
	}

	// Join the voice channel
	dgv, err := s.ChannelVoiceJoin(i.GuildID, string(channelID), false, true)
	if err != nil {
		Log.Verbose.Printf("[MusicBot] Failed to join voice channel: %s", err)
		util.EditResponse(s, i, "**Failed to join voice channel.**\nPlease try again. (maybe not your fault)")
		return
	}
	Log.Verbose.Printf("[MusicBot] Joined voice channel: %s", channelID)

	// Music download thread
	isReady := make(chan bool) // check first music is ready

	go func() {
		// Building the response message
		var respMsg string

		// Download the music from result of the query
		for j, v := range m {
			// Download file and save it
			err := DownloadMusic(v.RawUrl, v.Id)
			if err != nil {
				util.EditResponse(s, i, "**Failed to download music.**\nPlease try again.")
				return
			}

			Log.Verbose.Printf("[MusicBot] (%d/%d) Downloaded music: %s", j+1, len(m), v.Title)
			GetState(channelID).Enqueue(v)

			// Update the response message
			if j == 0 {
				respMsg += fmt.Sprintf("**Added to queue:**\n-> **%s**", v.Title)
				isReady <- true // if the first music is ready, start playing
			} else {
				respMsg += fmt.Sprintf("\n-> %s", v.Title)
			}

			util.EditResponse(s, i, respMsg)

			time.Sleep(time.Second * 10) // wait 10 seconds (prevent rate limit)
		}
	}()
	<-isReady // wait for the first music to be downloaded

	// Play the music
	if !GetState(channelID).IsPlaying() {
		playMusic(s, dgv)
	}
}

func Remove(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the index of the music to remove
	index, err := util.Str2Int64(i.ApplicationCommandData().Options[0].StringValue())
	if err != nil { // if the index is not a number
		util.EphemeralResponse(s, i, "**Invalid index!**\nOnly positive integers are allowed.\n(Did you put something other than a number?)")
		return
	}
	Log.Verbose.Printf("[MusicBot] Dequeue: %d", index)

	// Get the channel ID of the voice channel
	channelID := getChannelIdByUser(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EditResponse(s, i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	music, err := GetState(channelID).Remove(int(index - 1))

	if errors.Is(err, errIndexCannotBeNegative) {
		util.EphemeralResponse(s, i, "**Invalid index!**\nOnly positive integers are allowed.\n(The 0th song is the currently playing song, so you should use /skip)")
		return
	}

	if errors.Is(err, errEmptyQueue) {
		util.EphemeralResponse(s, i, "**Cannot find queue!**\nPlease play a song first.")
		return
	}

	if errors.Is(err, errIndexOutOfRange) {
		util.EphemeralResponse(s, i, "**Invalid index!**\nPlease input a valid index.")
		return
	}

	// If successfully removed, send a message
	util.EphemeralResponse(s, i, fmt.Sprintf("Removing: **#%d** - %s", index, music.Title))
}

func List(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := getChannelIdByUser(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EditResponse(s, i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	queue := GetState(channelID).GetQueue()
	if len(queue) == 0 {
		util.EphemeralResponse(s, i, "**Queue is empty!**\nPlease play a song first.")
		return
	}

	// Create a message to send
	respMsg := fmt.Sprintf("**Now Playing: %s**\n\nQueue:\n", queue[0].Title)
	for i, music := range queue {
		if i == 0 { // if the music is the currently playing music
			continue
		}

		respMsg += fmt.Sprintf("**#%d** - %s\n", i, music.Title)
	}

	// Send a message to the channel
	util.EphemeralResponse(s, i, respMsg)
}

func Pause(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := getChannelIdByUser(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EphemeralResponse(s, i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	err := GetState(channelID).Pause()

	if errors.Is(err, errSignalTimeout) {
		util.EphemeralResponse(s, i, "**Failed to pause/resume music.**\nPlease try again. (If the problem persists, please contact the developer.)")
		Log.Warn.Println("[MusicBot] Failed to pause/resume music. (channel timeout)")
		return
	}

	util.EphemeralResponse(s, i, "**Music paused/resumed.**")
}

func Skip(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := getChannelIdByUser(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EphemeralResponse(s, i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	err := GetState(channelID).Skip()

	if errors.Is(err, errSignalTimeout) {
		util.EphemeralResponse(s, i, "**Failed to skip music.**\nPlease try again. (If the problem persists, please contact the developer.)")
		Log.Warn.Println("[MusicBot] Failed to skip music. (channel timeout)")
		return
	}

	util.EphemeralResponse(s, i, "**Music skipped.**")
}

func Loop(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := getChannelIdByUser(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EphemeralResponse(s, i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	// toggle the loop state
	isLoopMode := GetState(channelID).ToggleLoop()

	// Send a message to the channel
	message := "Loop mode is now off."
	if isLoopMode {
		message = "Loop mode is now on."
	}

	util.EphemeralResponse(s, i, fmt.Sprintf("**Loop mode switched!**\n%s", message))
}

func playMusic(s *discordgo.Session, dgv *discordgo.VoiceConnection) {
	for {
		// Get the state of the channel
		state := GetState(ChannelID(dgv.ChannelID))
		state.SetIsPlaying(true) // set the state to playing

		// Get the first element from the queue
		nowMusic := state.GetFront()

		// Check if the queue is empty
		if state.IsQueueEmpty() {
			state.SetIsPlaying(false)
			err := dgv.Disconnect()
			if err != nil {
				Log.Warn.Printf("[MusicBot] Failed to disconnect from voice channel: %v", err)
			}

			RemoveStatusEmbed(s, dgv.ChannelID)
			return
		}

		// Set a message to the channel
		SetStatusEmbed(s, dgv.ChannelID, EmbedState{
			Title:        nowMusic.Title,
			ThumbnailUrl: nowMusic.RawUrl,
		})

		// Start playing the music
		Log.Info.Printf("[MusicBot] Playing music: %s", nowMusic.Title)
		PlayMusic(dgv, nowMusic.Id, state.pause, state.skip)

		util.WithRLock(&state.RWMutex, func() {
			// Remove the first element from the queue
			front := state.Pop()

			if state.loop {
				// Add the first element to the end of the queue
				state.queue = append(state.queue, front)
			}

			// Ignore removing songs after scanning if the same song is in the queue
			isDupilcated := false
			for _, m := range state.queue {
				if m.Id == front.Id {
					isDupilcated = true
				}
			}

			// if the same song is not in the queue, remove the music.
			// also if loop mode is on, the same song will be at the end of the queue, so it won't be removed.
			if !isDupilcated {
				RemoveMusic(nowMusic.Id)
			}
		})

		// Play the next song
		time.Sleep(1 * time.Second)
	}
}

func getChannelIdByUser(s *discordgo.Session, guildID, userID string) ChannelID {
	// Get the voice state of the user
	guild, err := s.State.Guild(guildID)
	if err != nil {
		Log.Warn.Printf("[MusicBot] Failed to get guild: %v", err)
		return ""
	}

	// loop through the voice states to find the user's voice state
	for _, v := range guild.VoiceStates {
		if v.UserID == userID {
			// Return the channel ID if the user is in a voice channel
			Log.Verbose.Printf("[MusicBot] VC_STATE_HIT => C:%s, U:%s", v.ChannelID, v.UserID)
			return ChannelID(v.ChannelID)
		}

		Log.Verbose.Printf("[MusicBot] VC_STATE_MISS => C:%s, U:%s", v.ChannelID, v.UserID)
	}

	// if the user is not in a voice channel
	return ""
}
