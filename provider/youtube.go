package Provider

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

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
		Log.Info.Printf("[MusicBot] yt-dlp is already up to date. (v.%s)", strings.TrimSuffix(string(beforeVersion), "\n"))
	} else {
		Log.Info.Printf("[MusicBot] yt-dlp updated: %s => %s", strings.TrimSuffix(string(beforeVersion), "\n"), strings.TrimSuffix(string(atferVersion), "\n"))
	}

	go func() {
		for {
			// update yt-dlp every 12 hours
			time.Sleep(12 * time.Hour)
			err = exec.Command(util.GetYtdlpPath(), "-U", "--quiet", "--no-warnings").Run()
			if err != nil {
				Log.Error.Printf("[MusicBot] Failed to update yt-dlp: %v", err)
			}

			atferVersion, err = exec.Command(util.GetYtdlpPath(), "--version", "--quiet", "--no-warnings").Output()
			if err != nil {
				Log.Error.Printf("[MusicBot] Failed to get yt-dlp version: %v", err)
			}

			if string(beforeVersion) == string(atferVersion) {
				Log.Info.Printf("[MusicBot] yt-dlp auto-updated: %s => %s", strings.TrimSuffix(string(beforeVersion), "\n"), strings.TrimSuffix(string(atferVersion), "\n"))
				beforeVersion = atferVersion // overwrite the version
			} else {
				Log.Verbose.Printf("[MusicBot] yt-dlp is already up to date. (v.%s)", strings.TrimSuffix(string(atferVersion), "\n"))
			}
		}
	}()
}

func (y *Youtube) GetByQuery(query string) ([]Music, error) {
	// check if the query is a playlist or video URL
	if util.IsYoutubeUrl(query) {
		Log.Verbose.Printf("[MusicBot] Query is a URL: %s", query)
		return getUrl(query)
	}

	// else, search for the query
	Log.Verbose.Printf("[MusicBot] Query is a search: %s", query)
	return getSearch(query)
}

func getSearch(query string) ([]Music, error) {
	exec := exec.Command(util.GetYtdlpPath(), fmt.Sprintf("ytsearch:'%s (Lyrics)'", query), "--quiet", "--no-warnings", "--skip-download", "--format=bestaudio/best", "-O", "id,title,url,thumbnail")
	r, err := exec.Output()
	if err != nil {
		return nil, err
	}

	result := strings.Split(string(r), "\n")
	if len(result) < 4 {
		return nil, fmt.Errorf("no results found or invalid query (len() < 4)")
	}

	isVaildUrl := util.IsUrl(result[2]) && util.IsUrl(result[3])
	if !isVaildUrl {
		return nil, fmt.Errorf("invalid query response (invaild url)")
	}

	return []Music{
		{
			Id:           "YT:" + util.GetSha256Hash(result[0]),
			Title:        result[1],
			RawUrl:       result[2],
			ThumbnailUrl: result[3],
			Type:         "youtube",
		},
	}, nil
}

func getUrl(url string) ([]Music, error) {
	exec := exec.Command(util.GetYtdlpPath(), url, "--quiet", "--no-warnings", "--skip-download", "--format=bestaudio/best", "-O", "id,title,url,thumbnail")
	r, err := exec.Output()
	if err != nil {
		return nil, err
	}

	execResult := strings.Split(strings.TrimSuffix(string(r), "\n"), "\n")
	result := []Music{}

	if len(execResult) < 4 {
		return nil, fmt.Errorf("no results found or invalid query (len() < 4)")
	}

	for i := 0; i < len(execResult); i += 4 {
		if execResult[i] == "" { // last line (N+1) is always empty
			break
		}

		isVaildUrl := util.IsUrl(execResult[i+2]) && util.IsUrl(execResult[i+3])
		if !isVaildUrl {
			return nil, fmt.Errorf("invalid query response (invaild url)")
		}

		if i+3 > len(execResult) {
			Log.Verbose.Println("[MusicBot] Failed to parse result partially: result length is not a multiple of 4")
			break
		}

		result = append(result, Music{
			Id:           "YT:" + util.GetSha256Hash(execResult[i]),
			Title:        execResult[i+1],
			RawUrl:       execResult[i+2],
			ThumbnailUrl: execResult[i+3],
			Type:         "youtube",
		})
	}

	if len(result) == 0 {
		Log.Verbose.Println("[MusicBot] cannot find result")
	}

	return result, nil
}
