package html

type renderState struct {
	ulOpened, olOpened bool
	isFirst, isLast    bool

	h *HTML
}

func (rs *renderState) OpenUL() {
	if rs.ulOpened {
		return
	}
	if rs.olOpened {
		rs.Close()
	}
	rs.h.buf.WriteString(`<ul style="font-size:15px;">`)
	rs.ulOpened = true
}

func (rs *renderState) OpenOL() {
	if rs.olOpened {
		return
	}
	if rs.ulOpened {
		rs.Close()
	}
	rs.h.buf.WriteString("<ol style=\"font-size:15px;\">")
	rs.olOpened = true
}

func (rs *renderState) Close() {
	if rs.ulOpened {
		rs.h.buf.WriteString("</ul>")
		rs.ulOpened = false
	} else if rs.olOpened {
		rs.h.buf.WriteString("</ol>")
		rs.olOpened = false
	}
}
