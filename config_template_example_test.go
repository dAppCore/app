// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go"
)

func Example_configTemplate() {
	rendered, err := renderConfigTemplateText(
		`{"size":{{ .size }},"enabled":{{ .enabled }}}`,
		map[string]any{
			"size":    256,
			"enabled": true,
		},
	)
	if err != nil {
		core.Println(err)
		return
	}

	core.Println(rendered)
	// Output:
	// {"size":256,"enabled":true}
}
