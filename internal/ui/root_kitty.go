package ui

import (
	"image"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// transmitPhotoCmd transmits one photo to the terminal at the chat's current
// photo width and creates its virtual placement. No-op unless in Kitty mode.
func (m RootModel) transmitPhotoCmd(photoID int64, img image.Image) tea.Cmd {
	if m.imageMode != media.ModeKitty || img == nil {
		return nil
	}
	id := m.kittyStore.IDFor(photoID)
	b := img.Bounds()
	// Use the message-aware box (sticker cap vs photo cap) so the transmitted
	// width matches the rendered placeholder width; otherwise the Kitty placement
	// is never marked ready and the image stays a placeholder box.
	cols, rows := m.chat.MediaBoxForID(photoID, b.Dx(), b.Dy())
	// Order matters: write the placement to the terminal FIRST, then mark the
	// image ready (kittyTransmittedMsg) so the next render emits placeholders
	// only once the placement exists. Marking ready before the transmit lands
	// races the repaint and intermittently leaves the photo mispositioned.
	return tea.Sequence(
		func() tea.Msg {
			seq, err := media.TransmitSeq(id, img, cols, rows)
			if err != nil {
				return nil
			}
			return tea.Raw(seq)()
		},
		func() tea.Msg {
			return kittyTransmittedMsg{photoID: photoID, cols: cols}
		},
	)
}

// retransmitDebounce is the quiet period after the last photo-width change
// before images are re-transmitted. A resize drag fires many WindowSizeMsgs in
// quick succession; debouncing collapses them into a single retransmit at the
// final width. Without it, overlapping async transmits land out of order and
// leave the Kitty placement at a stale size (photo renders smaller than grid).
const retransmitDebounce = 90 * time.Millisecond

// retransmitOnColsChange schedules a debounced retransmit when the photo content
// width (in cells) actually changed. Photo width is photoContentCols (chat-pane,
// capped), not the window width, so this fires on any layout change that affects
// it (window resize, folder bar show/hide) and skips changes that leave the
// column count unchanged. Only the latest scheduled tick performs the work.
func (m *RootModel) retransmitOnColsChange() tea.Cmd {
	cols := m.chat.PhotoContentCols()
	// A tall photo's effective width depends on the pane height (the 2/3-viewport
	// and 480px height caps shrink cols), so a height-only resize can change a
	// photo's box without changing photoContentCols. Track both.
	paneH := m.chat.PhotoViewHeight()
	if cols == m.lastPhotoCols && paneH == m.lastPaneHeight {
		return nil
	}
	m.lastPhotoCols = cols
	m.lastPaneHeight = paneH
	m.retransmitGen++
	gen := m.retransmitGen
	return tea.Tick(retransmitDebounce, func(time.Time) tea.Msg {
		return retransmitTickMsg{gen: gen}
	})
}

// requestKittyReset asks the next reconcile to delete every placement and
// re-transmit the now-visible images. Used on chat switch and photo-width change
// (the deleted images belong to a different chat or a stale cell width).
func (m *RootModel) requestKittyReset() {
	if m.imageMode == media.ModeKitty {
		m.kittyResetPending = true
	}
}

// defaultKittyPlacementCap is the fallback cap when photos.kitty_placement_cap
// is unset (or non-positive). See PhotosConfig.KittyPlacementCap.
const defaultKittyPlacementCap = 16

// reconcileKittyCmd is the single place that issues Kitty transmits and deletes.
// It transmits visible images that are not yet live and evicts the
// least-recently-visible placements beyond the cap. No-op outside Kitty mode or
// the main screen.
func (m *RootModel) reconcileKittyCmd() tea.Cmd {
	if m.imageMode != media.ModeKitty || m.screen != ScreenMain {
		return nil
	}

	var pre tea.Cmd
	if m.kittyResetPending {
		m.kittyResetPending = false
		// Delete each currently-live placement by its own id (d=I) rather than a
		// blanket d=A, which is ambiguous for virtual (U=1) placements. Build the
		// sequence from the live set before clearing it. See #94.
		live := make([]int64, 0, len(m.kittyLive))
		for id := range m.kittyLive {
			live = append(live, id)
		}
		if seq := m.kittyStore.DeleteLiveSeq(live); seq != "" {
			pre = func() tea.Msg { return tea.Raw(seq)() }
		}
		m.kittyStore.Clear()
		m.kittyLive = make(map[int64]bool)
		m.kittyLRU = nil
	}

	visible := m.chat.VisiblePhotoIDs()
	visSet := make(map[int64]bool, len(visible))
	for _, id := range visible {
		visSet[id] = true
	}

	var cmds []tea.Cmd
	for _, id := range visible {
		if m.kittyLive[id] {
			m.kittyLRU = touchID(m.kittyLRU, id)
			continue
		}
		if img, ok := m.imageCache.Get(id); ok {
			if c := m.transmitPhotoCmd(id, img); c != nil {
				cmds = append(cmds, c)
				m.kittyLive[id] = true
				m.kittyLRU = append(m.kittyLRU, id)
			}
		}
	}

	// Evict the least-recently-visible placements beyond the cap; never evict a
	// currently-visible image.
	capN := m.kittyCap
	if capN <= 0 {
		capN = defaultKittyPlacementCap
	}
	for len(m.kittyLive) > capN {
		evicted := false
		for i, id := range m.kittyLRU {
			if visSet[id] {
				continue
			}
			cmds = append(cmds, tea.Raw(media.DeleteSeq(m.kittyStore.IDFor(id))))
			m.kittyStore.Untransmit(id)
			delete(m.kittyLive, id)
			m.kittyLRU = append(m.kittyLRU[:i], m.kittyLRU[i+1:]...)
			evicted = true
			break
		}
		if !evicted {
			break
		}
	}

	body := tea.Batch(cmds...)
	switch {
	case pre != nil && len(cmds) > 0:
		// The delete-all must reach the terminal before the re-transmits, so
		// sequence them; the transmits/deletes among themselves target distinct
		// ids and can run concurrently.
		return tea.Sequence(pre, body)
	case pre != nil:
		return pre
	case len(cmds) > 0:
		return body
	default:
		return nil
	}
}

// touchID moves id to the most-recently-visible end of the LRU order.
func touchID(s []int64, id int64) []int64 {
	out := make([]int64, 0, len(s))
	for _, v := range s {
		if v != id {
			out = append(out, v)
		}
	}
	return append(out, id)
}
