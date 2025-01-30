package util

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	Url "net/url"
	"os"
	"path"

	"github.com/thirdscam/chatanium/src/Util/Log"
)

func Str2Int64(s string) int64 {
	n := new(big.Int)
	n, ok := n.SetString(s, 10)
	if !ok {
		Log.Error.Printf("[MusicBot] Failed to convert ID: %v", s)
	}
	return n.Int64()
}

func GetYtdlpPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		Log.Error.Printf("[MusicBot] Failed to get home directory: %v", err)
	}

	result := path.Join(homeDir, ".local", "bin", "yt-dlp")
	return result
}

func GetSha256Hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func Str2ptr(s string) *string {
	return &s
}

func IsUrl(url string) bool {
	_, err := Url.ParseRequestURI(url)
	return err == nil
}

func IsYoutubeUrl(url string) bool {
	u, err := Url.ParseRequestURI(url)
	if err != nil {
		return false
	}

	return u.Host == "www.youtube.com" || u.Host == "youtube.com" || u.Host == "youtu.be"
}

func IsYoutubePlaylist(url string) bool {
	u, err := Url.ParseRequestURI(url)
	if err != nil {
		return false
	}

	return u.Host == "www.youtube.com" && u.Path == "/playlist"
}
