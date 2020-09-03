package common

type Route interface {
	AddDestWithOrigin(string) error
}
