package main

import (
	_ "embed"
	"mute-api/sh"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/samber/lo"
)

//go:embed Audio.ps1
var AudioPS1 string

var AudioMuteLoop = `
for (;;) {
    [Audio]::Mute
    Start-Sleep -Seconds 2
}
`

const (
	MuteState = iota
	MuteStateOn
	MuteStateOff
	MuteStateToggle
)

func Mute(l echo.Logger, state int) (*bool, error) {
	var command string
	switch state {
	case MuteStateOn:
		command = "[Audio]::Mute = $true"
	case MuteStateOff:
		command = "[Audio]::Mute = $false"
	case MuteStateToggle:
		command = "[Audio]::Mute = ![Audio]::Mute"
	default:
	}

	err, out, errout := sh.PowershellOutput(strings.Join([]string{AudioPS1, command, "[Audio]::Mute"}, "\n"))
	if err != nil {
		l.Error(err)
	}
	if err != nil {
		l.Error("error:", errout)
	}

	return muteState(out), err
}

type MuteStatus struct {
	ChangedFn func(state *bool) bool
	status    string
	state     *bool
}

func (s *MuteStatus) Loop() {
	ch := make(chan string)
	go sh.PowershellChan(ch, strings.Join([]string{AudioPS1, AudioMuteLoop}, "\n"))

	for status := range ch {
		state := muteState(status)
		if status != s.status {
			s.status = status
			s.state = state
			s.ChangedFn(s.state)
		}
	}
}

func muteState(status string) *bool {
	status = strings.TrimSpace(status)
	switch status {
	case "True":
		return lo.ToPtr(true)
	case "False":
		return lo.ToPtr(false)
	default:
		return nil
	}
}

func muteStatus(state *bool) string {
	var status string
	if state == nil {
		status = "unknown"
	} else {
		switch *state {
		case true:
			status = "active"
		case false:
			status = "inactive"
		}
	}
	return status
}
