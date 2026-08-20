package theme

import "charm.land/lipgloss/v2"

// Styles are the ready-made styles derived from the current theme — the ones
// that were package-level vars in the components before colors were tokenised.
// They are rebuilt once per Apply, so reading one on every render costs a
// pointer load.
//
// A style whose construction needs runtime input (a container background to
// bake in, a computed width) is not here; those are built in place from T().
type Styles struct {
	// Body is the text the theme has no more specific token for: message text,
	// chat titles, folder labels, search rows. With Text unset it renders
	// exactly as an unstyled string does, so it is safe to apply everywhere the
	// terminal's foreground used to show through.
	Body     lipgloss.Style
	BodyBold lipgloss.Style

	// Message list.
	NameIncoming    lipgloss.Style
	NameEditing     lipgloss.Style
	Timestamp       lipgloss.Style
	TickSent        lipgloss.Style
	TickOutbox      lipgloss.Style
	TickRead        lipgloss.Style
	TickFailed      lipgloss.Style
	Indicator       lipgloss.Style
	Quote           lipgloss.Style
	Separator       lipgloss.Style
	UnreadSeparator lipgloss.Style
	WaveformPlayed  lipgloss.Style

	// Status bar.
	Bar        lipgloss.Style
	BarSep     lipgloss.Style
	ModeNormal lipgloss.Style
	ModeInsert lipgloss.Style

	// Overlay hints.
	OverlayHintDim    lipgloss.Style
	OverlayHintAccent lipgloss.Style

	// Popup menus and the reaction picker.
	MenuBg         lipgloss.Style
	MenuSelected   lipgloss.Style
	PickerBg       lipgloss.Style
	PickerSelected lipgloss.Style
	PickerChosen   lipgloss.Style

	// Mention popup.
	MentionRow    lipgloss.Style
	MentionSel    lipgloss.Style
	MentionDim    lipgloss.Style
	MentionSelDim lipgloss.Style
	MentionStatus lipgloss.Style

	// Help modal.
	HelpBg      lipgloss.Style
	HelpTitle   lipgloss.Style
	HelpKey     lipgloss.Style
	HelpSection lipgloss.Style
	HelpDesc    lipgloss.Style
	// HelpFaint is secondary text on the same panel: the notes after a settings
	// value, which must read as being about the value rather than as more of it.
	HelpFaint lipgloss.Style

	// Chat list.
	SelectedChat   lipgloss.Style
	MutedChat      lipgloss.Style
	OnlineDot      lipgloss.Style
	UnreadReaction lipgloss.Style
	UnreadMention  lipgloss.Style

	// Search, file picker, folders.
	SearchActiveRow  lipgloss.Style
	SearchPrompt     lipgloss.Style
	SearchHeader     lipgloss.Style
	SelectedFolder   lipgloss.Style
	PickerEmptyLabel lipgloss.Style

	// Message markup.
	SelfMention lipgloss.Style
}

func buildStyles(t Theme) Styles {
	// Every style starts from the canvas, so one that sets no background of its
	// own still paints rather than letting the terminal through. A style that
	// does set one overwrites it, which is what a surface is.
	n := func() lipgloss.Style { return newStyle(t) }
	return Styles{
		Body:     n().Foreground(t.Text),
		BodyBold: n().Foreground(t.Text).Bold(true),

		NameIncoming:    n().Foreground(t.NameIncoming).Bold(true),
		NameEditing:     n().Foreground(t.NameEditing).Bold(true),
		Timestamp:       n().Foreground(t.TextDim),
		TickSent:        n().Foreground(t.TickSent),
		TickOutbox:      n().Foreground(t.TickOutbox),
		TickRead:        n().Foreground(t.TickRead),
		TickFailed:      n().Foreground(t.TickFailed),
		Indicator:       n().Foreground(t.Indicator),
		Quote:           n().Foreground(t.TextDim),
		Separator:       n().Foreground(t.TextDim),
		UnreadSeparator: n().Foreground(t.UnreadSeparator),
		WaveformPlayed:  n().Foreground(t.WaveformPlayed),

		Bar:        n().Background(t.SurfaceStatusBar).Foreground(t.TextStatusBar),
		BarSep:     n().Background(t.SurfaceStatusBar).Foreground(t.BorderStatusSep),
		ModeNormal: n().Bold(true).Padding(0, 1).Foreground(t.TextModeLabel).Background(t.AccentModeNormal),
		ModeInsert: n().Bold(true).Padding(0, 1).Foreground(t.TextModeLabel).Background(t.AccentModeInsert),

		OverlayHintDim:    n().Foreground(t.TextFaint),
		OverlayHintAccent: n().Foreground(t.Accent),

		MenuBg:         n().Background(t.SurfaceOverlay).Foreground(t.TextOnSurface),
		MenuSelected:   n().Background(t.SurfaceSelected).Foreground(t.TextOnSelected),
		PickerBg:       n().Background(t.SurfaceOverlay).Foreground(t.TextOnSurface),
		PickerSelected: n().Background(t.SurfaceSelected).Foreground(t.TextOnSelected),
		PickerChosen:   n().Foreground(t.ReactionChosen).Bold(true),

		MentionRow:    n().Background(t.SurfaceOverlay).Foreground(t.TextOnSurface),
		MentionSel:    n().Background(t.SurfaceSelected).Foreground(t.TextOnSelected),
		MentionDim:    n().Foreground(t.TextDim).Background(t.SurfaceOverlay),
		MentionSelDim: n().Foreground(t.TextOnSelectedMuted).Background(t.SurfaceSelected),
		MentionStatus: n().Background(t.SurfaceOverlay).Foreground(t.TextDim).Italic(true),

		HelpBg:      n().Background(t.SurfaceHelp).Foreground(t.TextOnSurface),
		HelpTitle:   n().Background(t.SurfaceHelp).Foreground(t.AccentOnSurface).Bold(true),
		HelpKey:     n().Background(t.SurfaceHelp).Foreground(t.AccentOnSurface),
		HelpSection: n().Background(t.SurfaceHelp).Foreground(t.TextOnSurface).Bold(true),
		HelpDesc:    n().Background(t.SurfaceHelp).Foreground(t.TextOnSurface),
		HelpFaint:   n().Background(t.SurfaceHelp).Foreground(t.TextFaint),

		SelectedChat:   n().Background(t.SurfaceSelected).Foreground(t.TextOnSelected),
		MutedChat:      n().Foreground(t.TextMuted),
		OnlineDot:      n().Foreground(t.StatusOnline),
		UnreadReaction: n().Foreground(t.UnreadReaction),
		UnreadMention:  n().Foreground(t.UnreadMention),

		SearchActiveRow:  n().Background(t.SurfaceSelected).Foreground(t.TextOnSelected),
		SearchPrompt:     n().Foreground(t.SurfaceSelected),
		SearchHeader:     n().Foreground(t.TextFaint),
		SelectedFolder:   n().Background(t.SurfaceSelected).Foreground(t.TextOnSelected),
		PickerEmptyLabel: n().Foreground(t.TextFaint),

		SelfMention: n().Bold(true).Foreground(t.MarkupSelfMentionFg).Background(t.SurfaceSelfMention),
	}
}
