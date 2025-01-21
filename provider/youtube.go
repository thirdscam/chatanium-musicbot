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

	beforeVersion, err := exec.Command(util.GetYtdlpPath(), "--version").Output()
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to get yt-dlp version: %v", err)
	}

	err = exec.Command(util.GetYtdlpPath(), "-U").Run()
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to update yt-dlp: %v", err)
	}

	atferVersion, err := exec.Command(util.GetYtdlpPath(), "--version").Output()
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to get yt-dlp version: %v", err)
	}

	if string(beforeVersion) == string(atferVersion) {
		Log.Info.Println("[MusicBot] yt-dlp is already up to date.")
	} else {
		Log.Info.Printf("[MusicBot] yt-dlp updated: %s => %s", string(beforeVersion), string(atferVersion))
	}
}

func (y *Youtube) GetByQuery(query string) (Music, error) {
	exec := exec.Command(util.GetYtdlpPath(), fmt.Sprintf("ytsearch:'%s'", query), "--skip-download", "--format=bestaudio/best", "-O", "id,title,url,thumbnail")
	r, err := exec.Output()
	if err != nil {
		return Music{}, err
	}

	result := strings.Split(string(r), "\n")

	return Music{
		Id:     "YT:" + util.GetSha256Hash(result[0]), // TODO: use hash of the Youtube watch URL as key (https://youtube.com/watch?v=dQw4w9WgXcQ)
		Title:  result[1],
		RawUrl: result[2],
		Type:   "youtube",

		ThumbnailUrl: result[3],
	}, nil
}
