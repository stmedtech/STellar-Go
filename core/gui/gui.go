package gui

import (
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"slices"
	"stellar/core/constant"
	"stellar/core/device"
	"stellar/core/protocols/compute"
	"stellar/p2p/node"
	"stellar/p2p/protocols/file"
	"stellar/p2p/protocols/proxy"
	"strconv"
	"strings"
	"time"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var logger = golog.Logger("stellar-core-gui")

type GUIApp struct {
	Bypass bool

	node  *node.Node
	proxy *proxy.ProxyManager

	a fyne.App
	w fyne.Window

	overviewContainer *fyne.Container

	devices          binding.ExternalStringList
	selectedDeviceId binding.String

	proxies       binding.ExternalStringList
	selectedProxy binding.String

	policyEnable binding.Bool
	whitelist    binding.ExternalStringList
}

func NewGUIApp() (*GUIApp, error) {
	var err error

	a := app.NewWithID(constant.StellarAppID)

	app := GUIApp{
		a: a,

		overviewContainer: container.NewVBox(),

		devices: binding.BindStringList(
			&[]string{},
		),
		selectedDeviceId: binding.NewString(),

		proxies: binding.BindStringList(
			&[]string{},
		),
		selectedProxy: binding.NewString(),
	}

	return &app, err
}

func (app *GUIApp) showErrWithWindow(err error, window fyne.Window) {
	logger.Warn(err)
	dialog.ShowError(err, window)
}

func (app *GUIApp) showErr(err error) {
	logger.Warn(err)
	dialog.ShowError(err, app.w)
}

func (app *GUIApp) deviceSelect() *widget.RadioGroup {
	radio := widget.NewRadioGroup(slices.Sorted(maps.Keys(app.node.Devices)), func(value string) {})
	return radio
}

func (app *GUIApp) initOverview() fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil, nil, app.overviewContainer)
}

func prettyByteSize(b int) string {
	bf := float64(b)
	for _, unit := range []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi"} {
		if math.Abs(bf) < 1024.0 {
			return fmt.Sprintf("%3.1f%sB", bf, unit)
		}
		bf /= 1024.0
	}
	return fmt.Sprintf("%.1fYiB", bf)
}

func (app *GUIApp) initDevices() fyne.CanvasObject {
	str := binding.NewString()
	str.Set("Please select a device")

	connectDevice := widget.NewButton("Connect Device", func() {
		w := app.a.NewWindow("Connect to new Device")
		w.Resize(fyne.NewSize(800, 600))

		deviceAddress := widget.NewEntry()

		form := &widget.Form{
			Items: []*widget.FormItem{
				{Text: "Address", Widget: deviceAddress},
			},
			OnSubmit: func() {
				peer, addrErr := peer.AddrInfoFromString(deviceAddress.Text)
				if addrErr != nil {
					app.showErrWithWindow(addrErr, w)
					return
				}

				device, connectErr := app.node.ConnectDevice(*peer)
				if connectErr != nil {
					app.showErrWithWindow(connectErr, w)
					return
				}

				dialog.ShowInformation("Connected to Device", device.ID.String(), w)
			},
		}

		w.SetContent(form)
		w.Show()
	})

	list := widget.NewListWithData(app.devices,
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			o.(*widget.Label).Bind(i.(binding.String))
		})

	list.OnSelected = func(id widget.ListItemID) {
		deviceId, err := app.devices.GetValue(id)
		if err != nil {
			return
		}
		app.selectedDeviceId.Set(deviceId)
	}

	content := container.NewBorder(connectDevice, nil, nil, nil, list)

	app.selectedDeviceId.AddListener(binding.NewDataListener(func() {
		deviceId, err := app.selectedDeviceId.Get()
		if err != nil {
			return
		}
		if deviceId == "" {
			return
		}

		device, err := app.node.GetDevice(deviceId)
		if err != nil {
			app.showErr(err)
			return
		}

		lines := make([]string, 0)
		lines = append(lines, fmt.Sprintf("ID:\t\t\t\t%v", device.ID))
		lines = append(lines, fmt.Sprintf("Reference Token:\t%v", device.ReferenceToken))
		lines = append(lines, fmt.Sprintf("Status:\t\t\t%v", device.Status))
		lines = append(lines, fmt.Sprintf("System Info:\t\t%v", device.SysInfo))
		lines = append(lines, fmt.Sprintf("Last Healthcheck:\t\t\t%v", device.Timestamp.In(time.Local)))

		str.Set(strings.Join(lines, "\n"))
	}))

	contentText := widget.NewLabelWithData(str)
	contentText.Wrapping = fyne.TextWrapWord

	filesTree := widget.NewButton("Files Tree", func() {
		deviceId, err := app.selectedDeviceId.Get()
		if err != nil {
			return
		}
		if deviceId == "" {
			return
		}

		device, err := app.node.GetDevice(deviceId)
		if err != nil {
			app.showErr(err)
			return
		}

		w := app.a.NewWindow(fmt.Sprintf("Files Tree View of device %v", deviceId))
		w.Resize(fyne.NewSize(800, 600))

		files, lsErr := file.ListFullTree(app.node, device.ID)
		if lsErr != nil {
			app.showErr(lsErr)
			return
		}

		var findEntryRecur func(path string, fs []file.FileEntry) *file.FileEntry
		findEntryRecur = func(path string, fs []file.FileEntry) *file.FileEntry {
			if path == "" {
				return nil
			}

			SEP := "/"

			spts := strings.Split(path, SEP)
			if len(spts) == 0 {
				return nil
			}

			search := spts[0]
			trailing := strings.Join(spts[1:], SEP)

			for _, f := range fs {
				if f.Filename == search {
					if len(spts) == 1 {
						return &f
					}

					if f.IsDir && len(spts) > 1 {
						return findEntryRecur(trailing, f.Children)
					}
				}
			}

			return nil
		}

		findEntry := func(id widget.TreeNodeID) *file.FileEntry {
			switch id {
			case "":
				return &file.FileEntry{
					Children: files,
				}
			default:
				return findEntryRecur(id, files)
			}
		}

		tree := widget.NewTree(
			func(id widget.TreeNodeID) (nodes []widget.TreeNodeID) {
				dir := findEntry(id)
				if dir == nil {
					return
				}

				for _, entry := range dir.Children {
					nodes = append(nodes, entry.FullName())
				}
				return
			},
			func(id widget.TreeNodeID) bool {
				if id == "" {
					return true
				}

				f := findEntry(id)
				if f == nil {
					return false
				}

				return f.IsDir
			},
			func(branch bool) fyne.CanvasObject {
				if branch {
					return widget.NewLabel("Branch template")
				}
				return widget.NewLabel("Leaf template")
			},
			func(id widget.TreeNodeID, branch bool, o fyne.CanvasObject) {
				text := id

				o.(*widget.Label).SetText(text)

				if id == "" {
					return
				}

				f := findEntry(id)
				if f == nil {
					return
				}

				if f.IsDir {
					text = filepath.Base(id)
				} else {
					text = fmt.Sprintf("[%s] %s", prettyByteSize(int(f.Size)), filepath.Base(id))
				}
				o.(*widget.Label).SetText(text)
			})
		tree.OnSelected = func(id widget.TreeNodeID) {
			f := findEntry(id)
			if f == nil {
				return
			}
			if f.IsDir {
				return
			}

			filePath, err := file.Download(app.node, device.ID, f.FullName(), file.DataDir)
			if err != nil {
				app.showErrWithWindow(err, w)
				return
			}

			dialog.ShowInformation("Downloaded", filePath, w)
		}

		w.SetContent(tree)
		w.Show()
	})

	sendFile := widget.NewButton("Send File", func() {
		deviceId, err := app.selectedDeviceId.Get()
		if err != nil {
			return
		}
		if deviceId == "" {
			return
		}

		device, err := app.node.GetDevice(deviceId)
		if err != nil {
			app.showErr(err)
			return
		}

		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				app.showErr(err)
				return
			}
			if reader == nil {
				return
			}

			fpath := reader.URI().Path()
			logger.Infof("uploading file %s to device %s...", fpath, deviceId)

			err = file.Upload(app.node, device.ID, fpath, filepath.Base(fpath))
			if err != nil {
				app.showErr(err)
				return
			}

			dialog.ShowInformation("Sent", fpath, app.w)
		}, app.w)
		fd.Show()
	})

	prepareCondaPython := widget.NewButton("Prepare Python Env", func() {
		deviceId, err := app.selectedDeviceId.Get()
		if err != nil {
			return
		}
		if deviceId == "" {
			return
		}

		device, err := app.node.GetDevice(deviceId)
		if err != nil {
			app.showErr(err)
			return
		}

		w := app.a.NewWindow(fmt.Sprintf("Prepare Python Environment for device %v", deviceId))
		w.Resize(fyne.NewSize(800, 600))

		envName := widget.NewEntry()
		envVersion := widget.NewEntry()
		envVersion.Text = "3.11"

		form := &widget.Form{
			Items: []*widget.FormItem{
				{Text: "Environment Name", Widget: envName},
				{Text: "Environment Version", Widget: envVersion},
			},
			OnSubmit: func() {
				fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
					if err != nil {
						app.showErrWithWindow(err, w)
						return
					}
					if reader == nil {
						return
					}

					fpath := reader.URI().Path()
					form := compute.CondaPythonPreparation{
						Env:         envName.Text,
						Version:     envVersion.Text,
						EnvYamlPath: fpath,
					}
					envPath, envErr := compute.PrepareCondaPython(app.node, device.ID, form)
					if envErr != nil {
						app.showErrWithWindow(envErr, w)
						return
					}

					dialog.ShowInformation("Python Environment", envPath, w)
				}, w)
				fd.SetFilter(storage.NewExtensionFileFilter([]string{".yml", ".yaml"}))
				fd.Show()
			},
		}

		w.SetContent(form)
		w.Show()
	})

	executeCondaPythonScript := widget.NewButton("Execute Python Script", func() {
		deviceId, err := app.selectedDeviceId.Get()
		if err != nil {
			return
		}
		if deviceId == "" {
			return
		}

		device, err := app.node.GetDevice(deviceId)
		if err != nil {
			app.showErr(err)
			return
		}

		w := app.a.NewWindow(fmt.Sprintf("Execute Python Script with device %v", deviceId))
		w.Resize(fyne.NewSize(800, 600))

		envs, err := compute.ListCondaPythonEnvs(app.node, device.ID)
		if err != nil {
			app.showErr(err)
			return
		}
		options := slices.Sorted(maps.Keys(envs))
		envName := widget.NewRadioGroup(options, func(value string) {})
		if len(options) > 0 {
			envName.Selected = options[0]
		}

		form := &widget.Form{
			Items: []*widget.FormItem{
				{Text: "Select Environment", Widget: envName},
			},
			OnSubmit: func() {
				fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
					if err != nil {
						app.showErrWithWindow(err, w)
						return
					}
					if reader == nil {
						return
					}

					fpath := reader.URI().Path()
					form := compute.CondaPythonScriptExecution{
						Env:        envName.Selected,
						ScriptPath: fpath,
					}
					result, envErr := compute.ExecuteCondaPythonScript(app.node, device.ID, form)
					if envErr != nil {
						app.showErrWithWindow(envErr, w)
						return
					}

					dialog.ShowInformation("Python Script Execution", result, w)
				}, w)
				fd.SetFilter(storage.NewExtensionFileFilter([]string{".py"}))
				fd.Show()
			},
		}

		w.SetContent(form)
		w.Show()
	})

	executeCondaPythonWorkspace := widget.NewButton("Execute Python Workspace", func() {
		deviceId, err := app.selectedDeviceId.Get()
		if err != nil {
			return
		}
		if deviceId == "" {
			return
		}

		device, err := app.node.GetDevice(deviceId)
		if err != nil {
			app.showErr(err)
			return
		}

		w := app.a.NewWindow(fmt.Sprintf("Execute Python Workspace with device %v", deviceId))
		w.Resize(fyne.NewSize(800, 600))

		envs, err := compute.ListCondaPythonEnvs(app.node, device.ID)
		if err != nil {
			app.showErr(err)
			return
		}
		options := slices.Sorted(maps.Keys(envs))
		envName := widget.NewRadioGroup(options, func(value string) {})
		if len(options) > 0 {
			envName.Selected = options[0]
		}

		form := &widget.Form{
			Items: []*widget.FormItem{
				{Text: "Select Environment", Widget: envName},
			},
			OnSubmit: func() {
				fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
					if err != nil {
						app.showErrWithWindow(err, w)
						return
					}
					if reader == nil {
						return
					}

					fpath := reader.URI().Path()
					form := compute.CondaPythonWorkspaceExecution{
						Env:           envName.Selected,
						WorkspacePath: fpath,
					}
					result, envErr := compute.ExecuteCondaPythonWorkspace(app.node, device.ID, form)
					if envErr != nil {
						app.showErrWithWindow(envErr, w)
						return
					}

					dialog.ShowInformation("Python Workspace Execution", result, w)
				}, w)
				fd.SetFilter(storage.NewExtensionFileFilter([]string{".zip"}))
				fd.Show()
			},
		}

		w.SetContent(form)
		w.Show()
	})

	deviceControls := container.NewHBox(filesTree, sendFile, prepareCondaPython, executeCondaPythonScript, executeCondaPythonWorkspace)

	split := container.NewHSplit(content, container.NewVBox(contentText, deviceControls))
	split.Offset = 0.2

	return split
}

func (app *GUIApp) initProxies() fyne.CanvasObject {
	str := binding.NewString()
	str.Set("Please select a proxy or create one")

	app.selectedProxy.AddListener(binding.NewDataListener(func() {
		proxyId, err := app.selectedProxy.Get()
		if err != nil {
			return
		}
		if proxyId == "" {
			return
		}

		spts := strings.Split(proxyId, "/")
		if len(spts) != 3 {
			str.Set("something wrong with the selected proxy")
			return
		}

		lines := make([]string, 0)
		lines = append(lines, fmt.Sprintf("Proxy Port:\t\t%s", spts[0]))
		lines = append(lines, fmt.Sprintf("Proxy Address:\t%s", spts[1]))
		lines = append(lines, fmt.Sprintf("ID:\t\t\t%s", spts[2]))

		str.Set(strings.Join(lines, "\n"))
	}))

	contentText := widget.NewLabelWithData(str)
	contentText.Wrapping = fyne.TextWrapWord

	list := widget.NewListWithData(app.proxies,
		func() fyne.CanvasObject {
			delete := widget.NewButton("Delete", func() {})
			label := widget.NewLabel("template")
			return container.NewHBox(delete, label)
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			cont := o.(*fyne.Container)
			cont.Objects[0].(*widget.Button).OnTapped = func() {
				data := i.(binding.String)
				portStr, err := data.Get()
				if err != nil {
					return
				}

				portStr = strings.Split(portStr, "/")[0]

				port, portErr := strconv.ParseUint(portStr, 10, 64)
				if portErr != nil {
					app.showErr(portErr)
					return
				}

				app.proxy.Close(port)
			}
			cont.Objects[1].(*widget.Label).Bind(i.(binding.String))
		})

	list.OnSelected = func(id widget.ListItemID) {
		proxyId, err := app.proxies.GetValue(id)
		if err != nil {
			return
		}
		app.selectedProxy.Set(proxyId)
	}

	createProxy := widget.NewButton("Create Proxy", func() {
		w := app.a.NewWindow("Create Proxy")
		w.Resize(fyne.NewSize(800, 600))

		deviceSelect := app.deviceSelect()
		proxyAddress := widget.NewEntry()
		port := widget.NewEntry()

		form := &widget.Form{
			Items: []*widget.FormItem{
				{Text: "Select Device", Widget: deviceSelect},
				{Text: "Local Port", Widget: port},
				{Text: "Proxy Address", Widget: proxyAddress},
			},
			OnSubmit: func() {
				device, err := app.node.GetDevice(deviceSelect.Selected)
				if err != nil {
					app.showErrWithWindow(err, w)
					return
				}

				port, portErr := strconv.ParseUint(port.Text, 10, 64)
				if portErr != nil {
					app.showErrWithWindow(portErr, w)
					return
				}

				if _, proxyErr := app.proxy.Proxy(device.ID, port, proxyAddress.Text); proxyErr != nil {
					app.showErrWithWindow(proxyErr, w)
					return
				}

				w.Close()
			},
		}
		w.SetContent(form)
		w.Show()
	})

	split := container.NewHSplit(container.NewBorder(createProxy, nil, nil, nil, list), container.NewVBox(contentText))
	split.Offset = 0.2

	return split
}

func (app *GUIApp) initWhiteList() fyne.CanvasObject {
	toggle := widget.NewCheckWithData("Enable Security Policy", app.policyEnable)

	deviceId := widget.NewEntry()
	create := widget.NewButton("Add", func() {
		idErr := app.node.Policy.AddWhiteList(deviceId.Text)
		if idErr != nil {
			app.showErr(idErr)
		}
		deviceId.Text = ""
		deviceId.Refresh()
	})
	createCont := container.NewBorder(nil, nil, nil, create, deviceId)

	top := container.NewVBox(toggle, createCont)

	list := widget.NewListWithData(app.whitelist,
		func() fyne.CanvasObject {
			delete := widget.NewButton("Delete", func() {})
			label := widget.NewLabel("template")
			return container.NewHBox(delete, label)
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			cont := o.(*fyne.Container)
			cont.Objects[0].(*widget.Button).OnTapped = func() {
				data := i.(binding.String)
				deviceId, err := data.Get()
				if err != nil {
					return
				}

				if wlErr := app.node.Policy.RemoveWhiteList(deviceId); wlErr != nil {
					app.showErr(wlErr)
				}
			}
			cont.Objects[1].(*widget.Label).Bind(i.(binding.String))
		})

	return container.NewBorder(top, nil, nil, nil, list)
}

func (app *GUIApp) Setup() {
	icon, _ := fyne.LoadResourceFromPath("assets/stellar.png")
	app.a.SetIcon(icon)

	w := app.a.NewWindow("Stellar Setup Node")
	w.Resize(fyne.NewSize(800, 600))

	pref := app.a.Preferences()

	privkey := widget.NewEntry()
	privkey.Text = pref.StringWithFallback("stellarPrivkey", "0")
	listenHost := widget.NewEntry()
	listenHost.Text = pref.StringWithFallback("stellarListenHost", "0.0.0.0")
	listenPort := widget.NewEntry()
	listenPort.Text = pref.StringWithFallback("stellarListenPort", "0")
	referenceToken := widget.NewEntry()
	referenceToken.Text = pref.String("stellarReferenceToken")
	metrics := widget.NewCheck("", func(b bool) {})
	metrics.Checked = pref.Bool("stellarMetrics")
	api := widget.NewCheck("", func(b bool) {})
	api.Checked = pref.BoolWithFallback("stellarAPI", true)

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Private Key", Widget: privkey},
			{Text: "Listen Host", Widget: listenHost},
			{Text: "Listen Port", Widget: listenPort},
			{Text: "Reference Token", Widget: referenceToken},
			{Text: "Enable Metrics Server", Widget: metrics},
		},
		OnSubmit: func() {
			pref.SetString("stellarPrivkey", privkey.Text)
			pref.SetString("stellarListenHost", listenHost.Text)
			pref.SetString("stellarListenPort", listenPort.Text)
			pref.SetString("stellarReferenceToken", referenceToken.Text)
			pref.SetBool("stellarMetrics", metrics.Checked)

			device := device.Device{}

			if seed, seedErr := strconv.ParseInt(privkey.Text, 10, 64); seedErr != nil {
				device.ImportKey(privkey.Text)
			} else {
				device.GenerateKey(seed)
			}

			port, portErr := strconv.ParseUint(listenPort.Text, 10, 64)
			if portErr != nil {
				app.showErr(portErr)
				return
			}

			device.Init(listenHost.Text, port)

			device.SetReferenceToken(referenceToken.Text)

			if metrics.Checked {
				device.Node.StartMetricsServer(5001)
			}

			device.StartDiscovery()
			device.StartUnixSocket()
			device.StartAPI(1524)

			app.node = device.Node
			app.proxy = device.Proxy

			app.policyEnable = binding.BindBool(&app.node.Policy.Enable)
			app.whitelist = binding.BindStringList(&app.node.Policy.WhiteList)

			app.SetupMain()

			w.Close()
		},
	}
	form.SubmitText = "Start Node"

	if app.Bypass {
		form.OnSubmit()
	} else {
		app.w = w
		w.SetContent(form)
		w.Show()
	}
}

func (app *GUIApp) SetupMain() {
	w := app.a.NewWindow("Stellar Debug GUI")
	w.Resize(fyne.NewSize(800, 600))

	tabs := container.NewAppTabs(
		container.NewTabItem("Overview", app.initOverview()),
		container.NewTabItem("Devices", app.initDevices()),
		container.NewTabItem("Proxies", app.initProxies()),
		container.NewTabItem("White List", app.initWhiteList()),
	)
	tabs.SetTabLocation(container.TabLocationLeading)

	app.w = w
	w.SetContent(tabs)
	w.SetOnClosed(func() {
		app.a.Quit()
	})
	w.Show()
}

func (app *GUIApp) Loop() {
	if app.node != nil {
		fyne.Do(func() {
			n := app.node
			lines := make([]string, 0)
			lines = append(lines, fmt.Sprintf("ID:\t\t\t\t%v", n.ID().String()))
			lines = append(lines, fmt.Sprintf("Reference Token:\t%v", n.ReferenceToken))
			lines = append(lines, "Broadcasted Addresses:")

			app.overviewContainer.RemoveAll()
			app.overviewContainer.Add(widget.NewLabel(strings.Join(lines, "\n")))
			for _, addr := range n.Host.Addrs() {
				ent := widget.NewEntry()
				ent.Text = addr.Encapsulate(ma.StringCast("/p2p/" + n.ID().String())).String()
				ent.Disable()

				copy := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
					app.a.Clipboard().SetContent(ent.Text)
				})

				cont := container.NewBorder(nil, nil, copy, nil, ent)

				app.overviewContainer.Add(cont)
			}
			app.overviewContainer.Refresh()

			app.devices.Set(slices.Sorted(maps.Keys(app.node.Devices)))

			proxies := make([]string, 0)
			for _, proxy := range app.proxy.Proxies() {
				proxies = append(proxies, fmt.Sprintf("%d/%s/%s", proxy.Port, proxy.DestAddr, proxy.Dest.String()))
			}
			app.proxies.Set(proxies)

			app.whitelist.Set(app.node.Policy.WhiteList)
		})
	}
}

func (app *GUIApp) Cleanup() {
	if app.node != nil {
		app.node.Close()
	}
}

func (app *GUIApp) Run() {
	app.Setup()

	go func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			app.Loop()
		}
	}()

	app.a.Run()
	app.Cleanup()
}
