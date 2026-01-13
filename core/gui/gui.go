package gui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"path/filepath"
	"slices"
	"stellar/core/config"
	"stellar/core/constant"
	"stellar/core/device"
	"stellar/core/utils"

	// "stellar/core/device" // Temporarily unused (Phase 0 cleanup)
	// "stellar/core/protocols/compute" // Temporarily disabled (Phase 0 cleanup)
	"stellar/p2p/node"
	p2p_compute "stellar/p2p/protocols/compute"
	compute_service "stellar/p2p/protocols/compute/service"
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
	radio := widget.NewRadioGroup(slices.Sorted(maps.Keys(app.node.Devices())), func(value string) {})
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

			for i := range fs {
				entry := &fs[i]
				if entry.FullName() == path {
					return entry
				}
				if entry.IsDir {
					if found := findEntryRecur(path, entry.Children); found != nil {
						return found
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

			filePath, err := file.Download(app.node, device.ID, f.FullName(), filepath.Join(file.DataDir, f.Filename))
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

	openTerminal := widget.NewButton("Terminal", func() {
		deviceId, err := app.selectedDeviceId.Get()
		if err != nil || deviceId == "" {
			return
		}
		dev, err := app.node.GetDevice(deviceId)
		if err != nil {
			app.showErr(err)
			return
		}
		app.openTerminalWindow(dev.ID, deviceId)
	})

	deviceControls := container.NewHBox(filesTree, sendFile, openTerminal)

	split := container.NewHSplit(content, container.NewVBox(contentText, deviceControls))
	split.Offset = 0.7

	return split
}

func (app *GUIApp) openTerminalWindow(peerID peer.ID, deviceID string) {
	w := app.a.NewWindow(fmt.Sprintf("Terminal - %s", deviceID))
	w.Resize(fyne.NewSize(900, 650))

	status := widget.NewLabel("Ready")

	output := widget.NewMultiLineEntry()
	output.Disable()
	output.Wrapping = fyne.TextWrapOff

	scroll := container.NewScroll(output)
	scroll.SetMinSize(fyne.NewSize(880, 500))

	stdinEntry := widget.NewEntry()
	stdinEntry.SetPlaceHolder("Type command and press Enter (or Ctrl+D to close stdin)")

	cancelBtn := widget.NewButton("Cancel", func() {})
	cancelBtn.Disable()

	appendCh := make(chan string, 256)
	closeCh := make(chan struct{})

	var client *compute_service.Client
	var currentHandle *compute_service.RawExecutionHandle
	var isConnecting bool

	flush := func(s string) {
		// Best-effort append; keep it simple (terminal output sizes are user-driven).
		output.SetText(output.Text + s)
		// Auto-scroll to bottom
		scroll.ScrollToBottom()
	}

	go func() {
		for {
			select {
			case <-closeCh:
				return
			case s := <-appendCh:
				fyne.Do(func() { flush(s) })
			}
		}
	}()

	connect := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		c, err := p2p_compute.DialComputeClient(ctx, app.node, peerID)
		if err != nil {
			return err
		}
		client = c
		status.SetText("Connected")
		return nil
	}

	readStream := func(r io.Reader) {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				appendCh <- string(buf[:n])
			}
			if err != nil {
				if err != io.EOF {
					appendCh <- fmt.Sprintf("\n[stream error] %v\n", err)
				}
				return
			}
		}
	}

	readLog := func(r io.Reader) {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var e map[string]any
			if err := json.Unmarshal([]byte(line), &e); err == nil {
				typ, _ := e["type"].(string)
				if typ != "" {
					// Log entries are typically for debugging; we can show them optionally
					// For now, skip them to keep output clean
					continue
				}
			}
		}
	}

	// Execute a command dynamically
	executeCommand := func(cmdLine string) error {
		if client == nil {
			return fmt.Errorf("not connected")
		}

		// Parse command line with proper quote and escape handling
		cmd, args := utils.ParseCommandLine(cmdLine)
		if cmd == "" {
			return fmt.Errorf("empty command")
		}

		// Show command being executed
		appendCh <- fmt.Sprintf("$ %s\n", cmdLine)

		h, err := client.Run(context.Background(), compute_service.RunRequest{
			RunID:   "",
			Command: cmd,
			Args:    args,
		})
		if err != nil {
			return err
		}

		currentHandle = h

		// Start reading streams
		go readStream(h.Stdout)
		go readStream(h.Stderr)
		go readLog(h.Log)

		// Monitor completion
		go func() {
			err := <-h.Done
			code := <-h.ExitCode
			fyne.Do(func() {
				if err != nil {
					appendCh <- fmt.Sprintf("[exit] error: %v\n", err)
				} else {
					appendCh <- fmt.Sprintf("[exit] code=%d\n", code)
				}
				// Clear current handle when done
				if currentHandle == h {
					currentHandle = nil
					cancelBtn.Disable()
				}
			})
		}()

		return nil
	}

	// Execute command from input
	executeInput := func() {
		txt := strings.TrimSpace(stdinEntry.Text)
		if txt == "" {
			return
		}

		// If not connected, connect first, then execute the command
		if client == nil {
			if isConnecting {
				return // Already connecting
			}
			isConnecting = true
			status.SetText("Connecting...")
			stdinEntry.Disable()

			// Save the command to execute after connection
			cmdToExecute := txt
			stdinEntry.SetText("")

			go func() {
				if err := connect(); err != nil {
					fyne.Do(func() {
						app.showErrWithWindow(err, w)
						status.SetText("Connection failed (policy may block; whitelist the peer in 'White List')")
						stdinEntry.Enable()
						isConnecting = false
					})
					return
				}
				// Execute the first command after connection
				if err := executeCommand(cmdToExecute); err != nil {
					fyne.Do(func() {
						app.showErrWithWindow(err, w)
						status.SetText("Command execution failed")
						stdinEntry.Enable()
						isConnecting = false
					})
					return
				}
				fyne.Do(func() {
					status.SetText("Connected")
					stdinEntry.Enable()
					cancelBtn.Enable()
					isConnecting = false
				})
			}()
			return
		}

		// If a command is still running, send input to it
		if currentHandle != nil && currentHandle.Stdin != nil {
			stdinEntry.SetText("")
			txt += "\n"
			_, err := currentHandle.Stdin.Write([]byte(txt))
			if err != nil {
				fyne.Do(func() {
					app.showErrWithWindow(err, w)
				})
			}
			return
		}

		// Execute new command
		stdinEntry.SetText("")
		if err := executeCommand(txt); err != nil {
			fyne.Do(func() {
				app.showErrWithWindow(err, w)
			})
			return
		}
		cancelBtn.Enable()
	}

	stdinEntry.OnSubmitted = func(_ string) { executeInput() }

	cancelBtn.OnTapped = func() {
		if currentHandle == nil {
			return
		}
		if err := currentHandle.Cancel(); err != nil {
			fyne.Do(func() {
				app.showErrWithWindow(err, w)
			})
		}
	}

	w.SetOnClosed(func() {
		close(closeCh)
		if currentHandle != nil {
			_ = currentHandle.Cancel()
		}
		if client != nil {
			_ = client.Close()
		}
	})

	top := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("Device: %s", deviceID)),
		status,
	)
	// Make stdinEntry expand to max width by placing it in the center of a border container
	bottom := container.NewBorder(nil, nil, nil, cancelBtn, stdinEntry)
	w.SetContent(container.NewBorder(top, bottom, nil, nil, scroll))
	w.Show()
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
	w.Resize(fyne.NewSize(800, 700))

	// Load config from file
	cfg, _, err := config.LoadConfig()
	if err != nil {
		logger.Warnf("Failed to load config: %v, using defaults", err)
		cfg = config.DefaultConfig()
	}

	privkey := widget.NewEntry()
	if cfg.B64PrivKey != "" {
		privkey.Text = cfg.B64PrivKey
	} else if cfg.Seed != 0 {
		privkey.Text = strconv.FormatInt(cfg.Seed, 10)
	} else {
		privkey.Text = "0"
	}
	listenHost := widget.NewEntry()
	listenHost.Text = cfg.ListenHost
	listenPort := widget.NewEntry()
	listenPort.Text = strconv.Itoa(cfg.ListenPort)
	referenceToken := widget.NewEntry()
	referenceToken.Text = cfg.ReferenceToken
	bootstrapper := widget.NewCheck("", func(b bool) {})
	bootstrapper.Checked = cfg.Bootstrapper
	relay := widget.NewCheck("", func(b bool) {})
	relay.Checked = cfg.Relay
	metrics := widget.NewCheck("", func(b bool) {})
	metrics.Checked = cfg.Metrics
	metricsPort := widget.NewEntry()
	metricsPort.Text = strconv.Itoa(cfg.MetricsPort)
	api := widget.NewCheck("", func(b bool) {})
	api.Checked = cfg.APIServer
	apiPort := widget.NewEntry()
	apiPort.Text = strconv.Itoa(cfg.APIPort)
	disablePolicy := widget.NewCheck("", func(b bool) {})
	disablePolicy.Checked = cfg.DisablePolicy
	noSocket := widget.NewCheck("", func(b bool) {})
	noSocket.Checked = cfg.NoSocketServer
	debug := widget.NewCheck("", func(b bool) {})
	debug.Checked = cfg.Debug

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Private Key (base64 or seed)", Widget: privkey},
			{Text: "Listen Host", Widget: listenHost},
			{Text: "Listen Port", Widget: listenPort},
			{Text: "Reference Token", Widget: referenceToken},
			{Text: "Run as Bootstrapper", Widget: bootstrapper},
			{Text: "Enable Relay", Widget: relay},
			{Text: "Enable Metrics Server", Widget: metrics},
			{Text: "Metrics Port", Widget: metricsPort},
			{Text: "Enable API Server", Widget: api},
			{Text: "API Port", Widget: apiPort},
			{Text: "Disable Policy", Widget: disablePolicy},
			{Text: "Disable Socket Server", Widget: noSocket},
			{Text: "Debug Mode", Widget: debug},
		},
		OnSubmit: func() {
			// Update config
			cfg.ListenHost = listenHost.Text
			if port, err := strconv.Atoi(listenPort.Text); err == nil {
				cfg.ListenPort = port
			}
			cfg.ReferenceToken = referenceToken.Text
			cfg.Bootstrapper = bootstrapper.Checked
			cfg.Relay = relay.Checked
			cfg.Metrics = metrics.Checked
			if port, err := strconv.Atoi(metricsPort.Text); err == nil {
				cfg.MetricsPort = port
			}
			cfg.APIServer = api.Checked
			if port, err := strconv.Atoi(apiPort.Text); err == nil {
				cfg.APIPort = port
			}
			cfg.DisablePolicy = disablePolicy.Checked
			cfg.NoSocketServer = noSocket.Checked
			cfg.Debug = debug.Checked

			// Parse private key
			if seed, seedErr := strconv.ParseInt(privkey.Text, 10, 64); seedErr != nil {
				cfg.B64PrivKey = privkey.Text
				cfg.Seed = 0
			} else {
				cfg.Seed = seed
				cfg.B64PrivKey = ""
			}

			// Save config
			if saveErr := config.SaveConfig(cfg); saveErr != nil {
				app.showErr(saveErr)
				return
			}

			// Start node (bootstrapper or regular)
			device := device.Device{}

			// Only set key via opts if not using b64privkey directly
			if cfg.B64PrivKey == "" {
				device.GenerateKey(cfg.Seed)
			}

			if cfg.DisablePolicy {
				logger.Warn("Device Policy disabled, it is recommended to turn it on in production environment.")
			}

			// Initialize device with bootstrapper options
			device.InitWithOptions(
				cfg.ListenHost,
				uint64(cfg.ListenPort),
				cfg.Bootstrapper,
				cfg.Relay,
				cfg.B64PrivKey,
				cfg.Debug,
			)

			device.SetReferenceToken(cfg.ReferenceToken)

			device.Node.Policy.Enable = !cfg.DisablePolicy

			if cfg.Metrics {
				device.Node.StartMetricsServer(uint64(cfg.MetricsPort))
			}

			device.StartDiscovery()

			if !cfg.NoSocketServer {
				device.StartUnixSocket()
			}

			if cfg.APIServer {
				device.StartAPI(uint64(cfg.APIPort))
			}

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

func (app *GUIApp) initConfig() fyne.CanvasObject {
	cfg, _, err := config.LoadConfig()
	if err != nil {
		logger.Warnf("Failed to load config: %v, using defaults", err)
		cfg = config.DefaultConfig()
	}

	privkey := widget.NewEntry()
	if cfg.B64PrivKey != "" {
		privkey.Text = cfg.B64PrivKey
	} else if cfg.Seed != 0 {
		privkey.Text = strconv.FormatInt(cfg.Seed, 10)
	} else {
		privkey.Text = "0"
	}
	listenHost := widget.NewEntry()
	listenHost.Text = cfg.ListenHost
	listenPort := widget.NewEntry()
	listenPort.Text = strconv.Itoa(cfg.ListenPort)
	referenceToken := widget.NewEntry()
	referenceToken.Text = cfg.ReferenceToken
	bootstrapper := widget.NewCheck("", func(b bool) {})
	bootstrapper.Checked = cfg.Bootstrapper
	relay := widget.NewCheck("", func(b bool) {})
	relay.Checked = cfg.Relay
	metrics := widget.NewCheck("", func(b bool) {})
	metrics.Checked = cfg.Metrics
	metricsPort := widget.NewEntry()
	metricsPort.Text = strconv.Itoa(cfg.MetricsPort)
	api := widget.NewCheck("", func(b bool) {})
	api.Checked = cfg.APIServer
	apiPort := widget.NewEntry()
	apiPort.Text = strconv.Itoa(cfg.APIPort)
	disablePolicy := widget.NewCheck("", func(b bool) {})
	disablePolicy.Checked = cfg.DisablePolicy
	noSocket := widget.NewCheck("", func(b bool) {})
	noSocket.Checked = cfg.NoSocketServer
	debug := widget.NewCheck("", func(b bool) {})
	debug.Checked = cfg.Debug

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Private Key (base64 or seed)", Widget: privkey},
			{Text: "Listen Host", Widget: listenHost},
			{Text: "Listen Port", Widget: listenPort},
			{Text: "Reference Token", Widget: referenceToken},
			{Text: "Run as Bootstrapper", Widget: bootstrapper},
			{Text: "Enable Relay", Widget: relay},
			{Text: "Enable Metrics Server", Widget: metrics},
			{Text: "Metrics Port", Widget: metricsPort},
			{Text: "Enable API Server", Widget: api},
			{Text: "API Port", Widget: apiPort},
			{Text: "Disable Policy", Widget: disablePolicy},
			{Text: "Disable Socket Server", Widget: noSocket},
			{Text: "Debug Mode", Widget: debug},
		},
		OnSubmit: func() {
			// Update config
			cfg.ListenHost = listenHost.Text
			if port, err := strconv.Atoi(listenPort.Text); err == nil {
				cfg.ListenPort = port
			}
			cfg.ReferenceToken = referenceToken.Text
			cfg.Bootstrapper = bootstrapper.Checked
			cfg.Relay = relay.Checked
			cfg.Metrics = metrics.Checked
			if port, err := strconv.Atoi(metricsPort.Text); err == nil {
				cfg.MetricsPort = port
			}
			cfg.APIServer = api.Checked
			if port, err := strconv.Atoi(apiPort.Text); err == nil {
				cfg.APIPort = port
			}
			cfg.DisablePolicy = disablePolicy.Checked
			cfg.NoSocketServer = noSocket.Checked
			cfg.Debug = debug.Checked

			// Parse private key
			if seed, seedErr := strconv.ParseInt(privkey.Text, 10, 64); seedErr != nil {
				cfg.B64PrivKey = privkey.Text
				cfg.Seed = 0
			} else {
				cfg.Seed = seed
				cfg.B64PrivKey = ""
			}

			// Save config
			if saveErr := config.SaveConfig(cfg); saveErr != nil {
				app.showErr(saveErr)
				return
			}

			dialog.ShowInformation("Success", "Configuration saved successfully!", app.w)
		},
	}
	form.SubmitText = "Save Configuration"

	return container.NewScroll(form)
}

func (app *GUIApp) SetupMain() {
	w := app.a.NewWindow("Stellar Debug GUI")
	w.Resize(fyne.NewSize(800, 600))

	tabs := container.NewAppTabs(
		container.NewTabItem("Overview", app.initOverview()),
		container.NewTabItem("Devices", app.initDevices()),
		container.NewTabItem("Proxies", app.initProxies()),
		container.NewTabItem("White List", app.initWhiteList()),
		container.NewTabItem("Configuration", app.initConfig()),
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

			app.devices.Set(slices.Sorted(maps.Keys(app.node.Devices())))

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
