package main

import (
	_ "embed"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

func main() {
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

	// Routes
	e.GET("/status", new(Action).StatusAction)
	e.POST("/mute", new(Action).MuteAction)
	e.POST("/leave", new(Action).LeaveAction)

	// Start server
	e.Logger.Fatal(e.Start(":3491"))
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
	mute := Mute(c.Logger(), MuteState)
	var muteState string
	if mute == nil {
		muteState = "unknown"
	} else {
		switch *mute {
		case true:
			muteState = "active"
		case false:
			muteState = "inactive"
		}
	}
	a.Mute = muteState
	a.Control = "system"
	a.Status = http.StatusOK
	return a.Handle(c)
}

func (a *Action) MuteAction(c echo.Context) error {
	app := c.QueryParam("app")
	state := c.QueryParam("state")
	a.Status = http.StatusOK

	var err error
	switch state {
	case "on":
		//keybdEvent.SetKeys(keybd_event.VK_P)
		//keybdEvent.HasSHIFT(true)
		//keybdEvent.HasALT(true)
		//err = keybdEvent.Launching()
		Mute(c.Logger(), MuteStateOn)

	case "off":
		//keybdEvent.SetKeys(keybd_event.VK_O)
		//keybdEvent.HasSHIFT(true)
		//keybdEvent.HasALT(true)
		//err = keybdEvent.Launching()
		Mute(c.Logger(), MuteStateOff)
	case "toggle":
		switch app {
		case "slack":
			keybdEvent.SetKeys(keybd_event.VK_SPACE)
			keybdEvent.HasSHIFT(true)
			keybdEvent.HasCTRL(true)
			err = keybdEvent.Launching()
		default:
			Mute(c.Logger(), MuteStateToggle)
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
