package cryptonym

import (
	"bytes"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/canvas"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"github.com/fioprotocol/fio-go/eos/ecc"
	"github.com/skip2/go-qrcode"
	"image"
	"image/color"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var imageSize = func() (size int) {
	size = 196
	scale := os.Getenv("FYNE_SCALE")
	if scale != "" {
		new, err := strconv.Atoi(scale)
		if err != nil {
			return
		}
		size = ((new * 10) * size) / 10
	}
	return
}()

var RefreshQr = make(chan bool)

func KeyGenTab() *widget.Box {

	keyStr := ""
	waitMsg := "      ... waiting for entropy from operating system ...              "
	entry := widget.NewEntry()
	var key *ecc.PrivateKey
	var showingPriv bool
	var err error

	vanityQuit := make(chan bool)
	vanityOpt := &vanityOptions{}
	vanityOpt.threads = runtime.NumCPU()
	vanitySearch := widget.NewSelect([]string{"Actor", "Pubkey", "Either"}, func(s string) {
		switch s {
		case "Actor":
			vanityOpt.actor = true
			vanityOpt.pub = false
		case "Pubkey":
			vanityOpt.actor = false
			vanityOpt.pub = true
		default:
			vanityOpt.actor = true
			vanityOpt.pub = true
		}
	})
	vanitySearch.SetSelected("Actor")
	vanitySearch.Hide()
	vanityMatch := widget.NewCheck("match anywhere", func(b bool) {
		if b {
			vanityOpt.anywhere = true
			return
		}
		vanityOpt.anywhere = false
	})
	vanityMatch.Hide()
	vanityLabel := widget.NewLabel(" ")
	vanityEntry := NewClickEntry(&widget.Button{})
	vanityEntry.SetPlaceHolder("enter string to search for")
	vanityEntry.OnChanged = func(s string) {
		vanityLabel.SetText(" ")
		if len(s) >= 6 {
			vanityLabel.SetText("Note: searching for 6 or more characters can take a very long time.")
		}
		vanityOpt.word = strings.ToLower(s)
		vanityLabel.Refresh()
	}
	vanityEntry.Hide()
	vanityStopButton := &widget.Button{}
	vanityStopButton = widget.NewButtonWithIcon("Stop Searching", theme.CancelIcon(), func() {
		vanityQuit <- true
		entry.SetText("Vanity key generation cancelled")
		vanityStopButton.Hide()
	})
	vanityStopButton.Hide()
	vanityCheck := widget.NewCheck("Generate Vanity Address", func(b bool) {
		if b {
			waitMsg = "Please wait, generating vanity key"
			vanitySearch.Show()
			vanityMatch.Show()
			vanityEntry.Show()
			return
		}
		waitMsg = "      ... waiting for entropy from operating system ...              "
		vanitySearch.Hide()
		vanityMatch.Hide()
		vanityEntry.Hide()
		vanityStopButton.Hide()
	})
	vanityBox := widget.NewVBox(
		widget.NewHBox(
			layout.NewSpacer(),
			vanityCheck,
			vanitySearch,
			vanityMatch,
			vanityEntry,
			layout.NewSpacer(),
		),
		widget.NewHBox(
			layout.NewSpacer(),
			vanityLabel,
			layout.NewSpacer(),
		),
	)

	emptyQr := disabledImage(imageSize, imageSize)
	qrImage := canvas.NewImageFromImage(emptyQr)
	qrImage.FillMode = canvas.ImageFillOriginal
	newQrPub := image.Image(emptyQr)
	newQrPriv := image.Image(emptyQr)
	copyToClip := widget.NewButton("", nil)
	swapQrButton := widget.NewButton("", nil)

	setWait := func(s string) {
		keyStr = s
		swapQrButton.Disable()
		copyToClip.Disable()
		qrImage.Image = emptyQr
		entry.SetText(keyStr)
		copyToClip.Refresh()
		swapQrButton.Refresh()
		qrImage.Refresh()
		entry.Refresh()
	}
	setWait(waitMsg)

	qrPriv := make([]byte, 0)
	qrPub := make([]byte, 0)
	qrLabel := widget.NewLabel("Public Key:")
	qrLabel.Alignment = fyne.TextAlignCenter

	swapQr := func() {
		switch showingPriv {
		case false:
			swapQrButton.Text = "Show Pub Key QR Code"
			qrLabel.SetText("Private Key:")
			qrImage.Image = newQrPriv
			showingPriv = true
		case true:
			swapQrButton.Text = "Show Priv Key QR Code"
			qrLabel.SetText("Public Key:")
			qrImage.Image = newQrPub
			showingPriv = false
		}
		qrLabel.Refresh()
		qrImage.Refresh()
		swapQrButton.Refresh()
	}
	swapQrButton = widget.NewButtonWithIcon("Show Private Key QR Code", theme.VisibilityIcon(), swapQr)

	regenButton := &widget.Button{}
	newKey := true
	var setBusy bool
	setKey := func() {
		time.Sleep(20 * time.Millisecond) // lame, but prevents a double event on darwin?!?
		if setBusy {
			return
		}
		setBusy = true
		regenButton.Disable()
		go func() {
			defer func() {
				vanityStopButton.Hide()
				regenButton.Enable()
				setBusy = false
			}()
			type ki struct {
				kq []byte
				pq []byte
				k  string
				e  error
			}
			result := make(chan ki)
			go func() {
				if vanityCheck.Checked {
					if vanityOpt.word == "" {
						keyStr = "empty search string provided!"
						result <- ki{}
						return
					}
					vanityStopButton.Show()
					acc, err := vanityKey(vanityOpt, vanityQuit)
					if err != nil {
						keyStr = "Sorry, there was a problem generating the key\n" + err.Error()
						result <- ki{}
						return
					}
					if acc == nil || acc.KeyBag == nil || acc.KeyBag.Keys[0] == nil {
						keyStr = "Sorry, there was a problem generating the key - got empty key!\n"
						result <- ki{}
						return
					}
					vanityStopButton.Hide()
					key = acc.KeyBag.Keys[0]
				} else if newKey {
					key, err = ecc.NewRandomPrivateKey()
					if err != nil {
						keyStr = "Sorry, there was a problem generating the key\n" + err.Error()
						result <- ki{}
						return
					}
				}
				newKey = true
				keyInfo := ki{}
				keyInfo.k, keyInfo.kq, keyInfo.pq, keyInfo.e = genKey(key)
				result <- keyInfo
			}()
			// don't show the wait message right away to avoid flicker:
			tick := time.NewTicker(time.Second)
			var skipSetWait bool
			for {
				select {
				case _ = <-tick.C:
					if skipSetWait {
						return
					}
					qrImage.Image = disabledImage(imageSize, imageSize)
					qrImage.Refresh()
					setWait(waitMsg)
				case ki := <-result:
					if ki.kq == nil {
						return
					}
					skipSetWait = true
					keyStr, qrPriv, qrPub, err = ki.k, ki.kq, ki.pq, ki.e
					if err != nil {
						keyStr = "Sorry, there was a problem generating the key\n" + err.Error()
					}
					qrReaderPriv := bytes.NewReader(qrPriv)
					newQrPriv, _, err = image.Decode(qrReaderPriv)
					if err != nil {
						keyStr = "Sorry, there was a problem generating the qr code\n" + err.Error()
					}

					qrReader := bytes.NewReader(qrPub)
					newQrPub, _, err = image.Decode(qrReader)
					if err != nil {
						keyStr = "Sorry, there was a problem generating the qr code\n" + err.Error()
					}
					qrImage.Image = newQrPub
					swapQrButton.Enable()
					copyToClip.Enable()
					swapQrButton.Refresh()
					copyToClip.Refresh()
					entry.SetText(keyStr)
					entry.OnChanged = func(string) {
						entry.SetText(keyStr)
					}
					qrImage.Refresh()
					showingPriv = true
					swapQr()
					return
				}
			}
		}()
	}

	clipped := func() {
		go func() {
			clip := Win.Clipboard()
			clip.SetContent(keyStr)
			copyToClip.Text = "Copied!"
			if keyStr != clip.Content() {
				clip.SetContent("Failed to Copy!")
			}
			copyToClip.Refresh()
			time.Sleep(2 * time.Second)
			copyToClip.Text = "Copy To Clipboard"
			copyToClip.Refresh()
		}()
	}
	copyToClip = widget.NewButtonWithIcon("Copy To Clipboard", theme.ContentCopyIcon(), clipped)

	go setKey()

	go func() {
		for {
			select {
			case <-RefreshQr:
				newKey = false
				setKey()
			}
		}
	}()

	regenButton = widget.NewButtonWithIcon("Regenerate", theme.ViewRefreshIcon(), setKey)
	vanityEntry.Button = regenButton
	return widget.NewVBox(
		layout.NewSpacer(),
		vanityBox,
		fyne.NewContainerWithLayout(layout.NewGridLayout(3),
			widget.NewHBox(
				layout.NewSpacer(),
				widget.NewVBox(
					qrLabel,
					qrImage,
					swapQrButton,
				),
			),
			widget.NewHBox(
				widget.NewLabel(" "),
				widget.NewVBox(
					layout.NewSpacer(),
					widget.NewHBox(
						entry,
					),
					widget.NewHBox(
						regenButton,
						copyToClip,
						vanityStopButton,
						layout.NewSpacer(),
					),
				),
			),
			layout.NewSpacer(),
		),
		layout.NewSpacer(),
	)
}

func disabledImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rr, gg, bb, _ := App.Settings().Theme().ButtonColor().RGBA()
	c := color.RGBA{R: uint8(rr), G: uint8(gg), B: uint8(bb), A: math.MaxUint8}
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func genKey(key *ecc.PrivateKey) (keyinfo string, privQr []byte, pubQr []byte, e error) {
	var err error
	// convert the WIF into an EOS account structure
	kb := eos.NewKeyBag()
	err = kb.ImportPrivateKey(key.String())
	if err != nil {
		return "", nil, nil, err
	}

	// get the FIO actor name (an EOS account derived from the public key.)
	// in FIO an account has a 1:1 relationship to an account, and the account is
	// created automatically
	fioActor, err := fio.ActorFromPub(kb.Keys[0].PublicKey().String())
	if err != nil {
		return "", nil, nil, err
	}

	// Public key URI formatted
	var pngPub []byte
	qPub, err := qrcode.New("fio:FIO"+kb.Keys[0].PublicKey().String()[3:]+"?label="+string(fioActor), qrcode.Medium)
	if err != nil {
		return "", nil, nil, err
	}
	if strings.Contains(WinSettings.T, "Dark") {
		qPub.ForegroundColor = darkestGrey
		qPub.BackgroundColor = lightestGrey
	} else {
		qPub.ForegroundColor = color.Black
		qPub.BackgroundColor = color.White
	}
	pngPub, err = qPub.PNG(imageSize)
	if err != nil {
		return "", nil, nil, err
	}

	// Private Key
	var pngPriv []byte
	qPriv, err := qrcode.New(kb.Keys[0].String(), qrcode.Medium)
	if err != nil {
		return "", nil, nil, err
	}
	if strings.Contains(WinSettings.T, "Dark") {
		qPriv.ForegroundColor = darkestGrey
		qPriv.BackgroundColor = lightestGrey
	} else {
		qPriv.ForegroundColor = color.Black
		qPriv.BackgroundColor = color.White
	}
	pngPriv, err = qPriv.PNG(imageSize)
	if err != nil {
		return "", nil, nil, err
	}

	b := bytes.NewBuffer([]byte{})
	// print out the result: a FIO public key is really an EOS key, with FIO at the beginning
	b.WriteString(fmt.Sprintln("Private Key:   ", kb.Keys[0].String()))
	b.WriteString(fmt.Sprintln("Public Key:     FIO" + kb.Keys[0].PublicKey().String()[3:]))
	b.WriteString(fmt.Sprintln("Account Name:  ", fioActor))

	return b.String(), pngPriv, pngPub, nil
}
