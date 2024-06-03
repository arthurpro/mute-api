package main

import (
	_ "embed"
	"strings"

	"github.com/abdfnx/gosh"
	"github.com/labstack/echo/v4"
	"github.com/samber/lo"
)

//go:embed Audio.ps1
var AudioPS1 string

const (
	MuteState = iota
	MuteStateOn
	MuteStateOff
	MuteStateToggle
)

func Mute(l echo.Logger, state int) *bool {
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

	err, out, errout := gosh.PowershellOutput(strings.Join([]string{AudioPS1, command, "[Audio]::Mute"}, "\n"))
	if err != nil {
		l.Error(err)
	}
	if err != nil {
		l.Error("error:", errout)
	}

	status := strings.TrimSpace(out)
	switch status {
	case "True":
		return lo.ToPtr(true)
	case "False":
		return lo.ToPtr(false)
	default:
		return nil
	}
}
