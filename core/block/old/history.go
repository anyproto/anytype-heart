package old

func (p *commonSmart) Undo() (err error) {
	p.m.Lock()
	defer p.m.Unlock()

	action, err := p.history.Previous()
	if err != nil {
		return
	}

	s := p.newState()

	for _, b := range action.Add {
		s.remove(b.Model().Id)
		s.removeFromChilds(b.Model().Id)
	}
	for _, b := range action.Remove {
		s.set(b)
	}
	for _, b := range action.Change {
		s.set(b.Before)
	}

	return p.applyAndSendEventHist(s, false, true)
}

func (p *commonSmart) Redo() (err error) {
	p.m.Lock()
	defer p.m.Unlock()

	action, err := p.history.Next()
	if err != nil {
		return
	}

	s := p.newState()

	for _, b := range action.Add {
		s.set(b)
	}
	for _, b := range action.Remove {
		s.remove(b.Model().Id)
		s.removeFromChilds(b.Model().Id)
	}
	for _, b := range action.Change {
		s.set(b.After)
	}

	return p.applyAndSendEventHist(s, false, true)
}
