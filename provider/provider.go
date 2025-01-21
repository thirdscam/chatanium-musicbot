package Provider

type Music struct {
	Id     string // unique identifier for the music
	Title  string
	RawUrl string
	Type   string

	ThumbnailUrl string
	Duration     string
}

type Interface interface {
	Start()
	GetByQuery(query string) (Music, error)
}

func GetProviders() map[string]Interface {
	return map[string]Interface{
		"youtube": &Youtube{},
	}
}
