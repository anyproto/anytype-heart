package space

func (s *service) initTechSpace() (err error) {
	s.techSpace, err = s.factory.CreateAndSetTechSpace(s.ctx)
	return
}
