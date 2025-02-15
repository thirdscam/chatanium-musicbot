package main

import (
	"bufio"
	"errors"
	"io"
	Url "net/url"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jogramming/dca"
	Provider "github.com/thirdscam/chatanium-musicbot/provider"
	"github.com/thirdscam/chatanium/src/Util/Log"
)

const MUSIC_PATH = "./.musicbot"

var ErrSkipped = errors.New("the song is skipped")

// DownloadMusic() downloads the music from the given URL and returns the path to the downloaded file.
//
// key is used to generate the file name, so it must be unique.
func DownloadMusic(rawURL string, musicId Provider.MusicID) error {
	// 1. check if the music file already exists.
	if isExistMusic(musicId) {
		return nil
	}

	// 2. Create a directory to store the music files.
	makeDirectory()

	// 3. Check if the URL is valid
	_, err := Url.ParseRequestURI(rawURL)
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to parse URL: %v", err)
		return err
	}

	// 4. Create a file to store the music file.
	file, err := os.Create(getMusicPath(musicId))
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to create file: %v", err)
		return err
	}

	// 5. Download the music file.
	ok, err := download(rawURL, file)
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to download file: %v", err)
		return err
	}

	// 6. waiting for the download stream to be first buffer written
	<-ok

	return nil
}

// Remove music from the local storage.
//
// use it when the music file is no longer needed.
func RemoveMusic(musicId Provider.MusicID) {
	err := os.Remove(getMusicPath(musicId))
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to remove file: %v", err)
	}
}

// PlayMusic plays the music file to the given voice channel.
//
// It returns an error if the music file is not found.
// so it must be checked before called DownloadMusic().
func PlayMusic(dgv *discordgo.VoiceConnection, musicId Provider.MusicID, pause chan bool, skip chan bool) error {
	// Open the music file
	file, err := os.Open(getMusicPath(musicId))
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to open file: %v", err)
		return err
	}
	defer file.Close()

	// Create a decoder for the audio file
	decoder := dca.NewDecoder(file)

	// Start streaming the audio to the voice connection
	stop := make(chan error)
	stream := dca.NewStream(decoder, dgv, stop)

	// Variable to keep track of the pause state
	isPaused := false

	// Make thread of the music player
	finish := make(chan bool)
	go func() {
		for {
			// Awaiting control signals (pause, stop, etc.)
			select {

			// pause signal actually toggle the pause state (can be paused/resumed)
			case <-pause:
				// Toggle the pause state
				isPaused = !isPaused
				stream.SetPaused(isPaused)
				if isPaused {
					Log.Verbose.Println("[MusicBot] Music paused")
				} else {
					Log.Verbose.Println("[MusicBot] Music resumed")
				}

			// stop signal to stop the music player
			case err := <-stop:
				if errors.Is(err, dca.ErrVoiceConnClosed) {
					Log.Warn.Println("[MusicBot] Voice connection closed. trying to reconnect...")

					// For unknown reasons, the channel's voice connection sometimes closes.
					// However, it will reconnect automatically after a few seconds, so wait about 2 seconds.
					stream.SetPaused(true)
					time.Sleep(2 * time.Second)
					stream.SetPaused(false)
					continue
				}

				if errors.Is(err, io.EOF) {
					Log.Verbose.Println("[MusicBot] Playback finished")
					finish <- true
					return
				}

				if err != nil {
					Log.Error.Printf("[MusicBot] Stream error: %v", err)
					finish <- true
					return
				}

				Log.Warn.Println("[MusicBot] Stop signal received, but error is nil")
				return

			case <-skip:
				stream.SetPaused(true) // Pause the stream
				Log.Verbose.Println("[MusicBot] Playback skipped")
				finish <- true
				return
			}
		}
	}()

	// Wait until streaming is done
	<-finish

	Log.Verbose.Println("[MusicBot] Playback ended.")
	return nil
}

// make directory from MUSIC_PATH.
// if it doesn't exist, create it.
func makeDirectory() error {
	if _, err := os.Stat(MUSIC_PATH); os.IsNotExist(err) {
		err := os.MkdirAll(MUSIC_PATH, 0o755)
		if err != nil {
			Log.Error.Fatalf("[MusicBot] Failed to create directory: %v", err)
		}
	}

	return nil
}

// get music file path from MUSIC_PATH.
func getMusicPath(musicId Provider.MusicID) string {
	return MUSIC_PATH + "/" + string(musicId)
}

// check if the music file exists.
func isExistMusic(musicId Provider.MusicID) bool {
	path := MUSIC_PATH + "/" + string(musicId)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	} else if err != nil {
		Log.Verbose.Printf("[MusicBot] File not found: %v", err)
	}
	return true
}

func download(rawURL string, file *os.File) (chan bool, error) {
	Log.Verbose.Println(rawURL)

	// 1. Get file path
	encodeSession, err := dca.EncodeFile(rawURL, dca.StdEncodeOptions)
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to encode file: %v", err)
		return nil, err
	}

	// 2. Create a buffer to read ffmpeg output and write to file
	reader := bufio.NewReader(encodeSession)
	writer := bufio.NewWriter(file)

	// 3. Start encode session
	isWriting := make(chan bool)
	go func() {
		chunkCnt := 0
		for {
			buf := make([]byte, 4096)
			n, err := reader.Read(buf)
			if err != nil { // session is closed
				file.Close()
				encodeSession.Cleanup()
				if err == io.EOF {
					return
				}
				Log.Verbose.Printf("[MusicBot/Internal] ffmpeg Read Error: %v", err)
				return
			}

			_, err = writer.Write(buf[:n])
			if err != nil {
				Log.Verbose.Printf("[MusicBot/Internal] ffmpeg Write Error: %v", err)
				return
			}

			err = writer.Flush()
			if err != nil {
				Log.Verbose.Printf("[MusicBot/Internal] ffmpeg Flush Error: %v", err)
				return
			}

			// if the first write is done, send the signal to the channel
			if chunkCnt == 5 {
				Log.Verbose.Printf("[MusicBot/Internal] Buffer Received.")
				isWriting <- true
			}

			chunkCnt++
		}
	}()

	return isWriting, nil
}
