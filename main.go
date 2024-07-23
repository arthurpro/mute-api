//go:generate go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/teivah/broadcast"
	"mute-api/echo"
	"mute-api/mute"
	"mute-api/systray"
)

var appDirectory string

func init() {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	appDirectory = filepath.Dir(ex)
}

func main() {
	logFile, _ := os.OpenFile(filepath.Join(appDirectory, "mute-api.log"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	os.Stdout = logFile
	os.Stderr = logFile
	logWriter := NewLogWriter(logFile)

	exitRelay := broadcast.NewRelay[bool]()
	muteRelay := broadcast.NewRelay[mute.RelayMessage]()

	defer func() {
		//exitRelay.Notify(true)
	}()

	systray.Run(func() {
		echo.Init(logWriter, exitRelay, muteRelay)
	}, func() {
		fmt.Println("notifying via exit relay in systray")
		exitRelay.Notify(true)
		exitRelay.Close()
		muteRelay.Close()
		logFile.Close()
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}, muteRelay)
}

type LogWriter struct {
	w io.Writer
}

func NewLogWriter(w io.Writer) *LogWriter {
	return &LogWriter{w: w}
}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	fmt.Fprint(lw.w, string(p))
	return len(p), nil
}
