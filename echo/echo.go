package echo

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/teivah/broadcast"
	"mute-api/mute"
)

func Init(
	logWriter io.Writer,
	exitRelay *broadcast.Relay[bool],
	muteRelay *broadcast.Relay[mute.RelayMessage],
) {
	ws := echo.New()
	ws.HideBanner, ws.HidePort = true, false
	ws.Logger.SetLevel(log.INFO) // log.OFF to disable
	ws.Logger.SetOutput(logWriter)
	ws.Use(middleware.Logger())
	ws.Use(middleware.Recover())
	wsh := NewWebsocketHandler(muteRelay)
	ws.GET("/", wsh.Handle)

	e := initEcho()
	e.Logger.SetLevel(log.INFO)
	e.Logger.SetOutput(logWriter)
	ms := new(mute.MuteStatus)
	ms.ChangedFn = func(state *bool) bool {
		muteRelay.Broadcast(mute.RelayMessage{
			Status: mute.State2Status(state),
			Sender: ms,
		})
		return true
	}
	go ms.Loop(exitRelay)

	// Start websocket server
	go func() {
		ws.Logger.Fatal(ws.Start(":3492"))
	}()

	// Routes
	e.GET("/status", new(Action).StatusAction)
	e.POST("/mute", new(Action).MuteAction)
	e.POST("/toggle_mute", new(Action).ToggleMuteAction)
	e.POST("/leave", new(Action).LeaveAction)

	// Start http server
	go func() {
		e.Logger.Fatal(e.Start(":3491"))
	}()

	fmt.Println("⇨ echo initialization complete")
}

func initEcho() *echo.Echo {
	// Echo instance
	e := echo.New()
	e.HideBanner, e.HidePort = true, false

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
	relay *broadcast.Relay[mute.RelayMessage]
	ctx   echo.Context
}

func NewWebsocketHandler(relay *broadcast.Relay[mute.RelayMessage]) *WebsocketHandler {
	h := &WebsocketHandler{
		relay: relay,
	}
	return h
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

	relay := broadcast.NewRelay[[]byte]()
	defer relay.Close()
	go func() {
		l := relay.Listener(1)
		defer l.Close()

		for msg := range l.Ch() {
			func() {
				defer func() {
					if r := recover(); r != nil {
						h.LogErrorf("recovering: %v", r)
					}
				}()
				h.Log("outgoing: ", string(msg))
				err := ws.WriteMessage(websocket.TextMessage, msg)
				if err != nil {
					h.LogError(err)
				}
			}()
		}
	}()

	go func() {
		l := h.relay.Listener(1)
		defer l.Close()
		for m := range l.Ch() {
			if m.Sender != h {
				outgoing := WebsocketMessage{
					Source: "api",
					Action: "update-status",
					Mute:   m.Status,
				}
				msg, _ := json.Marshal(outgoing)
				relay.Notify(msg)
			}
		}
	}()

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			h.LogError(err)
			continue
		}
		h.Log("incoming: ", string(msg))
		incoming := &WebsocketMessage{}
		json.Unmarshal(msg, incoming)
		switch incoming.Action {
		case "identify":
			continue
		case "toggle_mute":
			state, err := mute.Mute(c.Logger(), mute.StateToggle)
			if err != nil {
				h.LogError(err)
			} else {
				status := mute.State2Status(state)
				h.relay.Broadcast(mute.RelayMessage{
					Status: status,
					Sender: h,
				})
				//outgoing := WebsocketMessage{
				//	Source: "api",
				//	Action: "update-status",
				//	Mute:   status,
				//}
				//msg, _ := json.Marshal(outgoing)
				//relay.Notify(msg)
			}
		}
	}
}

func (h *WebsocketHandler) Log(args ...any) {
	h.ctx.Logger().Info(args...)
}

func (h *WebsocketHandler) LogError(args ...any) {
	h.ctx.Logger().Error(args...)
}

func (h *WebsocketHandler) LogErrorf(format string, args ...any) {
	h.ctx.Logger().Errorf(format, args...)
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
	state, err := mute.Mute(c.Logger(), mute.State)
	status := mute.State2Status(state)
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
		_, err = mute.Mute(c.Logger(), mute.StateOn)
	case "off":
		_, err = mute.Mute(c.Logger(), mute.StateOff)
	case "toggle":
		switch app {
		//case "slack":
		//Not implemented
		default:
			_, err = mute.Mute(c.Logger(), mute.StateToggle)
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

	_, err := mute.Mute(c.Logger(), mute.StateToggle)
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
	//case "slack":
	//Not implemented
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
