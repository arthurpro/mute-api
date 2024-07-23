package resources

import _ "embed"

//go:embed mic_off.ico
var micOff []byte

//go:embed mic_on.ico
var micOn []byte

func IconMicOff() []byte {
	return micOff
}

func IconMicOn() []byte {
	return micOn
}
