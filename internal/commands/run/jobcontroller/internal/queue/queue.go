package queue

import "github.com/artuross/github-actions-runner/internal/commands/run/step"

type OrderedQueue struct {
	queue []step.Step
}

func (oq *OrderedQueue) HasChildren(id string) bool {
	for _, item := range oq.queue {
		if item.ParentID() == id {
			return true
		}
	}

	return false
}

func (oq *OrderedQueue) HasNext() bool {
	return len(oq.queue) > 0
}

// Pop returns and removes first element from the queue.
func (oq *OrderedQueue) Pop() step.Step {
	if len(oq.queue) == 0 {
		return nil
	}

	step := oq.queue[0]
	oq.queue = oq.queue[1:]

	return step
}

// Push adds an item to the queue in the correct order.
// Order is:
// - items with parentID are inserted right after their parent
// - if the there are items already with the same parentID, they are added after the last element with the same parentID
func (oq *OrderedQueue) Push(item step.Step) {
	lastIndex := len(oq.queue) - 1

	insertIndex := 0
	insertIndex = lastIndex + 1
	for i := lastIndex; i >= 0; i-- {
		if isParentOrSibling(item.ID(), oq.queue[i]) {
			insertIndex = i + 1
			break
		}
	}

	queue := make([]step.Step, 0, len(oq.queue)+1)
	queue = append(queue, oq.queue[:insertIndex]...)
	queue = append(queue, item)
	queue = append(queue, oq.queue[insertIndex:]...)

	oq.queue = append(oq.queue, item)
}

func isParentOrSibling(id string, s step.Step) bool {
	return id == s.ParentID() || id == s.ID()
}
