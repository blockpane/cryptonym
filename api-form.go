package cryptonym

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"fyne.io/fyne"
	"fyne.io/fyne/layout"
	"fyne.io/fyne/theme"
	"fyne.io/fyne/widget"
	fioassets "github.com/blockpane/cryptonym/assets"
	errs "github.com/blockpane/cryptonym/errLog"
	"gopkg.in/alessio/shellescape.v1"
	"io/ioutil"
	"math"
	"net/http"
	"strings"
	"time"
)

func NewApiRequestTab(container chan fyne.Container) {
	apiList := SupportedApis{Apis: []string{"/v1/chain/get_info"}}
	err := apiList.Update(Uri, false)
	if err != nil {
		errs.ErrChan <- "Error updating list of available APIs: " + errs.Detailed(err)
	}
	inputEntry := widget.NewMultiLineEntry()
	outputEntry := widget.NewMultiLineEntry()
	statusLabel := widget.NewLabel("")
	submit := &widget.Button{}
	inputTab := &widget.TabItem{}
	outputTab := &widget.TabItem{}
	apiTabs := &widget.TabContainer{}

	submit = widget.NewButtonWithIcon("Submit", fioassets.NewFioLogoResource(), func() {
		submit.Disable()
		statusLabel.SetText("")
		outputEntry.SetText("")
		outputEntry.OnChanged = func(string) {}
		apiTabs.SelectTab(outputTab)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
		done := make(chan bool, 1)
		defer func() {
			submit.Enable()
			outputEntry.Refresh()
			cancel()
		}()
		go func() {
			defer func() {
				done <- true
			}()
			resp, err := http.Post(Uri+apiEndPointActive, "application/json", bytes.NewReader([]byte(inputEntry.Text)))
			if err != nil {
				outputEntry.SetText(err.Error())
				errs.ErrChan <- err.Error()
				return
			}
			statusLabel.SetText(fmt.Sprintf("POST %s -- %s", Uri+apiEndPointActive, resp.Status))
			if resp.Body != nil {
				defer func() {
					resp.Body.Close()
					outputEntry.Refresh()
				}()
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					outputEntry.SetText(err.Error())
					errs.ErrChan <- err.Error()
					return
				}
				if len(body) > 131072 {
					outputEntry.SetText("Response body is too big to show")
					errs.ErrChan <- "Response body is too big to show"
					return
				}
				j, err := json.MarshalIndent(json.RawMessage(body), "", "  ")
				if err != nil {
					outputEntry.SetText(err.Error())
					errs.ErrChan <- err.Error()
					return
				}
				txt := string(j)
				func(s string) {
					outputEntry.OnChanged = func(string) {
						outputEntry.SetText(s)
					}
				}(txt) // deref
				outputEntry.SetText(txt)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				outputEntry.SetText("Request timed out.")
				return
			case <-done:
				return
			}
		}
	}) // NewButtonWithIcon

	apiRadio := &widget.Radio{}
	epbFilter := widget.NewEntry()
	epbFilter.SetPlaceHolder("Filter Endpoints")

	apiUpdate := func(ep string) {
		apiEndPointActive = ep
		submit.SetText(ep)
		submit.Refresh()
		inputEntry.SetText(DefaultJsonFor(ep))
		inputEntry.Refresh()
	}
	apiRadio = widget.NewRadio(apiList.Apis, apiUpdate)

	copyToClip := &widget.Button{}
	clipped := func() {
		go func() {
			clip := Win.Clipboard()
			curl := fmt.Sprintf(`curl -s -XPOST %s -d %s`, shellescape.Quote(Uri+apiEndPointActive), shellescape.Quote(inputEntry.Text))
			clip.SetContent(curl)
			copyToClip.Text = "Copied!"
			if curl != clip.Content() {
				clip.SetContent("Failed to Copy!")
			}
			copyToClip.Refresh()
			time.Sleep(2 * time.Second)
			copyToClip.Text = "Copy as curl"
			copyToClip.Refresh()
		}()
	}
	copyToClip = widget.NewButtonWithIcon("Copy as curl", theme.ContentCopyIcon(), clipped)

	hideSigned := &widget.Check{}
	hide := func(b bool) {
		apiRadio.Options = nil
		apiRadio.Refresh()
		newList := make([]string, 0)
		for i := range apiList.Apis {
			switch true {
			case hideSigned.Checked && isSigned(apiList.Apis[i]):
				break
			case epbFilter.Text == "":
				newList = append(newList, apiList.Apis[i])
			case strings.Contains(apiList.Apis[i], epbFilter.Text):
				newList = append(newList, apiList.Apis[i])
			}
		}
		apiRadio.Options = newList
		apiRadio.Refresh()
	}
	hideSigned = widget.NewCheck("Hide Signed", hide)

	submit.SetText(apiEndPointActive)
	submit.Refresh()
	inputTab = widget.NewTabItem("Request",
		fyne.NewContainerWithLayout(layout.NewHBoxLayout(),
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.Size{Width: apiRadio.MinSize().Width + 10, Height: int(math.Round(float64(H) * .63))}),
				widget.NewVBox(
					widget.NewHBox(hideSigned, epbFilter),
					fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.Size{Width: apiRadio.MinSize().Width + 10, Height: int(math.Round(float64(H) * .63))}),
						widget.NewGroupWithScroller("Endpoint", apiRadio),
					),
				),
			),
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.Size{Width: 30, Height: H / 3})),
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.Size{Width: W - apiRadio.MinSize().Width - 360, Height: int(math.Round(float64(H) * .63))}),
				widget.NewScrollContainer(widget.NewVBox(
					widget.NewHBox(
						layout.NewSpacer(),
						submit,
						copyToClip,
						layout.NewSpacer(),
					),
					widget.NewLabelWithStyle("Request JSON:", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
					inputEntry,
					layout.NewSpacer(),
				))),
		),
	)
	outputTab = widget.NewTabItem("Response",
		widget.NewScrollContainer(widget.NewVBox(
			widget.NewHBox(widget.NewLabel("Resend:"), submit, layout.NewSpacer(), statusLabel, layout.NewSpacer(), layout.NewSpacer()),
			outputEntry,
			fyne.NewContainerWithLayout(layout.NewFixedGridLayout(fyne.Size{
				Width:  100,
				Height: 100,
			})),
		)),
	)
	epbFilter.OnChanged = func(string) {
		hide(hideSigned.Checked)
	}
	hideSigned.SetChecked(true)
	apiTabs = widget.NewTabContainer(inputTab, outputTab)
	container <- *fyne.NewContainerWithLayout(layout.NewMaxLayout(), apiTabs)
}

func isSigned(s string) bool {
	switch {
	case strings.Contains(s, "get_"):
		return false
	case strings.Contains(s, "_to_"):
		return false
	case strings.Contains(s, "_json"):
		return false
	case strings.Contains(s, "_check"):
		return false
	case strings.Contains(s, "db_size/"):
		return false
	case strings.Contains(s, "history/"):
		return false
	case strings.Contains(s, "connections"):
		return false
	}

	return true
}
