package Provider

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lrstanley/go-ytdlp"
	"github.com/thirdscam/chatanium-musicbot/util"
	"github.com/thirdscam/chatanium/src/Util/Log"
)

type Youtube struct{}

func (y *Youtube) Start() {
	ytdlp.MustInstall(context.Background(), nil)
	Log.Info.Println("[MusicBot] yt-dlp installed, starting...")

	beforeVersion, err := exec.Command(util.GetYtdlpPath(), "--version", "--quiet", "--no-warnings").Output()
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to get yt-dlp version: %v", err)
	}

	err = exec.Command(util.GetYtdlpPath(), "-U", "--quiet", "--no-warnings").Run()
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to update yt-dlp: %v", err)
	}

	atferVersion, err := exec.Command(util.GetYtdlpPath(), "--version", "--quiet", "--no-warnings").Output()
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to get yt-dlp version: %v", err)
	}

	if string(beforeVersion) == string(atferVersion) {
		Log.Info.Printf("[MusicBot] yt-dlp is already up to date. (v.%s)", string(beforeVersion)[:len(beforeVersion)-1])
	} else {
		Log.Info.Printf("[MusicBot] yt-dlp updated: %s => %s", string(beforeVersion)[:len(beforeVersion)-1], string(atferVersion)[:len(atferVersion)-1])
	}
}

func (y *Youtube) GetByQuery(query string) ([]Music, error) {
	// check if the query is a playlist or video URL
	if util.IsYoutubeUrl(query) {
		return getUrl(query)
	}

	// else, search for the query
	return getSearch(query)
}

func getSearch(query string) ([]Music, error) {
	exec := exec.Command(util.GetYtdlpPath(), fmt.Sprintf("ytsearch:'%s'", query), "--quiet", "--no-warnings", "--skip-download", "--format=bestaudio/best", "-O", "id,title,url,thumbnail")
	r, err := exec.Output()
	if err != nil {
		return nil, err
	}

	result := strings.Split(string(r), "\n")

	return []Music{
		{
			Id:     result[0],
			Title:  result[1],
			RawUrl: result[2],
			Type:   "youtube",
		},
	}, nil
}

func getUrl(url string) ([]Music, error) {
	exec := exec.Command(util.GetYtdlpPath(), url, "--quiet", "--no-warnings", "--skip-download", "--format=bestaudio/best", "-O", "id,title,url,thumbnail")
	r, err := exec.Output()
	if err != nil {
		return nil, err
	}

	execResult := strings.Split(string(r), "\n")
	result := []Music{}

	for i := 0; i < len(result); i += 3 {
		if i+2 >= len(execResult) {
			Log.Warn.Println("[MusicBot] Failed to parse result partially: result length is not a multiple of 3")
			break
		}
		result = append(result, Music{
			Id:     execResult[i],
			Title:  execResult[i+1],
			RawUrl: execResult[i+2],
			Type:   "youtube",
		})
	}

	return result, nil
}
