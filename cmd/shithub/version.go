// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/build"
)

// versionInfo is the on-the-wire shape for `shithub version --json`.
// Field names are stable: scripts depend on them.
type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

var versionJSON bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return printVersion(cmd.OutOrStdout(), versionJSON)
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "emit version info as JSON")
}

func printVersion(out io.Writer, asJSON bool) error {
	info := versionInfo{
		Version:   build.Version,
		Commit:    build.Commit,
		Date:      build.Date,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}
	// Human format: one line, matches gh's `gh --version` shape.
	_, err := fmt.Fprintf(out, "shithub %s (%s) built %s\n", info.Version, info.Commit, info.Date)
	return err
}
