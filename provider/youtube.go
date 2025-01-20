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
}

func (y *Youtube) GetByQuery(query string) (Music, error) {
	exec := exec.Command(util.GetYtdlpPath(), fmt.Sprintf("ytsearch:'%s'", query), "--skip-download", "--format=bestaudio/best", "-O", "title,thumbnail,url")
	r, err := exec.Output()
	if err != nil {
		return Music{}, err
	}

	result := strings.Split(string(r), "\n")

	return Music{
		Id:     util.GetSha256Hash(result[2]), // TODO: use hash of the Youtube watch URL as key (https://youtube.com/watch?v=dQw4w9WgXcQ)
		Title:  result[0],
		RawUrl: result[2],
		Type:   "youtube",
	}, nil
}
