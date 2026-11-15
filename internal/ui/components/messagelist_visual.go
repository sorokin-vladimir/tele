package components

import "strings"

func (ml *MessageList) StartVisual() bool {
	id := ml.computeSelectedMsgID()
	if id == 0 {
		return false
	}
	ml.setCursor(id, "")
	ml.visualAnchorID = id
	return true
}

func (ml *MessageList) ClearVisual() { ml.visualAnchorID = 0 }

func (ml *MessageList) InVisual() bool {
	_, _, ok := ml.visualRange()
	return ok
}

func (ml *MessageList) visualRange() (lo, hi int, ok bool) {
	if ml.visualAnchorID == 0 {
		return 0, 0, false
	}
	anchor := -1
	for i := range ml.items {
		if ml.items[i].kind == itemMessage && ml.items[i].msg.ID == ml.visualAnchorID {
			anchor = i
			break
		}
	}
	cursor := ml.cursorIndex()
	if anchor < 0 || cursor < 0 {
		return 0, 0, false
	}
	return min(anchor, cursor), max(anchor, cursor), true
}

func (ml *MessageList) VisualText() string {
	lo, hi, ok := ml.visualRange()
	if !ok {
		return ""
	}
	var texts []string
	for i := lo; i <= hi; i++ {
		if ml.items[i].kind != itemMessage {
			continue
		}
		for _, part := range ml.items[i].parts {
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
	}
	return strings.Join(texts, "\n\n")
}
