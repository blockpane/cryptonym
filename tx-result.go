package cryptonym

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	Results       = make([]TxResult, 0)
	requestText   = widget.NewMultiLineEntry()
	responseText  = widget.NewMultiLineEntry()
	stopRequested = make(chan bool)
)

type TxResult struct {
	FullResp []byte
	FullReq  []byte
	Resp     []byte
	Req      []byte
	Success  bool
	Index    int
	Summary  string
}

type TxSummary struct {
	TransactionId string `json:"transaction_id" yaml:"Transaction Id"`
	Processed     struct {
		BlockNum  uint32 `json:"block_num" yaml:"Block Number"`
		BlockTime string `json:"block_time" yaml:"Block Time"`
		Receipt   struct {
			Status string `json:"status" yaml:"Status"`
		} `json:"receipt" yaml:"Receipt,omitempty"`
	} `json:"processed" yaml:"Processed,omitempty"`
	ErrorCode  interface{} `json:"error_code" yaml:"Error,omitempty"`                             // is this a string, int, varies on context?
	TotalBytes int         `json:"total_bytes,omitempty" yaml:"TX Size of All Actions,omitempty"` // this is field we calculate later
}

// to get the *real* size of what was transacted, we need to dig into the action traces and look at the length
// of the hex_data field, which is buried in the response.
type txTraces struct {
	Processed struct {
		ActionTraces []struct {
			Act struct {
				HexData string `json:"hex_data"`
			} `json:"act"`
		} `json:"action_traces"`
	} `json:"processed"`
}

func (tt txTraces) size() int {
	if len(tt.Processed.ActionTraces) == 0 {
		return 0
	}
	var sz int
	for _, t := range tt.Processed.ActionTraces {
		sz = sz + (len(t.Act.HexData) / 2)
	}
	return sz
}

type txResultOpts struct {
	repeat      int
	loop        bool
	threads     string
	hideFail    bool
	hideSucc    bool
	window      fyne.Window
	gone        bool
	msig        bool
	msigSigners string
	msigAccount string
	msigName    func() string
	wrap        bool
	wrapActor   string
}

func TxResultsWindow(win *txResultOpts, api *fio.API, opts *fio.TxOptions, account *fio.Account) {
	//var window fyne.Window
	if win.window == nil {
		win.window = App.NewWindow("TX Result")
	}

	workers, e := strconv.Atoi(win.threads)
	if e != nil {
		workers = 1
	}

	var (
		grid              *fyne.Container
		b                 *widget.Button
		stopButton        *widget.Button
		closeRow          *widget.Group
		running           bool
		exit              bool
		fullResponseIndex int
	)

	successLabel := widget.NewLabel("")
	failedLabel := widget.NewLabel("")
	successChan := make(chan bool)
	failedChan := make(chan bool)
	go func(s chan bool, f chan bool) {
		time.Sleep(100 * time.Millisecond)
		BalanceChan <- true
		tick := time.NewTicker(time.Second)
		update := false
		updateBalance := false
		successCount := 0
		failedCount := 0
		for {
			select {
			case <-tick.C:
				if updateBalance {
					BalanceChan <- true
					updateBalance = false
				}
				if update {
					successLabel.SetText(p.Sprintf("%d", successCount))
					failedLabel.SetText(p.Sprintf("%d", failedCount))
					successLabel.Refresh()
					failedLabel.Refresh()
					update = false
				}
			case <-f:
				update = true
				failedCount = failedCount + 1
			case <-s:
				update = true
				updateBalance = true
				successCount = successCount + 1
			}
		}
	}(successChan, failedChan)

	run := func() {}
	mux := sync.Mutex{}
	Results = make([]TxResult, 0)

	summaryGroup := widget.NewGroupWithScroller("Transaction Result")
	showFullResponseButton := widget.NewButtonWithIcon("Show Response Details", theme.VisibilityIcon(), func() {
		// avoid nil pointer
		if len(Results) <= fullResponseIndex {
			errs.ErrChan <- "could not show full response: invalid result index - this shouldn't happen!"
			return
		}
		if len(Results[fullResponseIndex].FullResp) == 0 {
			errs.ErrChan <- "could not show full response: empty string"
			return
		}
		ShowFullResponse(Results[fullResponseIndex].FullResp, win.window)
	})
	showFullRequestButton := widget.NewButtonWithIcon("Show Request JSON", theme.VisibilityIcon(), func() {
		// avoid nil pointer
		if len(Results) <= fullResponseIndex {
			errs.ErrChan <- "could not show full request: invalid result index - this shouldn't happen!"
			return
		}
		if len(Results[fullResponseIndex].FullReq) == 0 {
			errs.ErrChan <- "could not show full request: empty string"
			return
		}
		ShowFullRequest(Results[fullResponseIndex].FullReq, win.window)
	})

	textUpdateDone := make(chan interface{})
	textUpdateReq := make(chan string)
	textUpdateResp := make(chan string)
	go func() {
		for {
			select {
			case <-textUpdateDone:
				return
			case s := <-textUpdateReq:
				requestText.OnChanged = func(string) {
					requestText.SetText(s)
				}
				requestText.SetText(s)
			case s := <-textUpdateResp:
				responseText.OnChanged = func(string) {
					responseText.SetText(s)
				}
				responseText.SetText(s)
			}
		}
	}()

	setGrid := func() {
		grid = fyne.NewContainerWithLayout(layout.NewHBoxLayout(),
			fyne.NewContainerWithLayout(layout.NewGridLayoutWithRows(1),
				closeRow,
				fyne.NewContainerWithLayout(layout.NewMaxLayout(),
					summaryGroup,
				),
			),
			widget.NewVBox(
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(txW, 30)),
					fyne.NewContainerWithLayout(layout.NewGridLayout(2), showFullResponseButton, showFullRequestButton),
				),
				widget.NewLabel("Request:"),
				requestText,
				widget.NewLabel("Response Summary:"),
				responseText,
			),
		)
		win.window.Resize(fyne.NewSize(txW, txH))
		win.window.SetContent(grid)
		//win.window.CenterOnScreen()
	}

	clear := func() {
		mux.Lock()
		Results = make([]TxResult, 0)
		summaryGroup = widget.NewGroupWithScroller("Transaction Result")
		summaryGroup.Refresh()
		textUpdateResp <- ""
		textUpdateReq <-""
		setGrid()
		mux.Unlock()
	}

	closeButton := widget.NewButtonWithIcon(
		"close",
		theme.DeleteIcon(),
		func() {
			if running {
				stopRequested <- true
			}
			win.gone = true
			clear()
			//Win.RequestFocus()
			win.window.Close()
		},
	)
	resendButton := widget.NewButtonWithIcon("resend", theme.ViewRefreshIcon(), func() {
		if running {
			return
		}
		exit = false
		go run()
	})
	stopButton = widget.NewButtonWithIcon("stop", theme.CancelIcon(), func() {
		if running {
			stopRequested <- true
		}
	})

	clearButton := widget.NewButtonWithIcon("clear results", theme.ContentRemoveIcon(), func() {
		clear()
	})
	closeRow = widget.NewGroup(" Control ",
		stopButton,
		resendButton,
		clearButton,
		closeButton,
		layout.NewSpacer(),
		BalanceLabel,
		widget.NewLabel("Successful Requests:"),
		successLabel,
		widget.NewLabel("Failed Requests:"),
		failedLabel,
	)
	closeRow.Show()

	reqChan := make(chan string)
	respChan := make(chan string)
	fullRespChan := make(chan int)

	trimDisplayed := func(s string) string {
		re := regexp.MustCompile(`[[^:ascii:]]`)
		var displayed string
		s = s + "\n"
		reader := strings.NewReader(s)
		buf := bufio.NewReader(reader)
		var lines int
		for {
			lines = lines + 1
			line, err := buf.ReadString('\n')
			if err != nil {
				break
			}
			line, _ = strconv.Unquote(strconv.QuoteToASCII(line))
			if len(line) > 128+21 {
				line = fmt.Sprintf("%s ... trimmed %d chars ...\n", line[:128], len(line)-128)
			}
			displayed = displayed + line
			if lines > 31 {
				displayed = displayed + "\n ... too many lines to display ..."
				break
			}
		}
		return re.ReplaceAllString(displayed, "?")
	}

	go func(rq chan string, rs chan string, frs chan int) {
		for {
			select {
			case q := <-rq:
				mux.Lock()
				textUpdateReq <-trimDisplayed(q)
				mux.Unlock()
			case s := <-rs:
				mux.Lock()
				textUpdateResp <-trimDisplayed(s)
				mux.Unlock()
			case fullResponseIndex = <-frs:
			}
		}
	}(reqChan, respChan, fullRespChan)
	reqChan <- ""
	respChan <- ""

	repaint := func() {
		mux.Lock()
		closeRow.Refresh()
		summaryGroup.Refresh()
		responseText.Refresh()
		requestText.Refresh()
		if grid != nil {
			grid.Refresh()
		}
		mux.Unlock()
	}

	newButton := func(title string, index int, failed bool) {
		if failed {
			failedChan <- false
		} else {
			successChan <- true
		}
		if (!failed && win.hideSucc) || (failed && win.hideFail) {
			return
		}
		// possible race while clearing the screen
		if index > len(Results) {
			return
		}
		deRef := &index
		i := *deRef
		if i-1 > len(Results) || len(Results) == 0 {
			return
		}
		if len(Results) > 256 {
			clear()
		}
		mux.Lock()
		icon := theme.ConfirmIcon()
		if failed {
			icon = theme.CancelIcon()
		}

		b = widget.NewButtonWithIcon(title, icon, func() {
			if i >= len(Results) {
				return
			}
			reqChan <- string(Results[i].Req)
			respChan <- string(Results[i].Resp)
			fullRespChan <- i
		})
		summaryGroup.Append(b)
		mux.Unlock()
		repaint()
	}

	run = func() {
		defer func() {
			if running {
				stopRequested <- true
			}
		}()
		// give each thread it's own http client pool:
		workerApi, workerOpts, err := fio.NewConnection(account.KeyBag, api.BaseURL)
		if err != nil {
			errs.ErrChan <- err.Error()
			errs.ErrChan <- "ERROR: could not get new client connection"
			return
		}
		workerApi.Header.Set("User-Agent", "fio-cryptonym-wallet")
		running = true
		stopButton.Enable()
		bombsAway.Disable()
		resendButton.Disable()
		closeButton.Disable()

		defer func() {
			running = false
			stopButton.Disable()
			bombsAway.Enable()
			resendButton.Enable()
			closeButton.Enable()
		}()

		var end int
		switch {
		case win.loop:
			end = math.MaxInt32
		case win.repeat > 1:
			end = win.repeat
		default:
			end = 1
		}
		finished := make(chan bool)
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer func() {
				running = false
				finished <- true
				wg.Done()
			}()
			for i := 0; i < end; i++ {
				if exit {
					return
				}
				output := TxResult{
					Summary: fmt.Sprintf("%s", time.Now().Format("05.000")),
					Index:   i,
				}
				e := FormState.GeneratePayloads(account)
				if e != nil {
					errs.ErrChan <- e.Error()
					errs.ErrChan <- "there was a problem generating dynamic payloads"
					output.Resp = []byte(e.Error())
					Results = append(Results, output)
					newButton(output.Summary, len(Results)-1, true)
					continue
				}
				if exit {
					return
				}
				raw, tx, err := FormState.PackAndSign(workerApi, workerOpts, account, win.msig)
				if tx == nil || tx.PackedTransaction == nil {
					errs.ErrChan <- "sending a signed transaction with null action data"
					empty := fio.NewAction(eos.AccountName(FormState.Contract), eos.ActionName(FormState.Action), account.Actor, nil)
					_, tx, err = workerApi.SignTransaction(fio.NewTransaction([]*fio.Action{empty}, workerOpts), workerOpts.ChainID, fio.CompressionNone)
					if err != nil {
						errs.ErrChan <- err.Error()
						continue
					}
				}
				if win.msig && err == nil {
					if tx == nil || tx.PackedTransaction == nil {
						errs.ErrChan <- "did not build a valid transaction, refusing to continue."
						output.Resp = []byte("could not build the transaction. Don't worry it's me, not you.")
						Results = append(Results, output)
						newButton(output.Summary, len(Results)-1, true)
						continue
					}
					ntx, err := tx.Unpack()
					if err != nil {
						errs.ErrChan <- "Problem repacking transaction to embed in msig propose " + err.Error()
						output.Resp = []byte(err.Error())
						Results = append(Results, output)
						newButton(output.Summary, len(Results)-1, true)
						continue
					}
					// convert to transaction, without signature:
					untx := eos.Transaction{}
					if win.wrap {
						untx = eos.Transaction{
							TransactionHeader:  ntx.TransactionHeader,
							ContextFreeActions: ntx.ContextFreeActions,
							Actions:            ntx.Actions,
							Extensions:         ntx.Extensions,
						}
						untx.Expiration = eos.JSONTime{Time: time.Unix(0, 0)}
						untx.RefBlockNum = 0
						untx.RefBlockPrefix = 0
						for i := range untx.Actions {
							untx.Actions[i].Authorization = []eos.PermissionLevel{
								{Actor: eos.AccountName(win.wrapActor), Permission: "active"},
							}
						}
					} else {
						for i := range ntx.Actions {
							ntx.Actions[i].Authorization = []eos.PermissionLevel{{
								Actor:      eos.AccountName(win.msigAccount),
								Permission: "active",
							}}
						}
					}
					requested := make([]*fio.PermissionLevel, 0)
					signers := strings.Split(win.msigSigners, ",")
					sort.Strings(signers)
					for _, s := range signers {
						requested = append(requested, &fio.PermissionLevel{
							Actor:      eos.AccountName(strings.ReplaceAll(s, " ", "")),
							Permission: "active",
						})
					}
					packed, _ := ntx.Pack(fio.CompressionNone)
					//propose := fio.MsigPropose{}
					//wrapPropose := fio.MsigWrappedPropose{}
					var propose interface{}
					if win.msig && !win.wrap {
						ntx.Expiration = eos.JSONTime{Time: time.Now().Add(60 * time.Minute)}
						propose = fio.MsigPropose{
							Proposer:     account.Actor,
							ProposalName: eos.Name(win.msigName()),
							Requested:    requested,
							MaxFee:       fio.Tokens(fio.GetMaxFee(fio.FeeMsigPropose))*uint64(len(packed.PackedTransaction)/1000) + fio.Tokens(1.0),
							Trx:          ntx,
						}
					} else if win.wrap {
						//wrap := fio.NewWrapExecute(account.Actor, eos.AccountName(win.msigAccount), ntx)
						wrap := fio.NewWrapExecute("eosio.wrap", account.Actor, &untx)
						wrap.Authorization = []eos.PermissionLevel{
							//{Actor: account.Actor, Permission: "active"},
							{Actor: "eosio.wrap", Permission: "active"},
						}
						wTx := fio.NewTransaction([]*fio.Action{wrap}, opts)
						wTx.Expiration = eos.JSONTime{Time: time.Now().Add(60 * time.Minute)}
						wTx.RefBlockNum = 0
						wTx.RefBlockPrefix = 0
						propose = fio.MsigWrappedPropose{
							Proposer:     account.Actor,
							ProposalName: eos.Name(win.msigName()),
							Requested:    requested,
							MaxFee:       fio.Tokens(fio.GetMaxFee(fio.FeeMsigPropose))*uint64(len(packed.PackedTransaction)/1000) + fio.Tokens(1.0),
							Trx:          wTx,
						}
					}
					_, tx, err = workerApi.SignTransaction(
						fio.NewTransaction(
							[]*fio.Action{
								fio.NewAction(
									"eosio.msig",
									"propose",
									account.Actor,
									propose,
								),
							},
							workerOpts),
						workerOpts.ChainID, fio.CompressionNone,
					)
					if err != nil {
						errs.ErrChan <- "Problem signing msig propose " + err.Error()
						output.Resp = []byte(err.Error())
						Results = append(Results, output)
						newButton(output.Summary, len(Results)-1, true)
						continue
					}
					raw, _ = json.Marshal(propose)
				}
				j, _ := json.MarshalIndent(raw, "", "  ")
				packed, _ := json.MarshalIndent(tx, "", "  ")
				output.Req = append(append(j, []byte("\n\nPacked Tx:\n\n")...), packed...)
				if err != nil {
					errs.ErrChan <- err.Error()
					errs.ErrChan <- "could not marshall into a TX"
					output.Resp = []byte(err.Error())
					Results = append(Results, output)
					newButton(output.Summary, len(Results)-1, true)
					continue
				}
				if tx == nil || tx.PackedTransaction == nil {
					errs.ErrChan <- "did not build a valid transaction, refusing to continue."
					output.Resp = []byte("could not build the transaction. Don't worry it's me, not you.")
					Results = append(Results, output)
					newButton(output.Summary, len(Results)-1, true)
					continue
				}
				output.Req = append(output.Req, []byte(p.Sprintf("\n\nSize of Packed TX (bytes): %d", len(tx.PackedTransaction)))...)
				reqBuf := bytes.Buffer{}
				reqZWriter, _ := zlib.NewWriterLevel(&reqBuf, zlib.BestCompression)
				reqZWriter.Write(j)
				reqZWriter.Close()
				output.FullReq = reqBuf.Bytes()
				if exit {
					return
				}
				result, err := workerApi.PushEndpointRaw(actionEndPointActive, tx)
				if err != nil {
					errs.ErrChan <- err.Error()
					if win.hideFail {
						failedChan <- true
						continue
					}
					output.Resp = []byte(err.Error())
					output.Summary = fmt.Sprintf("%s", time.Now().Format("05.000"))
					buf := bytes.Buffer{}
					zWriter, _ := zlib.NewWriterLevel(&buf, zlib.BestCompression)
					if len(result) > 0 {
						zWriter.Write(result)
					} else {
						zWriter.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())))
					}
					zWriter.Close()
					output.FullResp = buf.Bytes()
					Results = append(Results, output)
					newButton(output.Summary, len(Results)-1, true)
					continue
				}
				if exit {
					return
				}
				// store two responses, the summary -- displayed by default, and zlib compressed full response.
				// the full response is huge, and will seriously screw up the display and consume a lot of memory!
				summary := &TxSummary{}
				err = json.Unmarshal(result, summary)
				if err != nil {
					errs.ErrChan <- err.Error()
					output.Resp = []byte(err.Error())
					output.Summary = fmt.Sprintf("%s", time.Now().Format("05.000"))
					if win.hideFail {
						failedChan <- true
						continue
					}
					Results = append(Results, output)
					newButton(output.Summary, len(Results)-1, true)
					continue
				}
				// get the real tx size, since we got this far assume we have a valid tx result
				sz := &txTraces{}
				_ = json.Unmarshal(result, sz)
				summary.TotalBytes = sz.size()

				if win.hideSucc {
					successChan <- true
					continue
				}
				j, _ = yaml.Marshal(summary)
				output.Resp = j
				buf := bytes.Buffer{}
				zWriter, _ := zlib.NewWriterLevel(&buf, zlib.BestCompression)
				zWriter.Write(result)
				zWriter.Close()
				output.FullResp = buf.Bytes()
				Results = append(Results, output)
				newButton(output.Summary, len(Results)-1, false)
			}
		}()
		for {
			select {
			case _ = <-stopRequested:
				exit = true
			case _ = <-finished:
				wg.Wait()
				return
			}
		}
	}

	for w := 0; w < workers; w++ {
		go run()
	}
	time.Sleep(250 * time.Millisecond)
	setGrid()
	if len(Results) > 0 && !win.hideFail && !win.hideSucc {
		textUpdateResp <-trimDisplayed(string(Results[0].Resp))
		textUpdateReq <-trimDisplayed(string(Results[0].Req))
	}
	if !running {
		stopButton.Disable()
	}
	repaint()
	win.window.SetOnClosed(func() {
		close(textUpdateDone)
		win.gone = true
		exit = true
		win.window = App.NewWindow("Tx Results")
		win.window.Resize(fyne.NewSize(txW, txH))
		win.window.Hide()
		textUpdateReq <-""
		textUpdateResp <-""
		for i := 0; i < 10; i++ {
			if bombsAway.Disabled() {
				exit = true
				time.Sleep(500 * time.Millisecond)
			} else {
				Win.RequestFocus()
				return
			}
		}
	})
	if win.gone {
		win.gone = false
		win.window.Show()
	} else {
		repaint()
	}
}

func ShowFullResponse(b []byte, win fyne.Window) {
	FullResponseText := widget.NewMultiLineEntry()
	FullActionRespWin := App.NewWindow("Full Response")
	FullActionRespWin.Hide()
	FullActionRespWin.Resize(fyne.NewSize(W, H))
	FullActionRespWin.SetContent(
		fyne.NewContainerWithLayout(layout.NewMaxLayout(),
			widget.NewScrollContainer(
				FullResponseText,
			),
		),
	)
	FullActionRespWin.SetOnClosed(func() {
		go func() {
			// bug in fyne 1.3 where we need a very short wait to grab a child window
			time.Sleep(100 * time.Millisecond)
			for _, w := range fyne.CurrentApp().Driver().AllWindows() {
				if w.Title() == "Tx Results" {
					w.RequestFocus()
					log.Println("found parent")
					return
				}
			}
			win.RequestFocus()
		}()
	})
	set := func(s string) {
		FullResponseText.OnChanged = func(string) {
			FullResponseText.SetText(s)
		}
		FullResponseText.SetText(s)
		FullResponseText.Refresh()
		FullActionRespWin.Show()
	}
	reader := bufio.NewReader(bytes.NewReader(b))
	zlReader, err := zlib.NewReader(reader)
	if err != nil {
		set(err.Error())
		return
	}
	defer zlReader.Close()
	j, err := ioutil.ReadAll(zlReader)
	if err != nil {
		set(err.Error())
		return
	}
	full, err := json.MarshalIndent(json.RawMessage(j), "", "  ")
	if err != nil {
		set(err.Error())
		return
	}
	set(string(full))
}

func ShowFullRequest(b []byte, win fyne.Window) {
	fullRequestText := widget.NewMultiLineEntry()
	fullActionRespWin := App.NewWindow("Full Request")
	fullActionRespWin.Hide()
	fullActionRespWin.Resize(fyne.NewSize(W, H))
	fullActionRespWin.SetContent(
		fyne.NewContainerWithLayout(layout.NewMaxLayout(),
			widget.NewScrollContainer(
				fullRequestText,
			),
		),
	)
	fullActionRespWin.SetOnClosed(func() {
		go func() {
			time.Sleep(100 * time.Millisecond)
			for _, w := range fyne.CurrentApp().Driver().AllWindows() {
				if w.Title() == "Tx Results" {
					w.RequestFocus()
					log.Println("found parent")
					return
				}
			}
			win.RequestFocus()
		}()
	})
	set := func(s string) {
		fullRequestText.OnChanged = func(string) {
			fullRequestText.SetText(s)
		}
		fullRequestText.SetText(s)
		fullRequestText.Refresh()
		fullActionRespWin.Show()
	}
	reader := bufio.NewReader(bytes.NewReader(b))
	zlReader, err := zlib.NewReader(reader)
	if err != nil {
		set(err.Error())
		return
	}
	defer zlReader.Close()
	j, err := ioutil.ReadAll(zlReader)
	if err != nil {
		set(err.Error())
		return
	}
	full, err := json.MarshalIndent(json.RawMessage(j), "", "  ")
	if err != nil {
		set(err.Error())
		return
	}
	set(string(full))
}
