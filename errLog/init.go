package errs

import (
	"fmt"
	"fyne.io/fyne/widget"
	"github.com/fioprotocol/fio-go/eos"
	"log"
	"strings"
	"time"
)

var (
	ErrChan        = make(chan string)
	DisconnectChan = make(chan bool)
	ErrTxt         = make([]string, 50)
	ErrMsgs        = widget.NewMultiLineEntry()
	RefreshChan    = make(chan bool)
	Connected      bool
)

func init() {
	go func(msg chan string, disconnected chan bool) {
		last := time.Now()
		t := time.NewTicker(500 * time.Millisecond)
		for {
			select {
			case d := <-disconnected:
				Connected = d
				time.Sleep(250 * time.Millisecond)
			case m := <-msg:
				log.Println(m)
				ErrTxt = append([]string{time.Now().Format(time.Stamp) + " -- " + m}, ErrTxt[:len(ErrTxt)-1]...)
				last = time.Now()
			case <-t.C:
				if time.Now().After(last.Add(500 * time.Millisecond)) {
					txt := strings.Join(ErrTxt, "\n")
					func(s string) {
						ErrMsgs.OnChanged = func(string) {
							ErrMsgs.SetText(s)
						}
					}(txt)
					ErrMsgs.SetText(txt)
				}
			}
		}
	}(ErrChan, DisconnectChan)
}

func Detailed(e error) string {
	switch e.(type) {
	case eos.APIError:
		return fmt.Sprintf("%s - %+v", e.Error(), e.(eos.APIError).ErrorStruct)
	default:
		return e.Error()
	}
}
