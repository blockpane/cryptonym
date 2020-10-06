package main

import (
	"context"
	"encoding/json"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	explorer "github.com/blockpane/cryptonym"
	fioassets "github.com/blockpane/cryptonym/assets"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type tabs struct {
	Editor      widget.TabItem
	Api         widget.TabItem
	Info        widget.TabItem
	Browser     widget.TabItem
	Abi         widget.TabItem
	AccountInfo widget.TabItem
	KeyGen      widget.TabItem
	Msig        widget.TabItem
	Vote        widget.TabItem
	Requests    widget.TabItem
}

var (
	uri   = &explorer.Uri
	proxy = func() *string {
		s := "127.0.0.1:8080"
		return &s
	}()
	api           = explorer.Api
	opts          = explorer.Opts
	account       = explorer.Account
	balance       float64
	actionsGroup  = &widget.Group{}
	connectButton = &widget.Button{}
	proxyCheck    = &widget.Check{}
	keyContent    = &widget.Box{}
	tabContent    = &widget.TabContainer{}
	tabEntries    = tabs{}
	hostEntry     = explorer.NewClickEntry(connectButton)
	myFioAddress  = widget.NewEntry()
	moneyBags     = widget.NewSelect(moneySlice(), func(s string) {})
	wifEntry      = widget.NewPasswordEntry()
	balanceLabel  = widget.NewLabel("Balance: unknown")
	loadButton    = &widget.Button{}
	importButton  = &widget.Button{}
	balanceButton = &widget.Button{}
	regenButton   = &widget.Button{}
	uriContent    = uriInput(true)
	uriContainer  = &fyne.Container{}
	ready         = false
	connectedChan = make(chan bool, 1)
	p             = message.NewPrinter(language.English)
	keyBox        = &widget.Box{}
	serverInfoCh  = make(chan explorer.ServerInfo)
	serverInfoRef = make(chan bool)
	serverInfoBox = explorer.InitServerInfo(serverInfoCh, serverInfoRef)
)

// ActionButtons is a slice of pointers to our action buttons, this way we can set them to hidden if using
// the filter ....
var (
	ActionButtons = make([]*widget.Button, 0)
	ActionLabels  = make([]*widget.Label, 0)
	filterActions = &widget.Entry{}
	filterCheck   = &widget.Check{}
	prodsCheck    = &widget.Check{}
)

var savedKeys = map[string]string{
	"devnet vote1":   "5JBbUG5SDpLWxvBKihMeXLENinUzdNKNeozLas23Mj6ZNhz3hLS",
	"devnet vote2":   "5KC6Edd4BcKTLnRuGj2c8TRT9oLuuXLd3ZuCGxM9iNngc3D8S93",
	"devnet bp1":     "5KQ6f9ZgUtagD3LZ4wcMKhhvK9qy4BuwL3L1pkm6E2v62HCne2R",
	"devnet locked1": "5HwvMtAEd7kwDPtKhZrwA41eRMdFH5AaBKPRim6KxkTXcg5M9L5",
}

func main() {
	// the MacOS resolver causes serious performance issues, if GODEBUG is empty, then set it to force pure go resolver.
	if runtime.GOOS == "darwin" {
		gdb := os.Getenv("GODEBUG")
		if gdb == "" {
			_ = os.Setenv("GODEBUG", "netdns=go")
		}
	}
	topLayout := &fyne.Container{}
	errs.ErrTxt[0] = fmt.Sprintf("\nEvent Log: started at %s", time.Now().Format(time.Stamp))
	errs.ErrMsgs.SetText(strings.Join(errs.ErrTxt, "\n"))
	keyContent = keyBoxContent()
	myFioAddress.Hide()

	loadButton.Disable()
	balanceButton.Disable()
	regenButton.Disable()

	space := strings.Repeat("  ", 55)
	go func() {
		for {
			select {
			case <-connectedChan:
				time.Sleep(time.Second)
				serverInfoRef <- true
				explorer.Connected = true
				uriContainer.Objects = []fyne.CanvasObject{
					widget.NewVBox(
						widget.NewLabel(" "),
						widget.NewHBox(
							widget.NewLabel(space),
							widget.NewLabel(" nodeos @ "+*uri+" "),
							widget.NewLabel(space),
						),
					),
				}
				loadButton.Enable()
				balanceButton.Enable()
				regenButton.Enable()
				refreshMyName()
			case <-errs.RefreshChan:
				if !ready {
					continue
				}
				refreshNotNil(loadButton)
				refreshNotNil(balanceButton)
				refreshNotNil(regenButton)
				refreshNotNil(actionsGroup)
				refreshNotNil(hostEntry)
				refreshNotNil(uriContent)
				refreshNotNil(keyContent)
				refreshNotNil(topLayout)
				refreshNotNil(errs.ErrMsgs)
				refreshNotNil(tabEntries.Info.Content)
				refreshNotNil(tabEntries.Editor.Content)
				refreshNotNil(tabEntries.Api.Content)
				refreshNotNil(tabEntries.Msig.Content)
				if explorer.TableIndex.IsCreated() {
					refreshNotNil(tabEntries.Browser.Content)
					refreshNotNil(tabEntries.Abi.Content)
				}
				refreshNotNil(tabContent)
				if moneyBags.Hidden {
					moneyBags.Show()
				}
			}
		}
	}()

	if reconnect(account) {
		connectedChan <- true
	}

	updateActions(ready, opts)
	tabEntries = makeTabs()
	// KeyGen has to be created after others to prevent a race:
	tabContent = widget.NewTabContainer(
		&tabEntries.Info,
		&tabEntries.AccountInfo,
		&tabEntries.Abi,
		&tabEntries.Browser,
		&tabEntries.Editor,
		&tabEntries.Api,
		widget.NewTabItem("Key Gen", explorer.KeyGenTab()),
		&tabEntries.Vote,
		&tabEntries.Msig,
		&tabEntries.Requests,
	)

	uriContainer = fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(5, 35)),
		uriContent,
	)
	tabEntries.Info.Content = widget.NewVBox(
		layout.NewSpacer(),
		uriContainer,
		layout.NewSpacer(),
	)
	refreshNotNil(tabEntries.Info.Content)

	topLayout = fyne.NewContainerWithLayout(layout.NewHBoxLayout(),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(explorer.ActionW, explorer.PctHeight())),
			actionsGroup,
		),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(10, explorer.PctHeight())),
			layout.NewSpacer(),
		),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(explorer.RWidth(), explorer.PctHeight())),
			fyne.NewContainerWithLayout(layout.NewVBoxLayout(),
				tabContent,
				layout.NewSpacer(),
				fyne.NewContainerWithLayout(layout.NewVBoxLayout(),
					keyContent,
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(explorer.RWidth(), 150)),
						widget.NewScrollContainer(
							errs.ErrMsgs,
						),
					),
				),
			),
		),
	)

	go func(repaint chan bool) {
		for {
			select {
			case <-repaint:
				// bug in Refresh doesn't get the group title, hide then show works
				actionsGroup.Hide()
				actionsGroup.Show()
				if tabContent != nil {
					tabContent.SelectTabIndex(0)
				}
				explorer.Win.Content().Refresh()
				explorer.RefreshQr <- true
				errs.RefreshChan <- true
			}
		}
	}(explorer.RepaintChan)

	explorer.Win.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("Settings",
		fyne.NewMenuItem("Options", func() {
			go explorer.SettingsWindow()
		}),
		fyne.NewMenuItem("Reload Saved Settings", func() {
			go explorer.PromptForPassword()
		}),
		fyne.NewMenuItem("Connect to different server", func() {
			go uriModal()
		}),
		fyne.NewMenuItem("", func() {}),
		fyne.NewMenuItem("Dark Theme", func() {
			explorer.WinSettings.T = "Dark"
			explorer.RefreshQr <- true
			fyne.CurrentApp().Settings().SetTheme(explorer.CustomTheme())
			explorer.RepaintChan <- true
		}),
		fyne.NewMenuItem("Darker Theme", func() {
			explorer.WinSettings.T = "Darker"
			explorer.RefreshQr <- true
			fyne.CurrentApp().Settings().SetTheme(explorer.DarkerTheme().ToFyneTheme())
			explorer.RepaintChan <- true
		}),
		fyne.NewMenuItem("Light Theme", func() {
			explorer.WinSettings.T = "Light"
			explorer.RefreshQr <- true
			fyne.CurrentApp().Settings().SetTheme(explorer.ExLightTheme().ToFyneTheme())
			explorer.RepaintChan <- true
		}),
		fyne.NewMenuItem("Grey Theme", func() {
			explorer.WinSettings.T = "Grey"
			explorer.RefreshQr <- true
			fyne.CurrentApp().Settings().SetTheme(explorer.ExGreyTheme().ToFyneTheme())
			explorer.RepaintChan <- true
		}),
	)))

	ready = true
	updateActions(ready, opts)
	explorer.Win.Resize(fyne.NewSize(explorer.W-10, (explorer.H*95)/100))
	explorer.Win.SetFixedSize(true)
	*uri = "http://127.0.0.1:8888"
	hostEntry.SetText(*uri)
	errs.RefreshChan <- true
	explorer.Win.SetContent(topLayout)
	explorer.Win.SetMaster()
	explorer.Win.SetOnClosed(func() {
		explorer.App.Quit()
	})
	go func() {
		time.Sleep(100 * time.Millisecond)
		switch explorer.WinSettings.T {
		case "Dark":
			fyne.CurrentApp().Settings().SetTheme(explorer.CustomTheme())
		case "Darker":
			fyne.CurrentApp().Settings().SetTheme(explorer.DarkerTheme().ToFyneTheme())
		case "Grey":
			fyne.CurrentApp().Settings().SetTheme(explorer.ExGreyTheme().ToFyneTheme())
		case "Light":
			fyne.CurrentApp().Settings().SetTheme(explorer.ExLightTheme().ToFyneTheme())
		}
		go explorer.PromptForPassword()
		go settingsReload(explorer.SettingsLoaded)
	}()
	explorer.Win.ShowAndRun()
}

func refreshNotNil(object fyne.CanvasObject) {
	if object != nil {
		object.Refresh()
	}
}

func settingsReload(newSettings chan *explorer.FioSettings) {
	for {
		select {
		case s := <-newSettings:
			if !strings.HasPrefix(s.Server, "http") {
				s.Server = "http://" + s.Server
			}
			*uri = s.Server
			hostEntry.SetText(*uri)
			savedKeys = map[string]string{
				s.DefaultKeyDesc: s.DefaultKey,
				s.FavKey2Desc:    s.FavKey2,
				s.FavKey3Desc:    s.FavKey3,
				s.FavKey4Desc:    s.FavKey4,
			}
			moneyBags.Options = moneySlice()
			newAccount, err := fio.NewAccountFromWif(s.DefaultKey)
			keyContent.Children = keyBoxContent().Children
			if err != nil {
				errs.ErrChan <- "error loading key from saved settings. " + err.Error()
			} else {
				dr := *newAccount
				account = &dr
				explorer.Account = &dr
				wifEntry.SetText(s.DefaultKey)
				importButton.OnTapped()
				tabContent.SelectTabIndex(0)
			}
		}
	}
}

func refreshMyName() {
	if account.Addresses != nil && len(account.Addresses) > 0 {
		txt := account.Addresses[0].FioAddress
		func(s string) {
			myFioAddress.OnChanged = func(string) {
				myFioAddress.SetText(s)
			}
		}(txt)
		myFioAddress.SetText(txt)
		explorer.DefaultFioAddress = account.Addresses[0].FioAddress
	} else {
		if found, _, e := account.GetNames(api); e == nil && found > 0 {
			txt := account.Addresses[0].FioAddress
			func(s string) {
				myFioAddress.OnChanged = func(string) {
					myFioAddress.SetText(s)
				}
			}(txt)
			myFioAddress.SetText(txt)
			explorer.DefaultFioAddress = account.Addresses[0].FioAddress
		} else {
			myFioAddress.OnChanged = func(string) {
				myFioAddress.SetText("")
			}
			explorer.DefaultFioAddress = ""
			myFioAddress.SetText("")
			myFioAddress.Hide()
		}
	}
	if myFioAddress.Text == "" && !myFioAddress.Hidden {
		myFioAddress.Hide()
	} else if myFioAddress.Hidden {
		myFioAddress.Show()
	}
	refreshNotNil(myFioAddress)
	refreshNotNil(keyContent)
}

var clientMux = sync.Mutex{}

var apiDeadCounter int

func refreshInfo(deadline time.Duration) (string, bool) {
	if api == nil || explorer.Api == nil || api.BaseURL == "" {
		return "", false
	}
	d := time.Now().Add(deadline)
	ctx, cancel := context.WithDeadline(context.Background(), d)
	defer cancel()
	resultChan := make(chan string, 1)
	go func() {
		clientMux.Lock()
		defer clientMux.Unlock()
		i, e := api.GetInfo()
		if e != nil {
			apiDeadCounter += 1
			errs.ErrChan <- e.Error()
			if apiDeadCounter >= 10 {
				errs.ErrChan <- "connection seems to be having issues, trying to reconnect"
				var err error
				explorer.Api, explorer.Opts, err = fio.NewConnection(explorer.Account.KeyBag, explorer.Uri)
				if err != nil {
					errs.ErrChan <- err.Error()
					return
				}
				api = explorer.Api
				opts = explorer.Opts
			}
			return
		}
		apiDeadCounter = 0
		j, _ := json.MarshalIndent(i, "", "  ")
		serverInfoCh <- explorer.ServerInfo{
			Info: i,
			Uri:  api.BaseURL,
		}
		resultChan <- string(j)
	}()
	select {
	case s := <-resultChan:
		return s, true
	case <-ctx.Done():
		errs.ErrChan <- "failed to update server info in time, reducing poll frequency."
		return "", false
	}
}

func reconnect(account *fio.Account) (result bool) {
	errs.DisconnectChan <- result
	defer func() {
		errs.DisconnectChan <- result
	}()
	clientMux.Lock()
	defer clientMux.Unlock()
	var err error
	api, opts, err = fio.NewConnection(account.KeyBag, *uri)
	if err != nil {
		if *uri != "" {
			errs.ErrChan <- err.Error()
		}
		return
	}
	api.Header.Set("User-Agent", "fio-cryptonym-wallet")
	explorer.Api, explorer.Opts, _ = fio.NewConnection(account.KeyBag, *uri)
	explorer.Api.Header.Set("User-Agent", "fio-cryptonym-wallet")
	errs.ErrChan <- "connected to nodeos at " + *uri
	explorer.Win.SetTitle(fmt.Sprintf("Cryptonym - nodeos @ %s", *uri))
	errs.RefreshChan <- true
	go func() {
		time.Sleep(2 * time.Second)
		explorer.BalanceChan <- true
	}()
	explorer.ResetTxResult()
	result = true
	return
}

func updateActions(ready bool, opts *fio.TxOptions) {
	newGroup := true
	if actionsGroup == nil || actionsGroup.Text == "" {
		actionsGroup = widget.NewGroupWithScroller("Not Connected")
	}
	if !ready {
		return
	}
	if len(ActionButtons) > 0 {
		newGroup = false
	}
	var err error
	clientMux.Lock()
	api, opts, err = fio.NewConnection(account.KeyBag, *uri)
	if err != nil {
		errs.ErrChan <- "not connected to nodeos server"
		clientMux.Unlock()
		return
	}
	api.Header.Set("User-Agent", "fio-cryptonym-wallet")
	explorer.Api, explorer.Opts, _ = fio.NewConnection(account.KeyBag, *uri)
	explorer.Api.Header.Set("User-Agent", "fio-cryptonym-wallet")
	_, err = api.GetInfo()
	if err != nil {
		errs.ErrChan <- "not connected to nodeos server"
		clientMux.Unlock()
		return
	}
	actionsGroup.Text = "Actions"

	if newGroup {
		filterActions = widget.NewEntry()
		filterActions.SetPlaceHolder("Filter Actions")
		filterActions.OnChanged = showHideActions
		actionsGroup.Append(filterActions)
		filterCheck = widget.NewCheck("Hide Privileged", func(bool) {
			showHideActions(filterActions.Text)
		})
		filterCheck.SetChecked(true)
		actionsGroup.Append(widget.NewHBox(layout.NewSpacer(), filterCheck, layout.NewSpacer()))
		prodsCheck = widget.NewCheck("Hide Producer", func(bool) {
			showHideActions(filterActions.Text)
		})
		prodsCheck.SetChecked(true)
		actionsGroup.Append(widget.NewHBox(layout.NewSpacer(), prodsCheck, layout.NewSpacer()))
	} else {
		filterActions.SetText("")
	}

	a, err := explorer.GetAccountSummary(api)
	clientMux.Unlock()
	if err != nil {
		errs.ErrChan <- err.Error()
		return
	}
	if a == nil || a.Actions == nil || len(a.Actions) == 0 {
		errs.ErrChan <- "could not find any ABIs"
		return
	}
	found := 0
	if !newGroup {
		for _, l := range ActionLabels {
			if l != nil {
				l.SetText("disabled")
				l.Hide()
			}
		}
		for _, b := range ActionButtons {
			if b != nil {
				b.SetText("disabled")
				b.Hide()
			}
		}
	}
	for _, contract := range a.Index {
		label := widget.NewLabel(strings.ToUpper(contract))
		actionsGroup.Append(label)
		ActionLabels = append(ActionLabels, label)
		sort.Strings(a.Actions[contract])
		for _, b := range a.Actions[contract] {
			button := &widget.Button{}
			button = widget.NewButton(fmt.Sprintf("%s::%s", contract, b), func() {
				tabContent.SelectTabIndex(4)
				if form, e := explorer.GetAbiForm(button.Text, account, api, opts); e == nil {
					tabEntries.Editor.Content = form
					tabEntries.Editor.Text = "Action - " + button.Text
					errs.RefreshChan <- true
				}
			})
			ActionButtons = append(ActionButtons, button)
			button.Style = 0
			actionsGroup.Append(button)
			found = found + 1
		}
	}
	if found > 0 {
		errs.ErrChan <- fmt.Sprintf("found %d actions", found)
		tabEntries.Info = *widget.NewTabItem("Server",
			widget.NewVBox(
				serverInfoBox,
			))
		if browser, ok := explorer.GetTableBrowser(explorer.W, explorer.H, api); ok {
			tabEntries.Browser = *widget.NewTabItem("Tables", browser)
		}
		if abiView, ok := explorer.GetAbiViewer(explorer.W, explorer.H, api); ok {
			tabEntries.Abi = *widget.NewTabItem("ABIs", abiView)
		}
		updateTabChan := make(chan fyne.Container)
		go func(newBox chan fyne.Container) {
			for {
				select {
				case nb := <-newBox:
					tabEntries.AccountInfo = *widget.NewTabItem("Accounts", &nb)
					errs.RefreshChan <- true
				}
			}
		}(updateTabChan)
		updateApiChan := make(chan fyne.Container)
		go func(newApiTab chan fyne.Container) {
			for {
				select {
				case nb := <-newApiTab:
					tabEntries.Api = *widget.NewTabItem("APIs", &nb)
					errs.RefreshChan <- true
				}
			}
		}(updateApiChan)
		explorer.NewAccountSearchTab(updateTabChan, account)
		explorer.NewApiRequestTab(updateApiChan)
		updateMsigChan := make(chan fyne.Container)
		go func(newMsigTab chan fyne.Container) {
			for {
				select {
				case nb := <-newMsigTab:
					tabEntries.Msig = *widget.NewTabItem("mSig", &nb)
					// TODO: use a cancel context to timeout if nothing is listening on the channel:
					go func() {
						time.Sleep(time.Second)
						if explorer.MsigLoaded {
							explorer.MsigRefreshRequests <- false
						}
					}()
					errs.RefreshChan <- true
				}
			}
		}(updateMsigChan)
		go func() {
			time.Sleep(2 * time.Second)
			explorer.UpdateAuthContent(updateMsigChan, api, opts, account)
		}()
		updateVoteChan := make(chan fyne.CanvasObject)
		go func(content chan fyne.CanvasObject) {
			for {
				select {
				case c := <-content:
					tabEntries.Vote = *widget.NewTabItem("Vote", fyne.NewContainerWithLayout(
						layout.NewFixedGridLayout(fyne.NewSize(explorer.RWidth(), explorer.PctHeight()-250)),
						c,
					))
					tabContent.Refresh()
				}
			}
		}(updateVoteChan)
		go func() {
			time.Sleep(4 * time.Second)
			explorer.VoteContent(updateVoteChan, explorer.RefreshVotesChan)
		}()
		updateRequestChan := make(chan fyne.CanvasObject)
		go func(content chan fyne.CanvasObject) {
			for {
				select {
				case c := <-content:
					tabEntries.Requests = *widget.NewTabItem("Requests", fyne.NewContainerWithLayout(
						layout.NewFixedGridLayout(fyne.NewSize(explorer.RWidth(), explorer.PctHeight()-250)),
						c,
					))
					tabContent.Refresh()
				}
			}
		}(updateRequestChan)
		go func() {
			time.Sleep(3 * time.Second)
			explorer.RequestContent(updateRequestChan, explorer.RefreshRequestsChan)
		}()
		showHideActions("")
		errs.RefreshChan <- true
		connectedChan <- true
	}
}

func showHideActions(s string) {
	for _, b := range ActionButtons {
		if b == nil {
			continue
		}
		switch {
		case b.Text == "disabled":
			b.Hide()
		case (s != "" && !strings.Contains(b.Text, s)) || (filterCheck.Checked && explorer.PrivilegedActions[b.Text]) ||
			(prodsCheck.Checked && explorer.ProducerActions[b.Text]):
			b.Hide()
		default:
			b.Show()
		}
	}
	for _, l := range ActionLabels {
		if l == nil {
			continue
		}
		if l.Text == "disabled" {
			l.Hide()
		}
	}
}

func moneySlice() []string {
	o := make([]string, 0)
	for k := range savedKeys {
		o = append(o, k)
	}
	sort.Strings(o)
	return o
}

func makeTabs() tabs {
	return tabs{
		Editor:      *widget.NewTabItem("Actions", widget.NewLabel("Select a Contract Action on the left")),
		Api:         *widget.NewTabItem("APIs", widget.NewLabel("Not Connected")),
		Info:        *widget.NewTabItem("Server", widget.NewLabel("Not Connected")),
		Browser:     *widget.NewTabItem("Tables", widget.NewLabel("Not Connected")),
		Abi:         *widget.NewTabItem("ABIs", widget.NewLabel("Not Connected")),
		AccountInfo: *widget.NewTabItem("Accounts", widget.NewLabel("Not Connected")),
		KeyGen:      *widget.NewTabItem("Key Gen", widget.NewLabel("")),
		Msig:        *widget.NewTabItem("mSig", widget.NewLabel("Not Connected")),
		Vote:        *widget.NewTabItem("Vote", widget.NewLabel("Not Connected")),
		Requests:    *widget.NewTabItem("Requests", widget.NewLabel("Not Connected")),
	}
}

func uriModal() {
	if explorer.PasswordVisible {
		return
	}
	go uriSimplerInput()
}

func uriSimplerInput() {
	nw := explorer.App.NewWindow("new connection")
	hEntry := &explorer.EnterEntry{}
	cancelButton := widget.NewButton("cancel", func() {
		nw.Hide()
	})
	connectButton = widget.NewButtonWithIcon("connect", fioassets.NewFioLogoResource(), func() {
		connectButton.Disable()
		myFioAddress.OnChanged = func(string) {
			myFioAddress.SetText("")
		}
		explorer.DefaultFioAddress = ""
		myFioAddress.SetText("")
		myFioAddress.Hide()
		host := hostEntry.Text
		if !strings.HasPrefix(host, "http") {
			host = "http://" + host
		}
		*uri = host
		if reconnect(account) {
			updateActions(true, opts)
			nw.Hide()
			nw = nil
		} else {
			connectButton.Enable()
		}
	})
	hostEntry.Button = connectButton
	hEntry = explorer.NewEnterEntry(func() {
		connectButton.Disable()
		host := hostEntry.Text
		if !strings.HasPrefix(host, "http") {
			host = "http://" + host
		}
		*uri = host
		if reconnect(account) {
			updateActions(true, opts)
			nw.Hide()
			nw = nil
		} else {
			connectButton.Enable()
		}
	})
	mainSelect := widget.NewSelect(explorer.MainnetApi, func(s string) {
		hostEntry.SetText(s)
	})
	mainSelect.PlaceHolder = "Mainnet Nodes"
	hEntry.Text = *uri
	nw.SetContent(widget.NewVBox(
		layout.NewSpacer(),
		widget.NewHBox(layout.NewSpacer(), hostEntry, cancelButton, connectButton, layout.NewSpacer()),
		widget.NewHBox(layout.NewSpacer(), mainSelect, layout.NewSpacer()),
		layout.NewSpacer()),
	)
	nw.Resize(fyne.NewSize(400, 200))
	//nw.SetFixedSize(true)
	nw.Show()
}

func uriInput(showProxy bool) *widget.Box {
	hostEntry.Text = *uri
	proxyProto := widget.NewSelect([]string{"http://", "https://"}, func(s string) {})
	proxyProto.Hide()
	proxyUrl := &widget.Entry{}
	proxyUrl = widget.NewEntry()
	proxyUrl.OnChanged = func(s string) {
		*proxy = s
	}
	proxyUrl.SetText(*proxy)
	proxyUrl.Hide()
	proxyCheck = widget.NewCheck("Use a proxy", func(b bool) {
		switch b {
		case true:
			proxyProto.Show()
			proxyProto.SetSelected("http://")
			proxyUrl.Show()
		default:
			proxyProto.Hide()
			proxyUrl.Hide()
		}
	})
	if !showProxy || os.Getenv("ADVANCED") == "" {
		proxyCheck.Hide()
	}
	connectButton = widget.NewButtonWithIcon("connect", fioassets.NewFioLogoResource(), func() {
		connectButton.Disable()
		if proxyCheck.Checked {
			if _, e := url.Parse(proxyProto.Selected + proxyUrl.Text); e != nil {
				errs.ErrChan <- "invalid proxy URL specified"
				connectButton.Enable()
				return
			}
			if strings.Contains(hostEntry.Text, "127.0.0.1") || strings.Contains(hostEntry.Text, "localhost") {
				errs.ErrChan <- "Warning: will not use proxy for connections to localhost"
			}
			// all we need to do is set the ENV vars and eos-go will pick up our settings
			refreshNotNil(proxyUrl)
			switch proxyProto.Selected {
			case "https://":
				os.Unsetenv("HTTP_PROXY")
				os.Unsetenv("HTTPS_PROXY")
				os.Setenv("HTTPS_PROXY", proxyProto.Selected+*proxy)
			default:
				os.Unsetenv("HTTP_PROXY")
				os.Unsetenv("HTTPS_PROXY")
				os.Setenv("HTTP_PROXY", proxyProto.Selected+*proxy)
			}
		}
		host := hostEntry.Text
		if !strings.HasPrefix(host, "http") {
			host = "http://" + host
		}
		*uri = host
		if reconnect(account) {
			updateActions(true, opts)
		} else {
			connectButton.Enable()
		}
	})
	hostEntry.Button = connectButton
	if !showProxy {
		connectButton.Hide()
	}

	return widget.NewHBox(connectButton, hostEntry, proxyCheck, proxyProto, proxyUrl)
}

var (
	refreshWorkerCounter int
)

func keyBoxContent() *widget.Box {
	doImport := func() {}
	entryChan := make(chan string)
	moneyBags.OnChanged = func(s string) {
		if s != "" && s != moneyBags.PlaceHolder && wifEntry != nil {
			entryChan <- s
		}
	}
	moneyBags.PlaceHolder = "Quick Load Saved Key"
	moneyBags.Refresh()
	pubkey := widget.NewEntry()
	func(s string) {
		pubkey.OnChanged = func(string) {
			pubkey.SetText(s)
		}
	}(account.PubKey) // deref
	pubkey.SetText(account.PubKey)
	actor := widget.NewEntry()
	func(s string) {
		actor.OnChanged = func(string) {
			actor.SetText(s)
		}
	}(string(account.Actor)) // deref
	actor.SetText(string(account.Actor))
	var txt string
	if account.Addresses != nil && len(account.Addresses) > 0 {
		txt = account.Addresses[0].FioAddress
	} else {
		myFioAddress.Hide()
	}
	func(s string) {
		myFioAddress.OnChanged = func(string) {
			myFioAddress.SetText(s)
		}
	}(txt)
	myFioAddress.SetText(txt)

	wifWindow := explorer.App.NewWindow("Import WIF")
	doImport = func() {
		explorer.Win.RequestFocus()
		newAcc, err := fio.NewAccountFromWif(wifEntry.Text)
		errs.ErrChan <- "Importing new WIF ..."
		if err != nil {
			errs.ErrChan <- "import failed: " + err.Error()
			//wifWindow.Hide()
			return
		}
		tabContent.SelectTabIndex(1)
		myFioAddress.OnChanged = func(string) {
			myFioAddress.SetText("")
		}
		myFioAddress.SetText("")
		myFioAddress.Hide()
		derefAcc := *newAcc
		account = &derefAcc
		explorer.Account = &derefAcc
		func(s string) {
			actor.OnChanged = func(string) {
				actor.SetText(s)
			}
		}(string(account.Actor))
		actor.SetText(string(account.Actor))
		updateActions(reconnect(account), opts)
		func(s string) {
			pubkey.OnChanged = func(string) {
				pubkey.SetText(s)
			}
		}(account.PubKey)
		pubkey.SetText(account.PubKey)
		myFioAddress.OnChanged = func(string) {
			myFioAddress.SetText("")
		}
		myFioAddress.SetText("")
		if !myFioAddress.Hidden {
			myFioAddress.Hide()
			go refreshMyName()
			explorer.RefreshVotesChan <- true
			explorer.RefreshRequestsChan <- true
		}
	}

	importButton = widget.NewButton("Import", func() {
		doImport()
		wifWindow.Close()
	})
	loadButton = widget.NewButtonWithIcon("Load Key", theme.MenuDropUpIcon(), func() {
		wifEntry.SetPlaceHolder("5xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
		wifWindow := explorer.App.NewWindow("Import WIF")
		wifWindow.Resize(fyne.NewSize(450, 180))
		keyBox = widget.NewVBox(
			wifEntry,
			layout.NewSpacer(),
			widget.NewHBox(
				widget.NewButton("Import", func() {
					doImport()
					wifWindow.Close()
				}),
				widget.NewButton(
					"Cancel", func() {
						wifWindow.Close()
						explorer.Win.RequestFocus()
						return
					},
				),
				layout.NewSpacer(), moneyBags,
			),
		)
		wifWindow.SetContent(keyBox)
		wifWindow.Show()
	})

	balanceButton = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		errs.ErrChan <- "refreshing"
		explorer.BalanceChan <- true
		refreshMyName()
	})
	balanceButton.Disable()
	updateBal := func() {
		if !explorer.Connected {
			balanceButton.Disable()
		}
		if account != nil && api.BaseURL != "" {
			if !balanceButton.Disabled() {
				balanceButton.Disable()
			}
			fioBalance, e := api.GetBalance(account.Actor)
			balanceButton.Enable()
			if e != nil {
				errs.ErrChan <- "Error getting balance: " + e.Error()
				return
			}
			if explorer.TxResultBalanceChanOpen {
				explorer.TxResultBalanceChan <- p.Sprintf("FIO Balance:\n%.9g", fioBalance)
			}
			if balance != fioBalance {
				errs.ErrChan <- p.Sprintf("balance changed by: %f", fioBalance-balance)
				balanceLabel.SetText(p.Sprintf("FIO Balance: %.9g", fioBalance))
				balanceLabel.Refresh()
				balance = fioBalance
			}
		}
	}
	go func() {
		refreshWorkerCounter += 1
		balance = 0.0
		tick := time.NewTicker(30 * time.Second)
		infoTickDuration := 3 * time.Second
		infoTick := time.NewTicker(infoTickDuration)
		for {
			select {
			case <-tick.C:
				if errs.Connected {
					updateBal()
				}
			case <-infoTick.C:
				// try to catch a race when loading settings since this can get called twice in some circumstances
				if refreshWorkerCounter > 1 {
					refreshWorkerCounter -= 1
					return
				}

				if explorer.Connected && tabContent.CurrentTab().Text == "Server" {
					if _, updated := refreshInfo(infoTickDuration / time.Duration(2)); updated {
						if infoTickDuration > 2*time.Second {
							infoTickDuration -= time.Second
							infoTick = time.NewTicker(infoTickDuration)
						}
						continue
					}
					// server is responding slowly, poll less frequently
					infoTickDuration = infoTickDuration * 2
					infoTick = time.NewTicker(infoTickDuration)
				}
			case <-explorer.BalanceChan:
				if errs.Connected {
					updateBal()
				}
			case newWif := <-entryChan:
				infoTickDuration = time.Second
				infoTick = time.NewTicker(infoTickDuration)
				if savedKeys[newWif] != "" {
					wifEntry.SetText(savedKeys[newWif])
				}
			}
		}
	}()

	return widget.NewVBox(
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(explorer.RWidth(), 90)),
			fyne.NewContainerWithLayout(layout.NewGridLayoutWithRows(2),
				widget.NewHBox(
					widget.NewLabel("Current Key: "),
					pubkey,
					actor,
					myFioAddress,
				),
				widget.NewHBox(balanceLabel, balanceButton, loadButton),
			),
		),
	)
}
