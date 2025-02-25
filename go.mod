module github.com/thirdscam/chatanium-musicbot

go 1.23.2

require github.com/bwmarrin/discordgo v0.28.1 // direct

require (
	github.com/jogramming/dca v0.0.0-20210930103944-155f5e5f0cc7
	github.com/lrstanley/go-ytdlp v0.0.0-20250219030852-4f99aecdc40c
	github.com/thirdscam/chatanium v1.0.0-local
)

require (
	github.com/ProtonMail/go-crypto v1.1.5 // indirect
	github.com/cloudflare/circl v1.6.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jonas747/ogg v0.0.0-20161220051205-b4f6f4cf3757 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/crypto v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
)

// The runtime API will be received as a relative path via symlink.
// So you'll need to work from a path of the form <RUNTIME_PATH>/<MODULE>
//
// For example: ~/<SOME_FOLDER>/chatanium/modules/MusicBot
replace github.com/thirdscam/chatanium v1.0.0-local => ./../..
