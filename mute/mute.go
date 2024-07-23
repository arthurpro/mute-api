package mute

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/teivah/broadcast"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/samber/lo"
	"mute-api/sh"
)

//go:embed mute.ps1
var mutePS1 string

var muteLoopPS1 = `
for (;;) {
    [Audio]::Mute
    Start-Sleep -Seconds 2
}
`

const (
	State = iota
	StateOn
	StateOff
	StateToggle

	StatusActive   = "active"
	StatusInactive = "inactive"
)

type RelayMessage struct {
	Status string
	Sender any
}

func Mute(l echo.Logger, state int) (*bool, error) {
	var command string
	switch state {
	case State:
		command = ""
	case StateOn:
		command = "[Audio]::Mute = $true"
	case StateOff:
		command = "[Audio]::Mute = $false"
	case StateToggle:
		command = "[Audio]::Mute = ![Audio]::Mute"
	}

	err, out, errout := sh.PowershellOutput(strings.Join([]string{mutePS1, command, "[Audio]::Mute"}, "\n"))
	if err != nil {
		l.Error(err)
	}
	if errout != "" {
		l.Error("error:", errout)
	}

	return Bool2State(fmt.Sprintf("%v", strings.TrimSpace(out))), err
}

type MuteStatus struct {
	ChangedFn func(state *bool) bool
	status    string
	state     *bool
}

func (s *MuteStatus) Loop(relay *broadcast.Relay[bool]) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan string)
	go sh.PowershellChan(ctx, ch, strings.Join([]string{mutePS1, muteLoopPS1}, "\n"))

	l := relay.Listener(1)
	for {
		select {
		case msg := <-ch:
			state := Bool2State(msg)
			status := State2Status(state)
			if status != s.status {
				s.status = status
				s.state = state
				go s.ChangedFn(s.state)
			}
		case <-l.Ch():
			cancel()
		}
	}
}

func Bool2State(status string) *bool {
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

func State2Status(state *bool) string {
	var status string
	if state == nil {
		status = "unknown"
	} else {
		switch *state {
		case true:
			status = StatusActive
		case false:
			status = StatusInactive
		}
	}
	return status
}
