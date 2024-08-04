package base

type CircularQueue[T any] struct {
	items []T
	front int
	rear  int
}

func NewCircularQueue[T any](capacity int) *CircularQueue[T] {
	return &CircularQueue[T]{
		items: make([]T, capacity+1),
		front: 0,
		rear:  0,
	}
}

func (q *CircularQueue[T]) Enqueue(item T) {
	size := len(q.items)
	if (q.rear+1)%size == q.front { // full
		items := make([]T, 2*size)
		for i := 0; i < size-1; i++ { // copy
			items[i] = q.items[(q.front+i)%size]
		}
		q.items = items
		size *= 2
		q.front = 0
		q.rear = size - 1
	}
	q.items[q.rear] = item
	q.rear = (q.rear + 1) % size
}

func (q *CircularQueue[T]) Dequeue() (item T, ok bool) {
	if q.front == q.rear { // empty
		return
	}
	item = q.items[q.front]
	ok = true
	var empty T
	q.items[q.front] = empty
	q.front = (q.front + 1) % len(q.items)
	return
}
