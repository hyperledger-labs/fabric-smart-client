package utils

type Iterator[V any] interface {
	Next() (V, error)
	Close()
}

func Map[A any, B any](iterator Iterator[A], transformer func(A) (B, error)) Iterator[B] {}
