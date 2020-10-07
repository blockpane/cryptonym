package cryptonym

import (
	"encoding/json"
	"fmt"
	"fyne.io/fyne/app"
	"fyne.io/fyne/widget"
	"github.com/fioprotocol/fio-go"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"os"
	"runtime"
)

const (
	AppTitle = "Cryptonym"
)

var (
	WinSettings             = getSavedWindowSettings()
	W                       = WinSettings.W
	H                       = WinSettings.H
	txW                     = 1200
	txH                     = 500
	ActionW                 = 220 // width of action buttons on left side
	WidthReduce             = 26  // trim down size of right window this much to account for padding
	App                     = app.NewWithID("explorer")
	Win                     = App.NewWindow(AppTitle)
	BalanceChan             = make(chan bool)
	BalanceLabel            = widget.NewLabel("")
	DefaultFioAddress       = ""
	TableIndex              = NewTableIndex()
	delayTxSec              = 1000000000
	EndPoints               = SupportedApis{Apis: []string{"/v1/chain/push_transaction"}}
	actionEndPointActive    = "/v1/chain/push_transaction"
	apiEndPointActive       = "/v1/chain/get_info"
	p                       = message.NewPrinter(language.English)
	RepaintChan             = make(chan bool)
	PasswordVisible         bool
	SettingsLoaded          = make(chan *FioSettings)
	Settings                = DefaultSettings()
	TxResultBalanceChan     = make(chan string)
	TxResultBalanceChanOpen = false
	useZlib                 = false
	deferTx                 = false
	Connected               bool
	Uri                     = ""
	Api                     = &fio.API{}
	Opts                    = &fio.TxOptions{}
	Account                 = func() *fio.Account {
		a, _ := fio.NewAccountFromWif("5JBbUG5SDpLWxvBKihMeXLENinUzdNKNeozLas23Mj6ZNhz3hLS") // vote1@dapixdev
		return a
	}()
)

func init() {
	txW = (W * 65) / 100
	txH = (H * 85) / 100
	go func() {
		TxResultBalanceChanOpen = true
		defer func() {
			TxResultBalanceChanOpen = false
		}()
		for {
			select {
			case bal := <-TxResultBalanceChan:
				BalanceLabel.SetText(bal)
				BalanceLabel.Refresh()
			}
		}
	}()
	startErrLog()
}

type winSettings struct {
	W int    `json:"w"`
	H int    `json:"h"`
	T string `json:"t"`
}

func getSavedWindowSettings() winSettings {
	def := winSettings{
		W: 1440,
		H: 900,
		T: "Light",
	}
	d, e := os.UserConfigDir()
	if e != nil || d == "" {
		return def
	}
	f, e := os.Open(fmt.Sprintf("%s%c%s%cwindow.json", d, os.PathSeparator, settingsDir, os.PathSeparator))
	if e != nil {
		return def
	}
	defer f.Close()
	s, e := os.Stat(fmt.Sprintf("%s%c%s%cwindow.json", d, os.PathSeparator, settingsDir, os.PathSeparator))
	if e != nil {
		return def
	}
	b := make([]byte, s.Size())
	_, e = f.Read(b)
	if e != nil {
		return def
	}
	wSet := winSettings{}
	e = json.Unmarshal(b, &wSet)
	if e != nil {
		return def
	}
	if wSet.W == 0 || wSet.H == 0 {
		wSet = def
	}
	if wSet.T == "" {
		wSet.T = "Light"
	}

	if runtime.GOOS != "darwin" {
		wSet.H -= 50
	}
	return wSet
}

func saveWindowSettings(w int, h int, t string) bool {
	d, e := os.UserConfigDir()
	if e != nil || d == "" {
		return false
	}
	if ok, _ := MkDir(); !ok {
		return false
	}
	fn := fmt.Sprintf("%s%c%s%cwindow.json", d, os.PathSeparator, settingsDir, os.PathSeparator)
	f, e := os.OpenFile(fn, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if e != nil {
		return false
	}
	defer f.Close()
	j, _ := json.Marshal(&winSettings{W: w, H: h, T: t})
	_, e = f.Write(j)
	if e != nil {
		return false
	}
	return true
}

func RWidth() int {
	return W - ActionW - WidthReduce
}

func PctHeight() int {
	return (H * 95) / 100
}
