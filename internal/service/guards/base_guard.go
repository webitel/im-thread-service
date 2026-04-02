package guards

type ValidationGuard[T any] func(req T) error
