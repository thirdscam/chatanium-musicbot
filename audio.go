package main

import (
	"bufio"
	"io"
	Url "net/url"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/jogramming/dca"
	"github.com/thirdscam/chatanium/src/Util/Log"
)

const MUSIC_PATH = "./.musicbot"

// MusicID is a unique identifier for a music.
// It is used to download file name as MusicID.
//
// recommended to use hash of the URL as key.
type MusicID string

// DownloadMusic() downloads the music from the given URL and returns the path to the downloaded file.
//
// key is used to generate the file name, so it must be unique.
func DownloadMusic(rawURL string, musicId MusicID) error {
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
func RemoveMusic(musicId MusicID) {
	err := os.Remove(getMusicPath(musicId))
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to remove file: %v", err)
	}
}

// PlayMusic plays the music file to the given voice channel.
//
// It returns an error if the music file is not found.
// so it must be checked before called DownloadMusic().
func PlayMusic(dgv *discordgo.VoiceConnection, musicId MusicID) error {
	// Get file stream
	file, err := os.Open(getMusicPath(musicId))
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to open file: %v", err)
		return err
	}
	defer file.Close()

	// Get opus audio from the file
	decoder := dca.NewDecoder(file)

	// Play the music
	done := make(chan error)
	dca.NewStream(decoder, dgv, done)
	err = <-done
	if err != nil && err != io.EOF {
		Log.Error.Printf("[MusicBot] Failed to play music: %v", err)
	}

	Log.Verbose.Printf("[MusicBot] End!")
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
func getMusicPath(musicId MusicID) string {
	return MUSIC_PATH + "/" + string(musicId)
}

// check if the music file exists.
func isExistMusic(musicId MusicID) bool {
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

// func download(file *os.File, url string) (chan bool, error) {
// 	// 1. Create a shell command "object" to run.
// 	run := exec.Command(
// 		"ffmpeg",
// 		"-i", url,
// 		"-f", "s16le",
// 		"-reconnect", "1",
// 		"-reconnect_at_eof", "1",
// 		"-reconnect_streamed", "1",
// 		"-reconnect_delay_max", "3",
// 		"-vn",
// 		"-ar", strconv.Itoa(frameRate),
// 		"-ac", strconv.Itoa(channels),
// 		"-blocksize", "8192",
// 		"pipe:1")
// 	ffmpegout, err := run.StdoutPipe()
// 	if err != nil {
// 		Log.Verbose.Printf("[MusicBot/Internal] StdoutPipe Error: %v", err)
// 		return nil, err
// 	}

// 	// 1-1. prevent memory leak from residual ffmpeg streams
// 	defer run.Process.Kill()

// 	// 2. create buffer to read ffmpeg output and write to file
// 	ffmpegReader := bufio.NewReaderSize(ffmpegout, 16384)
// 	fileWriter := bufio.NewWriter(file)

// 	// 3. Starts the ffmpeg command
// 	err = run.Start()
// 	if err != nil {
// 		Log.Verbose.Printf("[MusicBot/Internal] ffmpeg Start Error: %v", err)
// 		return nil, err
// 	}

// 	isWriting := make(chan bool)

// 	// 4. read ffmpeg buffer and write to file
// 	go func() {
// 		isFirstWritten := false
// 		for {
// 			buf := make([]byte, 4096)
// 			n, err := ffmpegReader.Read(buf)
// 			if err != nil {
// 				if err == io.EOF {
// 					return
// 				}
// 				Log.Verbose.Printf("[MusicBot/Internal] ffmpeg Read Error: %v", err)
// 				return
// 			}
// 			_, err = fileWriter.Write(buf[:n])
// 			if err != nil {
// 				Log.Verbose.Printf("[MusicBot/Internal] ffmpeg Write Error: %v", err)
// 				return
// 			}

// 			// if the first write is done, send the signal to the channel
// 			if !isFirstWritten {
// 				isWriting <- true
// 				isFirstWritten = true
// 			}
// 		}
// 	}()

// 	return isWriting, nil
// }
