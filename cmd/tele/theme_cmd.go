package main

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// loadThemes resolves the themes cfg names, before anything is drawn, so a bad
// theme file is reported like a config warning rather than something the
// interface has to cope with. It returns the themes directory, the themes, and
// what is printed and logged at startup: the config's warnings and the
// themes'. Gathered here rather than written into the config, which is what
// the file says and is replaced by a reload (#239).
func loadThemes(cfg *config.Config) (string, theme.Loaded, []config.Warning) {
	themesDir := cfg.ThemesDir
	themes := theme.LoadSlots(themesDir, cfg.UI.ThemeSlots.Dark, cfg.UI.ThemeSlots.Light)
	theme.SetSlots(themes.Slots())
	warnings := slices.Clone(cfg.Warnings)
	for _, w := range themes.Warnings {
		warnings = append(warnings, config.Warning{Text: "theme: " + w})
	}
	return themesDir, themes, warnings
}

// newThemeCmd is `tele theme`: the tools for writing a theme, which used to
// be flags of the window (#213).
func newThemeCmd(o *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "theme",
		Short: "Inspect themes and write them out as files",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "check [NAME]",
		Short: "Say which theme each slot resolved to and where its tokens came from; name a theme to inspect one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			s, err := newSetup(o.cfgPath)
			if err != nil {
				return err
			}
			themesDir, themes, warnings := loadThemes(s.store.Current())
			_, err = fmt.Fprint(c.OutOrStdout(), themeReport(themesDir, themes, firstArg(args), warnings))
			return err
		},
	}, &cobra.Command{
		Use:   "dump [NAME]",
		Short: "Write a slot's theme (dark or light) out as a complete theme file; name a theme to write that one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			s, err := newSetup(o.cfgPath)
			if err != nil {
				return err
			}
			themesDir, themes, _ := loadThemes(s.store.Current())
			out, err := themeDumpText(themesDir, themes, firstArg(args))
			if err != nil {
				return fmt.Errorf("theme: %w", err)
			}
			_, err = fmt.Fprint(c.OutOrStdout(), out)
			return err
		},
	})
	return cmd
}

// firstArg is the one optional argument a command takes, "" when none is
// given.
func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
