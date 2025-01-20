package Provider

type Music struct {
	Id     string // unique identifier for the music
	Title  string
	RawUrl string
	Type   string
}

type Interface interface {
	Start()
	GetByQuery(query string) (Music, error)
}
