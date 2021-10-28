package threads


func (s *service) handleMissingReplicatorsAndThreadsInQueue() {
	go func() {
		err := s.addMissingReplicators()
		if err != nil {
			log.Errorf("addMissingReplicators: %s", err.Error())
		}
	}()
}

