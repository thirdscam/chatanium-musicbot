package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path"
	"strings"

	"antegr.al/chatanium-bot/v1/src/Backends/Discord/Interface/Slash"
	"antegr.al/chatanium-bot/v1/src/Util/Log"
	"github.com/bwmarrin/discordgo"
	"github.com/lrstanley/go-ytdlp"
)

var MANIFEST_VERSION = 1

var (
	NAME       = "MusicBot"
	BACKEND    = "discord"
	VERSION    = "0.0.1"
	AUTHOR     = "ANTEGRAL"
	REPOSITORY = "github:thirdscam/chatanium"
)

var DEFINE_SLASHCMD = Slash.Commands{
	{
		Name:        "play",
		Description: "Play music",
		Options: []*discordgo.ApplicationCommandOption{
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

type music struct {
	Title string
	Type  string
	URL   string
}

var musicQueue map[string][]music = make(map[string][]music)

func Start() {
	Log.Verbose.Println("[MusicBot] Initializing...")
	ytdlp.MustInstall(context.Background(), nil)
	Log.Info.Println("[MusicBot] yt-dlp installed, starting...")
}

func Play(s *discordgo.Session, i *discordgo.InteractionCreate) {
	Log.Verbose.Printf("[MusicBot] Play command called by %s (C:%s, %s)", i.Member.User.Username, i.ChannelID, i.ApplicationCommandData().Options[0].StringValue())

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "**Adding song to queue...**\nThe first playback of the queue might take a while.",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	// m := getMusicByQuery(i.ApplicationCommandData().Options[0].StringValue())
	// musicQueue[i.ChannelID] = append(musicQueue[i.ChannelID], m)
	musicQueue[i.ChannelID] = append(musicQueue[i.ChannelID], getLocalTestSet()[0])

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

	if len(musicQueue[i.ChannelID]) <= 1 {
		PlayMusic(s, dgv, i.ChannelID)
	}
}

func Dequeue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the index of the music to remove
	index := str2Int64(i.ApplicationCommandData().Options[0].StringValue())
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

func PlayMusic(s *discordgo.Session, dgv *discordgo.VoiceConnection, channel string) {
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
	ch := make(chan bool)
	PlayAudioFile(dgv, musicQueue[channel][0].URL, ch)

	Log.Verbose.Printf("[MusicBot] End!")

	// Remove the first element from the queue
	musicQueue[channel] = musicQueue[channel][1:]

	// Play the next song
	PlayMusic(s, dgv, channel)
}

func getMusicByQuery(query string) music {
	// Search for the video
	exec := exec.Command(getYtdlpPath(), fmt.Sprintf("ytsearch:'%s audio'", query), "--skip-download", "--format=bestaudio/best", "-O", "title,thumbnail,url")
	r, err := exec.Output()
	if err != nil {
		Log.Warn.Printf("[MusicBot] Failed to search video: %v", err)
	}

	Log.Verbose.Printf("[MusicBot] Result:\n%s", r)

	result := strings.Split(string(r), "\n")

	Log.Verbose.Printf("[MusicBot] %s", result)

	return music{
		Title: result[0],
		Type:  "youtube",
		URL:   result[2],
	}
}

func str2Int64(s string) int64 {
	n := new(big.Int)
	n, ok := n.SetString(s, 10)
	if !ok {
		Log.Error.Printf("[MusicBot] Failed to convert ID: %v", s)
	}
	return n.Int64()
}

func getLocalTestSet() []music {
	// m := music{
	// 	Title: "[TEST] Dua lipa - Physical",
	// 	Type:  "youtube",
	// 	URL:   "./test.weba",
	// }
	// m := music{
	// 	Title: "[TEST] Dua lipa - Levitating",
	// 	Type:  "youtube",
	// 	URL:   "./test2.weba",
	// }

	return []music{
		{
			Title: "[TEST] OneRepublic - Serotonin",
			Type:  "youtube",
			URL:   "./test4.weba",
		},
		{
			Title: "[TEST] Dua lipa - Levitating",
			Type:  "youtube",
			URL:   "./test2.weba",
		},
		{
			Title: "[TEST] Marshmello ft. Bastille - Happier",
			Type:  "youtube",
			URL:   "./test3.weba",
		},
		{
			Title: "[TEST] Dua lipa - Physical",
			Type:  "youtube",
			URL:   "./test1.weba",
		},
	}
}

func getYtdlpPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to get home directory: %v", err)
	}

	result := path.Join(homeDir, ".local", "bin", "yt-dlp")
	return result
}

// func getMusicUrlByUrl(url string) music {
// 	Log.Verbose.Printf("[MusicBot] Getting info: %s", url)

// 	// Download the file
// 	dl := ytdlp.New().SkipDownload().Format("bestaudio/best").Print("url")
// 	r, err := dl.Run(context.TODO(), url)
// 	if err != nil {
// 		Log.Warn.Printf("[MusicBot] Failed to extract URL: %v", err)
// 	}

// 	Log.Verbose.Printf("[MusicBot] %s => %s", url, r.Stdout)
// 	return music{
// 		Title: "asdffff",
// 		Type:  "youtube",
// 		URL:   r.Stdout,
// 	}
// }

// func getMD5Hash(text string) string {
// 	hasher := md5.New()
// 	hasher.Write([]byte(text))
// 	return hex.EncodeToString(hasher.Sum(nil))
// }

// func mkdirMusic() string {
// 	path := "./.musicbot"
// 	// Check if the directory exists

// 	if _, err := os.Stat(path); os.IsNotExist(err) {
// 		// Create the directory
// 		err := os.MkdirAll(path, 0o755)
// 		if err != nil {
// 			Log.Error.Fatalf("[MusicBot] Failed to create directory: %v", err)
// 		}
// 	}

// 	// Get folder full path
// 	path, err := filepath.Abs(path)
// 	if err != nil {
// 		Log.Error.Fatalf("[MusicBot] Failed to get path: %v", err)
// 	}

// 	return path
// }
