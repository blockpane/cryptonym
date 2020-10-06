package cryptonym

import (
	"errors"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/canvas"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	fioassets "github.com/blockpane/cryptonym/assets"
	errs "github.com/blockpane/cryptonym/errLog"
	"net/url"
	"os"
	"strconv"
)

const settingsTitle = "Cryptonym Settings"

var MainnetApi = []string{
	// These allow TLS 1.0, excluding
	//"https://fio.eoscannon.io",
	//"https://fio-mainnet.eosblocksmith.io",
	//"https://fio.eos.barcelona",
	//"https://fio.eosargentina.io",
	//"https://api.fio.services",

	// Does not allow access to get_supported_apis endpoint:
	//"https://fioapi.nodeone.io",
	//"https://fio.maltablock.org",

	"https://fio.eosdac.io",           //ok
	"https://fio.eosphere.io",         //ok
	"https://fio.eosrio.io",           //ok
	"https://fio.eosusa.news",         //ok
	"https://api.fio.alohaeos.com",    //ok
	"https://fio.genereos.io",         //ok
	"https://fio.greymass.com",        //ok
	"https://api.fio.eosdetroit.io",   // ok
	"https://fio.zenblocks.io",        // ok
	"https://api.fio.currencyhub.io",  // ok
	"https://fio.cryptolions.io",      // ok
	"https://fio.eosdublin.io",        // ok
	"https://api.fio.greeneosio.com",  // ok
	"https://api.fiosweden.org",       // ok
	"https://fio.eu.eosamsterdam.net", //ok
	"https://fioapi.ledgerwise.io",    // sort of ok, lots of errors
	"https://fio.acherontrading.com",  //ok
}

func SettingsWindow() {
	if PasswordVisible {
		return
	}
	w := App.NewWindow(settingsTitle)
	w.Resize(fyne.NewSize(600, 800))
	w.SetOnClosed(func() {
		for _, w := range fyne.CurrentApp().Driver().AllWindows() {
			if w.Title() == AppTitle {
				w.RequestFocus()
				return
			}
		}
	})

	if Settings == nil || Settings.Server == "" {
		Settings = DefaultSettings()
	}

	var filename string
	updateFieldsFromSettings := func() {}
	settingsFileLabel := widget.NewLabel("")
	serverEntry := widget.NewEntry()
	proxyEntry := widget.NewEntry()
	widthEntry := widget.NewEntry()
	heightEntry := widget.NewEntry()
	tpidEntry := widget.NewEntry()
	advanced := widget.NewCheck("Enable Advanced (expert) Features", func(b bool) {
		Settings.AdvancedFeatures = b
		if b {
			_ = os.Setenv("ADVANCED", "true")
			return
		}
		_ = os.Setenv("ADVANCED", "")
	})
	if os.Getenv("ADVANCED") != "" {
		advanced.SetChecked(true)
	}

	sizeRow := widget.NewHBox(
		layout.NewSpacer(),
		widget.NewLabel("Initial window size: "),
		widthEntry,
		widget.NewLabel(" X "),
		heightEntry,
		widget.NewLabelWithStyle(" (requires restart)", fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
		layout.NewSpacer(),
	)

	advancedRow := widget.NewHBox(
		layout.NewSpacer(),
		advanced,
		layout.NewSpacer(),
	)

	themeSelect := widget.NewSelect([]string{"Dark", "Darker", "Grey", "Light"}, func(s string) {
		switch s {
		case "Dark":
			fyne.CurrentApp().Settings().SetTheme(CustomTheme())
			RepaintChan <- true
		case "Light":
			fyne.CurrentApp().Settings().SetTheme(ExLightTheme().ToFyneTheme())
			RepaintChan <- true
		case "Darker":
			fyne.CurrentApp().Settings().SetTheme(DarkerTheme().ToFyneTheme())
			RepaintChan <- true
		case "Grey":
			fyne.CurrentApp().Settings().SetTheme(ExGreyTheme().ToFyneTheme())
			RepaintChan <- true
		}
		WinSettings.T = s
		RefreshQr <- true
	})

	defKeyEntry := widget.NewPasswordEntry()
	defKeyEntry.SetPlaceHolder("WIF Private Key")
	defKeyDescEntry := widget.NewEntry()
	defKeyDescEntry.SetPlaceHolder("Description")

	favKey2Entry := widget.NewPasswordEntry()
	favKey2Entry.SetPlaceHolder("WIF Private Key")
	favKey2DescEntry := widget.NewEntry()
	favKey2DescEntry.SetPlaceHolder("Description")

	favKey3Entry := widget.NewPasswordEntry()
	favKey3Entry.SetPlaceHolder("WIF Private Key")
	favKey3DescEntry := widget.NewEntry()
	favKey3DescEntry.SetPlaceHolder("Description")

	favKey4Entry := widget.NewPasswordEntry()
	favKey4Entry.SetPlaceHolder("WIF Private Key")
	favKey4DescEntry := widget.NewEntry()
	favKey4DescEntry.SetPlaceHolder("Description")

	msigDefaultEntry := widget.NewEntry()
	msigDefaultEntry.SetPlaceHolder("abcdefghi")

	defaultsButton := widget.NewButton("Load Defaults", func() {
		Settings = DefaultSettings()
		updateFieldsFromSettings()
	})

	passEntry := widget.NewPasswordEntry()
	passConfirm := widget.NewPasswordEntry()
	saveButton := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		if passEntry.Text == "" {
			return
		}
		updateSize := true
		width, err := strconv.Atoi(widthEntry.Text)
		if err != nil {
			updateSize = false
			errs.ErrChan <- "Settings: got invalid width setting for window size"
		}
		height, err := strconv.Atoi(heightEntry.Text)
		if err != nil {
			updateSize = false
			errs.ErrChan <- "Settings: got invalid height setting for window size"
		}

		Settings.Server = serverEntry.Text
		Settings.Proxy = proxyEntry.Text
		Settings.DefaultKey = defKeyEntry.Text
		Settings.DefaultKeyDesc = defKeyDescEntry.Text
		Settings.FavKey2 = favKey2Entry.Text
		Settings.FavKey2Desc = favKey2DescEntry.Text
		Settings.FavKey3 = favKey3Entry.Text
		Settings.FavKey3Desc = favKey3DescEntry.Text
		Settings.FavKey4 = favKey4Entry.Text
		Settings.FavKey4Desc = favKey4DescEntry.Text
		Settings.MsigAccount = msigDefaultEntry.Text
		Settings.AdvancedFeatures = advanced.Checked
		if Settings.AdvancedFeatures {
			_ = os.Setenv("ADVANCED", "true")
		}
		Settings.Tpid = tpidEntry.Text
		ok, err := SaveEncryptedSettings(passEntry.Text, Settings)
		if ok {
			if updateSize {
				if ok := saveWindowSettings(width, height, themeSelect.Selected); !ok {
					errs.ErrChan <- "Settings: was unable to save window size."
				}
			}
			SettingsLoaded <- Settings
			w.Close()
			return
		}
		msg := "Could not save config file! "
		if err != nil {
			msg = msg + err.Error()
		}
		myWindow := func() fyne.Window {
			for _, window := range App.Driver().AllWindows() {
				if window.Title() == settingsTitle {
					return window
				}
			}
			return App.Driver().AllWindows()[0]
		}
		dialog.ShowError(errors.New(msg), myWindow())
	})
	saveButton.Disable()

	warningIcon := fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(24, 24)),
		canvas.NewImageFromResource(theme.WarningIcon()),
	)
	matchWarningIcon := fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(24, 24)),
		canvas.NewImageFromResource(theme.WarningIcon()),
	)
	matchWarningIcon.Hide()
	passEntry.SetPlaceHolder("please enter a password")
	passEntry.OnChanged = func(s string) {
		if len(s) < 8 {
			warningIcon.Show()
			saveButton.Disable()
			return
		}
		warningIcon.Hide()
		if passConfirm.Text != s {
			matchWarningIcon.Show()
			saveButton.Disable()
			return
		}
		matchWarningIcon.Hide()
		warningIcon.Hide()
		saveButton.Enable()
	}
	passConfirm.SetPlaceHolder("confirm the password")
	passConfirm.OnChanged = func(s string) {
		if len(passEntry.Text) < 8 {
			warningIcon.Show()
			saveButton.Disable()
			return
		}
		warningIcon.Hide()
		if passEntry.Text != s {
			matchWarningIcon.Show()
			saveButton.Disable()
			return
		}
		matchWarningIcon.Hide()
		warningIcon.Hide()
		saveButton.Enable()
	}

	cancelButton := widget.NewButton("Cancel", func() {
		w.Close()
	})

	updateFieldsFromSettings = func() {
		heightEntry.SetText(fmt.Sprint(H))
		widthEntry.SetText(fmt.Sprint(W))
		serverEntry.SetText(Settings.Server)
		proxyEntry.SetText(Settings.Proxy)
		defKeyEntry.SetText(Settings.DefaultKey)
		defKeyDescEntry.SetText(Settings.DefaultKeyDesc)
		favKey2Entry.SetText(Settings.FavKey2)
		favKey2DescEntry.SetText(Settings.FavKey2Desc)
		favKey3Entry.SetText(Settings.FavKey3)
		favKey3DescEntry.SetText(Settings.FavKey3Desc)
		favKey4Entry.SetText(Settings.FavKey4)
		favKey4DescEntry.SetText(Settings.FavKey4Desc)
		msigDefaultEntry.SetText(Settings.MsigAccount)
		tpidEntry.SetText(Settings.Tpid)
		advanced.SetChecked(Settings.AdvancedFeatures)
		if Settings.AdvancedFeatures {
			_ = os.Setenv("ADVANCED", "true")
		}
		themeSelect.Selected = WinSettings.T
		themeSelect.Refresh()
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		w.SetContent(widget.NewLabel("Unable to determine config directory: " + err.Error()))
	} else {
		filename = fmt.Sprintf("%s%c%s%c%s", configDir, os.PathSeparator, settingsDir, os.PathSeparator, settingsFileName)
		settingsFileLabel.SetText(filename)
		w.SetContent(
			widget.NewVBox(
				fyne.NewContainerWithLayout(layout.NewGridLayoutWithRows(2),
					fyne.NewContainerWithLayout(layout.NewGridLayout(2),
						widget.NewHBox(layout.NewSpacer(), warningIcon, widget.NewLabel("Password for encryption: ")),
						passEntry,
					),
					fyne.NewContainerWithLayout(layout.NewGridLayout(2),
						widget.NewHBox(layout.NewSpacer(), matchWarningIcon, widget.NewLabel("Confirm: ")),
						passConfirm,
					),
				),
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(50, 50)), layout.NewSpacer()),
				widget.NewHBox(
					layout.NewSpacer(),
					widget.NewLabelWithStyle("Preferred Server", fyne.TextAlignTrailing, fyne.TextStyle{}),
					serverEntry, layout.NewSpacer(),
				),
				//widget.NewHBox(
				//	layout.NewSpacer(),
				//	widget.NewLabelWithStyle("Preferred Proxy", fyne.TextAlignTrailing, fyne.TextStyle{}),
				//	proxyEntry, layout.NewSpacer(),
				//),
				widget.NewHBox(
					layout.NewSpacer(),
					widget.NewLabelWithStyle("Default Theme", fyne.TextAlignTrailing, fyne.TextStyle{}),
					themeSelect, layout.NewSpacer(),
				),
				sizeRow,
				advancedRow,
				widget.NewHBox(
					layout.NewSpacer(),
					widget.NewLabelWithStyle("Preferred MSIG Account for Proposals", fyne.TextAlignTrailing, fyne.TextStyle{}),
					msigDefaultEntry, layout.NewSpacer(),
				),
				widget.NewHBox(
					layout.NewSpacer(),
					widget.NewLabelWithStyle("Preferred TPID", fyne.TextAlignTrailing, fyne.TextStyle{}),
					tpidEntry, layout.NewSpacer(),
				),
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(50, 50)), layout.NewSpacer()),
				widget.NewHBox(
					widget.NewLabelWithStyle("*Quick Load Key", fyne.TextAlignLeading, fyne.TextStyle{}),
					widget.NewHBox(
						defKeyEntry, layout.NewSpacer(), defKeyDescEntry,
					),
					layout.NewSpacer(),
				),
				widget.NewHBox(
					widget.NewLabelWithStyle(" Quick Load Key  ", fyne.TextAlignLeading, fyne.TextStyle{}),
					widget.NewHBox(
						favKey2Entry, layout.NewSpacer(), favKey2DescEntry,
					),
					layout.NewSpacer(),
				),
				widget.NewHBox(
					widget.NewLabelWithStyle(" Quick Load Key  ", fyne.TextAlignLeading, fyne.TextStyle{}),
					widget.NewHBox(
						favKey3Entry, layout.NewSpacer(), favKey3DescEntry,
					),
					layout.NewSpacer(),
				),
				widget.NewHBox(
					widget.NewLabelWithStyle(" Quick Load Key  ", fyne.TextAlignLeading, fyne.TextStyle{}),
					widget.NewHBox(
						favKey4Entry, layout.NewSpacer(), favKey4DescEntry,
					),
					layout.NewSpacer(),
				),
				widget.NewLabelWithStyle("* - Default Key", fyne.TextAlignCenter, fyne.TextStyle{}),
				fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(50, 50)), layout.NewSpacer()),
				layout.NewSpacer(),
				widget.NewHBox(layout.NewSpacer(), saveButton, defaultsButton, cancelButton, layout.NewSpacer()),
				widget.NewHBox(
					layout.NewSpacer(),
					widget.NewLabelWithStyle("Config File Location", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
					settingsFileLabel,
					layout.NewSpacer(),
				),
			),
		)
	}
	updateFieldsFromSettings()
	w.Show()
}

type unlockEntry struct {
	widget.Entry
	Action func(bool)
}

func (e *unlockEntry) onEnter() {
	e.Action(true)
}

func newUnlockEntry(f func(bool)) *unlockEntry {
	entry := &unlockEntry{
		Entry:  widget.Entry{Password: true},
		Action: f,
	}
	entry.ExtendBaseWidget(entry)
	return entry
}

func (e *unlockEntry) KeyDown(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyReturn:
		e.onEnter()
	default:
		e.Entry.KeyDown(key)
	}
}

func PromptForPassword() {
	if PasswordVisible {
		return
	}
	PasswordVisible = true
	defer func() { PasswordVisible = false }()
	ok, fileLen, _, err := LoadEncryptedSettings("")
	if !ok && fileLen == 0 && err == nil {
		//no config file to load, just continue.
		return
	}
	var serverOverride string
	mainnetSelect := widget.NewSelect(MainnetApi, func(s string) {
		if s != "" {
			serverOverride = s
		}
	})
	mainnetSelect.PlaceHolder = "Override Saved Server"
	deferCheck := widget.NewCheck("defer connect", func(b bool) {
		if b {
			mainnetSelect.Hide()
			return
		}
		mainnetSelect.Show()
	})
	resultLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	resultBox := widget.NewHBox(
		layout.NewSpacer(),
		fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(35, 35)),
			canvas.NewImageFromResource(theme.WarningIcon()),
		),
		resultLabel,
		layout.NewSpacer(),
	)
	resultBox.Hide()
	passCallBack := func(b bool) {}
	pop := &widget.PopUp{}

	passEntry := newUnlockEntry(func(b bool) {})
	passEntry.SetPlaceHolder("Enter your password")
	passEntry.OnChanged = func(s string) {
		resultBox.Hide()
	}
	cancelButton := widget.NewButtonWithIcon(" Defaults ", theme.CancelIcon(), func() {
		pop.Hide()
	})
	submitButton := widget.NewButtonWithIcon(" Load ", theme.ConfirmIcon(), func() {
		passCallBack(true)
	})

	link, _ := url.Parse("https://fioprotocol.io/")
	contents := widget.NewVBox(
		widget.NewLabelWithStyle("Enter your password to load saved settings", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewHBox(
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(150, 250)),
				widget.NewVBox(
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(140, 20)), layout.NewSpacer()),
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(140, 140)),
						canvas.NewImageFromResource(fioassets.NewFioLogoResource()),
					),
					widget.NewHyperlinkWithStyle("fioprotocol.io", link, fyne.TextAlignCenter, fyne.TextStyle{}),
					layout.NewSpacer(),
				),
			),
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(400, 280)),
				widget.NewVBox(
					layout.NewSpacer(),
					widget.NewHBox(layout.NewSpacer(), resultBox, widget.NewLabel(""), layout.NewSpacer()),
					layout.NewSpacer(),
					widget.NewHBox(layout.NewSpacer(), passEntry, layout.NewSpacer()),
					layout.NewSpacer(),
					fyne.NewContainerWithLayout(layout.NewGridLayout(4),
						layout.NewSpacer(), cancelButton, submitButton, layout.NewSpacer(),
					),
					widget.NewHBox(deferCheck),
					widget.NewHBox(mainnetSelect),
					widget.NewHBox(widget.NewLabel(" ")),
				),
			),
		),
	)
	passCallBack = func(b bool) {
		if !b {
			return
		}
		passEntry.Disable()
		defer passEntry.Enable()
		ok, _, newConfig, err := LoadEncryptedSettings(passEntry.Text)
		switch {
		case err != nil && err.Error() == "cipher: message authentication failed":
			resultLabel.SetText("Incorrect Password")
			resultBox.Show()
		case err != nil:
			resultLabel.SetText(err.Error())
			resultBox.Show()
		case !ok:
			resultLabel.SetText("Could not decrypt settings")
			resultBox.Show()
		case ok:
			Settings.Server = newConfig.Server
			if deferCheck.Checked {
				Settings.Server = ""
			} else if serverOverride != "" {
				Settings.Server = serverOverride
			}
			Settings.Proxy = newConfig.Proxy
			Settings.DefaultKey = newConfig.DefaultKey
			Settings.DefaultKeyDesc = newConfig.DefaultKeyDesc
			Settings.FavKey2 = newConfig.FavKey2
			Settings.FavKey2Desc = newConfig.FavKey2Desc
			Settings.FavKey3 = newConfig.FavKey3
			Settings.FavKey3Desc = newConfig.FavKey3Desc
			Settings.FavKey4 = newConfig.FavKey4
			Settings.FavKey4Desc = newConfig.FavKey4Desc
			Settings.MsigAccount = newConfig.MsigAccount
			Settings.Tpid = newConfig.Tpid

			SettingsLoaded <- Settings
			pop.Hide()
			return
		}
		//show()
	}
	pop = widget.NewModalPopUp(contents, Win.Canvas())
	passEntry.Action = passCallBack
}

type EnterEntry struct {
	widget.Entry
	Action func()
}

func (e *EnterEntry) onEnter() {
	e.Action()
}

func NewEnterEntry(f func()) *EnterEntry {
	entry := &EnterEntry{
		Entry:  widget.Entry{},
		Action: f,
	}
	entry.ExtendBaseWidget(entry)
	return entry
}

func (e *EnterEntry) KeyDown(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyReturn:
		e.onEnter()
	default:
		e.Entry.KeyDown(key)
	}
}

type EnterSelectEntry struct {
	widget.SelectEntry
	Action func()
}

func (e *EnterSelectEntry) onEnter() {
	e.Action()
}

func NewEnterSelectEntry(entries []string, f func()) *EnterSelectEntry {
	entry := &EnterSelectEntry{Action: f}
	entry.SetOptions(entries)
	entry.ExtendBaseWidget(entry)
	return entry
}

func (e *EnterSelectEntry) KeyDown(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyReturn:
		e.onEnter()
	default:
		e.Entry.KeyDown(key)
	}
}
