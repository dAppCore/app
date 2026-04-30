// SPDX-License-Identifier: EUPL-1.2

package app_test

import (
	core "dappco.re/go"
	"dappco.re/go/app"
	coreio "dappco.re/go/io"
)

func ExampleWrapPWA() {
	manifest := app.WrapPWA(&app.PWAManifest{
		Name:        "Play Example",
		ShortName:   "play",
		StartURL:    "https://play.example.com/",
		Permissions: []string{"notifications"},
	}, app.WrapPWAOptions{
		TargetURL: "https://play.example.com/",
	})

	core.Println(manifest.Code)
	core.Println(manifest.Config["type"])
	core.Println(manifest.Permissions.Notifications)
	core.Println(manifest.Config["store"])
	// Output:
	// play
	// pwa
	// true
	// true
}

func ExampleWrapElectron() {
	manifest := app.WrapElectron(&app.ElectronPackageJSON{
		ProductName: "Electron Miner",
		Version:     "1.2.3",
		Main:        "main.js",
	}, &app.ElectronScanResult{
		FS:          true,
		IPCChannels: []string{"miner:start"},
	}, app.WrapElectronOptions{})

	core.Println(manifest.Code)
	core.Println(manifest.Config["type"])
	core.Println(manifest.Config["main"])
	core.Println(manifest.Permissions.Read[0])
	// Output:
	// electron-miner
	// electron
	// main.js
	// ./data/
}

func ExampleWrapWeb() {
	root := core.MkdirTemp("", "wrap-web-example").Value.(string)
	defer core.RemoveAll(root)

	site := core.Path(root, "marketing-site")
	_ = coreio.Local.EnsureDir(site)
	_ = coreio.Local.Write(core.Path(site, "index.html"), "<html><body>Landing</body></html>")

	manifest, _ := app.WrapWeb(coreio.Local, site, app.WrapWebOptions{})
	core.Println(manifest.Code)
	core.Println(manifest.Name)
	core.Println(manifest.Config["entry"])
	// Output:
	// marketing-site
	// Marketing Site
	// index.html
}
