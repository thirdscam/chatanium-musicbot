package Provider

// MusicID is a unique identifier for a music.
// It is used to download file name as MusicID.
//
// recommended to use hash of the URL as key.
type MusicID string

type Music struct {
	Id     MusicID // unique identifier for the music
	Title  string
	RawUrl string
	Type   string

	ThumbnailUrl string
	Duration     string
}

type Interface interface {
	Start()
	GetMusic(query string) ([]Music, error)
}

func GetProviders() map[string]Interface {
	return map[string]Interface{
		"youtube": &Youtube{},
	}
}
