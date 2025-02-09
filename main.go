package main

import (
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
	{
		Name:        "pause",
		Description: "Pause/Resume music",
	}: Pause,
	{
		Name:        "skip",
		Description: "Skip music",
	}: Skip,
}

type state struct {
	queue []Provider.Music
	pause chan bool
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

	channelID := getJoinedVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EditResponse(s, i, "**Failed to join voice channel.**\nPlease rejoin the voice channel and try again. (or you're not in a voice channel)")
		return
	}

	// Get the music
	m, err := provider.GetByQuery(query)
	if err != nil {
		Log.Verbose.Printf("[MusicBot] Failed to query music: %s", err)
		util.EditResponse(s, i, "**Failed to query music.**\nPlease try again or input another query.")
		return
	}

	// Join the voice channel
	dgv, err := s.ChannelVoiceJoin(i.GuildID, channelID, false, true)
	if err != nil {
		Log.Verbose.Printf("[MusicBot] Failed to join voice channel: %s", err)
		util.EditResponse(s, i, "**Failed to join voice channel.**\nPlease try again. (maybe not your fault)")
		return
	}
	Log.Verbose.Printf("[MusicBot] Joined voice channel: %s", channelID)

	// Download the music
	isReady := make(chan bool) // check first music is ready
	go func() {
		for j, v := range m {
			// download the music
			if err := DownloadMusic(v.RawUrl, MusicID(v.Id)); err != nil {
				util.EditResponse(s, i, "**Failed to download music.**\nPlease try again.")
				return
			}
			Log.Verbose.Printf("[MusicBot] (%d/%d) Downloaded music: %s", j+1, len(m), v.Title)

			if j == 0 {
				// if the first music is ready, send the message
				isReady <- true
			}

			time.Sleep(time.Second * 10) // wait 10 seconds (prevent rate limit)
		}
	}()
	<-isReady // wait for the first music to be downloaded

	// Add the music to the queue
	if _, ok := musicQueue[channelID]; !ok {
		musicQueue[channelID] = state{
			queue: []Provider.Music{},
			pause: musicQueue[channelID].pause,
			skip:  musicQueue[channelID].skip,
		}
	}

	queueState := musicQueue[channelID]
	queueState.queue = append(queueState.queue, m...)
	musicQueue[channelID] = queueState

	// Building the response message
	var newRespMsg string
	for j, v := range m {
		if j == 0 {
			newRespMsg = fmt.Sprintf("**Added to queue!**\n-> **%s**", v.Title)
			continue
		}

		newRespMsg += fmt.Sprintf("%s\n-> **%s**", newRespMsg, v.Title)
	}

	// Send the response message
	util.EditResponse(s, i, newRespMsg)

	// Play the music
	if len(musicQueue[channelID].queue) <= 1 {
		playMusic(s, dgv)
	}
}

func Dequeue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the index of the music to remove
	index, err := util.Str2Int64(i.ApplicationCommandData().Options[0].StringValue())
	if err != nil { // if the index is not a number
		util.EphemeralResponse(s, i, "**Invalid index!**\nOnly positive integers are allowed.\n(Did you put something other than a number?)")
		return
	}
	Log.Verbose.Printf("[MusicBot] Dequeue: %d", index)

	if index <= 0 {
		util.EphemeralResponse(s, i, "**Invalid index!**\nOnly positive integers are allowed.\n(The 0th song is the currently playing song, so you should use /skip)")
		return
	}

	channelID := getJoinedVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EditResponse(s, i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	if _, ok := musicQueue[channelID]; !ok {
		util.EphemeralResponse(s, i, fmt.Sprintf("**Invalid index!** (Queue Length: %d)", 0))
		return
	}

	// Check if the index is valid
	if index < 0 || index >= int64(len(musicQueue[channelID].queue)) {
		util.EphemeralResponse(s, i, fmt.Sprintf("**Invalid index!** (Queue Length: %d)", len(musicQueue[channelID].queue)))
		return
	}

	// Send a message to the channel
	util.EphemeralResponse(s, i, fmt.Sprintf("Removing: **#%d** - %s", index, musicQueue[channelID].queue[index].Title))

	// Remove the music from the queue
	queueState := musicQueue[channelID]
	queueState.queue = append(queueState.queue[:index], queueState.queue[index+1:]...)
	musicQueue[channelID] = queueState
}

func Queue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := getJoinedVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EditResponse(s, i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	if _, ok := musicQueue[channelID]; !ok {
		util.EphemeralResponse(s, i, "Queue is empty!")
		return
	}

	// Check if the queue is empty
	if len(musicQueue[channelID].queue) == 0 {
		util.EphemeralResponse(s, i, "Queue is empty!")
		return
	}

	// Create a message to send
	queueMessage := fmt.Sprintf("**Now Playing: %s**\n\nQueue:\n", musicQueue[channelID].queue[0])
	for i, music := range musicQueue[channelID].queue {
		if i == 0 { // if the music is the currently playing music
			continue
		}

		queueMessage += fmt.Sprintf("**#%d** - %s\n", i, music.Title)
	}

	// Send a message to the channel
	util.EphemeralResponse(s, i, queueMessage)
}

func Pause(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := getJoinedVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EphemeralResponse(s, i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	completed := make(chan bool)
	go func() {
		musicQueue[channelID].pause <- true
		completed <- true
	}()

	select {
	case <-time.After(3 * time.Second):
		util.EphemeralResponse(s, i, "**Failed to pause/resume music.**")
		Log.Warn.Println("[MusicBot] Failed to pause/resume music. (channel timeout)")

	case <-completed:
		util.EphemeralResponse(s, i, "**Music paused/resumed.**")
	}
}

func Skip(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := getJoinedVoiceChannel(s, i.GuildID, i.Member.User.ID)
	if channelID == "" {
		util.EphemeralResponse(s, i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	completed := make(chan bool)
	go func() {
		musicQueue[channelID].skip <- true
		completed <- true
	}()

	select {
	case <-time.After(3 * time.Second):
		util.EphemeralResponse(s, i, "**Failed to skip music.**")

	case <-completed:
		util.EphemeralResponse(s, i, "**Music skipped.**")
	}
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

	// Get the music ID
	musicId := MusicID(musicQueue[dgv.ChannelID].queue[0].Id)

	// Get the music queue state
	queueState := musicQueue[dgv.ChannelID]
	queueState.pause = make(chan bool)
	queueState.skip = make(chan bool)
	musicQueue[dgv.ChannelID] = queueState

	// Start playing the music
	Log.Verbose.Printf("[MusicBot] Started!")
	PlayMusic(dgv, musicId, queueState.pause, queueState.skip)

	// Remove the first element from the queue
	queueState.queue = musicQueue[dgv.ChannelID].queue[1:]
	musicQueue[dgv.ChannelID] = queueState

	// Ignore removing songs after scanning if the same song is in the queue
	isDupilcated := false
	for _, each := range musicQueue[dgv.ChannelID].queue {
		if MusicID(each.Id) == musicId {
			isDupilcated = true
			break
		}
	}

	if !isDupilcated {
		RemoveMusic(musicId)
	}

	// Play the next song
	time.Sleep(1 * time.Second)
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
