package space

type statusChecker interface {
}

// it should check the statuses on coordinator
// it should ping space service to update the status
// we don't delete the space if it is deleted in the coordinator
