package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/micmonay/keybd_event"
)

var keybdEvent keybd_event.KeyBonding

func init() {

	var err error
	keybdEvent, err = keybd_event.NewKeyBonding()
	if err != nil {
		panic(err)
	}
}

var currentMuteStatus string

func main() {
	ws := echo.New()
	ws.HideBanner, ws.HidePort = true, true
	ws.Logger.SetLevel(log.INFO)
	ws.Use(middleware.Logger())
	ws.Use(middleware.Recover())
	wsh := new(WebsocketHandler)
	wsStatusCh := make(chan string)
	wsh.StatusChannel = wsStatusCh
	ws.GET("/", wsh.Handle)

	ms := new(MuteStatus)
	ms.ChangedFn = func(state *bool) bool {
		currentMuteStatus = muteStatus(state)
		wsStatusCh <- currentMuteStatus
		return true
	}
	go ms.Loop()

	// Start websocket server
	go ws.Logger.Fatal(ws.Start(":3492"))
	fmt.Println("websocket server started on [::]:3492")

	e := initEcho()
	e.Logger.SetLevel(log.INFO)
	// Routes
	e.GET("/status", new(Action).StatusAction)
	e.POST("/mute", new(Action).MuteAction)
	e.POST("/toggle_mute", new(Action).ToggleMuteAction)
	e.POST("/leave", new(Action).LeaveAction)
	// Start http server
	e.Logger.Fatal(e.Start(":3491"))
}

func initEcho() *echo.Echo {
	// Echo instance
	e := echo.New()
	e.HideBanner, e.HidePort = true, true

	e.IPExtractor = echo.ExtractIPFromXFFHeader()

	// Middleware
	//limiterConfig := middleware.DefaultRateLimiterConfig
	//limiterConfig.Store = middleware.NewRateLimiterMemoryStoreWithConfig(
	//	middleware.RateLimiterMemoryStoreConfig{
	//		Rate:      rate.Every(1 * time.Minute / 30),
	//		Burst:     6,
	//		ExpiresIn: time.Second * 10,
	//	})
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Pre(middleware.Rewrite(map[string]string{
		"/v1/*": "/$1",
	}))
	e.Use(middleware.CORS())
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Skipper:      middleware.DefaultSkipper,
		ErrorMessage: "timeout",
		OnTimeoutRouteErrorHandler: func(err error, c echo.Context) {
			c.Logger().Errorf("%s %s, request_id %s", err, c.Path(), c.Response().Header().Get(echo.HeaderXRequestID))
		},
		Timeout: 3 * time.Second,
	}))
	e.Use(middleware.RequestID())
	//e.Use(middleware.RateLimiterWithConfig(limiterConfig))
	return e
}

type WebsocketMessage struct {
	Source string `json:"source"`
	Action string `json:"action"`
	Mute   string `json:"mute,omitempty"`
}

type WebsocketHandler struct {
	StatusChannel chan string
	ctx           echo.Context
}

var wsUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
	return true
}}

func (h *WebsocketHandler) Handle(c echo.Context) error {
	h.ctx = c

	ws, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	go func() {
		for status := range h.StatusChannel {
			outgoing := WebsocketMessage{
				Source: "api",
				Action: "update-status",
				Mute:   status,
			}
			msg, _ := json.Marshal(outgoing)
			h.Log("outgoing: ", string(msg))
			err := ws.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				c.Logger().Error(err)
			}
		}
	}()

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			c.Logger().Error(err)
			continue
		}
		h.Log("incoming: ", string(msg))
		incoming := &WebsocketMessage{}
		json.Unmarshal(msg, incoming)
		switch incoming.Action {
		case "identify":
			continue
		case "toggle_mute":
			state, err := Mute(c.Logger(), MuteStateToggle)
			if err != nil {
				c.Logger().Error(err)
			} else {
				outgoing := WebsocketMessage{
					Source: "api",
					Action: "update-status",
					Mute:   muteStatus(state),
				}
				msg, _ := json.Marshal(outgoing)
				h.Log("outgoing: ", string(msg))
				err := ws.WriteMessage(websocket.TextMessage, msg)
				if err != nil {
					c.Logger().Error(err)
				}
			}
		}
	}
}

func (h *WebsocketHandler) Log(m ...any) {
	h.ctx.Logger().Info(m...)
}

// Actions

type Action struct {
	Control string `json:"control,omitempty"`
	Status  int    `json:"status,omitempty"`
	Mute    string `json:"mute,omitempty"`
	Error   string `json:"error,omitempty"`
	//Call    string `json:"call,omitempty"`
	//Record  string `json:"record,omitempty"`
	//Share   string `json:"share,omitempty"`
}

func (a *Action) StatusAction(c echo.Context) error {
	mute, err := Mute(c.Logger(), MuteState)
	status := muteStatus(mute)
	a.Mute = status
	a.Control = "system"
	a.Status = http.StatusOK
	if err != nil {
		a.Status = http.StatusInternalServerError
		a.Error = err.Error()
	}
	return a.Handle(c)
}

func (a *Action) MuteAction(c echo.Context) error {
	app := c.QueryParam("app")
	state := c.QueryParam("state")
	a.Status = http.StatusOK

	var err error
	switch state {
	case "on":
		_, err = Mute(c.Logger(), MuteStateOn)
	case "off":
		_, err = Mute(c.Logger(), MuteStateOff)
	case "toggle":
		switch app {
		case "slack":
			keybdEvent.SetKeys(keybd_event.VK_SPACE)
			keybdEvent.HasSHIFT(true)
			keybdEvent.HasCTRL(true)
			err = keybdEvent.Launching()
		default:
			_, err = Mute(c.Logger(), MuteStateToggle)
		}
	default:
		a.Status = http.StatusBadRequest
	}

	if err != nil {
		a.Status = http.StatusInternalServerError
		a.Error = err.Error()
	}
	return a.Handle(c)
}

func (a *Action) ToggleMuteAction(c echo.Context) error {
	a.Status = http.StatusOK

	_, err := Mute(c.Logger(), MuteStateToggle)
	if err != nil {
		a.Status = http.StatusInternalServerError
		a.Error = err.Error()
	}
	return a.Handle(c)
}

func (a *Action) LeaveAction(c echo.Context) error {
	app := c.QueryParam("app")
	a.Status = http.StatusOK

	var err error
	switch app {
	case "slack":
		keybdEvent.SetKeys(keybd_event.VK_H)
		keybdEvent.HasSHIFT(true)
		keybdEvent.HasCTRL(true)
		err = keybdEvent.Launching()
	default:
		a.Status = http.StatusBadRequest
	}

	if err != nil {
		a.Status = http.StatusInternalServerError
		a.Error = err.Error()
	}
	return a.Handle(c)
}

func (a *Action) Handle(c echo.Context) error {
	return c.JSONPretty(a.Status, a, "  ")
}
