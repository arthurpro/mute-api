package systray

import (
	"github.com/energye/systray"
	"github.com/samber/lo"
	"github.com/skratchdot/open-golang/open"
	"github.com/teivah/broadcast"
	"mute-api/mute"
	"mute-api/resources"
)

var state *bool

const AppName = "mute-api"

func Run(onReady func(), onExit func(), relay *broadcast.Relay[mute.RelayMessage]) {
	systray.Run(func() {
		//systray.SetIcon(resources.IconMicOff())
		systray.SetTitle(AppName)
		systray.SetTooltip(AppName)
		systray.SetOnDClick(func(menu systray.IMenu) {
			var err error
			if state != nil {
				state = lo.ToPtr(!*state)
				setIcon(mute.State2Status(state))
			}
			state, err = mute.Mute(nil, mute.StateToggle)
			if err != nil {
				return
			}
			status := mute.State2Status(state)
			setIcon(status)
			relay.Broadcast(mute.RelayMessage{
				Status: status,
				Sender: menu,
			})
		})
		systray.SetOnRClick(func(menu systray.IMenu) {
			if menu != nil {
				menu.ShowMenu()
			}
		})

		mStatus := systray.AddMenuItem("Status", "")
		mStatus.Click(func() {
			open.Run("http://localhost:3491/status")
		})

		systray.AddSeparator()

		mQuit := systray.AddMenuItem("Quit "+AppName, "Quit "+AppName)
		//mQuit.Enable()
		mQuit.Click(func() {
			systray.Quit()
		})

		go func() {
			l := relay.Listener(1)
			defer l.Close()
			for m := range l.Ch() {
				setIcon(m.Status)
			}
		}()

		onReady()
	}, func() {
		onExit()
	})

}

func setIcon(status string) {
	systray.SetIcon(getStatusIcon(status))
}

func getStatusIcon(status string) []byte {
	switch status {
	case "active":
		return resources.IconMicOff()
	case "inactive":
		return resources.IconMicOn()
	}
	return nil
}
