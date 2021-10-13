package cryptonym

import (
	"context"
	"errors"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	errs "github.com/blockpane/cryptonym/errLog"
	"github.com/fioprotocol/fio-go"
	"strings"
	"time"
)

func NewAccountSearchTab(box chan fyne.Container, account *fio.Account) {

	accountOuput := &widget.Box{}
	accountInput := NewClickEntry(&widget.Button{})
	accountSelect := widget.NewSelect(accountSearchType, func(s string) {
		accountInput.Refresh()
	})
	accountSelect.SetSelected(accountSearchType[0])
	accountInput.SetText(account.PubKey)
	accountInput.OnChanged = func(s string) {
		selected := accountSelect.Selected
		switch {
		case len(s) == 53 && strings.HasPrefix(s, "FIO"):
			accountSelect.SetSelected("Public Key")
		case len(s) == 51 && strings.HasPrefix(s, "5"):
			accountSelect.SetSelected("Private Key")
		case strings.Contains(s, "@"):
			accountSelect.SetSelected("Fio Address")
		case len(s) == 12:
			accountSelect.SetSelected("Actor/Account")
		case selected != "Fio Domain":
			accountSelect.SetSelected("Fio Domain")
		}
		accountInput.SetText(s)
		go func() {
			time.Sleep(100 * time.Millisecond)
			accountInput.Refresh()
		}()
	}

	accountSubmit := &widget.Button{}
	emptyBox := func() *widget.Box {
		return widget.NewVBox(
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth(), 40)),
				widget.NewHBox(
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(accountSelect.MinSize()), accountSelect),
					accountInput,
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(accountSubmit.MinSize()), accountSubmit),
				),
			),
			layout.NewSpacer(),
		)
	}

	mkBox := func(accountOutput *widget.Box) *widget.Box {
		accountBox := widget.NewVBox(
			accountOuput,
		)
		accountOuput.Refresh()
		return widget.NewVBox(
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.NewSize(RWidth(), 40)),
				widget.NewHBox(
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(accountSelect.MinSize()), accountSelect),
					accountInput,
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(accountSubmit.MinSize()), accountSubmit),
				),
			),
			fyne.NewContainerWithLayout(layout.NewVBoxLayout(),
				accountBox,
			),
		)
	}

	accountSubmit = widget.NewButtonWithIcon("Search", theme.SearchIcon(), func() {
		go func() {
			accountSubmit.Disable()
			defer accountSubmit.Enable()
			d := time.Now().Add(5 * time.Second)
			ctx, cancel := context.WithDeadline(context.Background(), d)
			defer cancel()
			finished := make(chan bool)
			go func() {
				box <- *fyne.NewContainerWithLayout(layout.NewVBoxLayout(),
					emptyBox(),
					layout.NewSpacer(),
				)
				info, err := AccountSearch(accountSelect.Selected, accountInput.Text)
				if err != nil {
					errs.ErrChan <- errs.Detailed(err)
					return
				}
				ao, err := info.report()
				if err != nil {
					errs.ErrChan <- errs.Detailed(err)
					return
				}
				deRef := *ao
				accountOuput = &deRef
				accountOuput.Refresh()
				accountOuput.Show()
				box <- *fyne.NewContainerWithLayout(
					layout.NewMaxLayout(),
					widget.NewScrollContainer(mkBox(&deRef)),
				)
				finished <- true
			}()
			for {
				select {
				case <-finished:
					return
				case <-ctx.Done():
					errs.ErrChan <- "hit time limit while getting account information"
					return
				}
			}
		}()
	})
	accountInput.Button = accountSubmit
	box <- *fyne.NewContainerWithLayout(layout.NewVBoxLayout(),
		emptyBox(),
		layout.NewSpacer(),
	)
}

func (as *AccountInformation) report() (*widget.Box, error) {
	if as.Actor == "" || as.PubKey == "" {
		return nil, errors.New("nothing to report, account or key name empty")
	}
	vSpace := widget.NewHBox(widget.NewLabel(" "))

	names := func() *widget.Box {
		b := widget.NewVBox()
		if len(as.FioNames) > 0 {
			for _, r := range as.FioNames {
				b.Append(vSpace)
				fioName := widget.NewEntry()
				func(n string) {
					fioName.SetText(n)
					fioName.OnChanged = func(string) {
						fioName.SetText(n)
					}
				}(r.Name) // deref
				b.Append(fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
					widget.NewLabelWithStyle("Fio Name:", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
					fioName,
					layout.NewSpacer(),
				))

				expires := widget.NewEntry()
				func(s string) {
					expires.SetText(s)
					expires.OnChanged = func(string) {
						expires.SetText(s)
					}
				}(time.Unix(r.Expiration, 0).String()) // deref
				b.Append(fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
					widget.NewLabelWithStyle("Expiration:", fyne.TextAlignTrailing, fyne.TextStyle{}),
					expires,
					layout.NewSpacer(),
				))

				bundle := widget.NewEntry()
				func(s string) {
					bundle.SetText(s)
					bundle.OnChanged = func(string) {
						bundle.SetText(s)
					}
				}(fmt.Sprintf("%d", r.BundleCount)) // deref

				b.Append(fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
					widget.NewLabelWithStyle("Free Bundled TX remaining:", fyne.TextAlignTrailing, fyne.TextStyle{}),
					bundle,
					layout.NewSpacer(),
				))

				addressBox := make([]*fyne.Container, 0)
				for _, public := range r.Addresses {
					if public.PublicAddress == as.PubKey {
						continue
					}
					pubAddr := widget.NewEntry()
					var symbol string
					switch {
					case public.TokenCode != "" && public.ChainCode != "":
						symbol = fmt.Sprintf("%s • %s", public.TokenCode, public.ChainCode)
					case public.TokenCode != "":
						symbol = public.TokenCode
					default:
						symbol = public.ChainCode
					}
					func(s string) {
						pubAddr.SetText(s)
						pubAddr.OnChanged = func(string) {
							pubAddr.SetText(s)
						}
					}(public.PublicAddress) // deref

					addressBox = append(addressBox,
						fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
							widget.NewLabelWithStyle(symbol, fyne.TextAlignTrailing, fyne.TextStyle{Italic: true}),
							pubAddr,
							layout.NewSpacer(),
						))
				}
				for _, app := range addressBox {
					b.Append(app)
				}

			}
		}
		b.Append(fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.Size{
			Width:  20,
			Height: 50,
		})))
		return b
	}()

	domains := func() *widget.Box {
		b := widget.NewVBox()
		if len(as.FioDomains) > 0 {
			for _, r := range as.FioDomains {

				b.Append(fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(1), widget.NewLabel(" ")))
				fioDomain := widget.NewEntry()
				func(s string) {
					fioDomain.SetText(s)
					fioDomain.OnChanged = func(string) {
						fioDomain.SetText(s)
					}
				}(r.Name) // deref

				b.Append(fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
					widget.NewLabelWithStyle("Fio Domain:", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
					fioDomain,
					layout.NewSpacer(),
				))

				public := widget.NewEntry()
				toBool := func() string {
					if r.IsPublic > 0 {
						return "True"
					}
					return "False"
				}
				func(s string) {
					public.SetText(s)
					public.OnChanged = func(string) {
						public.SetText(s)
					}
				}(toBool())

				b.Append(fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
					widget.NewLabelWithStyle("Allows Public Registration:", fyne.TextAlignTrailing, fyne.TextStyle{}),
					public,
					layout.NewSpacer(),
				))

				expires := widget.NewEntry()
				func(s string) {
					expires.SetText(s)
					expires.OnChanged = func(string) {
						expires.SetText(s)
					}
				}(time.Unix(r.Expiration, 0).String()) // deref

				b.Append(fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
					widget.NewLabelWithStyle("Expiration:", fyne.TextAlignTrailing, fyne.TextStyle{}),
					expires,
					layout.NewSpacer(),
				))

			}
		}
		return b
	}()

	report := &widget.Box{}

	entryActor := widget.NewEntry()
	entryActor.SetText(as.Actor)
	entryActor.OnChanged = func(string) {
		entryActor.SetText(as.Actor)
	}
	entryPub := widget.NewEntry()
	entryPub.SetText(as.PubKey)
	entryPub.OnChanged = func(string) {
		entryPub.SetText(as.PubKey)
	}
	entryBal := widget.NewEntry()
	entryBal.SetText(p.Sprintf("ᵮ %.9g", float64(as.Balance)/1000000000.0))
	entryBal.OnChanged = func(string) {
		entryBal.SetText(p.Sprintf("ᵮ %.9g", float64(as.Balance)/1000000000.0))
	}

	ramUsed := widget.NewEntry()
	ramContainer := fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
		widget.NewLabelWithStyle("Ram Used: ", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		ramUsed,
		layout.NewSpacer(),
	)
	ramContainer.Hide()
	if as.RamUsed != 0 {
		ramContainer.Show()
		ramUsed.SetText(p.Sprintf("%d", as.RamUsed))
		ramUsed.OnChanged = func(string) {
			ramUsed.SetText(p.Sprintf("%d", as.RamUsed))
		}
		ramUsed.Refresh()
	}

	msig := widget.NewMultiLineEntry()
	msigContainer := fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
		widget.NewLabelWithStyle("MultiSig Account Info: ", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		msig,
		layout.NewSpacer(),
	)
	msigContainer.Hide()
	if len(as.MsigOwners) != 0 {
		msigContainer.Show()
		msig.SetText(fmt.Sprintf("Required Threshold: %d\nOwners:\n - %s", as.MsigThresh, strings.Join(as.MsigOwners, "\n - ")))
		msig.OnChanged = func(string) {
			msig.SetText(fmt.Sprintf("Required Threshold: %d\nOwners:\n - %s", as.MsigThresh, strings.Join(as.MsigOwners, "\n - ")))
		}
		msig.Refresh()
	}

	report = widget.NewVBox(
		fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
			widget.NewLabelWithStyle("Actor: ", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			entryActor,
			layout.NewSpacer(),
		),
		fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
			widget.NewLabelWithStyle("Public Key: ", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			entryPub,
			layout.NewSpacer(),
		),
		fyne.NewContainerWithLayout(layout.NewGridLayoutWithColumns(3),
			widget.NewLabelWithStyle("Balance: ", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			entryBal,
			layout.NewSpacer(),
		),
		ramContainer,
		msigContainer,
		domains,

		names,
	)
	report.Refresh()

	return report, nil
}
